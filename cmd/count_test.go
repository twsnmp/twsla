package cmd

import (
	"testing"
)

func TestNormalizeLog(t *testing.T) {
	setupTimeGrinder()
	tests := []struct {
		input string
		want  string
	}{
		{
			input: "May  4 10:30:00 host connection from 192.168.1.1",
			want:  "#TIMESTAMP# host connection from #IP#",
		},
		{
			input: "2026-05-04T10:30:00Z error in session 550e8400-e29b-41d4-a716-446655440000",
			want:  "#TIMESTAMP# error in session #UUID#",
		},
		{
			input: "Email to test@example.com for user 123",
			want:  "Email to #EMAIL# for user #NUM#",
		},
		{
			input: "MAC address is 00:11:22:33:44:55",
			want:  "MAC address is #MAC#",
		},
	}

	for _, tt := range tests {
		got := normalizeLog(tt.input)
		if got != tt.want {
			t.Errorf("normalizeLog(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}
