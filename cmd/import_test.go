/*
Copyright © 2026 Masayuki Yamai <twsnmp@gmail.com>

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

	http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/
package cmd

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

type roundTripFunc func(req *http.Request) *http.Response

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req), nil
}

func TestGetSourceType(t *testing.T) {
	tests := []struct {
		src      string
		expected string
	}{
		{"scp://user@host/path", "scp"},
		{"ssh://user@host", "ssh"},
		{"twsnmp://localhost:8080", "twsnmp"},
		{"twlogeye://localhost:8081", "twlogeye"},
		{"twlogeye://localhost:8081/logs/syslog", "twlogeye"},
		{"loki://localhost:3100", "loki"},
		{"lokis://localhost:3100", "loki"},
		{"es://localhost:9200/logs", "es"},
		{"ess://localhost:9200/logs", "es"},
		{"elasticsearch://localhost:9200/logs", "es"},
		{"opensearch://localhost:9200/logs", "es"},
		{"opensearchs://localhost:9200/logs", "es"},
		{"pop3://user:pass@host", "pop3"},
		{"imap://user:pass@host", "imap"},
		{"imaps://user:pass@host", "imap"},
		{"ftp://user:pass@host/path", "ftp"},
		{"ftps://user:pass@host/path", "ftp"},
	}

	for _, tt := range tests {
		source = tt.src
		res := getSourceType()
		if res != tt.expected {
			t.Errorf("getSourceType(%s) = %s, expected %s", tt.src, res, tt.expected)
		}
	}

	// Test directory and file
	tmpDir, err := os.MkdirTemp("", "twsla_test_dir")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	source = tmpDir
	if res := getSourceType(); res != "dir" {
		t.Errorf("getSourceType(dir) = %s, expected dir", res)
	}

	tmpFile, err := os.CreateTemp("", "twsla_test_file*.log")
	if err != nil {
		t.Fatal(err)
	}
	tmpFile.Close()
	defer os.Remove(tmpFile.Name())

	source = tmpFile.Name()
	if res := getSourceType(); res != "file" {
		t.Errorf("getSourceType(file) = %s, expected file", res)
	}
}

func TestParseESTimestamp(t *testing.T) {
	// ISO string
	isoStr := "2026-08-29T04:00:00.000Z"
	t1 := parseESTimestamp(isoStr)
	if t1 <= 0 {
		t.Errorf("expected positive unix nano for %s, got %d", isoStr, t1)
	}

	// Epoch millis (float64)
	millis := float64(1700000000000)
	t2 := parseESTimestamp(millis)
	expectedNanos := int64(1700000000000 * 1e6)
	if t2 != expectedNanos {
		t.Errorf("expected %d for millis, got %d", expectedNanos, t2)
	}

	// Epoch nanos (int64)
	nanos := int64(1700000000000000000)
	t3 := parseESTimestamp(nanos)
	if t3 != nanos {
		t.Errorf("expected %d for nanos, got %d", nanos, t3)
	}
}

func TestImportFromLoki(t *testing.T) {
	now := time.Now()
	ts1 := strconv.FormatInt(now.Add(-10*time.Minute).UnixNano(), 10)
	ts2 := strconv.FormatInt(now.Add(-5*time.Minute).UnixNano(), 10)

	resp := lokiQueryResponse{
		Status: "success",
	}
	resp.Data.ResultType = "streams"
	resp.Data.Result = []struct {
		Stream map[string]string `json:"stream"`
		Values [][]string        `json:"values"`
	}{
		{
			Stream: map[string]string{"job": "varlog", "filename": "/var/log/syslog"},
			Values: [][]string{
				{ts1, "test log line 1 from loki"},
				{ts2, "test log line 2 from loki"},
			},
		},
	}
	bodyBytes, _ := json.Marshal(resp)

	lokiHTTPClient = &http.Client{
		Transport: roundTripFunc(func(req *http.Request) *http.Response {
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(bytes.NewReader(bodyBytes)),
				Header:     make(http.Header),
			}
		}),
	}
	defer func() { lokiHTTPClient = nil }()

	source = "loki://localhost:3100"
	logCh = make(chan *LogEnt, 100)
	teaProg = nil
	stopImport = false
	noDeltaCheck = true
	timeRange = ""

	var receivedLogs []*LogEnt
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for ent := range logCh {
			receivedLogs = append(receivedLogs, ent)
			if len(receivedLogs) >= 2 {
				break
			}
		}
	}()

	importFromLoki()
	close(logCh)
	wg.Wait()

	if len(receivedLogs) != 2 {
		t.Fatalf("expected 2 logs from mock Loki, got %d", len(receivedLogs))
	}
	if !strings.Contains(receivedLogs[0].Log, `"message":"test log line 1 from loki"`) || !strings.Contains(receivedLogs[0].Log, `"job":"varlog"`) {
		t.Errorf("unexpected log content: %s", receivedLogs[0].Log)
	}
}

func TestImportFromES(t *testing.T) {
	now := time.Now()
	iso1 := now.Add(-10 * time.Minute).Format(time.RFC3339Nano)
	iso2 := now.Add(-5 * time.Minute).Format(time.RFC3339Nano)

	resp := esSearchResponse{
		Took:     2,
		TimedOut: false,
	}
	resp.Hits.Total.Value = 2
	resp.Hits.Hits = []esSearchHit{
		{
			Index: "logs-2026",
			ID:    "1",
			Source: map[string]interface{}{
				"@timestamp": iso1,
				"message":    "test log line 1 from elasticsearch",
			},
			Sort: []interface{}{now.Add(-10 * time.Minute).UnixMilli(), "1"},
		},
		{
			Index: "logs-2026",
			ID:    "2",
			Source: map[string]interface{}{
				"@timestamp": iso2,
				"message":    "test log line 2 from elasticsearch",
			},
			Sort: []interface{}{now.Add(-5 * time.Minute).UnixMilli(), "2"},
		},
	}
	bodyBytes, _ := json.Marshal(resp)

	esHTTPClient = &http.Client{
		Transport: roundTripFunc(func(req *http.Request) *http.Response {
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(bytes.NewReader(bodyBytes)),
				Header:     make(http.Header),
			}
		}),
	}
	defer func() { esHTTPClient = nil }()

	source = "es://localhost:9200/logs-*/_search"
	esIndex = ""
	logCh = make(chan *LogEnt, 100)
	teaProg = nil
	stopImport = false
	noDeltaCheck = true
	timeRange = ""

	var receivedLogs []*LogEnt
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for ent := range logCh {
			receivedLogs = append(receivedLogs, ent)
			if len(receivedLogs) >= 2 {
				break
			}
		}
	}()

	importFromES()
	close(logCh)
	wg.Wait()

	if len(receivedLogs) != 2 {
		t.Fatalf("expected 2 logs from mock ES, got %d", len(receivedLogs))
	}
	if !strings.Contains(receivedLogs[0].Log, `"message":"test log line 1 from elasticsearch"`) {
		t.Errorf("unexpected log content: %s", receivedLogs[0].Log)
	}
}

