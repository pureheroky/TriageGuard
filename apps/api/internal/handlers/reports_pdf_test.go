package handlers

import "testing"

func TestFormatMetricMinutes(t *testing.T) {
	cases := []struct {
		avg     float64
		samples int
		want    string
	}{
		{0, 0, "-"},
		{12.34, 5, "12.3m"},
		{120.0, 2, "2.0h"},
	}
	for _, tc := range cases {
		got := formatMetricMinutes(tc.avg, tc.samples)
		if got != tc.want {
			t.Fatalf("formatMetricMinutes(%v, %d) = %q, want %q", tc.avg, tc.samples, got, tc.want)
		}
	}
}

func TestParseWindowDays(t *testing.T) {
	cases := []struct {
		in   string
		want int
	}{
		{"7", 7},
		{"30", 30},
		{"90", 90},
		{"", 30},
		{"1", 30},
		{"365", 30},
		{"abc", 30},
	}
	for _, tc := range cases {
		got := parseWindowDays(tc.in)
		if got != tc.want {
			t.Fatalf("parseWindowDays(%q) = %d, want %d", tc.in, got, tc.want)
		}
	}
}
