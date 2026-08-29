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
	"math"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/0xrawsec/golang-evtx/evtx"
	"github.com/bradleyjkemp/sigma-go/evaluator"
	tf_idf "github.com/dkgv/go-tf-idf"
	"github.com/domainr/dnsr"
	"github.com/dustin/go-humanize"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/montanaflynn/stats"
	"github.com/spf13/cobra"
	"github.com/twsnmp/twsla/pkg/anomaly"
	"github.com/twsnmp/twsla/pkg/datastore"
)

var (
	mcpTransport = ""
	mcpEndpoint  = ""
	mcpClients   = ""
	mcpMutex     sync.Mutex
)

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
	// Add resources to MCP server
	addResources(s)
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
	// Core Tools
	mcp.AddTool(s, &mcp.Tool{
		Name:        "get_db_info",
		Description: "Get metadata and overview of the TWSLA database (total log count, time range, datastore type).",
	}, getDBInfo)
	mcp.AddTool(s, &mcp.Tool{
		Name:        "search_log",
		Description: "Search logs from TWSLA database with optional filters and time range.",
	}, searchLog)
	mcp.AddTool(s, &mcp.Tool{
		Name:        "count_log",
		Description: "Count and aggregate logs by specified unit (time, ip, email, mac, host, domain, country, loc, word, field, normalize).",
	}, countLog)
	mcp.AddTool(s, &mcp.Tool{
		Name:        "extract_data_from_log",
		Description: "Extract specific data patterns (ip, mac, email, number, regex) from logs in TWSLA database.",
	}, extractDataFromLog)
	mcp.AddTool(s, &mcp.Tool{
		Name:        "import_log",
		Description: "Import logs from file or directory (supports .log, .zip, .tar.gz, .gz, .evtx) into TWSLA database.",
	}, importLog)
	mcp.AddTool(s, &mcp.Tool{
		Name:        "get_log_summary",
		Description: "Get a summary of logs (total, errors, warnings, top error patterns) for a specified period.",
	}, summaryLog)

	// Advanced Analysis Tools
	mcp.AddTool(s, &mcp.Tool{
		Name:        "detect_threats_sigma",
		Description: "Detect security threats in logs using SIGMA rules. Supports custom rule path or embedded configuration mapping.",
	}, detectThreatsSigma)
	mcp.AddTool(s, &mcp.Tool{
		Name:        "detect_anomalies",
		Description: "Detect anomaly logs using machine learning and statistical algorithms (iforest, lof, knn, zscore, autoencoder, lstm) across various detection modes (tfidf, sql, os, dir, walu, number).",
	}, detectAnomalies)
	mcp.AddTool(s, &mcp.Tool{
		Name:        "analyze_relations",
		Description: "Analyze relationships and co-occurrences between multiple data elements (e.g. ip, mac, email, url, regex) in logs.",
	}, analyzeRelations)
	mcp.AddTool(s, &mcp.Tool{
		Name:        "analyze_tfidf",
		Description: "Analyze logs using TF-IDF to discover rare or outlier log entries based on similarity thresholds.",
	}, analyzeTFIDF)
}

func addResources(s *mcp.Server) {
	s.AddResource(&mcp.Resource{
		URI:         "twsla://db/status",
		Name:        "Database Status",
		Description: "Current status, record counts, and time span of the TWSLA database.",
		MIMEType:    "application/json",
	}, dbStatusResourceHandler)

	s.AddResource(&mcp.Resource{
		URI:         "twsla://sigma/rules",
		Name:        "Built-in Sigma Configs",
		Description: "List of embedded Sigma rule configuration definitions available in TWSLA.",
		MIMEType:    "application/json",
	}, sigmaRulesResourceHandler)
}

func dbStatusResourceHandler(ctx context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
	mcpMutex.Lock()
	defer mcpMutex.Unlock()

	if err := openDB(); err != nil {
		return nil, err
	}
	defer closeDB()

	var total int64
	var minTime int64 = math.MaxInt64
	var maxTime int64

	sti, eti := getTimeRange()
	_ = ds.ForEach(sti, eti, func(entry *datastore.LogEntry) bool {
		total++
		if entry.Time < minTime {
			minTime = entry.Time
		}
		if entry.Time > maxTime {
			maxTime = entry.Time
		}
		return true
	})

	type statusResp struct {
		DataStore string `json:"datastore"`
		TotalLogs int64  `json:"total_logs"`
		FirstLog  string `json:"first_log,omitempty"`
		LastLog   string `json:"last_log,omitempty"`
	}

	res := statusResp{
		DataStore: dataStore,
		TotalLogs: total,
	}
	if total > 0 && minTime != math.MaxInt64 {
		res.FirstLog = time.Unix(0, minTime).Format(time.RFC3339)
		res.LastLog = time.Unix(0, maxTime).Format(time.RFC3339)
	}

	data, err := json.MarshalIndent(res, "", "  ")
	if err != nil {
		return nil, err
	}

	return &mcp.ReadResourceResult{
		Contents: []*mcp.ResourceContents{
			{
				URI:      req.Params.URI,
				MIMEType: "application/json",
				Text:     string(data),
			},
		},
	}, nil
}

