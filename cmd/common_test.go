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
