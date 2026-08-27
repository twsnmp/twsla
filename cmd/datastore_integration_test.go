package cmd

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/twsnmp/twsla/pkg/datastore"
)

func TestAllDataStoresIntegration(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "twsla_integration_*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	now := time.Now().UnixNano()
	entries := []*datastore.LogEntry{
		{
			Time:     now - 1000000000,
			Hash:     "h1",
			Line:     1,
			Log:      "2026-08-28 01:00:00 [ERROR] user login failed from 192.168.1.10",
			Delta:    0,
			HasDelta: false,
		},
		{
			Time:     now - 500000000,
			Hash:     "h1",
			Line:     2,
			Log:      "2026-08-28 01:01:00 [WARN] disk usage high on server1",
			Delta:    -1000,
			HasDelta: true,
		},
		{
			Time:     now,
			Hash:     "h1",
			Line:     3,
			Log:      "2026-08-28 01:02:00 [INFO] user login success from 192.168.1.11",
			Delta:    0,
			HasDelta: false,
		},
	}

	targets := []struct {
		name       string
		path       string
		engineType datastore.EngineType
	}{
		{"bbolt", filepath.Join(tmpDir, "test.db"), datastore.EngineBbolt},
		{"badger", filepath.Join(tmpDir, "test.badger"), datastore.EngineBadger},
		{"parquet", filepath.Join(tmpDir, "test.parquet"), datastore.EngineParquet},
	}

	for _, tt := range targets {
		t.Run(tt.name, func(t *testing.T) {
			dataStore = tt.path
			if err := openDB(); err != nil {
				t.Fatalf("[%s] openDB failed: %v", tt.name, err)
			}
			if ds.Type() != tt.engineType {
				t.Fatalf("[%s] expected type %v, got %v", tt.name, tt.engineType, ds.Type())
			}

			if err := ds.SaveLogs(entries); err != nil {
				t.Fatalf("[%s] SaveLogs failed: %v", tt.name, err)
			}

			// Test SPF if supported before closing
			if tt.engineType != datastore.EngineParquet {
				testMap := map[string]string{"user@test.com": "pass"}
				if err := ds.SaveSPF(testMap); err != nil {
					t.Fatalf("[%s] SaveSPF failed: %v", tt.name, err)
				}
				loaded := make(map[string]string)
				if err := ds.LoadSPF(loaded); err != nil {
					t.Fatalf("[%s] LoadSPF failed: %v", tt.name, err)
				}
				if loaded["user@test.com"] != "pass" {
					t.Errorf("[%s] expected pass, got %s", tt.name, loaded["user@test.com"])
				}
			}
			_ = closeDB()

			// Test MCP Search tool (opens and closes DB internally)
			res, _, err := searchLog(context.Background(), nil, searchLogParams{
				Filter: "[ERROR]",
				Limit:  10,
			})
			if err != nil {
				t.Fatalf("[%s] searchLog failed: %v", tt.name, err)
			}
			if len(res.Content) == 0 {
				t.Fatalf("[%s] expected search result", tt.name)
			}

			// Test MCP Count tool
			cRes, _, err := countLog(context.Background(), nil, countLogParams{
				Unit: "word",
			})
			if err != nil {
				t.Fatalf("[%s] countLog failed: %v", tt.name, err)
			}
			if len(cRes.Content) == 0 {
				t.Fatalf("[%s] expected count result", tt.name)
			}
		})
	}
}

func TestEmailParquetRejection(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "twsla_email_parquet_*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	dataStore = filepath.Join(tmpDir, "test.parquet")
	if err := openDB(); err != nil {
		t.Fatalf("openDB failed: %v", err)
	}
	defer closeDB()

	if ds.Type() != datastore.EngineParquet {
		t.Fatalf("expected parquet datastore")
	}

	// Verify SPF operations return ErrUnsupportedOperation
	if err := ds.SaveSPF(map[string]string{"a": "b"}); err != datastore.ErrUnsupportedOperation {
		t.Errorf("expected ErrUnsupportedOperation, got %v", err)
	}
	if _, err := ds.GetSPF("a"); err != datastore.ErrUnsupportedOperation {
		t.Errorf("expected ErrUnsupportedOperation, got %v", err)
	}
}

func TestParquetMultiFileScan(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "twsla_parquet_multi_*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	pqPath := filepath.Join(tmpDir, "test.parquet")
	ds, err := datastore.Open(pqPath)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer ds.Close()

	now := time.Now().UnixNano()

	// Batch 1
	b1 := []*datastore.LogEntry{
		{Time: now - 3000, Hash: "h1", Line: 1, Log: "batch 1 log 1"},
		{Time: now - 2000, Hash: "h1", Line: 2, Log: "batch 1 log 2"},
	}
	if err := ds.SaveLogs(b1); err != nil {
		t.Fatalf("SaveLogs batch 1 failed: %v", err)
	}

	time.Sleep(10 * time.Millisecond)

	// Batch 2
	b2 := []*datastore.LogEntry{
		{Time: now - 1000, Hash: "h1", Line: 3, Log: "batch 2 log 1"},
		{Time: now, Hash: "h1", Line: 4, Log: "batch 2 log 2"},
	}
	if err := ds.SaveLogs(b2); err != nil {
		t.Fatalf("SaveLogs batch 2 failed: %v", err)
	}

	// Verify multiple .parquet files are created
	files, _ := filepath.Glob(filepath.Join(pqPath, "*.parquet"))
	if len(files) != 2 {
		t.Fatalf("expected 2 parquet files in dir, got %d", len(files))
	}

	// Scan all
	var allLogs []string
	err = ds.ForEach(now-4000, now+1000, func(e *datastore.LogEntry) bool {
		allLogs = append(allLogs, e.Log)
		return true
	})
	if err != nil {
		t.Fatalf("ForEach failed: %v", err)
	}
	if len(allLogs) != 4 {
		t.Fatalf("expected 4 logs, got %d: %v", len(allLogs), allLogs)
	}

	// Filtered scan
	var filtered []string
	simpleFilter = "batch 1"
	setupFilter([]string{})
	err = ds.ForEach(now-4000, now+1000, func(e *datastore.LogEntry) bool {
		l := e.Log
		if matchFilter(&l) {
			filtered = append(filtered, l)
		}
		return true
	})
	if err != nil {
		t.Fatalf("ForEach with filter failed: %v", err)
	}
	if len(filtered) != 2 {
		t.Fatalf("expected 2 filtered logs, got %d: %v", len(filtered), filtered)
	}
	simpleFilter = ""
}
