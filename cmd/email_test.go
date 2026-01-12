package cmd

import (
	"testing"
)

func TestExtractHelo(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"from example.com (host.example.com [192.0.2.1])", "example.com"},
		{"from [127.0.0.1] (localhost [127.0.0.1])", "[127.0.0.1]"},
		{"FROM example.org (unknown [1.2.3.4])", "example.org"},
		{"from  multiple.spaces.com  (test)", "multiple.spaces.com"},
		{"by mx.google.com with ESMTPS", ""},
		{"received from helo.test (IP)", "helo.test"},
	}

	for _, tt := range tests {
		result := extractHelo(tt.input)
		if result != tt.expected {
			t.Errorf("extractHelo(%q) = %q, want %q", tt.input, result, tt.expected)
		}
	}
}

func TestExtractIP(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"from example.com (host.example.com [192.0.2.1])", "192.0.2.1"},
		{"from [192.0.2.2] (localhost [192.0.2.2])", "192.0.2.2"},
		{"no ip here", ""},
	}

	for _, tt := range tests {
		result := extractIP(tt.input)
		if tt.expected == "" {
			if result != nil {
				t.Errorf("extractIP(%q) = %v, want nil", tt.input, result)
			}
		} else {
			if result == nil || result.String() != tt.expected {
				t.Errorf("extractIP(%q) = %v, want %q", tt.input, result, tt.expected)
			}
		}
	}
}
