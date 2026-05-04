package cmd

import (
	"testing"
)

func TestGetRelationEntIndex(t *testing.T) {
	tests := []struct {
		input string
		want  int
	}{
		{"ip", 0},
		{"ip:0", 0},
		{"ip:1", 1},
		{"email:2", 2},
		{"regex/pattern/:3", 3},
	}

	for _, tt := range tests {
		got := getRelationEntIndex(tt.input)
		if got != tt.want {
			t.Errorf("getRelationEntIndex(%q) = %d, want %d", tt.input, got, tt.want)
		}
	}
}