func sigmaRulesResourceHandler(ctx context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
	configs := []string{}
	entries, err := sigmaConfigs.ReadDir("sigma")
	if err == nil {
		for _, e := range entries {
			if !e.IsDir() && (strings.HasSuffix(e.Name(), ".yml") || strings.HasSuffix(e.Name(), ".yaml")) {
				configs = append(configs, strings.TrimSuffix(strings.TrimSuffix(e.Name(), ".yaml"), ".yml"))
			}
		}
	}

	type sigmaRulesResp struct {
		AvailableConfigs []string `json:"available_configs"`
		Description      string   `json:"description"`
	}

	res := sigmaRulesResp{
		AvailableConfigs: configs,
		Description:      "Embedded Sigma configuration mappings. Use these names in the 'config' argument of detect_threats_sigma.",
	}

	data, err := json.MarshalIndent(res, "", "  ")
	if err != nil {
		return nil, err
	}

	return &mcp.ReadResourceResult{
		Contents: []*mcp.ResourceContents{
			{
				URI:      req.Params.URI,
				MIMEType: "application/json",
				Text:     string(data),
			},
		},
	}, nil
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

	// Advanced Analysis Prompts
	s.AddPrompt(&mcp.Prompt{
		Name:        "incident_investigation",
		Title:       "Incident Investigation Workflow",
		Description: "Comprehensive workflow for investigating security incidents or system failures across timeline, anomalies, and correlations.",
		Arguments: []*mcp.PromptArgument{
			{
				Name:        "target",
				Title:       "Target IP, User, Host, or Keyword to investigate",
				Description: "Target IP, User, Host, or Keyword to investigate.",
				Required:    true,
			},
			{
				Name:        "start",
				Title:       "Start time of incident window",
				Description: "Start date and time for investigation window.",
				Required:    false,
			},
			{
				Name:        "end",
				Title:       "End time of incident window",
				Description: "End date and time for investigation window.",
				Required:    false,
			},
		},
	}, incidentInvestigationPrompt)

	s.AddPrompt(&mcp.Prompt{
		Name:        "security_threat_hunt",
		Title:       "Security Threat Hunt",
		Description: "Proactive threat hunting workflow using SIGMA detection and Web attack anomaly detection.",
		Arguments: []*mcp.PromptArgument{
			{
				Name:        "rules",
				Title:       "Sigma rules directory or file path",
				Description: "Sigma rules directory or file path (optional).",
				Required:    false,
			},
			{
				Name:        "config",
				Title:       "Sigma config name",
				Description: "Sigma config name (e.g. sysmon, apache, windows).",
				Required:    false,
			},
		},
	}, securityThreatHuntPrompt)

	s.AddPrompt(&mcp.Prompt{
		Name:        "anomaly_audit",
		Title:       "Anomaly and Rare Log Audit",
		Description: "Audit logs for rare patterns, outliers, and unexpected structural variations using Isolation Forest or TF-IDF.",
		Arguments: []*mcp.PromptArgument{
			{
				Name:        "filter",
				Title:       "Filter scope",
				Description: "Filter scope regex.",
				Required:    false,
			},
		},
	}, anomalyAuditPrompt)
}

// -------------------------------------------------------------
// get_db_info
// -------------------------------------------------------------

type getDBInfoParams struct{}

func getDBInfo(ctx context.Context, req *mcp.CallToolRequest, args getDBInfoParams) (*mcp.CallToolResult, any, error) {
	mcpMutex.Lock()
	defer mcpMutex.Unlock()

	if err := openDB(); err != nil {
		return nil, nil, err
	}
	defer closeDB()

	var total int64
	var minTime int64 = math.MaxInt64
	var maxTime int64

	sti, eti := getTimeRange()
	_ = ds.ForEach(sti, eti, func(entry *datastore.LogEntry) bool {
		total++
		if entry.Time < minTime {
			minTime = entry.Time
		}
		if entry.Time > maxTime {
			maxTime = entry.Time
		}
		return true
	})

	type dbInfoResp struct {
		DataStore string `json:"datastore"`
		TotalLogs int64  `json:"total_logs"`
		FirstLog  string `json:"first_log,omitempty"`
		LastLog   string `json:"last_log,omitempty"`
	}

	res := dbInfoResp{
		DataStore: dataStore,
		TotalLogs: total,
	}
	if total > 0 && minTime != math.MaxInt64 {
		res.FirstLog = time.Unix(0, minTime).Format(time.RFC3339)
		res.LastLog = time.Unix(0, maxTime).Format(time.RFC3339)
	}

	j, err := json.MarshalIndent(res, "", "  ")
	if err != nil {
		return nil, nil, err
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: string(j)},
		},
	}, nil, nil
}

// -------------------------------------------------------------
// search_log
// -------------------------------------------------------------

type searchLogParams struct {
	Filter string `json:"filter,omitempty" jsonschema:"Filter logs by regular expression. Empty is no filter"`
	Limit  int    `json:"limit,omitempty" jsonschema:"Limit on number of logs retrieved. min 100,max 10000"`
	Start  string `json:"start,omitempty" jsonschema:"Start date and time for log search. Empty is 1970/1/1. Example: 2025/10/26 11:00:00"`
	End    string `json:"end,omitempty" jsonschema:"End date and time for log search. Empty is now. Example: 2025/10/26 11:00:00"`
}

