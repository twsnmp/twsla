package cmd

import (
	"testing"
	"time"
)

func TestGetTimeRange(t *testing.T) {
	now := time.Now()
	tests := []struct {
		name      string
		tr        string
		checkFunc func(t *testing.T, st, et time.Time)
	}{
		{
			name: "empty timeRange",
			tr:   "",
			checkFunc: func(t *testing.T, st, et time.Time) {
				if st.Unix() != 0 {
					t.Errorf("Empty range: start should be epoch, got %v", st)
				}
				if et.Before(now) {
					t.Errorf("Empty range: end should be in future, got %v", et)
				}
			},
		},
		{
			name: "duration 1h",
			tr:   "1h",
			checkFunc: func(t *testing.T, st, et time.Time) {
				diff := et.Sub(st)
				if diff != time.Hour {
					t.Errorf("1h: diff should be 1h, got %v", diff)
				}
				if et.After(now.Add(time.Second)) || et.Before(now.Add(-time.Second)) {
					t.Errorf("1h: end should be around now, got %v", et)
				}
			},
		},
		{
			name: "start date only",
			tr:   "2024-01-01",
			checkFunc: func(t *testing.T, st, et time.Time) {
				if st.Year() != 2024 || st.Month() != 1 || st.Day() != 1 {
					t.Errorf("Start date only: expected 2024-01-01, got %v", st)
				}
				if et.Before(now) {
					t.Errorf("Start date only: end should be in future, got %v", et)
				}
			},
		},
		{
			name: "start date and duration",
			tr:   "2024-01-01,1h",
			checkFunc: func(t *testing.T, st, et time.Time) {
				if st.Year() != 2024 || st.Month() != 1 || st.Day() != 1 {
					t.Errorf("Start date and duration: expected 2024-01-01, got %v", st)
				}
				diff := et.Sub(st)
				if diff != time.Hour {
					t.Errorf("Start date and duration: diff should be 1h, got %v", diff)
				}
			},
		},
		{
			name: "start and end dates",
			tr:   "2024-01-01,2024-01-02",
			checkFunc: func(t *testing.T, st, et time.Time) {
				if st.Year() != 2024 || st.Month() != 1 || st.Day() != 1 {
					t.Errorf("Start/End dates: expected start 2024-01-01, got %v", st)
				}
				if et.Year() != 2024 || et.Month() != 1 || et.Day() != 2 {
					t.Errorf("Start/End dates: expected end 2024-01-02, got %v", et)
				}
			},
		},
		{
			name: "ISO-like date with duration",
			tr:   "2026/02/24T10:30:10+0900,1h",
			checkFunc: func(t *testing.T, st, et time.Time) {
				// 2026/02/24T10:30:10+0900 is 2026-02-24 10:30:10 +0900
				jst := time.FixedZone("JST", 9*3600)
				st = st.In(jst)
				if st.Year() != 2026 || st.Month() != 2 || st.Day() != 24 || st.Hour() != 10 || st.Minute() != 30 || st.Second() != 10 {
					t.Errorf("ISO-like: expected 2026-02-24 10:30:10, got %v", st)
				}
				diff := et.Sub(st)
				if diff != time.Hour {
					t.Errorf("ISO-like: diff should be 1h, got %v", diff)
				}
			},
		},
		{
			name: "date and time with space and duration",
			tr:   "2026/02/26 10:30,1h",
			checkFunc: func(t *testing.T, st, et time.Time) {
				if st.Year() != 2026 || st.Month() != 2 || st.Day() != 26 || st.Hour() != 10 || st.Minute() != 30 || st.Second() != 0 {
					t.Errorf("Date/Time with space: expected 2026-02-26 10:30:00, got %v", st)
				}
				diff := et.Sub(st)
				if diff != time.Hour {
					t.Errorf("Date/Time with space: diff should be 1h, got %v", diff)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			timeRange = tt.tr
			stNano, etNano := getTimeRange()
			st := time.Unix(0, stNano)
			et := time.Unix(0, etNano)
			tt.checkFunc(t, st, et)
		})
	}
}

func TestSetExtPat(t *testing.T) {
	tests := []struct {
		pattern string
		pos     int
		input   string
		want    string
	}{
		{"ip", 1, "Connection from 192.168.1.1", "192.168.1.1"},
		{"email", 1, "Email to test@example.com", "test@example.com"},
		{"number", 1, "Value is 123.45", "123.45"},
		{"number", 2, "Values are 123.45 and 67.89", "67.89"},
		{"%{word} is 100", 1, "Price is 100", "Price"},
	}

	for _, tt := range tests {
		extract = tt.pattern
		pos = tt.pos
		err := setExtPat()
		if err != nil {
			t.Errorf("setExtPat(%q) error: %v", tt.pattern, err)
			continue
		}
		if extPat == nil {
			t.Errorf("setExtPat(%q) extPat is nil", tt.pattern)
			continue
		}
		matches := extPat.ExtReg.FindAllStringSubmatch(tt.input, -1)
		if len(matches) < extPat.Index {
			t.Errorf("setExtPat(%q) input %q: expected match at index %d, got %d matches", tt.pattern, tt.input, extPat.Index, len(matches))
			continue
		}
		got := matches[extPat.Index-1][1]
		if got != tt.want {
			t.Errorf("setExtPat(%q) input %q: got %q, want %q", tt.pattern, tt.input, got, tt.want)
		}
	}
}

func TestWrapString(t *testing.T) {
	tests := []struct {
		input string
		width int
		want  string
	}{
		{"abcdef", 3, "abc\ndef"},
		{"abc", 5, "abc"},
		{"", 3, ""},
	}

	for _, tt := range tests {
		got := wrapString(tt.input, tt.width)
		if got != tt.want {
			t.Errorf("wrapString(%q, %d) = %q, want %q", tt.input, tt.width, got, tt.want)
		}
	}
}

func TestFilters(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		regexp   string
		simple   string
		notF     string
		input    string
		expected bool
	}{
		{
			name:     "simple filter match",
			simple:   "fail",
			input:    "login failed",
			expected: true,
		},
		{
			name:     "simple filter mismatch",
			simple:   "fail",
			input:    "login success",
			expected: false,
		},
		{
			name:     "regexp filter match",
			regexp:   "user [0-9]+",
			input:    "login user 123",
			expected: true,
		},
		{
			name:     "regexp filter mismatch",
			regexp:   "user [0-9]+",
			input:    "login user guest",
			expected: false,
		},
		{
			name:     "combined match",
			simple:   "login",
			regexp:   "failed",
			input:    "login failed",
			expected: true,
		},
		{
			name:     "combined mismatch simple",
			simple:   "logout",
			regexp:   "failed",
			input:    "login failed",
			expected: false,
		},
		{
			name:     "not filter match",
			simple:   "login",
			notF:     "success",
			input:    "login failed",
			expected: true,
		},
		{
			name:     "not filter mismatch (excluded)",
			simple:   "login",
			notF:     "success",
			input:    "login success",
			expected: false,
		},
		{
			name:     "arg filter match",
			args:     []string{"login"},
			input:    "login failed",
			expected: true,
		},
		{
			name:     "arg not filter mismatch (excluded)",
			args:     []string{"^success"},
			input:    "login success",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			regexpFilter = tt.regexp
			simpleFilter = tt.simple
			notFilter = tt.notF
			setupFilter(tt.args)
			got := matchFilter(&tt.input)
			if got != tt.expected {
				t.Errorf("matchFilter(%q) = %v, want %v (args=%v, regexp=%q, simple=%q, notF=%q)",
					tt.input, got, tt.expected, tt.args, tt.regexp, tt.simple, tt.notF)
			}
		})
	}
}
