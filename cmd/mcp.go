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
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/spf13/cobra"
	"go.etcd.io/bbolt"
)

var mcpTransport = ""
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
	mcpCmd.Flags().StringVar(&mcpTransport, "transport", "stdio", "MCP server transport(stdio/sse/stream)")
	mcpCmd.Flags().StringVar(&mcpEndpoint, "endpoint", "127.0.0.1:8085", "MCP server endpoint(bind address:port)")
	mcpCmd.Flags().StringVar(&mcpClients, "clients", "", "IP address of MCP client to be allowed to connect Specify by comma delimiter")
	mcpCmd.Flags().StringVar(&geoipDBPath, "geoip", "", "geo IP database file")
}

func mcpServer() {
	// Create MCP Server
	s := mcp.NewServer(
		&mcp.Implementation{
			Name:    "TWSLA MCP Server",
			Version: Version,
		}, nil)

	// Add tools to MCP server
	addTools(s)
	// Add prompts to MCP server
	addPrompts(s)

	// Start MCP server
	switch mcpTransport {
	case "stdio":
		if err := s.Run(context.Background(), &mcp.StdioTransport{}); err != nil {
			log.Fatal(err)
		}
	case "sse":
		handler := mcp.NewSSEHandler(func(request *http.Request) *mcp.Server {
			return s
		}, nil)
		log.Printf("SSE server listening on %s", mcpEndpoint)
		if err := http.ListenAndServe(mcpEndpoint, handler); err != nil {
			log.Fatalf("Server error: %v", err)
		}
	case "stream":
		var clMap sync.Map
		if mcpClients != "" {
			for _, ip := range strings.Split(mcpClients, ",") {
				clMap.Store(ip, true)
			}
		}
		handler := mcp.NewStreamableHTTPHandler(func(r *http.Request) *mcp.Server {
			if mcpClients != "" {
				ip, err := net.ResolveTCPAddr("tcp", r.RemoteAddr)
				if err != nil {
					return nil
				}
				if _, ok := clMap.Load(ip.IP.String()); !ok {
					return nil
				}
			}
			return s
		}, nil)
		if err := http.ListenAndServe(mcpEndpoint, handler); err != nil {
			log.Fatalf("streamable server error: %v", err)
		}
	default:
		log.Fatalf("transport '%s' not supported", mcpTransport)
	}
}

func addTools(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name:        "search_log",
		Description: "Search log",
	}, searchLog)
	mcp.AddTool(s, &mcp.Tool{
		Name:        "count_log",
		Description: "Count log",
	}, countLog)
	mcp.AddTool(s, &mcp.Tool{
		Name:        "extract_data_from_log",
		Description: "This tool extracts data from the logs on the TWSLA database.",
	}, extractDataFromLog)
	mcp.AddTool(s, &mcp.Tool{
		Name:        "import_log",
		Description: "This tool imports the logs to the TWSLA database.",
	}, importLog)
	mcp.AddTool(s, &mcp.Tool{
		Name:        "get_log_summary",
		Description: "Get a summary of logs for a specified period from TWSLA DB",
	}, summaryLog)
}