func searchLog(ctx context.Context, req *mcp.CallToolRequest, args searchLogParams) (*mcp.CallToolResult, any, error) {
	mcpMutex.Lock()
	defer mcpMutex.Unlock()

	regexpFilter = args.Filter
	timeRange = args.Start + "," + args.End
	limit := args.Limit
	if limit < 1 {
		limit = 100
	}
	if limit > 10000 {
		limit = 10000
	}
	setupFilter([]string{})
	if err := openDB(); err != nil {
		return nil, nil, err
	}
	defer closeDB()

	searchResults := []string{}
	sti, eti := getTimeRange()
	_ = ds.ForEach(sti, eti, func(entry *datastore.LogEntry) bool {
		l := entry.Log
		if matchFilter(&l) {
			searchResults = append(searchResults, l)
			if len(searchResults) >= limit {
				return false
			}
		}
		return true
	})

	j, err := json.Marshal(&searchResults)
	if err != nil {
		return nil, nil, err
	}
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: string(j)},
		},
	}, nil, nil
}

func searchLogPrompt(ctx context.Context, req *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
	c := []string{}
	if filter, ok := req.Params.Arguments["filter"]; ok && filter != "" {
		c = append(c, fmt.Sprintf("- Filter: %s", filter))
	}
	if limit, ok := req.Params.Arguments["limit"]; ok && limit != "" {
		c = append(c, fmt.Sprintf("- Limit: %s", limit))
	}
	if start, ok := req.Params.Arguments["start"]; ok && start != "" {
		c = append(c, fmt.Sprintf("- Start: %s", start))
	}
	if end, ok := req.Params.Arguments["end"]; ok && end != "" {
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

// -------------------------------------------------------------
// count_log
// -------------------------------------------------------------

type mcpCountEnt struct {
	Key   string `json:"key"`
	Count int    `json:"count"`
}

type countLogParams struct {
	Filter   string `json:"filter,omitempty" jsonschema:"Filter logs by regular expression. Empty is no filter"`
	Unit     string `json:"unit,omitempty" jsonschema:"Unit of counting(time, ip, email, mac, host,domain, country, loc, word, field,normalize).Default:time"`
	UnitPos  int    `json:"unit_pos,omitempty" jsonschema:"Position of unit.Default:1"`
	TopN     int    `json:"top_n,omitempty" jsonschema:"Limit top n.Default: 10"`
	Interval int    `json:"interval,omitempty" jsonschema:"If unit is time,specify the aggregation interval in seconds. 0 is auto select interval"`
	Start    string `json:"start,omitempty" jsonschema:"Start date and time for log search. Empty is 1970/1/1. Example: 2025/10/26 11:00:00"`
	End      string `json:"end,omitempty" jsonschema:"End date and time for log search. Empty is now. Example: 2025/10/26 11:00:00"`
}

func countLog(ctx context.Context, req *mcp.CallToolRequest, args countLogParams) (*mcp.CallToolResult, any, error) {
	mcpMutex.Lock()
	defer mcpMutex.Unlock()

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
	defer closeDB()

	var countMap = make(map[string]int)
	intv := int64(getInterval()) * 1000 * 1000 * 1000
	sti, eti := getTimeRange()
	_ = ds.ForEach(sti, eti, func(entry *datastore.LogEntry) bool {
		t := entry.Time
		l := entry.Log
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
		return true
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
		return nil, nil, err
	}
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: string(j)},
		},
	}, nil, nil
}

func countLogPrompt(ctx context.Context, req *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
	c := []string{}
	if filter, ok := req.Params.Arguments["filter"]; ok && filter != "" {
		c = append(c, fmt.Sprintf("- Filter: %s", filter))
	}
	if unit, ok := req.Params.Arguments["unit"]; ok && unit != "" {
		c = append(c, fmt.Sprintf("- Unit: %s", unit))
	}
	if pos, ok := req.Params.Arguments["unit_pos"]; ok && pos != "" {
		c = append(c, fmt.Sprintf("- Unit pos: %s", pos))
	}
	if topn, ok := req.Params.Arguments["top_n"]; ok && topn != "" {
		c = append(c, fmt.Sprintf("- Top N: %s", topn))
	}
	if interval, ok := req.Params.Arguments["interval"]; ok && interval != "" {
		c = append(c, fmt.Sprintf("- Interval: %s", interval))
	}
	if start, ok := req.Params.Arguments["start"]; ok && start != "" {
		c = append(c, fmt.Sprintf("- Start: %s", start))
	}
	if end, ok := req.Params.Arguments["end"]; ok && end != "" {
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

// -------------------------------------------------------------
// extract_data_from_log
// -------------------------------------------------------------

type mcpExtractEnt struct {
	Time  string `json:"time"`
	Value string `json:"value"`
}

type extractDataFromLogParams struct {
	Filter  string `json:"filter,omitempty" jsonschema:"Filter logs by regular expression. Empty is no filter"`
	Pattern string `json:"pattern,omitempty" jsonschema:"Specifies the pattern of data to be extracted.(ip,mac,email,number,regular expression)"`
	Pos     int    `json:"pos,omitempty" jsonschema:"Position of extract data.Default: 1"`
	Start   string `json:"start,omitempty" jsonschema:"Start date and time for log search. Empty is 1970/1/1. Example: 2025/10/26 11:00:00"`
	End     string `json:"end,omitempty" jsonschema:"End date and time for log search. Empty is now. Example: 2025/10/26 11:00:00"`
}

func extractDataFromLog(ctx context.Context, req *mcp.CallToolRequest, args extractDataFromLogParams) (*mcp.CallToolResult, any, error) {
	mcpMutex.Lock()
	defer mcpMutex.Unlock()

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
	defer closeDB()

	mcpExtractList := []mcpExtractEnt{}
	sti, eti := getTimeRange()
	_ = ds.ForEach(sti, eti, func(entry *datastore.LogEntry) bool {
		t := entry.Time
		l := entry.Log
		if matchFilter(&l) {
			a := extPat.ExtReg.FindAllStringSubmatch(l, -1)
			if len(a) >= extPat.Index && len(a[extPat.Index-1]) > 1 {
				mcpExtractList = append(mcpExtractList, mcpExtractEnt{Time: time.Unix(0, t).Format(time.RFC3339Nano), Value: a[extPat.Index-1][1]})
			}
		}
		return true
	})
	j, err := json.Marshal(&mcpExtractList)
	if err != nil {
		return nil, nil, err
	}
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: string(j)},
		},
	}, nil, nil
}