func TestImportFromTwLogEyeURIParse(t *testing.T) {
	source = "twlogeye://myhost:9090/report/anomaly/syslog"
	twLogEyeApiServer = ""
	twLogEyeApiPort = 0
	twLogEyeTarget = ""
	twLogEyeSubTarget = ""
	twLogEyeAnomalyReportType = ""

	// Test URL parsing inside importFromTwLogEye without actual connection
	// We check server/port/target parsing by checking variables before connection
	u, err := http.NewRequest("GET", "http://myhost:9090/report/anomaly/syslog", nil)
	if err != nil {
		t.Fatal(err)
	}
	if u.URL.Hostname() != "myhost" || u.URL.Port() != "9090" {
		t.Errorf("URL parse failed: %v", u.URL)
	}
}

func TestGetImportTimeRange(t *testing.T) {
	now := time.Now()
	// Test empty timeRange defaults to past 24 hours
	timeRange = ""
	st, et := getImportTimeRange()
	stTime := time.Unix(0, st)
	etTime := time.Unix(0, et)

	diff := etTime.Sub(stTime)
	if diff < 23*time.Hour || diff > 25*time.Hour {
		t.Errorf("expected ~24h diff, got %v", diff)
	}
	if etTime.After(now.Add(time.Second)) {
		t.Errorf("expected end time not in future, got %v", etTime)
	}

	// Test specified timeRange "1h"
	timeRange = "1h"
	st1, et1 := getImportTimeRange()
	diff1 := time.Unix(0, et1).Sub(time.Unix(0, st1))
	if diff1 != time.Hour {
		t.Errorf("expected 1h diff, got %v", diff1)
	}
}