// Add prompts
func addPrompts(s *mcp.Server) {
	s.AddPrompt(&mcp.Prompt{
		Name:        "search_log",
		Title:       "Search log",
		Description: "Search log with filters.",
		Arguments: []*mcp.PromptArgument{
			{
				Name:        "filter",
				Title:       "Filter logs by regular expression. Empty is no filter.",
				Description: "Filter logs by regular expression. Empty is no filter.",
				Required:    false,
			},
			{
				Name:        "limit",
				Title:       "Limit on number of logs retrieved. min 100,max 10000",
				Description: "Limit on number of logs retrieved. min 100,max 10000",
				Required:    false,
			},
			{
				Name:        "start",
				Title:       "Start date and time for log search. Example: 2025/10/26 11:00:00",
				Description: "Start date and time for log search. Example: 2025/10/26 11:00:00",
				Required:    false,
			},
			{
				Name:        "end",
				Title:       "End date and time for log search. Example: 2025/10/26 11:00:00",
				Description: "End date and time for log search. Example: 2025/10/26 11:00:00",
				Required:    false,
			},
		},
	}, searchLogPrompt)
	s.AddPrompt(&mcp.Prompt{
		Name:        "count_log",
		Title:       "Count log",
		Description: "Count logs using the specified unit and filter.",
		Arguments: []*mcp.PromptArgument{
			{
				Name:        "filter",
				Title:       "Filter logs by regular expression. Empty is no filter.",
				Description: "Filter logs by regular expression. Empty is no filter.",
				Required:    false,
			},
			{
				Name:        "unit",
				Title:       "Unit of counting",
				Description: "Unit of counting(time, ip, email, mac, host,domain, country, loc, word, field,normalize).Default:time",
				Required:    false,
			},
			{
				Name:        "unit_pos",
				Title:       "Position of unit",
				Description: "Position of unit.Default:1",
				Required:    false,
			},
			{
				Name:        "top_n",
				Title:       "Limit top n",
				Description: "Limit top n.Default: 10",
				Required:    false,
			},
			{
				Name:        "interval",
				Title:       "If unit is time,specify the aggregation interval in seconds.",
				Description: "If unit is time,specify the aggregation interval in seconds. 0 is auto select interval",
				Required:    false,
			},
			{
				Name:        "start",
				Title:       "Start date and time for log search. Example: 2025/10/26 11:00:00",
				Description: "Start date and time for log search. Example: 2025/10/26 11:00:00",
				Required:    false,
			},
			{
				Name:        "end",
				Title:       "End date and time for log search. Example: 2025/10/26 11:00:00",
				Description: "End date and time for log search. Example: 2025/10/26 11:00:00",
				Required:    false,
			},
		},
	}, countLogPrompt)
	s.AddPrompt(&mcp.Prompt{
		Name:        "extract_data_from_log",
		Title:       "Extract data from the logs on the TWSLA database",
		Description: "Extract data from the logs on the TWSLA database.",
		Arguments: []*mcp.PromptArgument{
			{
				Name:        "filter",
				Title:       "Filter logs by regular expression. Empty is no filter.",
				Description: "Filter logs by regular expression. Empty is no filter.",
				Required:    false,
			},
			{
				Name:        "pattern",
				Title:       "Specifies the pattern of data to be extracted",
				Description: "Specifies the pattern of data to be extracted.(ip,mac,email,number,regular expression)",
				Required:    false,
			},
			{
				Name:        "pos",
				Title:       "Position of extract data",
				Description: "Position of extract data.Default: 1",
				Required:    false,
			},
			{
				Name:        "start",
				Title:       "Start date and time for log search. Example: 2025/10/26 11:00:00",
				Description: "Start date and time for log search. Example: 2025/10/26 11:00:00",
				Required:    false,
			},
			{
				Name:        "end",
				Title:       "End date and time for log search. Example: 2025/10/26 11:00:00",
				Description: "End date and time for log search. Example: 2025/10/26 11:00:00",
				Required:    false,
			},
		},
	}, extractDataFromLogPrompt)
	s.AddPrompt(&mcp.Prompt{
		Name:        "import_log",
		Title:       "Import the logs to the TWSLA database",
		Description: "Import the logs to the TWSLA database.",
		Arguments: []*mcp.PromptArgument{
			{
				Name:        "path",
				Title:       "Log file or directory path to import",
				Description: "Log file or directory path to import.Files inside archive files such as zip, tar.gz, gz, etc. can be targeted for import.",
				Required:    true,
			},
			{
				Name:        "pattern",
				Title:       "Log file name regular expression pattern filter to import.",
				Description: "Log file name regular expression pattern filter to import.This applies to files in directories and files in archive files such as ZIP.",
				Required:    false,
			},
		},
	}, importLogPrompt)
	s.AddPrompt(&mcp.Prompt{
		Name:        "get_log_summary",
		Title:       "Get a summary of logs for a specified period",
		Description: "Get a summary of logs for a specified period from TWSLA DB.",
		Arguments: []*mcp.PromptArgument{
			{
				Name:        "filter",
				Title:       "Filter logs by regular expression. Empty is no filter.",
				Description: "Filter logs by regular expression. Empty is no filter.",
				Required:    false,
			},
			{
				Name:        "top_n",
				Title:       "Limit top n error pattern",
				Description: "Limit top n error pattern.Default: 10",
				Required:    false,
			},
			{
				Name:        "start",
				Title:       "Start date and time for log search. Example: 2025/10/26 11:00:00",
				Description: "Start date and time for log search. Example: 2025/10/26 11:00:00",
				Required:    false,
			},
			{
				Name:        "end",
				Title:       "End date and time for log search. Example: 2025/10/26 11:00:00",
				Description: "End date and time for log search. Example: 2025/10/26 11:00:00",
				Required:    false,
			},
		},
	}, getLogSummaryPrompt)
}