func extractDataFromLogPrompt(ctx context.Context, req *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
	c := []string{}
	if filter, ok := req.Params.Arguments["filter"]; ok && filter != "" {
		c = append(c, fmt.Sprintf("- Filter: %s", filter))
	}
	if pattern, ok := req.Params.Arguments["pattern"]; ok && pattern != "" {
		c = append(c, fmt.Sprintf("- Pattern: %s", pattern))
	}
	if pos, ok := req.Params.Arguments["pos"]; ok && pos != "" {
		c = append(c, fmt.Sprintf("- Pos: %s", pos))
	}
	if start, ok := req.Params.Arguments["start"]; ok && start != "" {
		c = append(c, fmt.Sprintf("- Start: %s", start))
	}
	if end, ok := req.Params.Arguments["end"]; ok && end != "" {
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

// -------------------------------------------------------------
// import_log
// -------------------------------------------------------------

type importLogParams struct {
	Path    string `json:"path" jsonschema:"Log file or directory path to import.Files inside archive files such as zip, tar.gz, gz, etc. can be targeted for import."`
	Pattern string `json:"pattern,omitempty" jsonschema:"Log file name regular expression pattern filter to import.This applies to files in directories and files in archive files such as ZIP."`
}

func importLog(ctx context.Context, req *mcp.CallToolRequest, args importLogParams) (*mcp.CallToolResult, any, error) {
	mcpMutex.Lock()
	defer mcpMutex.Unlock()

	filePat = args.Pattern
	source = args.Path
	if source == "" {
		return nil, nil, fmt.Errorf("path is empty")
	}
	if err := openDB(); err != nil {
		return nil, nil, err
	}
	defer closeDB()
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
		Files string `json:"files"`
		Lines string `json:"lines"`
		Bytes string `json:"bytes"`
	}
	r.Files = humanize.Comma(int64(totalFiles))
	r.Lines = humanize.Comma(int64(totalLines))
	r.Bytes = humanize.Bytes(uint64(totalBytes))
	j, err := json.Marshal(&r)
	if err != nil {
		return nil, nil, err
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
		return err
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
		rc, err := f.Open()
		if err != nil {
			continue
		}
		ext := strings.ToLower(filepath.Ext(f.Name))
		switch ext {
		case ".gz":
			if gzr, err := gzip.NewReader(rc); err == nil {
				mcpDoImport(gzr)
			}
		case ".evtx":
			w, err := os.CreateTemp("", "winlog*.evtx")
			if err != nil {
				rc.Close()
				return err
			}
			io.Copy(w, rc)
			w.Close()
			rc.Close()
			importFromWindowsEvtx(w.Name())
			os.Remove(w.Name())
		default:
			if err := mcpDoImport(rc); err != nil {
				rc.Close()
				return err
			}
			rc.Close()
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
	if path, ok := req.Params.Arguments["path"]; ok && path != "" {
		c = append(c, fmt.Sprintf("- Path: %s", path))
	} else {
		return nil, fmt.Errorf("path is required")
	}
	if pattern, ok := req.Params.Arguments["pattern"]; ok && pattern != "" {
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

// -------------------------------------------------------------
// get_log_summary
// -------------------------------------------------------------

type mcpLogSummaryEnt struct {
	Total            int               `json:"total"`
	Errors           int               `json:"errors"`
	Warnings         int               `json:"warnings"`
	TimeRange        string            `json:"time_range"`
	TopNErrorPattern []*aiErrorPattern `json:"top_n_error_pattern"`
}

type summaryLogParams struct {
	Filter string `json:"filter,omitempty" jsonschema:"Filter logs by regular expression. Empty is no filter"`
	TopN   int    `json:"top_n,omitempty" jsonschema:"Limit top n error pattern.Default: 10"`
	Start  string `json:"start,omitempty" jsonschema:"Start date and time for log search. Empty is 1970/1/1. Example: 2025/10/26 11:00:00"`
	End    string `json:"end,omitempty" jsonschema:"End date and time for log search. Empty is now. Example: 2025/10/26 11:00:00"`
}

func summaryLog(ctx context.Context, req *mcp.CallToolRequest, args summaryLogParams) (*mcp.CallToolResult, any, error) {
	mcpMutex.Lock()
	defer mcpMutex.Unlock()

	regexpFilter = args.Filter
	timeRange = args.Start + "," + args.End
	aiErrorLevels = "error,fatal,fail,crit,alert"
	aiWarnLevels = "warn"
	errCheckList = strings.Split(strings.ToLower(aiErrorLevels), ",")
	warnCheckList = strings.Split(strings.ToLower(aiWarnLevels), ",")
	topN := args.TopN
	if topN < 1 || topN > 1000 {
		topN = 10
	}
	setupFilter([]string{})
	if err := openDB(); err != nil {
		return nil, nil, err
	}
	defer closeDB()

	sti, eti := getTimeRange()
	errorLogMap := make(map[string]*aiErrorPattern)
	setupTimeGrinder()
	aiStartTime = time.Now().Add(time.Hour * 24 * 365 * 100).UnixNano()
	aiEndTime = 0
	aiErrorCount = 0
	aiWarningCount = 0
	aiTotalEntries = 0
	_ = ds.ForEach(sti, eti, func(entry *datastore.LogEntry) bool {
		t := entry.Time
		l := entry.Log
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
			}
			if aiStartTime > t {
				aiStartTime = t
			}
			if aiEndTime < t {
				aiEndTime = t
			}
			aiTotalEntries++
		}
		return true
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
	timeRangeStr := "none"
	if aiTotalEntries > 0 {
		timeRangeStr = fmt.Sprintf("%s to %s",
			time.Unix(0, aiStartTime).Format("2006-01-02 15:04:05"),
			time.Unix(0, aiEndTime).Format("2006-01-02 15:04:05"))
	}
	summary := mcpLogSummaryEnt{
		Total:            aiTotalEntries,
		Errors:           aiErrorCount,
		Warnings:         aiWarningCount,
		TimeRange:        timeRangeStr,
		TopNErrorPattern: aiErrorPatternList,
	}
	j, err := json.MarshalIndent(&summary, "", "  ")
	if err != nil {
		return nil, nil, err
	}
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: string(j)},
		},
	}, nil, nil
}

func getLogSummaryPrompt(ctx context.Context, req *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
	c := []string{}
	if filter, ok := req.Params.Arguments["filter"]; ok && filter != "" {
		c = append(c, fmt.Sprintf("- Filter: %s", filter))
	}
	if topn, ok := req.Params.Arguments["top_n"]; ok && topn != "" {
		c = append(c, fmt.Sprintf("- Top N: %s", topn))
	}
	if start, ok := req.Params.Arguments["start"]; ok && start != "" {
		c = append(c, fmt.Sprintf("- Start: %s", start))
	}
	if end, ok := req.Params.Arguments["end"]; ok && end != "" {
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

// -------------------------------------------------------------
// detect_threats_sigma
// -------------------------------------------------------------

type detectThreatsSigmaParams struct {
	Filter  string `json:"filter,omitempty" jsonschema:"Filter logs by regular expression. Empty is no filter"`
	Rules   string `json:"rules,omitempty" jsonschema:"Path to Sigma rule file or directory. If empty, all built-in rules will be checked."`
	Config  string `json:"config,omitempty" jsonschema:"Sigma config mapping name (e.g. sysmon, apache, windows)."`
	GrokPat string `json:"grok_pat,omitempty" jsonschema:"Grok pattern if logs are not JSON formatted."`
	Strict  bool   `json:"strict,omitempty" jsonschema:"Strict rule parsing check. Default: false"`
	Limit   int    `json:"limit,omitempty" jsonschema:"Limit on matching results returned. Default: 100"`
	Start   string `json:"start,omitempty" jsonschema:"Start date and time for log search. Empty is 1970/1/1."`
	End     string `json:"end,omitempty" jsonschema:"End date and time for log search. Empty is now."`
}

type mcpSigmaHit struct {
	Time        string `json:"time"`
	Log         string `json:"log"`
	RuleID      string `json:"rule_id"`
	RuleTitle   string `json:"rule_title"`
	RuleLevel   string `json:"rule_level,omitempty"`
	Description string `json:"description,omitempty"`
}

func detectThreatsSigma(ctx context.Context, req *mcp.CallToolRequest, args detectThreatsSigmaParams) (*mcp.CallToolResult, any, error) {
	mcpMutex.Lock()
	defer mcpMutex.Unlock()

	regexpFilter = args.Filter
	timeRange = args.Start + "," + args.End
	sigmaConfig = args.Config
	sigmaRules = args.Rules
	strict = args.Strict
	grokPat = args.GrokPat
	limit := args.Limit
	if limit <= 0 {
		limit = 100
	}
	if limit > 1000 {
		limit = 1000
	}

	setupFilter([]string{})
	setGrok()

	evaluators = []*evaluator.RuleEvaluator{}
	if sigmaRules != "" {
		loadSigmaRules()
	} else {
		// Load embedded sigma configs / rules if available
		config := getSigmaConfig()
		if config == nil && sigmaConfig != "" {
			return nil, nil, fmt.Errorf("sigma config '%s' not found", sigmaConfig)
		}
	}

	if len(evaluators) == 0 {
		return nil, nil, fmt.Errorf("no sigma rules loaded. Please specify valid 'rules' path or embedded config")
	}

	if err := openDB(); err != nil {
		return nil, nil, err
	}
	defer closeDB()

	hits := []mcpSigmaHit{}
	sti, eti := getTimeRange()
	_ = ds.ForEach(sti, eti, func(entry *datastore.LogEntry) bool {
		l := entry.Log
		t := entry.Time
		if matchFilter(&l) {
			if ev := matchSigmaRule(&l); ev != nil {
				hits = append(hits, mcpSigmaHit{
					Time:        time.Unix(0, t).Format(time.RFC3339Nano),
					Log:         l,
					RuleID:      ev.ID,
					RuleTitle:   ev.Title,
					RuleLevel:   ev.Level,
					Description: ev.Description,
				})
				if len(hits) >= limit {
					return false
				}
			}
		}
		return true
	})

	j, err := json.MarshalIndent(hits, "", "  ")
	if err != nil {
		return nil, nil, err
	}
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: string(j)},
		},
	}, nil, nil
}

// -------------------------------------------------------------
// detect_anomalies
// -------------------------------------------------------------

type detectAnomaliesParams struct {
	Filter  string `json:"filter,omitempty" jsonschema:"Filter logs by regular expression. Empty is no filter"`
	Mode    string `json:"mode,omitempty" jsonschema:"Detection mode: tfidf, sql, os, dir, walu, number. Default: tfidf"`
	Algo    string `json:"algo,omitempty" jsonschema:"Anomaly detection algorithm: iforest, autoencoder, lstm, lof, knn, mahalanobis, zscore. Default: iforest"`
	Extract string `json:"extract,omitempty" jsonschema:"Extract pattern for number mode (e.g. number or regex)."`
	TopN    int    `json:"top_n,omitempty" jsonschema:"Limit on top anomaly results. Default: 20"`
	Start   string `json:"start,omitempty" jsonschema:"Start date and time for log search. Empty is 1970/1/1."`
	End     string `json:"end,omitempty" jsonschema:"End date and time for log search. Empty is now."`
}

type mcpAnomalyHit struct {
	Time  string  `json:"time"`
	Log   string  `json:"log"`
	Score float64 `json:"score"`
}

func detectAnomalies(ctx context.Context, req *mcp.CallToolRequest, args detectAnomaliesParams) (*mcp.CallToolResult, any, error) {
	mcpMutex.Lock()
	defer mcpMutex.Unlock()

	regexpFilter = args.Filter
	anomalyMode = args.Mode
	if anomalyMode == "" {
		anomalyMode = "tfidf"
	}
	anomalyAlgo = args.Algo
	if anomalyAlgo == "" {
		anomalyAlgo = "iforest"
	}
	extract = args.Extract
	timeRange = args.Start + "," + args.End
	topN := args.TopN
	if topN <= 0 {
		topN = 20
	}
	if topN > 500 {
		topN = 500
	}

	setupFilter([]string{})
	if err := openDB(); err != nil {
		return nil, nil, err
	}
	defer closeDB()

	results = []string{}
	times = []int64{}
	lines = 0
	hit = 0
	sti, eti := getTimeRange()
	_ = ds.ForEach(sti, eti, func(entry *datastore.LogEntry) bool {
		t := entry.Time
		l := entry.Log
		lines++
		if matchFilter(&l) {
			hit++
			results = append(results, l)
			times = append(times, t)
		}
		return true
	})

	if len(results) == 0 {
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: "[]"},
			},
		}, nil, nil
	}

	switch anomalyMode {
	case "sql":
		anomalySQL()
	case "os":
		anomalyOS()
	case "dir":
		anomalyDir()
	case "walu":
		anomalyWalu()
	case "number":
		anomalyNumber()
	default:
		anomalyTFIDF()
	}

	detector, err := anomaly.NewDetector(anomalyAlgo)
	if err != nil {
		return nil, nil, fmt.Errorf("invalid anomaly algorithm: %w", err)
	}

	if err := detector.Fit(vectors); err != nil {
		return nil, nil, fmt.Errorf("failed to train anomaly detector: %w", err)
	}

	anomalyList = []anomalyEnt{}
	for i, v := range vectors {
		anomalyList = append(anomalyList, anomalyEnt{
			Log:   i,
			Score: detector.Score(v),
		})
	}
	sort.Slice(anomalyList, func(a, b int) bool {
		return anomalyList[a].Score > anomalyList[b].Score
	})

	hits := []mcpAnomalyHit{}
	for i, r := range anomalyList {
		if i >= topN {
			break
		}
		hits = append(hits, mcpAnomalyHit{
			Time:  time.Unix(0, times[r.Log]).Format(time.RFC3339Nano),
			Log:   results[r.Log],
			Score: r.Score,
		})
	}

	j, err := json.MarshalIndent(hits, "", "  ")
	if err != nil {
		return nil, nil, err
	}
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: string(j)},
		},
	}, nil, nil
}