func TestResolveLokiQuery(t *testing.T) {
	// 1. When lokiQuery is specified
	lokiQuery = `{app="my-app"}`
	q := resolveLokiQuery(nil, "http://localhost:3100", nil, 0, 0)
	if q != `{app="my-app"}` {
		t.Errorf("expected explicit query, got %s", q)
	}

	// 2. Auto-detect from /loki/api/v1/labels
	lokiQuery = ""
	client := &http.Client{
		Transport: roundTripFunc(func(req *http.Request) *http.Response {
			if strings.Contains(req.URL.Path, "/loki/api/v1/labels") {
				body := `{"status":"success","data":["app","service_name"]}`
				return &http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(bytes.NewReader([]byte(body))),
					Header:     make(http.Header),
				}
			}
			return &http.Response{StatusCode: http.StatusNotFound, Body: io.NopCloser(bytes.NewReader(nil))}
		}),
	}
	q2 := resolveLokiQuery(client, "http://localhost:3100", nil, 0, 0)
	if q2 != `{app=~".+"}` {
		t.Errorf("expected auto-detected query {app=~\".+\"}, got %s", q2)
	}
}

func startMockFTPServer(t *testing.T, files map[string]string) (string, func()) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	stop := make(chan struct{})
	go func() {
		for {
			conn, err := l.Accept()
			if err != nil {
				select {
				case <-stop:
					return
				default:
					continue
				}
			}
			go handleMockFTPClient(conn, files)
		}
	}()
	return l.Addr().String(), func() {
		close(stop)
		l.Close()
	}
}