type searchLogParams struct {
	Filter string `json:"filter" jsonschema:"Filter logs by regular expression. Empty is no filter"`
	Limit  int    `json:"limit" jsonschema:"Limit on number of logs retrieved. min 100,max 10000"`
	Start  string `json:"start" jsonschema:"Start date and time for log search. Empty is 1970/1/1. Example: 2025/10/26 11:00:00"`
	End    string `json:"end" jsonschema:"End date and time for log search. Empty is now. Example: 2025/10/26 11:00:00"`
}

func searchLog(ctx context.Context, req *mcp.CallToolRequest, args searchLogParams) (*mcp.CallToolResult, any, error) {
	var err error
	regexpFilter = args.Filter
	timeRange = args.Start + "," + args.End
	limit := args.Limit
	if limit < 100 {
		limit = 100
	}
	if limit > 10000 {
		limit = 10000
	}
	setupFilter([]string{})
	if err := openDB(); err != nil {
		return nil, nil, err
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

	j, err := json.Marshal(&results)
	if err != nil {
		j = []byte(err.Error())
	}
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: string(j)},
		},
	}, nil, nil
}

func searchLogPrompt(ctx context.Context, req *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
	c := []string{}
	if filter, ok := req.Params.Arguments["filter"]; ok {
		c = append(c, fmt.Sprintf("- Filter: %s", filter))
	}
	if limit, ok := req.Params.Arguments["limit"]; ok {
		c = append(c, fmt.Sprintf("- Limit: %s", limit))
	}
	if start, ok := req.Params.Arguments["start"]; ok {
		c = append(c, fmt.Sprintf("- Start: %s", start))
	}
	if end, ok := req.Params.Arguments["end"]; ok {
		c = append(c, fmt.Sprintf("- End: %s", end))
	}
	p := "Search log in TWSLA database by using search_log tool"
	if len(c) > 0 {
		p = " with following conditions.\n" + strings.Join(c, "\n")
	} else {
		p += "."
	}
	return &mcp.GetPromptResult{
		Description: "search log prompt",
		Messages: []*mcp.PromptMessage{
			{
				Role:    "user",
				Content: &mcp.TextContent{Text: p},
			},
		},
	}, nil
}

type mcpCountEnt struct {
	Key   string
	Count int
}
type countLogParams struct {
	Filter   string `json:"filter" jsonschema:"Filter logs by regular expression. Empty is no filter"`
	Unit     string `json:"unit" jsonschema:"Unit of counting(time, ip, email, mac, host,domain, country, loc, word, field,normalize).Default:time"`
	UnitPos  int    `json:"unit_pos" jsonschema:"Position of unit.Default:1"`
	TopN     int    `json:"top_n" jsonschema:"Limit top n.Default: 10"`
	Interval int    `json:"interval" jsonschema:"If unit is time,specify the aggregation interval in seconds. 0 is auto select interval"`
	Start    string `json:"start" jsonschema:"Start date and time for log search. Empty is 1970/1/1. Example: 2025/10/26 11:00:00"`
	End      string `json:"end" jsonschema:"End date and time for log search. Empty is now. Example: 2025/10/26 11:00:00"`
}