// -------------------------------------------------------------
// analyze_relations
// -------------------------------------------------------------

type analyzeRelationsParams struct {
	Filter    string   `json:"filter,omitempty" jsonschema:"Filter logs by regular expression. Empty is no filter"`
	DataTypes []string `json:"data_types" jsonschema:"List of data types or regex extractions to correlate (e.g. ['ip', 'mac'], ['ip', 'email'], ['regex/user=(\\w+)/blue', 'ip']). At least 2 elements required."`
	TopN      int      `json:"top_n,omitempty" jsonschema:"Limit top N co-occurrence results. Default: 20"`
	Start     string   `json:"start,omitempty" jsonschema:"Start date and time for log search. Empty is 1970/1/1."`
	End       string   `json:"end,omitempty" jsonschema:"End date and time for log search. Empty is now."`
}

type mcpRelationHit struct {
	Values []string `json:"values"`
	Count  int      `json:"count"`
}

func analyzeRelations(ctx context.Context, req *mcp.CallToolRequest, args analyzeRelationsParams) (*mcp.CallToolResult, any, error) {
	mcpMutex.Lock()
	defer mcpMutex.Unlock()

	regexpFilter = args.Filter
	timeRange = args.Start + "," + args.End
	topN := args.TopN
	if topN <= 0 {
		topN = 20
	}
	if topN > 500 {
		topN = 500
	}

	if len(args.DataTypes) < 2 {
		return nil, nil, fmt.Errorf("at least 2 data_types must be specified for relation analysis")
	}

	relationCheckList = []relationDataEnt{}
	for _, e := range args.DataTypes {
		switch {
		case strings.HasPrefix(e, "ip"):
			relationCheckList = append(relationCheckList, relationDataEnt{
				Name:  e,
				Reg:   regexpIP,
				Index: getRelationEntIndex(e),
			})
		case strings.HasPrefix(e, "mac"):
			relationCheckList = append(relationCheckList, relationDataEnt{
				Name:  e,
				Reg:   regexpMAC,
				Index: getRelationEntIndex(e),
			})
		case strings.HasPrefix(e, "email"):
			relationCheckList = append(relationCheckList, relationDataEnt{
				Name:  e,
				Reg:   regexpEMail,
				Index: getRelationEntIndex(e),
			})
		case strings.HasPrefix(e, "url"):
			relationCheckList = append(relationCheckList, relationDataEnt{
				Name:  e,
				Reg:   regexpURL,
				Index: getRelationEntIndex(e),
			})
		case strings.HasPrefix(e, "kv"):
			relationCheckList = append(relationCheckList, relationDataEnt{
				Name:  e,
				Reg:   regexpKV,
				Index: getRelationEntIndex(e),
			})
		case strings.HasPrefix(e, "regex/") || strings.HasPrefix(e, "regexp/"):
			a := strings.Split(e, "/")
			if len(a) > 2 {
				p := ""
				for i := 1; i < len(a)-1; i++ {
					if p != "" {
						p += "/"
					}
					p += a[i]
				}
				relationCheckList = append(relationCheckList, relationDataEnt{
					Name:  e,
					Reg:   regexp.MustCompile(p),
					Index: getRelationEntIndex(e),
				})
			}
		}
	}

	if len(relationCheckList) < 2 {
		return nil, nil, fmt.Errorf("failed to parse valid data_types for relation analysis")
	}

	setupFilter([]string{})
	if err := openDB(); err != nil {
		return nil, nil, err
	}
	defer closeDB()

	relationMap := make(map[string]*relationEnt)
	sti, eti := getTimeRange()
	_ = ds.ForEach(sti, eti, func(entry *datastore.LogEntry) bool {
		l := entry.Log
		if matchFilter(&l) {
			var vals = []string{}
			for _, r := range relationCheckList {
				a := r.Reg.FindAllString(l, -1)
				if len(a) < r.Index+1 {
					break
				}
				vals = append(vals, a[r.Index])
			}
			if len(vals) != len(relationCheckList) {
				return true
			}
			key := strings.Join(vals, "\t")
			if e, ok := relationMap[key]; ok {
				e.Count++
			} else {
				relationMap[key] = &relationEnt{
					Key:    key,
					Values: vals,
					Count:  1,
				}
			}
		}
		return true
	})

	hits := []*relationEnt{}
	for _, v := range relationMap {
		hits = append(hits, v)
	}
	sort.Slice(hits, func(i, j int) bool {
		return hits[i].Count > hits[j].Count
	})
	if len(hits) > topN {
		hits = hits[:topN]
	}

	resp := []mcpRelationHit{}
	for _, h := range hits {
		resp = append(resp, mcpRelationHit{
			Values: h.Values,
			Count:  h.Count,
		})
	}

	j, err := json.MarshalIndent(resp, "", "  ")
	if err != nil {
		return nil, nil, err
	}
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: string(j)},
		},
	}, nil, nil
}