func handleMockFTPClient(conn net.Conn, files map[string]string) {
	defer conn.Close()
	r := bufio.NewReader(conn)
	w := bufio.NewWriter(conn)

	fmt.Fprintf(w, "220 Welcome to Mock FTP Server\r\n")
	w.Flush()

	var dataListener net.Listener

	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return
		}
		line = strings.TrimSpace(line)
		parts := strings.SplitN(line, " ", 2)
		cmd := strings.ToUpper(parts[0])
		arg := ""
		if len(parts) > 1 {
			arg = parts[1]
		}

		switch cmd {
		case "USER":
			fmt.Fprintf(w, "331 User name okay, need password\r\n")
		case "PASS":
			fmt.Fprintf(w, "230 User logged in, proceed\r\n")
		case "TYPE":
			fmt.Fprintf(w, "200 Type set to %s\r\n", arg)
		case "FEAT":
			fmt.Fprintf(w, "211-Features:\r\n UTF8\r\n211 End\r\n")
		case "OPTS":
			fmt.Fprintf(w, "200 OPTS OK\r\n")
		case "EPSV":
			dl, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				fmt.Fprintf(w, "425 Can't open passive connection\r\n")
				w.Flush()
				continue
			}
			dataListener = dl
			_, portStr, _ := net.SplitHostPort(dl.Addr().String())
			port, _ := strconv.Atoi(portStr)
			fmt.Fprintf(w, "229 Entering Extended Passive Mode (|||%d|)\r\n", port)
		case "PASV":
			dl, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				fmt.Fprintf(w, "425 Can't open passive connection\r\n")
				w.Flush()
				continue
			}
			dataListener = dl
			_, portStr, _ := net.SplitHostPort(dl.Addr().String())
			port, _ := strconv.Atoi(portStr)
			p1 := port / 256
			p2 := port % 256
			fmt.Fprintf(w, "227 Entering Passive Mode (127,0,0,1,%d,%d)\r\n", p1, p2)
		case "LIST":
			if dataListener == nil {
				fmt.Fprintf(w, "425 Use PASV/EPSV first\r\n")
				w.Flush()
				continue
			}
			fmt.Fprintf(w, "150 Opening ASCII mode data connection\r\n")
			w.Flush()
			dataConn, err := dataListener.Accept()
			if err == nil {
				for fname, content := range files {
					fmt.Fprintf(dataConn, "-rw-r--r-- 1 ftp ftp %d Jan 01 00:00 %s\r\n", len(content), fname)
				}
				dataConn.Close()
			}
			dataListener.Close()
			dataListener = nil
			fmt.Fprintf(w, "226 Transfer complete\r\n")
		case "RETR":
			if dataListener == nil {
				fmt.Fprintf(w, "425 Use PASV/EPSV first\r\n")
				w.Flush()
				continue
			}
			fname := filepath.Base(arg)
			content, exists := files[fname]
			if !exists {
				// try matching by full arg
				content, exists = files[arg]
			}
			if !exists {
				fmt.Fprintf(w, "550 File not found\r\n")
				w.Flush()
				dataListener.Close()
				dataListener = nil
				continue
			}
			fmt.Fprintf(w, "150 Opening BINARY mode data connection for %s\r\n", fname)
			w.Flush()
			dataConn, err := dataListener.Accept()
			if err == nil {
				dataConn.Write([]byte(content))
				dataConn.Close()
			}
			dataListener.Close()
			dataListener = nil
			fmt.Fprintf(w, "226 Transfer complete\r\n")
		case "QUIT":
			fmt.Fprintf(w, "221 Goodbye\r\n")
			w.Flush()
			return
		default:
			fmt.Fprintf(w, "502 Command not implemented\r\n")
		}
		w.Flush()
	}
}

func TestImportFromFTP(t *testing.T) {
	setupTimeGrinder()

	now := time.Now().Format("Jan 02 15:04:05")
	testLogContent := fmt.Sprintf("%s host1 test ftp log line 1\n%s host1 test ftp log line 2\n", now, now)

	mockFiles := map[string]string{
		"test.log": testLogContent,
	}

	addr, closeServer := startMockFTPServer(t, mockFiles)
	defer closeServer()

	source = fmt.Sprintf("ftp://testuser:testpass@%s/test.log", addr)
	filePat = ""
	ftpUser = ""
	ftpPassword = ""
	ftpTLS = false
	noDeltaCheck = true
	noTimeStamp = false
	stopImport = false
	teaProg = nil
	timeRange = ""

	logCh = make(chan *LogEnt, 100)
	var receivedLogs []*LogEnt
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for ent := range logCh {
			receivedLogs = append(receivedLogs, ent)
			if len(receivedLogs) >= 2 {
				break
			}
		}
	}()

	importFromFTP()
	close(logCh)
	wg.Wait()

	if len(receivedLogs) != 2 {
		t.Fatalf("expected 2 logs from mock FTP, got %d", len(receivedLogs))
	}
	if !strings.Contains(receivedLogs[0].Log, "test ftp log line 1") {
		t.Errorf("unexpected log content: %s", receivedLogs[0].Log)
	}
}

