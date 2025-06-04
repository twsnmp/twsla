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
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/domainr/dnsr"
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
	// MCPサーバーインスタンスの作成
	s := server.NewMCPServer(
		"TWSLA MCP Server",
		"1.0.0",
		server.WithToolCapabilities(true),
		server.WithLogging(),
	)
	addSearchTool(s)
	addCountTool(s)
	addExtractTool(s)

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
	searchTool := mcp.NewTool("search",
		mcp.WithDescription("Search logs from twsla DB"),
		mcp.WithString("filter",
			mcp.Description("Filter logs by regular expression"),
		),
		mcp.WithNumber("limit",
			mcp.DefaultNumber(100),
			mcp.Max(10000),
			mcp.Min(1),
			mcp.Description("Limit on number of logs retrieved. min 100,max 10000"),
		),
		mcp.WithString("timeRange",
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
		regexpFilter = request.GetString("filter", "")
		timeRange, err = request.RequireString("timeRange")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		limit := request.GetInt("limit", 100)
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
		return mcp.NewToolResultText(strings.Join(results, "\n")), nil
	})
}

func addCountTool(s *server.MCPServer) {
	searchTool := mcp.NewTool("count",
		mcp.WithDescription(
			`This tool counts the number of logs on the TWSLA database.
The number of logs can be counted by time, IP address, MAC address, e-mail address,host name,domain name,country or geo location.`),
		mcp.WithString("filter",
			mcp.Description("Filter logs by regular expression"),
		),
		mcp.WithString("unit",
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
`),
			mcp.Enum("time", "ip", "email", "mac", "host", "domain", "country", "loc"),
		),
		mcp.WithString("timeRange",
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
		mcp.WithNumber("pos",
			mcp.DefaultNumber(1),
			mcp.Max(100),
			mcp.Min(1),
			mcp.Description("position of unit"),
		),
		mcp.WithNumber("interval",
			mcp.DefaultNumber(0),
			mcp.Max(3600*24),
			mcp.Min(0),
			mcp.Description("Specify the aggregation interval in seconds."),
		),
	)

	s.AddTool(searchTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var err error
		regexpFilter = request.GetString("filter", "")
		timeMode := true
		ipm := 0
		extract = ""
		unit, err := request.RequireString("unit")
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
				return nil, err
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
		}
		timeRange, err = request.RequireString("timeRange")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		pos = request.GetInt("pos", 1)
		interval = request.GetInt("interval", 0)
		setupFilter([]string{})
		extPat = nil
		setExtPat()
		if !timeMode && extPat == nil {
			return nil, fmt.Errorf("invalid unit")
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
		cl := []countEnt{}
		for k, v := range countMap {
			cl = append(cl, countEnt{
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
		}
		results := []string{}
		for _, c := range cl {
			results = append(results, fmt.Sprintf("%s %d", c.Key, c.Count))
		}
		return mcp.NewToolResultText(strings.Join(results, "\n")), nil
	})
}

func addExtractTool(s *server.MCPServer) {
	searchTool := mcp.NewTool("extract",
		mcp.WithDescription(
			`This tool extracts data from the logs on the TWSLA database.`),
		mcp.WithString("filter",
			mcp.Description("Filter logs by regular expression"),
		),
		mcp.WithString("pattern",
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
		mcp.WithString("timeRange",
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
		mcp.WithNumber("pos",
			mcp.DefaultNumber(1),
			mcp.Max(100),
			mcp.Min(1),
			mcp.Description("position of pattern"),
		),
	)

	s.AddTool(searchTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var err error
		regexpFilter = request.GetString("filter", "")
		extract = request.GetString("pattern", "")
		pos = request.GetInt("pos", 1)
		timeRange, err = request.RequireString("timeRange")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		setupFilter([]string{})
		extPat = nil
		setExtPat()
		if extPat == nil {
			return nil, fmt.Errorf("invalid pattern")
		}
		if err := openDB(); err != nil {
			return nil, err
		}
		defer db.Close()
		extractList = []extractEnt{}
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
						extractList = append(extractList, extractEnt{Time: t, Value: a[extPat.Index-1][1]})
					}
				}
			}
			return nil
		})
		results := []string{}
		for _, e := range extractList {
			results = append(results, fmt.Sprintf("%s %s", time.Unix(0, e.Time).Format("2006/01/02T15:04:05.999"), e.Value))
		}
		return mcp.NewToolResultText(strings.Join(results, "\n")), nil
	})
}