func countLog(ctx context.Context, req *mcp.CallToolRequest, args countLogParams) (*mcp.CallToolResult, any, error) {
	var err error
	regexpFilter = args.Filter
	pos = args.UnitPos
	if pos < 1 || pos > 10 {
		pos = 1
	}
	interval = args.Interval
	if interval < 0 {
		interval = 0
	}
	topN := args.TopN
	if topN < 1 {
		topN = 10
	}
	mode := 1
	ipm := 0
	extract = ""
	unit := args.Unit
	switch unit {
	case "mac", "email":
		extract = unit
	case "ip":
		extract = "ip"
	case "host":
		ipm = 1
		extract = "ip"
		dnsResolver = dnsr.NewWithTimeout(10000, time.Millisecond*1000)
	case "domain":
		ipm = 2
		extract = "ip"
		dnsResolver = dnsr.NewWithTimeout(10000, time.Millisecond*1000)
	case "loc":
		if err := openGeoIPDB(); err != nil {
			return nil, nil, err
		}
		ipm = 3
		extract = "ip"
	case "country":
		if err := openGeoIPDB(); err != nil {
			return nil, nil, err
		}
		ipm = 4
		extract = "ip"
	case "normalize":
		mode = 2
	case "word":
		mode = 3
	case "field":
		mode = 4
		pos -= 1
		if pos < 0 {
			pos = 0
		}
	default:
		// Time mode
		mode = 0
	}
	timeRange = args.Start + "," + args.End
	setupFilter([]string{})
	extPat = nil
	setExtPat()
	if mode == 1 && extPat == nil {
		return nil, nil, fmt.Errorf("invalid unit")
	}
	if mode == 2 {
		setupTimeGrinder()
	}
	if err := openDB(); err != nil {
		return nil, nil, err
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
				switch mode {
				case 1:
					a := extPat.ExtReg.FindAllStringSubmatch(l, -1)
					if len(a) >= extPat.Index {
						ck := a[extPat.Index-1][1]
						if ipm > 0 {
							ck = getIPInfo(ck, ipm)
						}
						countMap[ck]++
					}
				case 2:
					ck := normalizeLog(l)
					countMap[ck]++
				case 3:
					// Word
					words := strings.Fields(strings.ToLower(l))
					for _, word := range words {
						if len(word) >= 2 && len(word) <= 50 {
							word = strings.Trim(word, ".,!?;:()[]{}\"'")
							if len(word) >= 2 {
								countMap[word]++
							}
						}
					}
				case 4:
					// Field
					f := strings.Fields(l)
					if len(f) > pos {
						k := f[pos]
						countMap[k]++
					}
				default:
					d := t / intv
					ck := time.Unix(0, d*intv).Format("2006/01/02 15:04")
					countMap[ck]++
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
	if mode == 0 {
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
	j, err := json.Marshal(&cl)
	if err != nil {
		j = []byte(err.Error())
	}
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: string(j)},
		},
	}, nil, nil
}

func countLogPrompt(ctx context.Context, req *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
	c := []string{}
	if filter, ok := req.Params.Arguments["filter"]; ok {
		c = append(c, fmt.Sprintf("- Filter: %s", filter))
	}
	if unit, ok := req.Params.Arguments["unit"]; ok {
		c = append(c, fmt.Sprintf("- Unit: %s", unit))
	}
	if pos, ok := req.Params.Arguments["unit_pos"]; ok {
		c = append(c, fmt.Sprintf("- Unit pos: %s", pos))
	}
	if topn, ok := req.Params.Arguments["top_n"]; ok {
		c = append(c, fmt.Sprintf("- Top N: %s", topn))
	}
	if interval, ok := req.Params.Arguments["interval"]; ok {
		c = append(c, fmt.Sprintf("- Interval: %s", interval))
	}
	if start, ok := req.Params.Arguments["start"]; ok {
		c = append(c, fmt.Sprintf("- Start: %s", start))
	}
	if end, ok := req.Params.Arguments["end"]; ok {
		c = append(c, fmt.Sprintf("- End: %s", end))
	}
	p := "Count logs in TWSLA database by using count_log tool"
	if len(c) > 0 {
		p = " with following conditions.\n" + strings.Join(c, "\n")
	} else {
		p += "."
	}
	return &mcp.GetPromptResult{
		Description: "count log prompt",
		Messages: []*mcp.PromptMessage{
			{
				Role:    "user",
				Content: &mcp.TextContent{Text: p},
			},
		},
	}, nil
}

