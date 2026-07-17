package decision

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// The v1 condition language (design §2.2) is a single comparison over one
// flat input key:
//
//	<field> <op> <literal>
//
// ops: == != < <= > >= in not_in
// literals: 'string' / "string" / bare-word, number, true/false,
// and [a, b, c] lists for in / not_in.
//
// It is deliberately not an expression engine — no boolean connectives, no
// nesting, no function calls. Anything it can't express stays in Go behind a
// named rule id so it still logs and versions.

type litKind int

const (
	litString litKind = iota
	litNumber
	litBool
	litList
)

type literal struct {
	kind litKind
	str  string
	num  float64
	b    bool
	list []literal
}

type condition struct {
	field string
	op    string
	lit   literal
}

var validOps = map[string]bool{
	"==": true, "!=": true, "<": true, "<=": true, ">": true, ">=": true,
	"in": true, "not_in": true,
}

// parseCondition parses "<field> <op> <literal>". Called at policy load time
// (Validate), so a malformed condition is a startup defect, not a runtime
// surprise.
func parseCondition(expr string) (condition, error) {
	s := strings.TrimSpace(expr)
	parts := strings.SplitN(s, " ", 3)
	if len(parts) != 3 {
		return condition{}, fmt.Errorf("condition %q: want '<field> <op> <literal>'", expr)
	}
	field, op, rest := parts[0], parts[1], strings.TrimSpace(parts[2])
	if !validOps[op] {
		return condition{}, fmt.Errorf("condition %q: unsupported operator %q", expr, op)
	}
	lit, err := parseLiteral(rest)
	if err != nil {
		return condition{}, fmt.Errorf("condition %q: %w", expr, err)
	}
	if (op == "in" || op == "not_in") && lit.kind != litList {
		return condition{}, fmt.Errorf("condition %q: %s requires a [list] literal", expr, op)
	}
	if lit.kind == litList && op != "in" && op != "not_in" {
		return condition{}, fmt.Errorf("condition %q: [list] literal requires in/not_in", expr)
	}
	if (op == "<" || op == "<=" || op == ">" || op == ">=") && lit.kind != litNumber {
		return condition{}, fmt.Errorf("condition %q: ordered comparison requires a numeric literal", expr)
	}
	return condition{field: field, op: op, lit: lit}, nil
}

func parseLiteral(s string) (literal, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return literal{}, fmt.Errorf("empty literal")
	}
	if strings.HasPrefix(s, "[") {
		if !strings.HasSuffix(s, "]") {
			return literal{}, fmt.Errorf("unterminated list literal %q", s)
		}
		inner := strings.TrimSpace(s[1 : len(s)-1])
		var list []literal
		if inner != "" {
			for _, part := range strings.Split(inner, ",") {
				el, err := parseLiteral(part)
				if err != nil {
					return literal{}, err
				}
				if el.kind == litList {
					return literal{}, fmt.Errorf("nested lists are not supported: %q", s)
				}
				list = append(list, el)
			}
		}
		return literal{kind: litList, list: list}, nil
	}
	if (strings.HasPrefix(s, "'") && strings.HasSuffix(s, "'") && len(s) >= 2) ||
		(strings.HasPrefix(s, `"`) && strings.HasSuffix(s, `"`) && len(s) >= 2) {
		return literal{kind: litString, str: s[1 : len(s)-1]}, nil
	}
	if s == "true" || s == "false" {
		return literal{kind: litBool, b: s == "true"}, nil
	}
	if n, err := strconv.ParseFloat(s, 64); err == nil {
		return literal{kind: litNumber, num: n}, nil
	}
	// Bare word: treated as a string (e.g. `band == A`).
	return literal{kind: litString, str: s}, nil
}

// eval evaluates the condition against the flat inputs map.
//
// Failure semantics are fail-closed for the guard idiom the language exists
// for (`kyc_status != 'PASSED'` ⇒ DECLINE): a MISSING input satisfies != and
// not_in, and fails == and in. Ordered comparisons over a missing or
// non-numeric input cannot be answered honestly either way and return an
// error, which the evaluator surfaces instead of guessing.
func (c condition) eval(inputs map[string]any) (bool, error) {
	val, present := inputs[c.field]
	switch c.op {
	case "==":
		return present && literalEquals(c.lit, val), nil
	case "!=":
		return !present || !literalEquals(c.lit, val), nil
	case "in":
		if !present {
			return false, nil
		}
		return listContains(c.lit.list, val), nil
	case "not_in":
		if !present {
			return true, nil
		}
		return !listContains(c.lit.list, val), nil
	case "<", "<=", ">", ">=":
		if !present {
			return false, fmt.Errorf("input %q is missing for ordered comparison", c.field)
		}
		n, ok := toFloat(val)
		if !ok {
			return false, fmt.Errorf("input %q (%T) is not numeric", c.field, val)
		}
		switch c.op {
		case "<":
			return n < c.lit.num, nil
		case "<=":
			return n <= c.lit.num, nil
		case ">":
			return n > c.lit.num, nil
		default:
			return n >= c.lit.num, nil
		}
	}
	return false, fmt.Errorf("unsupported operator %q", c.op)
}

func listContains(list []literal, val any) bool {
	for _, el := range list {
		if literalEquals(el, val) {
			return true
		}
	}
	return false
}

func literalEquals(lit literal, val any) bool {
	switch lit.kind {
	case litNumber:
		n, ok := toFloat(val)
		return ok && n == lit.num
	case litBool:
		b, ok := val.(bool)
		return ok && b == lit.b
	case litString:
		s, ok := val.(string)
		return ok && s == lit.str
	default:
		return false
	}
}

func toFloat(v any) (float64, bool) {
	switch n := v.(type) {
	case int:
		return float64(n), true
	case int32:
		return float64(n), true
	case int64:
		return float64(n), true
	case float32:
		return float64(n), true
	case float64:
		return n, true
	case json.Number:
		f, err := n.Float64()
		return f, err == nil
	default:
		return 0, false
	}
}
