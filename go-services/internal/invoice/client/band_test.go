package client

import "testing"

func TestBandFromScoreBand(t *testing.T) {
	cases := map[string]string{
		"Excellent": "A", "Very Good": "A", "VERY_GOOD": "A",
		"Good": "B", "Fair": "C", "Marginal": "C",
		"Poor": "D", "VERY_POOR": "D", "A": "A", "d": "D", "": "", "weird": "",
	}
	for in, want := range cases {
		if got := BandFromScoreBand(in); got != want {
			t.Errorf("BandFromScoreBand(%q) = %q, want %q", in, got, want)
		}
	}
}
