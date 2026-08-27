package datastore

import (
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/parquet-go/parquet-go"
)

type ParquetLogRecord struct {
	Time     int64  `parquet:"time,snappy"`
	Hash     string `parquet:"hash,dict,snappy"`
	Line     int32  `parquet:"line,snappy"`
	Log      string `parquet:"log,zstd"`
	Delta    int64  `parquet:"delta,snappy"`
	HasDelta bool   `parquet:"has_delta,snappy"`
}

type ParquetDataStore struct {
	dirPath string
}

func NewParquetDataStore() *ParquetDataStore {
	return &ParquetDataStore{}
}

func (s *ParquetDataStore) Type() EngineType {
	return EngineParquet
}

func (s *ParquetDataStore) Open(path string) error {
	s.dirPath = path
	// If path has .parquet extension or not, create it as a directory to hold part files
	if err := os.MkdirAll(s.dirPath, 0755); err != nil {
		return fmt.Errorf("create parquet directory: %w", err)
	}
	return nil
}

func (s *ParquetDataStore) Close() error {
	return nil
}

func (s *ParquetDataStore) SaveLogs(entries []*LogEntry) error {
	if len(entries) == 0 {
		return nil
	}

	records := make([]ParquetLogRecord, len(entries))
	for i, e := range entries {
		records[i] = ParquetLogRecord{
			Time:     e.Time,
			Hash:     e.Hash,
			Line:     int32(e.Line),
			Log:      e.Log,
			Delta:    e.Delta,
			HasDelta: e.HasDelta,
		}
	}

	var randBytes [4]byte
	_, _ = rand.Read(randBytes[:])
	randVal := binary.BigEndian.Uint32(randBytes[:])

	fileName := fmt.Sprintf("part_%016x_%08x.parquet", time.Now().UnixNano(), randVal)
	filePath := filepath.Join(s.dirPath, fileName)

	file, err := os.Create(filePath)
	if err != nil {
		return fmt.Errorf("create parquet file: %w", err)
	}
	defer file.Close()

	writer := parquet.NewGenericWriter[ParquetLogRecord](file)
	if _, err := writer.Write(records); err != nil {
		return fmt.Errorf("write parquet records: %w", err)
	}
	if err := writer.Close(); err != nil {
		return fmt.Errorf("close parquet writer: %w", err)
	}

	return nil
}

func (s *ParquetDataStore) ForEach(st, et int64, fn ScanCallback) error {
	files, err := filepath.Glob(filepath.Join(s.dirPath, "*.parquet"))
	if err != nil {
		return err
	}
	sort.Strings(files)

	for _, filePath := range files {
		f, err := os.Open(filePath)
		if err != nil {
			continue
		}

		fi, err := f.Stat()
		if err != nil || fi.Size() == 0 {
			f.Close()
			continue
		}

		pf, err := parquet.OpenFile(f, fi.Size())
		if err != nil {
			f.Close()
			continue
		}

		// Check row groups min/max if possible
		reader := parquet.NewGenericReader[ParquetLogRecord](pf)
		buf := make([]ParquetLogRecord, 1024)
		stop := false

		for {
			n, err := reader.Read(buf)
			if n > 0 {
				for i := 0; i < n; i++ {
					rec := &buf[i]
					if rec.Time < st {
						continue
					}
					if rec.Time > et {
						continue
					}
					entry := &LogEntry{
						Time:     rec.Time,
						Hash:     rec.Hash,
						Line:     int(rec.Line),
						Log:      rec.Log,
						Delta:    rec.Delta,
						HasDelta: rec.HasDelta,
					}
					if !fn(entry) {
						stop = true
						break
					}
				}
			}
			if stop || err != nil {
				if err != nil && err != io.EOF {
					// reading error
				}
				break
			}
		}
		reader.Close()
		f.Close()

		if stop {
			break
		}
	}
	return nil
}

func (s *ParquetDataStore) GetSPF(key string) (string, error) {
	return "", ErrUnsupportedOperation
}

func (s *ParquetDataStore) SaveSPF(spfMap map[string]string) error {
	return ErrUnsupportedOperation
}

func (s *ParquetDataStore) LoadSPF(dst map[string]string) error {
	return ErrUnsupportedOperation
}
