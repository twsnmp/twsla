package datastore

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDataStores(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "twsla_datastore_test_*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	now := time.Now().UnixNano()
	entries := []*LogEntry{
		{
			Time:     now - 2000,
			Hash:     "hash1",
			Line:     1,
			Log:      "log 1 outside range before",
			Delta:    0,
			HasDelta: false,
		},
		{
			Time:     now - 1000,
			Hash:     "hash1",
			Line:     2,
			Log:      "log 2 inside range",
			Delta:    -500,
			HasDelta: true,
		},
		{
			Time:     now,
			Hash:     "hash2",
			Line:     3,
			Log:      "log 3 inside range",
			Delta:    0,
			HasDelta: false,
		},
		{
			Time:     now + 2000,
			Hash:     "hash2",
			Line:     4,
			Log:      "log 4 outside range after",
			Delta:    0,
			HasDelta: false,
		},
	}

	tests := []struct {
		name       string
		path       string
		engineType EngineType
		supportSPF bool
	}{
		{
			name:       "bbolt",
			path:       filepath.Join(tmpDir, "test.db"),
			engineType: EngineBbolt,
			supportSPF: true,
		},
		{
			name:       "badger",
			path:       filepath.Join(tmpDir, "test.badger"),
			engineType: EngineBadger,
			supportSPF: true,
		},
		{
			name:       "parquet",
			path:       filepath.Join(tmpDir, "test.parquet"),
			engineType: EngineParquet,
			supportSPF: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ds, err := Open(tt.path)
			if err != nil {
				t.Fatalf("Open failed: %v", err)
			}
			defer ds.Close()

			if ds.Type() != tt.engineType {
				t.Errorf("expected engine %v, got %v", tt.engineType, ds.Type())
			}

			// Test SaveLogs
			if err := ds.SaveLogs(entries); err != nil {
				t.Fatalf("SaveLogs failed: %v", err)
			}

			// Test ForEach within [now-1000, now]
			var scanned []*LogEntry
			err = ds.ForEach(now-1000, now, func(e *LogEntry) bool {
				scanned = append(scanned, e)
				return true
			})
			if err != nil {
				t.Fatalf("ForEach failed: %v", err)
			}

			if len(scanned) != 2 {
				t.Fatalf("expected 2 scanned logs, got %d", len(scanned))
			}

			if scanned[0].Log != "log 2 inside range" || scanned[0].Delta != -500 || !scanned[0].HasDelta {
				t.Errorf("unexpected scanned[0]: %+v", scanned[0])
			}
			if scanned[1].Log != "log 3 inside range" {
				t.Errorf("unexpected scanned[1]: %+v", scanned[1])
			}

			// Test SPF
			if tt.supportSPF {
				spfData := map[string]string{"test@example.com": "pass"}
				if err := ds.SaveSPF(spfData); err != nil {
					t.Fatalf("SaveSPF failed: %v", err)
				}
				val, err := ds.GetSPF("test@example.com")
				if err != nil {
					t.Fatalf("GetSPF failed: %v", err)
				}
				if val != "pass" {
					t.Errorf("expected pass, got %s", val)
				}
			} else {
				if err := ds.SaveSPF(map[string]string{"a": "b"}); err != ErrUnsupportedOperation {
					t.Errorf("expected ErrUnsupportedOperation, got %v", err)
				}
				if _, err := ds.GetSPF("a"); err != ErrUnsupportedOperation {
					t.Errorf("expected ErrUnsupportedOperation, got %v", err)
				}
			}
		})
	}
}
