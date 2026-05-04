package cmd

import (
	"strings"
	"testing"
)

func TestCalcStats(t *testing.T) {
	timeList = []timeEnt{
		{Log: "log1", Time: 1000000000}, // 1s
		{Log: "log2", Time: 2000000000}, // 2s
		{Log: "log3", Time: 4000000000}, // 4s
	}
	got := calcStats()
	// Diff: log2-log1 = 1s, log3-log1 = 3s
	// Delta: log2-log1 = 1s, log3-log2 = 2s
	// data = [1.0, 2.0]
	// Mean = 1.5, Median = 1.5, Mode = [1.0, 2.0], StdDiv = 0.5
	if !strings.Contains(got, "Mean:1.500") {
		t.Errorf("calcStats() Mean mismatch, got %q", got)
	}
	if !strings.Contains(got, "Median:1.500") {
		t.Errorf("calcStats() Median mismatch, got %q", got)
	}
	if !strings.Contains(got, "StdDiv:0.500") {
		t.Errorf("calcStats() StdDiv mismatch, got %q", got)
	}
}

func TestUpdateTimeRows(t *testing.T) {
	timeList = []timeEnt{
		{Log: "log1", Time: 1000000000},
		{Log: "log2", Time: 2000000000},
	}
	updateTimeRows(0) // Mark log1
	if len(timeRows) != 2 {
		t.Fatalf("updateTimeRows() expected 2 rows, got %d", len(timeRows))
	}
	if timeList[0].Mark != true {
		t.Errorf("updateTimeRows(0) log1 should be marked")
	}
	if timeList[1].Mark != false {
		t.Errorf("updateTimeRows(0) log2 should not be marked")
	}
	if timeList[0].Diff != 0 {
		t.Errorf("updateTimeRows(0) log1 diff should be 0, got %f", timeList[0].Diff)
	}
	if timeList[1].Diff != 1.0 {
		t.Errorf("updateTimeRows(0) log2 diff should be 1.0, got %f", timeList[1].Diff)
	}
}
