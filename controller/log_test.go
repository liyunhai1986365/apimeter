package controller

import "testing"

func TestParseLogQueryIntAcceptsQuotedValues(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  int
	}{
		{name: "plain", value: "11", want: 11},
		{name: "double quoted", value: `"11"`, want: 11},
		{name: "single quoted", value: `'11'`, want: 11},
		{name: "spaced quoted", value: ` "11" `, want: 11},
		{name: "invalid", value: `"abc"`, want: 0},
		{name: "empty", value: "", want: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := parseLogQueryInt(tt.value); got != tt.want {
				t.Fatalf("parseLogQueryInt(%q) = %d, want %d", tt.value, got, tt.want)
			}
		})
	}
}