type mcpExtractEnt struct {
	Time  string
	Value string
}

type extractDataFromLogParams struct {
	Filter  string `json:"filter" jsonschema:"Filter logs by regular expression. Empty is no filter"`
	Pattern string `json:"pattern" jsonschema:"Specifies the pattern of data to be extracted.(ip,mac,email,number,regular expression)"`
	Pos     int    `json:"pos" jsonschema:"Position of extract data.Default: 1"`
	Start   string `json:"start" jsonschema:"Start date and time for log search. Empty is 1970/1/1. Example: 2025/10/26 11:00:00"`
	End     string `json:"end" jsonschema:"End date and time for log search. Empty is now. Example: 2025/10/26 11:00:00"`
}

func extractDataFromLog(ctx context.Context, req *mcp.CallToolRequest, args extractDataFromLogParams) (*mcp.CallToolResult, any, error) {
	var err error
	regexpFilter = args.Filter
	extract = args.Pattern
	pos = args.Pos
	if pos < 1 || pos > 100 {
		pos = 1
	}
	timeRange = args.Start + "," + args.End
	setupFilter([]string{})
	extPat = nil
	setExtPat()
	if extPat == nil {
		return nil, nil, fmt.Errorf("pattern is empty")
	}
	if err := openDB(); err != nil {
		return nil, nil, err
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
	j, err := json.Marshal(&mcpExtractList)
	if err != nil {
		j = []byte(err.Error())
	}
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: string(j)},
		},
	}, nil, nil
}

func extractDataFromLogPrompt(ctx context.Context, req *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
	c := []string{}
	if filter, ok := req.Params.Arguments["filter"]; ok {
		c = append(c, fmt.Sprintf("- Filter: %s", filter))
	}
	if pattern, ok := req.Params.Arguments["pattern"]; ok {
		c = append(c, fmt.Sprintf("- Pattern: %s", pattern))
	}
	if pos, ok := req.Params.Arguments["pos"]; ok {
		c = append(c, fmt.Sprintf("- Pos: %s", pos))
	}
	if start, ok := req.Params.Arguments["start"]; ok {
		c = append(c, fmt.Sprintf("- Start: %s", start))
	}
	if end, ok := req.Params.Arguments["end"]; ok {
		c = append(c, fmt.Sprintf("- End: %s", end))
	}
	p := "Extracts data from the logs on the TWSLA database by using extract_data_from_log tool"
	if len(c) > 0 {
		p = " with following conditions.\n" + strings.Join(c, "\n")
	} else {
		p += "."
	}
	return &mcp.GetPromptResult{
		Description: "extract data from log prompt",
		Messages: []*mcp.PromptMessage{
			{
				Role:    "user",
				Content: &mcp.TextContent{Text: p},
			},
		},
	}, nil
}

type importLogParams struct {
	Path    string `json:"path" jsonschema:"Log file or directory path to import.Files inside archive files such as zip, tar.gz, gz, etc. can be targeted for import."`
	Pattern string `json:"pattern" jsonschema:"Log file name regular expression pattern filter to import.This applies to files in directories and files in archive files such as ZIP."`
}

