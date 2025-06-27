/*
Copyright © 2025 Masayuki Yamai <twsnmp@gmail.com>

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
	"archive/tar"
	"archive/zip"
	"bufio"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/0xrawsec/golang-evtx/evtx"
	"github.com/domainr/dnsr"
	"github.com/dustin/go-humanize"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/spf13/cobra"
	"go.etcd.io/bbolt"
)

var mcpTrapsport = ""
var mcpEndpoint = ""
var mcpClients = ""

// mcpCmd represents the mcp command
var mcpCmd = &cobra.Command{
	Use:   "mcp",
	Short: "MCP server",
	Long:  `MCP server for AI agent`,
	Run: func(cmd *cobra.Command, args []string) {
		mcpServer()
	},
}

func init() {
	rootCmd.AddCommand(mcpCmd)
	mcpCmd.Flags().StringVar(&mcpTrapsport, "transport", "stdio", "MCP server transport(stdio/sse/stream)")
	mcpCmd.Flags().StringVar(&mcpEndpoint, "endpoint", "127.0.0.1:8085", "MCP server endpoint(bind address:port)")
	mcpCmd.Flags().StringVar(&mcpClients, "clients", "", "IP address of MCP client to be allowed to connect Specify by comma delimiter")
	mcpCmd.Flags().StringVar(&geoipDBPath, "geoip", "", "geo IP database file")
}

func mcpServer() {
	// Create MCP Server
	s := server.NewMCPServer(
		"TWSLA MCP Server",
		"1.0.0",
		server.WithToolCapabilities(true),
		server.WithLogging(),
	)
	// Add tools to MCP server
	addSearchTool(s)
	addCountTool(s)
	addExtractTool(s)
	addImportTool(s)
	addLogSummaryTool(s)

	// Start MCP server
	switch mcpTrapsport {
	case "stdio":
		if err := server.ServeStdio(s); err != nil {
			log.Printf("Server error: %v\n", err)
		}
	case "sse":
		sseServer := server.NewSSEServer(s)
		log.Printf("SSE server listening on %s", mcpEndpoint)
		if err := sseServer.Start(mcpEndpoint); err != nil {
			log.Fatalf("Server error: %v", err)
		}
	case "stream":
		streamServer := server.NewStreamableHTTPServer(s)
		log.Printf("streamable server listening on %s clients='%s'", mcpEndpoint, mcpClients)
		if mcpClients != "" {
			var clMap sync.Map
			for _, ip := range strings.Split(mcpClients, ",") {
				clMap.Store(ip, true)
			}
			http.Handle("/mcp", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				ip, err := net.ResolveTCPAddr("tcp", r.RemoteAddr)
				if err != nil {
					log.Printf("err=%v", err)
					w.WriteHeader(http.StatusBadRequest)
					return
				}
				if _, ok := clMap.Load(ip.IP.String()); !ok {
					log.Printf("connection refused from %s", ip.IP.String())
					w.WriteHeader(http.StatusNotFound)
					return
				}
				streamServer.ServeHTTP(w, r)
			}))
			if err := http.ListenAndServe(mcpEndpoint, nil); err != nil {
				log.Fatalf("streamable server error: %v", err)
			}
		} else {
			if err := streamServer.Start(mcpEndpoint); err != nil {
				log.Fatalf("streamable server error: %v", err)
			}
		}
	default:
		log.Fatalf("transport '%s' not suported", mcpTrapsport)
	}
}

func addSearchTool(s *server.MCPServer) {
	searchTool := mcp.NewTool("search_log",
		mcp.WithDescription("Search logs from TWSLA DB"),
		mcp.WithString("filter_log_content",
			mcp.Description("Filter logs by regular expression. Empty is no filter"),
		),
		mcp.WithNumber("limit_log_count",
			mcp.DefaultNumber(100),
			mcp.Max(10000),
			mcp.Min(1),
			mcp.Description("Limit on number of logs retrieved. min 100,max 10000"),
		),
		mcp.WithString("time_range",
			mcp.Required(),
			mcp.Description(
				`Time range of logs to search 
format is "start date/time, period" 
or "start date/time, end date/time".
Example: 
2025/05/07 05:59:00,1h 
2025/05/07 05:59:00,2025/05/07 06:59:00
`),
		),
	)

	s.AddTool(searchTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var err error
		regexpFilter = request.GetString("filter_log_content", "")
		timeRange, err = request.RequireString("time_range")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		limit := request.GetInt("limit_log_count", 100)
		setupFilter([]string{})
		if err := openDB(); err != nil {
			return nil, err
		}
		defer db.Close()
		results = []string{}
		sti, eti := getTimeRange()
		sk := fmt.Sprintf("%016x:", sti)
		db.View(func(tx *bbolt.Tx) error {
			b := tx.Bucket([]byte("logs"))
			c := b.Cursor()
			for k, v := c.Seek([]byte(sk)); k != nil; k, v = c.Next() {
				a := strings.Split(string(k), ":")
				if len(a) < 1 {
					continue
				}
				t, err := strconv.ParseInt(a[0], 16, 64)
				if err == nil && t > eti {
					break
				}
				l := string(v)
				if matchFilter(&l) {
					results = append(results, l)
					if len(results) >= limit {
						break
					}
				}
			}
			return nil
		})
		ret, err := json.Marshal(&results)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return mcp.NewToolResultText(string(ret)), nil
	})
}

type mcpCountEnt struct {
	Key   string
	Count int
}

func addCountTool(s *server.MCPServer) {
	searchTool := mcp.NewTool("count_log",
		mcp.WithDescription(
			`This tool counts the number of logs on the TWSLA database.
The number of logs can be counted by time, IP address, MAC address, e-mail address,host name,domain name,country or geo location.`),
		mcp.WithString("filter_log_content",
			mcp.Description("Filter logs by regular expression. Empty is no filter"),
		),
		mcp.WithString("count_unit",
			mcp.Required(),
			mcp.Description(
				`Unit of counting(time, ip, email, mac, host,domain, country,loc)
 time:Count hourly or minutely
 ip: Count by IP address
 email:Count  by email
 mac: Count by MAC address
 host: Count by host name of IP address
 domain: Count by domain name of IP address
 country: Count by country of IP address
 loc: Count by geo location of IP address
 normalize: Count by normalized pattern
`),
			mcp.Enum("time", "ip", "email", "mac", "host", "domain", "country", "loc", "normalize"),
		),
		mcp.WithString("time_range",
			mcp.Required(),
			mcp.Description(
				`Time range of logs to search 
format is "start date/time, period" 
or "start date/time, end date/time".
Example: 
2025/05/07 05:59:00,1h 
2025/05/07 05:59:00,2025/05/07 06:59:00
`),
		),
		mcp.WithNumber("unit_pos",
			mcp.DefaultNumber(1),
			mcp.Max(100),
			mcp.Min(1),
			mcp.Description("position of unit"),
		),
		mcp.WithNumber("top_n",
			mcp.DefaultNumber(10),
			mcp.Max(1000),
			mcp.Min(1),
			mcp.Description("limit top n"),
		),
		mcp.WithNumber("interval",
			mcp.DefaultNumber(0),
			mcp.Max(3600*24),
			mcp.Min(0),
			mcp.Description("If unit is time,specify the aggregation interval in seconds. 0 is auto select interval"),
		),
	)

	s.AddTool(searchTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var err error
		regexpFilter = request.GetString("filter_log_content", "")
		timeMode := true
		normalizeMode := false
		ipm := 0
		extract = ""
		unit, err := request.RequireString("count_unit")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		switch unit {
		case "mac", "email":
			extract = unit
			timeMode = false
		case "ip":
			extract = "ip"
			timeMode = false
		case "host":
			ipm = 1
			extract = "ip"
			timeMode = false
			dnsResolver = dnsr.NewWithTimeout(10000, time.Millisecond*1000)
		case "domain":
			ipm = 2
			extract = "ip"
			timeMode = false
			dnsResolver = dnsr.NewWithTimeout(10000, time.Millisecond*1000)
		case "loc":
			if err := openGeoIPDB(); err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			ipm = 3
			extract = "ip"
			timeMode = false
		case "country":
			if err := openGeoIPDB(); err != nil {
				return nil, err
			}
			ipm = 4
			extract = "ip"
			timeMode = false
		case "normalize":
			timeMode = false
			normalizeMode = true
		}
		timeRange, err = request.RequireString("time_range")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		pos = request.GetInt("pos", 1)
		interval = request.GetInt("interval", 0)
		topN := request.GetInt("topN", 10)
		setupFilter([]string{})
		extPat = nil
		setExtPat()
		if !timeMode && extPat == nil && !normalizeMode {
			return mcp.NewToolResultError("invalid unit"), nil
		}
		if normalizeMode {
			setupTimeGrinder()
		}
		if err := openDB(); err != nil {
			return nil, err
		}
		defer db.Close()
		var countMap = make(map[string]int)
		intv := int64(getInterval()) * 1000 * 1000 * 1000
		sti, eti := getTimeRange()
		sk := fmt.Sprintf("%016x:", sti)
		db.View(func(tx *bbolt.Tx) error {
			b := tx.Bucket([]byte("logs"))
			c := b.Cursor()
			for k, v := c.Seek([]byte(sk)); k != nil; k, v = c.Next() {
				a := strings.Split(string(k), ":")
				if len(a) < 1 {
					continue
				}
				t, err := strconv.ParseInt(a[0], 16, 64)
				if err == nil && t > eti {
					break
				}
				l := string(v)
				if matchFilter(&l) {
					if timeMode {
						d := t / intv
						ck := time.Unix(0, d*intv).Format("2006/01/02 15:04")
						countMap[ck]++
					} else if normalizeMode {
						ck := normalizeLog(l)
						countMap[ck]++
					} else {
						a := extPat.ExtReg.FindAllStringSubmatch(l, -1)
						if len(a) >= extPat.Index {
							ck := a[extPat.Index-1][1]
							if ipm > 0 {
								ck = getIPInfo(ck, ipm)
							}
							countMap[ck]++
						}
					}
				}
			}
			return nil
		})
		cl := []mcpCountEnt{}
		for k, v := range countMap {
			cl = append(cl, mcpCountEnt{
				Key:   k,
				Count: v,
			})
		}
		if timeMode {
			sort.Slice(cl, func(i, j int) bool {
				return cl[i].Key < cl[j].Key
			})
		} else {
			sort.Slice(cl, func(i, j int) bool {
				return cl[i].Count > cl[j].Count
			})
			if len(cl) > topN {
				cl = cl[:topN]
			}
		}
		ret, err := json.Marshal(&cl)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return mcp.NewToolResultText(string(ret)), nil
	})
}

type mcpExtractEnt struct {
	Time  string
	Value string
}

func addExtractTool(s *server.MCPServer) {
	searchTool := mcp.NewTool("extract_data_from_log",
		mcp.WithDescription(
			`This tool extracts data from the logs on the TWSLA database.`),
		mcp.WithString("filter_log_content",
			mcp.Description("Filter logs by regular expression"),
		),
		mcp.WithString("extract_pattern",
			mcp.Required(),
			mcp.Description(
				`Specifies the pattern of data to be extracted.
(ip,mac,email,number,regular expression)
 ip: IP address
 email: EMail address
 mac: MAC address
 number: Number
 regular expression example: 
 	cpu=([-+0-9.]+)
	ip=([0-9]{1,3}\.[0-9]{1,3}\.[0-9]{1,3}\.[0-9]{1,3})
 `),
		),
		mcp.WithString("time_range",
			mcp.Required(),
			mcp.Description(
				`Time range of logs to extract data.
format is "start date/time, period" 
or "start date/time, end date/time".
Example: 
2025/05/07 05:59:00,1h 
2025/05/07 05:59:00,2025/05/07 06:59:00
`),
		),
		mcp.WithNumber("pos",
			mcp.DefaultNumber(1),
			mcp.Max(100),
			mcp.Min(1),
			mcp.Description("position of extract data"),
		),
	)

	s.AddTool(searchTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var err error
		regexpFilter = request.GetString("filter_log_content", "")
		extract = request.GetString("extract_pattern", "")
		pos = request.GetInt("pos", 1)
		timeRange, err = request.RequireString("time_range")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		setupFilter([]string{})
		extPat = nil
		setExtPat()
		if extPat == nil {
			return mcp.NewToolResultError("invalid extract_pattern"), nil
		}
		if err := openDB(); err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		defer db.Close()
		mcpExtractList := []mcpExtractEnt{}
		sti, eti := getTimeRange()
		sk := fmt.Sprintf("%016x:", sti)
		db.View(func(tx *bbolt.Tx) error {
			b := tx.Bucket([]byte("logs"))
			c := b.Cursor()
			for k, v := c.Seek([]byte(sk)); k != nil; k, v = c.Next() {
				a := strings.Split(string(k), ":")
				if len(a) < 1 {
					continue
				}
				t, err := strconv.ParseInt(a[0], 16, 64)
				if err == nil && t > eti {
					break
				}
				l := string(v)
				if matchFilter(&l) {
					a := extPat.ExtReg.FindAllStringSubmatch(l, -1)
					if len(a) >= extPat.Index && len(a[extPat.Index-1]) > 1 {
						mcpExtractList = append(mcpExtractList, mcpExtractEnt{Time: time.Unix(0, t).Format(time.RFC3339Nano), Value: a[extPat.Index-1][1]})
					}
				}
			}
			return nil
		})
		ret, err := json.Marshal(&mcpExtractList)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return mcp.NewToolResultText(string(ret)), nil
	})
}

func addImportTool(s *server.MCPServer) {
	searchTool := mcp.NewTool("import_log",
		mcp.WithDescription(
			`This tool  import the logs to the TWSLA database.`),
		mcp.WithString("log_path",
			mcp.Required(),
			mcp.Description(
				`log file or directory path to import.
Files inside archive files such as zip, tar.gz, gz, etc. can be targeted for import.
If a directory is specified, all files in the directory are targeted.
Filenames matching filename_pattern are targeted.`),
		),
		mcp.WithString("filename_pattern",
			mcp.Description(
				`log file name regular expression pattern filter to import.
This applies to files in directories and files in archive files such as ZIP.`),
		),
	)

	s.AddTool(searchTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var err error
		filePat = request.GetString("filename_pattern", "")
		source, err = request.RequireString("log_path")

		log.Printf("mcp import source='%s' filename_pattern='%s'", source, filePat)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		if err := openDB(); err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		defer db.Close()
		totalFiles = 0
		totalLines = 0
		totalBytes = 0
		setupTimeGrinder()
		logCh = make(chan *LogEnt, 10000)
		var wg sync.WaitGroup
		wg.Add(1)
		go logSaver(&wg)
		if err := mcpImport(source); err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		close(logCh)
		wg.Wait()
		var r struct {
			Files string
			Lines string
			Bytes string
		}
		r.Files = humanize.Bytes(uint64(totalFiles))
		r.Lines = humanize.Bytes(uint64(totalLines))
		r.Bytes = humanize.Bytes(uint64(totalBytes))
		ret, err := json.Marshal(&r)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return mcp.NewToolResultText(string(ret)), nil
	})
}

func mcpImport(path string) error {
	s, err := os.Stat(path)
	if err != nil {
		return err
	}
	if s.IsDir() {
		return mcpImportFromDir(path)
	}
	return mcpImportFromFile(path)
}

func mcpImportFromFile(path string) error {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".zip":
		return mcpImortFromZIPFile(path)
	case ".evtx":
		return mcpImportFromWindowsEvtx(path)
	case ".tgz":
		return mcpImportFormTarGZFile(path)
	case ".gz":
		if strings.HasSuffix(path, ".tar.gz") {
			return mcpImportFormTarGZFile(path)
		}
	}
	r, err := os.Open(path)
	if err != nil {
		log.Panicln(err)
	}
	defer r.Close()
	if ext == ".gz" {
		if gzr, err := gzip.NewReader(r); err == nil {
			return mcpDoImport(gzr)
		} else {
			return err
		}
	}
	return mcpDoImport(r)
}

func mcpImortFromZIPFile(path string) error {
	r, err := zip.OpenReader(path)
	if err != nil {
		return err
	}
	defer r.Close()
	filter := getSimpleFilter(filePat)
	for _, f := range r.File {
		p := filepath.Base(f.Name)
		if filter != nil && !filter.MatchString(p) {
			continue
		}
		r, err := f.Open()
		if err != nil {
			continue
		}
		ext := strings.ToLower(filepath.Ext(f.Name))
		switch ext {
		case ".gz":
			if gzr, err := gzip.NewReader(r); err == nil {
				doImport(path+":"+f.Name, gzr)
			}
		case ".evtx":
			w, err := os.CreateTemp("", "winlog*.evtx")
			if err != nil {
				return err
			}
			defer os.Remove(w.Name())
			io.Copy(w, r)
			w.Close()
			importFromWindowsEvtx(w.Name())
		default:
			if err := mcpDoImport(r); err != nil {
				return err
			}
		}
	}
	return nil
}

func mcpImportFormTarGZFile(path string) error {
	r, err := os.Open(path)
	if err != nil {
		return err
	}
	defer r.Close()
	gzr, err := gzip.NewReader(r)
	if err != nil {
		return err
	}
	filter := getSimpleFilter(filePat)
	tgzr := tar.NewReader(gzr)
	for {
		f, err := tgzr.Next()
		if err != nil {
			return nil
		}
		if filter != nil && !filter.MatchString(f.Name) {
			continue
		}
		if strings.HasSuffix(f.Name, ".gz") {
			igzr, err := gzip.NewReader(tgzr)
			if err != nil {
				return err
			}
			if err := mcpDoImport(igzr); err != nil {
				return err
			}
		} else {
			if err := mcpDoImport(tgzr); err != nil {
				return err
			}
		}
	}
}

func mcpImportFromDir(path string) error {
	pat := "*"
	if filePat != "" {
		pat = filePat
	}
	files, err := filepath.Glob(filepath.Join(path, pat))
	if err != nil {
		return err
	}
	for _, f := range files {
		if err := mcpImportFromFile(f); err != nil {
			return err
		}
	}
	return nil
}

func mcpDoImport(r io.Reader) error {
	totalFiles++
	lastTime := int64(0)
	readLines := 0
	hash := fmt.Sprintf("%04x", totalFiles)
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		l := scanner.Text()
		ts, ok, _ := tg.Extract([]byte(l))
		if !ok {
			continue
		}
		t := ts.UnixNano()
		totalBytes += int64(len(l))
		readLines++
		totalLines++
		d := 0
		if lastTime > 0 {
			d = int(t - lastTime)
		}
		lastTime = t
		logCh <- &LogEnt{
			Time:  t,
			Log:   l,
			Delta: d,
			Hash:  hash,
			Line:  readLines,
		}
	}
	return nil
}

func mcpImportFromWindowsEvtx(path string) error {
	r, err := os.Open(path)
	if err != nil {
		return err
	}
	defer r.Close()
	ef, err := evtx.New(r)
	if err == nil {
		err = ef.Header.Verify()
	}
	if err != nil {
		err = ef.Header.Repair(r)
		if err != nil {
			return err
		}
	}
	totalFiles++
	hash := getSHA1(path)
	readLines := 0
	skipLines := 0
	for e := range ef.FastEvents() {
		if e == nil {
			skipLines++
			continue
		}
		readLines++
		totalLines++
		syst, err := e.GetTime(&evtx.SystemTimePath)
		if err != nil {
			skipLines++
			continue
		}
		t := syst.UnixNano()
		l := string(evtx.ToJSON(e))
		totalBytes += int64(len(l))
		logCh <- &LogEnt{
			Time: t,
			Log:  l,
			Hash: hash,
			Line: readLines,
		}
	}
	return nil
}

type mcpLogSummaryEnt struct {
	Total            int
	Errors           int
	Warnings         int
	TimeRange        string
	TopNErrorPattern []*aiErrorPattern
}

func addLogSummaryTool(s *server.MCPServer) {
	searchTool := mcp.NewTool("get_log_summary",
		mcp.WithDescription("Get a summary of logs for a specified period from TWSLA DB"),
		mcp.WithString("filter_log_content",
			mcp.Description("Filter logs by regular expression. Empty is no filter"),
		),
		mcp.WithString("error_words",
			mcp.DefaultString("error,fatal,fail,crit,alert"),
			mcp.Description("Specify keywords, separated by commas, that determine the level of logging as an error."),
		),
		mcp.WithString("warning_words",
			mcp.DefaultString("warn"),
			mcp.Description("Specify keywords, separated by commas, that determine the level of logging as an warning."),
		),
		mcp.WithNumber("error_top_n",
			mcp.DefaultNumber(10),
			mcp.Max(1000),
			mcp.Min(1),
			mcp.Description("limit top n error pattern"),
		),
		mcp.WithString("time_range",
			mcp.Required(),
			mcp.Description(
				`Time range of logs to search 
format is "start date/time, period" 
or "start date/time, end date/time".
Example: 
2025/05/07 05:59:00,1h 
2025/05/07 05:59:00,2025/05/07 06:59:00
`),
		),
	)

	s.AddTool(searchTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var err error
		regexpFilter = request.GetString("filter_log_content", "")
		timeRange, err = request.RequireString("time_range")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		aiErrorLevels = request.GetString("error_words", "error,fatal,fail,crit,alert")
		aiWarnLevels = request.GetString("warning_words", "warn")
		errCheckList = strings.Split(strings.ToLower(aiErrorLevels), ",")
		warnCheckList = strings.Split(strings.ToLower(aiWarnLevels), ",")
		topN := request.GetInt("error_top_n", 10)

		setupFilter([]string{})
		if err := openDB(); err != nil {
			return nil, err
		}
		defer db.Close()
		results = []string{}
		sti, eti := getTimeRange()
		sk := fmt.Sprintf("%016x:", sti)
		errorLogMap := make(map[string]*aiErrorPattern)
		setupTimeGrinder()
		aiStartTime = time.Now().Add(time.Hour * 24 * 365 * 100).UnixNano()
		aiEndTime = 0
		aiErrorCount = 0
		aiWarningCount = 0
		aiTotalEntries = 0
		db.View(func(tx *bbolt.Tx) error {
			b := tx.Bucket([]byte("logs"))
			c := b.Cursor()
			for k, v := c.Seek([]byte(sk)); k != nil; k, v = c.Next() {
				a := strings.Split(string(k), ":")
				if len(a) < 1 {
					continue
				}
				t, err := strconv.ParseInt(a[0], 16, 64)
				if err == nil && t > eti {
					break
				}
				l := string(v)
				if matchFilter(&l) {
					level := getAILogLevel(&l)
					switch level {
					case "ERROR":
						aiErrorCount++
						nl := normalizeLog(l)
						if p, ok := errorLogMap[nl]; !ok {
							errorLogMap[nl] = &aiErrorPattern{
								Pattern: nl,
								Count:   1,
								Example: l,
							}
						} else {
							p.Count++
						}
					case "WARN":
						aiWarningCount++
					default:
					}
					if aiStartTime > t {
						aiStartTime = t
					}
					if aiEndTime < t {
						aiEndTime = t
					}
					aiTotalEntries++
				}
			}
			return nil
		})
		aiErrorPatternList = []*aiErrorPattern{}
		for _, v := range errorLogMap {
			aiErrorPatternList = append(aiErrorPatternList, v)
		}
		sort.Slice(aiErrorPatternList, func(i, j int) bool {
			return aiErrorPatternList[i].Count > aiErrorPatternList[j].Count
		})
		if len(aiErrorPatternList) > topN {
			aiErrorPatternList = aiErrorPatternList[:topN]
		}
		summary := mcpLogSummaryEnt{
			Total:    aiTotalEntries,
			Errors:   aiErrorCount,
			Warnings: aiWarningCount,
			TimeRange: fmt.Sprintf("%s to %s",
				time.Unix(0, aiStartTime).Format("2006-01-02 15:04:05"),
				time.Unix(0, aiEndTime).Format("2006-01-02 15:04:05")),
			TopNErrorPattern: aiErrorPatternList,
		}
		ret, err := json.Marshal(&summary)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return mcp.NewToolResultText(string(ret)), nil
	})
}