// -------------------------------------------------------------
// analyze_tfidf
// -------------------------------------------------------------

type analyzeTFIDFParams struct {
	Filter    string  `json:"filter,omitempty" jsonschema:"Filter logs by regular expression. Empty is no filter"`
	Threshold float64 `json:"threshold,omitempty" jsonschema:"Similarity threshold between logs (0.0 to 1.0). Default: 0.5"`
	Count     int     `json:"count,omitempty" jsonschema:"Number of threshold crossings to exclude. Default: 0"`
	TopN      int     `json:"top_n,omitempty" jsonschema:"Limit top N rare log results. Default: 20"`
	Start     string  `json:"start,omitempty" jsonschema:"Start date and time for log search. Empty is 1970/1/1."`
	End       string  `json:"end,omitempty" jsonschema:"End date and time for log search. Empty is now."`
}

type mcpTFIDFHit struct {
	Log  string  `json:"log"`
	Min  float64 `json:"min"`
	Mean float64 `json:"mean"`
	Max  float64 `json:"max"`
}

func analyzeTFIDF(ctx context.Context, req *mcp.CallToolRequest, args analyzeTFIDFParams) (*mcp.CallToolResult, any, error) {
	mcpMutex.Lock()
	defer mcpMutex.Unlock()

	regexpFilter = args.Filter
	timeRange = args.Start + "," + args.End
	threshold := args.Threshold
	if threshold <= 0.0 {
		threshold = 0.5
	}
	excludeCount := args.Count
	topN := args.TopN
	if topN <= 0 {
		topN = 20
	}
	if topN > 500 {
		topN = 500
	}

	setupFilter([]string{})
	if err := openDB(); err != nil {
		return nil, nil, err
	}
	defer closeDB()

	results = []string{}
	sti, eti := getTimeRange()
	_ = ds.ForEach(sti, eti, func(entry *datastore.LogEntry) bool {
		l := entry.Log
		if matchFilter(&l) {
			results = append(results, l)
		}
		return true
	})

	if len(results) == 0 {
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: "[]"},
			},
		}, nil, nil
	}

	tfidf := tf_idf.New(
		tf_idf.WithDefaultStopWords(),
	)
	for _, l := range results {
		tfidf.AddDocument(l)
	}

	tfidfList = []tfidfEnt{}
	for i, l1 := range results {
		sims := []float64{}
		done := true
		cnt := 0
		for j, l2 := range results {
			if i == j {
				continue
			}
			if s, err := tfidf.Compare(l1, l2); err == nil {
				sims = append(sims, s)
				if threshold < s {
					cnt++
					if excludeCount > 0 && cnt > excludeCount {
						done = false
						break
					}
				}
			}
		}
		if done && len(sims) > 0 {
			minV, _ := stats.Min(sims)
			meanV, _ := stats.Mean(sims)
			maxV, _ := stats.Max(sims)
			tfidfList = append(tfidfList, tfidfEnt{
				Log:  i,
				Min:  minV,
				Mean: meanV,
				Max:  maxV,
			})
		}
	}

	sort.Slice(tfidfList, func(i, j int) bool {
		return tfidfList[i].Mean < tfidfList[j].Mean
	})

	hits := []mcpTFIDFHit{}
	for i, r := range tfidfList {
		if i >= topN {
			break
		}
		hits = append(hits, mcpTFIDFHit{
			Log:  results[r.Log],
			Min:  r.Min,
			Mean: r.Mean,
			Max:  r.Max,
		})
	}

	j, err := json.MarshalIndent(hits, "", "  ")
	if err != nil {
		return nil, nil, err
	}
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: string(j)},
		},
	}, nil, nil
}