func importLog(ctx context.Context, req *mcp.CallToolRequest, args importLogParams) (*mcp.CallToolResult, any, error) {
	var err error
	filePat = args.Pattern
	source = args.Path
	if source == "" {
		return nil, nil, fmt.Errorf("path is empty")
	}
	if err := openDB(); err != nil {
		return nil, nil, err
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
		return nil, nil, err
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
	j, err := json.Marshal(&r)
	if err != nil {
		j = []byte(err.Error())
	}
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: string(j)},
		},
	}, nil, nil
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
		return mcpImportFromZIPFile(path)
	case ".evtx":
		return mcpImportFromWindowsEvtx(path)
	case ".tgz":
		return mcpImportFromTarGZFile(path)
	case ".gz":
		if strings.HasSuffix(path, ".tar.gz") {
			return mcpImportFromTarGZFile(path)
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

func mcpImportFromZIPFile(path string) error {
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
				mcpDoImport(gzr)
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

func mcpImportFromTarGZFile(path string) error {
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

func importLogPrompt(ctx context.Context, req *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
	c := []string{}
	if path, ok := req.Params.Arguments["path"]; ok {
		c = append(c, fmt.Sprintf("- Path: %s", path))
	} else {
		return nil, fmt.Errorf("path is required")
	}
	if pattern, ok := req.Params.Arguments["pattern"]; ok {
		c = append(c, fmt.Sprintf("- Pattern: %s", pattern))
	}
	p := "Import the logs to the TWSLA database by using import_log tool"
	if len(c) > 0 {
		p = " with following conditions.\n" + strings.Join(c, "\n")
	} else {
		p += "."
	}
	return &mcp.GetPromptResult{
		Description: "import log prompt",
		Messages: []*mcp.PromptMessage{
			{
				Role:    "user",
				Content: &mcp.TextContent{Text: p},
			},
		},
	}, nil
}

type mcpLogSummaryEnt struct {
	Total            int
	Errors           int
	Warnings         int
	TimeRange        string
	TopNErrorPattern []*aiErrorPattern
}
type summaryLogParams struct {
	Filter string `json:"filter" jsonschema:"Filter logs by regular expression. Empty is no filter"`
	TopN   int    `json:"top_n" jsonschema:"Limit top n error pattern.Default: 10"`
	Start  string `json:"start" jsonschema:"Start date and time for log search. Empty is 1970/1/1. Example: 2025/10/26 11:00:00"`
	End    string `json:"end" jsonschema:"End date and time for log search. Empty is now. Example: 2025/10/26 11:00:00"`
}

func summaryLog(ctx context.Context, req *mcp.CallToolRequest, args summaryLogParams) (*mcp.CallToolResult, any, error) {
	var err error
	regexpFilter = args.Filter
	timeRange = args.Start + "," + args.End
	aiErrorLevels = "error,fatal,fail,crit,alert"
	aiWarnLevels = "warn"
	errCheckList = strings.Split(strings.ToLower(aiErrorLevels), ",")
	warnCheckList = strings.Split(strings.ToLower(aiWarnLevels), ",")
	topN := args.TopN
	if topN < 10 || topN > 1000 {
		topN = 10
	}
	setupFilter([]string{})
	if err := openDB(); err != nil {
		return nil, nil, err
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
	j, err := json.Marshal(&summary)
	if err != nil {
		j = []byte(err.Error())
	}
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: string(j)},
		},
	}, nil, nil
}

func getLogSummaryPrompt(ctx context.Context, req *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
	c := []string{}
	if filter, ok := req.Params.Arguments["filter"]; ok {
		c = append(c, fmt.Sprintf("- Filter: %s", filter))
	}
	if topn, ok := req.Params.Arguments["top_n"]; ok {
		c = append(c, fmt.Sprintf("- Top N: %s", topn))
	}
	if start, ok := req.Params.Arguments["start"]; ok {
		c = append(c, fmt.Sprintf("- Start: %s", start))
	}
	if end, ok := req.Params.Arguments["end"]; ok {
		c = append(c, fmt.Sprintf("- End: %s", end))
	}
	p := "Get a summary of logs for a specified period from TWSLA database by using get_log_summary tool"
	if len(c) > 0 {
		p = " with following conditions.\n" + strings.Join(c, "\n")
	} else {
		p += "."
	}
	return &mcp.GetPromptResult{
		Description: "Get a summary of logs prompt",
		Messages: []*mcp.PromptMessage{
			{
				Role:    "user",
				Content: &mcp.TextContent{Text: p},
			},
		},
	}, nil
}
