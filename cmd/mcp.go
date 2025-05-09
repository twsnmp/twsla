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
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/domainr/dnsr"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/spf13/cobra"
	"go.etcd.io/bbolt"
)

var mcpTrapsport = ""

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
	mcpCmd.Flags().StringVar(&mcpTrapsport, "transport", "stdio", "Help message for toggle")
	mcpCmd.Flags().StringVar(&geoipDBPath, "geoip", "", "geo IP database file")
}

func mcpServer() {
	// MCPサーバーインスタンスの作成
	s := server.NewMCPServer(
		"TWSLA MCP Server",
		"1.0.0",
		server.WithLogging(),
	)
	addSearchTool(s)
	addCountTool(s)
	addExtractTool(s)

	if mcpTrapsport != "stdio" {
		u, err := url.Parse(mcpTrapsport)
		if err != nil {
			log.Fatalf("mcp err=%v", err)
		}

		sseServer := server.NewSSEServer(s, server.WithBaseURL(mcpTrapsport))
		log.Printf("SSE server listening on :%s", u.Port())
		if err := sseServer.Start(fmt.Sprintf(":%s", u.Port())); err != nil {
			log.Fatalf("Server error: %v", err)
		}
		return
	}
	// サーバー起動
	if err := server.ServeStdio(s); err != nil {
		log.Printf("Server error: %v\n", err)
	}

}

func addSearchTool(s *server.MCPServer) {
	searchTool := mcp.NewTool("search",
		mcp.WithDescription("Search logs from twsla DB"),
		mcp.WithString("filter",
			mcp.Description("Filter logs by regular expression"),
		),
		mcp.WithNumber("limit",
			mcp.Required(),
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
		if v, ok := request.Params.Arguments["filter"]; ok {
			if f, ok := v.(string); ok {
				regexpFilter = f
			}
		}
		if v, ok := request.Params.Arguments["timeRange"]; ok {
			if tr, ok := v.(string); ok {
				timeRange = tr
			}
		}
		limit := 100
		if v, ok := request.Params.Arguments["limit"]; ok {
			if l, ok := v.(float64); ok {
				limit = int(l)
			}
		}
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
		if v, ok := request.Params.Arguments["filter"]; ok {
			if f, ok := v.(string); ok {
				regexpFilter = f
			}
		}
		timeMode := true
		ipm := 0
		extract = ""
		if v, ok := request.Params.Arguments["unit"]; ok {
			if u, ok := v.(string); ok {
				switch u {
				case "mac", "email":
					extract = u
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
			}
		}
		if v, ok := request.Params.Arguments["timeRange"]; ok {
			if tr, ok := v.(string); ok {
				timeRange = tr
			}
		}
		pos = 1
		if v, ok := request.Params.Arguments["pos"]; ok {
			if p, ok := v.(float64); ok {
				pos = int(p)
			}
		}
		if v, ok := request.Params.Arguments["interval"]; ok {
			if i, ok := v.(float64); ok {
				interval = int(i)
			}
		}
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
		if v, ok := request.Params.Arguments["filter"]; ok {
			if f, ok := v.(string); ok {
				regexpFilter = f
			}
		}
		extract = ""
		if v, ok := request.Params.Arguments["pattern"]; ok {
			if e, ok := v.(string); ok {
				extract = e
			}
		}
		pos = 1
		if v, ok := request.Params.Arguments["pos"]; ok {
			if p, ok := v.(float64); ok {
				pos = int(p)
			}
		}
		if v, ok := request.Params.Arguments["timeRange"]; ok {
			if tr, ok := v.(string); ok {
				timeRange = tr
			}
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