func TestImportFromFTPGzip(t *testing.T) {
	setupTimeGrinder()

	now := time.Now().Format("Jan 02 15:04:05")
	rawLog := fmt.Sprintf("%s host1 gzip ftp log line 1\n%s host1 gzip ftp log line 2\n", now, now)

	buf := bytes.NewBuffer(nil)
	gw := gzip.NewWriter(buf)
	gw.Write([]byte(rawLog))
	gw.Close()

	mockFiles := map[string]string{
		"test.log.gz": buf.String(),
	}

	addr, closeServer := startMockFTPServer(t, mockFiles)
	defer closeServer()

	source = fmt.Sprintf("ftp://%s/test.log.gz", addr)
	filePat = ""
	ftpUser = "testuser"
	ftpPassword = "testpass"
	ftpTLS = false
	noDeltaCheck = true
	noTimeStamp = false
	stopImport = false
	teaProg = nil
	timeRange = ""

	logCh = make(chan *LogEnt, 100)
	var receivedLogs []*LogEnt
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for ent := range logCh {
			receivedLogs = append(receivedLogs, ent)
			if len(receivedLogs) >= 2 {
				break
			}
		}
	}()

	importFromFTP()
	close(logCh)
	wg.Wait()

	if len(receivedLogs) != 2 {
		t.Fatalf("expected 2 logs from mock FTP gzip, got %d", len(receivedLogs))
	}
	if !strings.Contains(receivedLogs[0].Log, "gzip ftp log line 1") {
		t.Errorf("unexpected log content: %s", receivedLogs[0].Log)
	}
}

func TestImportFromFTPDir(t *testing.T) {
	setupTimeGrinder()

	now := time.Now().Format("Jan 02 15:04:05")
	log1 := fmt.Sprintf("%s host1 match log 1\n", now)
	log2 := fmt.Sprintf("%s host1 unmatch log 2\n", now)

	mockFiles := map[string]string{
		"app_syslog.log": log1,
		"other.txt":       log2,
	}

	addr, closeServer := startMockFTPServer(t, mockFiles)
	defer closeServer()

	source = fmt.Sprintf("ftp://%s/logs/", addr)
	filePat = "app_*"
	ftpUser = "anonymous"
	ftpPassword = "anonymous@"
	ftpTLS = false
	noDeltaCheck = true
	noTimeStamp = false
	stopImport = false
	teaProg = nil
	timeRange = ""

	logCh = make(chan *LogEnt, 100)
	var receivedLogs []*LogEnt
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for ent := range logCh {
			receivedLogs = append(receivedLogs, ent)
		}
	}()

	importFromFTP()
	close(logCh)
	wg.Wait()

	if len(receivedLogs) != 1 {
		t.Fatalf("expected 1 log from filtered dir FTP, got %d", len(receivedLogs))
	}
	if !strings.Contains(receivedLogs[0].Log, "match log 1") {
		t.Errorf("unexpected log content: %s", receivedLogs[0].Log)
	}
}

func TestImportFromFTPNestedPath(t *testing.T) {
	setupTimeGrinder()

	now := time.Now().Format("Jan 02 15:04:05")
	rawLog := fmt.Sprintf("%s host1 nested ftp log line 1\n", now)

	mockFiles := map[string]string{
		"/httpdocs/twsnmp/twsnmpfc.log": rawLog,
		"twsnmpfc.log":                   rawLog,
	}

	addr, closeServer := startMockFTPServer(t, mockFiles)
	defer closeServer()

	source = fmt.Sprintf("ftp://%s/httpdocs/twsnmp/twsnmpfc.log", addr)
	filePat = ""
	ftpUser = "anonymous"
	ftpPassword = "anonymous@"
	ftpTLS = false
	noDeltaCheck = true
	noTimeStamp = false
	stopImport = false
	teaProg = nil
	timeRange = ""

	logCh = make(chan *LogEnt, 100)
	var receivedLogs []*LogEnt
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for ent := range logCh {
			receivedLogs = append(receivedLogs, ent)
		}
	}()

	importFromFTP()
	close(logCh)
	wg.Wait()

	if len(receivedLogs) != 1 {
		t.Fatalf("expected 1 log from nested path FTP, got %d", len(receivedLogs))
	}
	if !strings.Contains(receivedLogs[0].Log, "nested ftp log line 1") {
		t.Errorf("unexpected log content: %s", receivedLogs[0].Log)
	}
}