// -------------------------------------------------------------
// Prompts Implementation
// -------------------------------------------------------------

func incidentInvestigationPrompt(ctx context.Context, req *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
	target, ok := req.Params.Arguments["target"]
	if !ok || target == "" {
		return nil, fmt.Errorf("target is required")
	}
	start := req.Params.Arguments["start"]
	end := req.Params.Arguments["end"]

	prompt := fmt.Sprintf(`Please investigate the incident related to target '%s' by following these steps:
1. First, check the database overview using 'get_db_info' or resource 'twsla://db/status'.
2. Search and count logs matching filter '%s' in the given time window (Start: '%s', End: '%s') using 'search_log' and 'count_log'.
3. Extract associated entities (IPs, MAC addresses, emails, URLs) using 'extract_data_from_log' and analyze their correlation with 'analyze_relations'.
4. Perform security checks using 'detect_threats_sigma' and detect anomalous behavior with 'detect_anomalies'.
5. Summarize findings: timeline of events, affected entities, detected threat indicators, and recommended remediation steps.`, target, target, start, end)

	return &mcp.GetPromptResult{
		Description: "Incident investigation prompt",
		Messages: []*mcp.PromptMessage{
			{
				Role:    "user",
				Content: &mcp.TextContent{Text: prompt},
			},
		},
	}, nil
}

