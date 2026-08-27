package datastore

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

var (
	ErrUnsupportedOperation = errors.New("unsupported operation for this datastore type")
)

type EngineType string

const (
	EngineBbolt   EngineType = "bbolt"
	EngineBadger  EngineType = "badger"
	EngineParquet EngineType = "parquet"
)

// LogEntry represents a single log record.
type LogEntry struct {
	Time     int64  // UnixNano
	Hash     string // Log source or format hash
	Line     int    // Line number
	Log      string // Raw log string
	Delta    int64  // Delay / delta in nanoseconds (valid if HasDelta is true)
	HasDelta bool
}

// ID returns the standard key format used in bbolt and badger.
func (l *LogEntry) ID() string {
	return fmt.Sprintf("%016x:%s:%x", l.Time, l.Hash, l.Line)
}

// ParseID parses a key string into Time, Hash, and Line.
func ParseID(id string) (int64, string, int, error) {
	parts := strings.Split(id, ":")
	if len(parts) < 3 {
		return 0, "", 0, fmt.Errorf("invalid log id: %s", id)
	}
	t, err := strconv.ParseInt(parts[0], 16, 64)
	if err != nil {
		return 0, "", 0, fmt.Errorf("invalid time in log id: %w", err)
	}
	line, err := strconv.ParseInt(parts[2], 16, 64)
	if err != nil {
		return t, parts[1], 0, fmt.Errorf("invalid line in log id: %w", err)
	}
	return t, parts[1], int(line), nil
}

// ScanCallback is a function called for each LogEntry during scan.
// Returning false stops the iteration.
type ScanCallback func(entry *LogEntry) bool

// DataStore represents the storage backend for twsla logs.
type DataStore interface {
	Open(path string) error
	Close() error
	Type() EngineType

	// SaveLogs saves a batch of log entries.
	SaveLogs(entries []*LogEntry) error

	// ForEach iterates over logs in the time range [st, et] (UnixNano).
	ForEach(st, et int64, fn ScanCallback) error

	// SPF cache for email command. Returns ErrUnsupportedOperation for Parquet.
	GetSPF(key string) (string, error)
	SaveSPF(spfMap map[string]string) error
	LoadSPF(dst map[string]string) error
}

// DetectEngineType determines the engine type based on path.
func DetectEngineType(path string) EngineType {
	lower := strings.ToLower(path)
	if strings.HasPrefix(lower, "badger://") || strings.HasSuffix(lower, ".badger") {
		return EngineBadger
	}
	if strings.HasPrefix(lower, "parquet://") || strings.HasSuffix(lower, ".parquet") || strings.HasSuffix(lower, ".pq") {
		return EngineParquet
	}
	if strings.HasPrefix(lower, "bbolt://") || strings.HasSuffix(lower, ".bbolt") || strings.HasSuffix(lower, ".db") {
		return EngineBbolt
	}

	// Check if path is an existing directory
	if fi, err := os.Stat(path); err == nil && fi.IsDir() {
		// If it contains MANIFEST (Badger)
		if _, err := os.Stat(filepath.Join(path, "MANIFEST")); err == nil {
			return EngineBadger
		}
		// If it contains parquet files
		matches, _ := filepath.Glob(filepath.Join(path, "*.parquet"))
		if len(matches) > 0 {
			return EngineParquet
		}
	}

	return EngineBbolt
}

// Open opens or creates a DataStore at the specified path.
func Open(path string) (DataStore, error) {
	cleanPath := path
	cleanPath = strings.TrimPrefix(cleanPath, "bbolt://")
	cleanPath = strings.TrimPrefix(cleanPath, "badger://")
	cleanPath = strings.TrimPrefix(cleanPath, "parquet://")

	engine := DetectEngineType(path)
	var ds DataStore
	switch engine {
	case EngineBadger:
		ds = NewBadgerDataStore()
	case EngineParquet:
		ds = NewParquetDataStore()
	case EngineBbolt:
		ds = NewBboltDataStore()
	default:
		ds = NewBboltDataStore()
	}

	if err := ds.Open(cleanPath); err != nil {
		return nil, err
	}
	return ds, nil
}
