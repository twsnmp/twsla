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

	func TestGetMimeDecodedWord(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"Hello World", "Hello World"},
		{`=?ISO-2022-JP?Q?=1B$B$4BP1~$N$*4j$$!'=1B(BMac_Installer_Dis?= =?ISO-2022-JP?Q?tribution=1B$B>ZL@=3Dq$N=1B(B?= =?ISO-2022-JP?Q?=1B$BM-8z4|8B$^$G$"$H=1B(B30=1B$BF|$G$9=1B(B?=`, "ご対応のお願い：Mac Installer Distribution証明書の有効期限まであと30日です"},
	}

	for _, tt := range tests {
		result := getMimeDecodedWord(tt.input)
		if result != tt.expected {
			t.Errorf("getMimeDecodedWord(%q) = %q, want %q", tt.input, result, tt.expected)
		}
	}
}