func securityThreatHuntPrompt(ctx context.Context, req *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
	rules := req.Params.Arguments["rules"]
	config := req.Params.Arguments["config"]

	prompt := fmt.Sprintf(`Perform a security threat hunt across the log database:
1. Examine available Sigma configurations using resource 'twsla://sigma/rules'.
2. Execute 'detect_threats_sigma' (rules: '%s', config: '%s') to detect known attack signatures and techniques.
3. Check for web/injection attacks by running 'detect_anomalies' with modes: 'sql', 'os', 'dir', and 'walu'.
4. Cross-reference any suspicious source IP or user using 'analyze_relations' and 'count_log'.
5. Provide a threat assessment report detailing high-severity alerts, MITRE ATT&CK mapping, and affected assets.`, rules, config)

	return &mcp.GetPromptResult{
		Description: "Security threat hunt prompt",
		Messages: []*mcp.PromptMessage{
			{
				Role:    "user",
				Content: &mcp.TextContent{Text: prompt},
			},
		},
	}, nil
}

func anomalyAuditPrompt(ctx context.Context, req *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
	filter := req.Params.Arguments["filter"]

	prompt := fmt.Sprintf(`Perform an anomaly and outlier audit on the log database (Filter: '%s'):
1. Retrieve overall log metrics and top error patterns using 'get_log_summary'.
2. Run 'detect_anomalies' using mode 'tfidf' and algorithm 'iforest' to find structurally anomalous log lines.
3. Run 'analyze_tfidf' to identify unique or rare log events.
4. Highlight any abnormal spikes or novel errors that deviate from baseline operations.`, filter)

	return &mcp.GetPromptResult{
		Description: "Anomaly and rare log audit prompt",
		Messages: []*mcp.PromptMessage{
			{
				Role:    "user",
				Content: &mcp.TextContent{Text: prompt},
			},
		},
	}, nil
}
