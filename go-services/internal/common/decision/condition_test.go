package decision

import (
	"strings"
	"testing"
)

func TestParseCondition_Errors(t *testing.T) {
	cases := []struct {
		expr    string
		wantErr string
	}{
		{"", "want '<field> <op> <literal>'"},
		{"kyc_status", "want '<field> <op> <literal>'"},
		{"kyc_status !=", "want '<field> <op> <literal>'"},
		{"kyc_status ~= 'PASSED'", "unsupported operator"},
		{"score in 5", "requires a [list] literal"},
		{"score not_in 'A'", "requires a [list] literal"},
		{"score == [1, 2]", "requires in/not_in"},
		{"score < 'high'", "ordered comparison requires a numeric literal"},
		{"score < [1]", "requires in/not_in"},
		{"band in [[1], 2]", "nested lists"},
		{"band in [1, 2", "unterminated list"},
	}
	for _, c := range cases {
		t.Run(c.expr, func(t *testing.T) {
			_, err := parseCondition(c.expr)
			if err == nil {
				t.Fatalf("parseCondition(%q): want error containing %q, got nil", c.expr, c.wantErr)
			}
			if !strings.Contains(err.Error(), c.wantErr) {
				t.Fatalf("parseCondition(%q) error = %v, want containing %q", c.expr, err, c.wantErr)
			}
		})
	}
}

func TestConditionEval(t *testing.T) {
	inputs := map[string]any{
		"kyc_status": "PASSED",
		"score":      650,
		"pd":         0.04,
		"band":       "B",
		"flagged":    true,
	}
	cases := []struct {
		expr string
		want bool
	}{
		{"kyc_status == 'PASSED'", true},
		{"kyc_status == \"PASSED\"", true},
		{"kyc_status != 'PASSED'", false},
		{"kyc_status == 'FAILED'", false},
		{"kyc_status != 'FAILED'", true},
		// Missing input: fail-closed guard idiom — != and not_in are
		// satisfied, == and in are not.
		{"missing_field != 'PASSED'", true},
		{"missing_field == 'PASSED'", false},
		{"missing_field in ['A', 'B']", false},
		{"missing_field not_in ['A', 'B']", true},
		// Numbers, including int/float coercion both ways.
		{"score == 650", true},
		{"score != 650", false},
		{"score < 651", true},
		{"score < 650", false},
		{"score <= 650", true},
		{"score > 649", true},
		{"score >= 650", true},
		{"score >= 651", false},
		{"pd < 0.05", true},
		{"pd > 0.05", false},
		// Sets.
		{"band in ['A', 'B']", true},
		{"band in ['C', 'D']", false},
		{"band not_in ['C', 'D']", true},
		{"score in [600, 650]", true},
		// Bools and bare words.
		{"flagged == true", true},
		{"flagged != true", false},
		{"band == B", true}, // bare-word string literal
	}
	for _, c := range cases {
		t.Run(c.expr, func(t *testing.T) {
			cond, err := parseCondition(c.expr)
			if err != nil {
				t.Fatalf("parseCondition(%q): %v", c.expr, err)
			}
			got, err := cond.eval(inputs)
			if err != nil {
				t.Fatalf("eval(%q): %v", c.expr, err)
			}
			if got != c.want {
				t.Errorf("eval(%q) = %v, want %v", c.expr, got, c.want)
			}
		})
	}
}

func TestConditionEval_OrderedErrors(t *testing.T) {
	inputs := map[string]any{"band": "B"}
	for _, expr := range []string{"score < 600", "band < 600"} {
		cond, err := parseCondition(expr)
		if err != nil {
			t.Fatalf("parseCondition(%q): %v", expr, err)
		}
		if _, err := cond.eval(inputs); err == nil {
			t.Errorf("eval(%q): want error for missing/non-numeric input, got nil", expr)
		}
	}
}
