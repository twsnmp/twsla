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
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/araddon/dateparse"
)

var esIndex string
var esQuery string
var esTimeField string = "@timestamp"
var esMessageField string = "message"
var esApiKey string
var esHTTPClient *http.Client

type esSearchHit struct {
	Index  string                 `json:"_index"`
	ID     string                 `json:"_id"`
	Source map[string]interface{} `json:"_source"`
	Sort   []interface{}          `json:"sort"`
}

type esSearchResponse struct {
	Took     int  `json:"took"`
	TimedOut bool `json:"timed_out"`
	Hits     struct {
		Total struct {
			Value int `json:"value"`
		} `json:"total"`
		Hits []esSearchHit `json:"hits"`
	} `json:"hits"`
	Error *struct {
		Reason string `json:"reason"`
		Type   string `json:"type"`
	} `json:"error,omitempty"`
}

func importFromES() {
	u, err := url.Parse(source)
	if err != nil {
		sendErrorMsg(err)
		return
	}

	scheme := "http"
	if u.Scheme == "ess" || u.Scheme == "opensearchs" || strings.HasPrefix(source, "https:") {
		scheme = "https"
	}
	host := u.Host
	if host == "" {
		host = u.Path
	}

	targetIndex := esIndex
	if targetIndex == "" {
		p := strings.Trim(u.Path, "/")
		p = strings.TrimSuffix(p, "/_search")
		p = strings.TrimSuffix(p, "_search")
		p = strings.Trim(p, "/")
		if p != "" {
			targetIndex = p
		} else {
			targetIndex = "*"
		}
	} else {
		targetIndex = strings.TrimSuffix(targetIndex, "/_search")
		targetIndex = strings.TrimSuffix(targetIndex, "_search")
		targetIndex = strings.Trim(targetIndex, "/")
		if targetIndex == "" {
			targetIndex = "*"
		}
	}

	searchURL := fmt.Sprintf("%s://%s/%s/_search", scheme, host, targetIndex)
	path := fmt.Sprintf("%s://%s/%s", scheme, host, targetIndex)
	hash := getSHA1(path)

	client := esHTTPClient
	if client == nil {
		client = &http.Client{
			Timeout: 60 * time.Second,
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{
					InsecureSkipVerify: apiSkip,
				},
			},
		}
	}

	st, et := getImportTimeRange()

	timeField := esTimeField
	if timeField == "" {
		timeField = "@timestamp"
	}
	msgField := esMessageField
	if msgField == "" {
		msgField = "message"
	}

	queryStr := esQuery
	if queryStr == "" {
		queryStr = "*"
	}

	size := 1000
	var lastSort []interface{}
	lastTime := int64(0)
	readBytes := int64(0)
	readLines := 0
	skipLines := 0
	totalFiles++

	for !stopImport {
		reqMap := map[string]interface{}{
			"size": size,
			"sort": []interface{}{
				map[string]interface{}{
					timeField: map[string]interface{}{
						"order":          "asc",
						"unmapped_type": "date",
					},
				},
				map[string]interface{}{
					"_id": map[string]interface{}{
						"order": "asc",
					},
				},
			},
		}

		var queryObj map[string]interface{}
		if strings.HasPrefix(strings.TrimSpace(queryStr), "{") {
			if err := json.Unmarshal([]byte(queryStr), &queryObj); err != nil {
				queryObj = map[string]interface{}{
					"query_string": map[string]interface{}{
						"query": queryStr,
					},
				}
			}
		} else {
			queryObj = map[string]interface{}{
				"query_string": map[string]interface{}{
					"query": queryStr,
				},
			}
		}

		var filterList []map[string]interface{}
		if timeRange != "" && (st > 0 || et > 0) {
			rangeMap := map[string]interface{}{}
			if st > 0 {
				rangeMap["gte"] = st / 1e6 // millis
			}
			if et > 0 {
				rangeMap["lte"] = et / 1e6 // millis
			}
			rangeMap["format"] = "epoch_millis"
			filterList = append(filterList, map[string]interface{}{
				"range": map[string]interface{}{
					timeField: rangeMap,
				},
			})
		}

		reqMap["query"] = map[string]interface{}{
			"bool": map[string]interface{}{
				"must":   queryObj,
				"filter": filterList,
			},
		}

		if len(lastSort) > 0 {
			reqMap["search_after"] = lastSort
		}

		reqBytes, err := json.Marshal(reqMap)
		if err != nil {
			sendErrorMsg(err)
			return
		}

		req, err := http.NewRequest("POST", searchURL, bytes.NewReader(reqBytes))
		if err != nil {
			sendErrorMsg(err)
			return
		}
		req.Header.Set("Content-Type", "application/json")

		if u.User != nil {
			pass, _ := u.User.Password()
			req.SetBasicAuth(u.User.Username(), pass)
		}
		if esApiKey != "" {
			req.Header.Set("Authorization", "ApiKey "+esApiKey)
		}

		resp, err := client.Do(req)
		if err != nil {
			sendErrorMsg(err)
			return
		}

		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			sendErrorMsg(err)
			return
		}

		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			sendErrorMsg(fmt.Errorf("elasticsearch/opensearch error [%d]: %s", resp.StatusCode, string(body)))
			return
		}

		var searchResp esSearchResponse
		if err := json.Unmarshal(body, &searchResp); err != nil {
			sendErrorMsg(fmt.Errorf("failed to parse search response: %w", err))
			return
		}

		if searchResp.Error != nil {
			sendErrorMsg(fmt.Errorf("elasticsearch/opensearch search error: %s (%s)", searchResp.Error.Reason, searchResp.Error.Type))
			return
		}

		hits := searchResp.Hits.Hits
		if len(hits) == 0 {
			break
		}

		for _, hit := range hits {
			if stopImport {
				return
			}
			lastSort = hit.Sort

			// Extract timestamp
			t := int64(0)
			if rawTs, ok := hit.Source[timeField]; ok {
				t = parseESTimestamp(rawTs)
			}
			if t == 0 {
				for _, tf := range []string{"timestamp", "time", "datetime", "created_at", "@time"} {
					if rawTs, ok := hit.Source[tf]; ok {
						if parsed := parseESTimestamp(rawTs); parsed > 0 {
							t = parsed
							break
						}
					}
				}
			}
			if t == 0 && !noTimeStamp {
				// Fallback to now if no valid timestamp
				t = time.Now().UnixNano()
			}

			// Format log as "TIMESTAMP JSON"
			jsonBytes, err := json.Marshal(hit.Source)
			var logContent string
			if err != nil {
				logContent = fmt.Sprintf("%s %v", time.Unix(0, t).Format(time.RFC3339Nano), hit.Source)
			} else {
				logContent = fmt.Sprintf("%s %s", time.Unix(0, t).Format(time.RFC3339Nano), string(jsonBytes))
			}

			readBytes += int64(len(logContent))
			totalBytes += int64(len(logContent))
			readLines++
			totalLines++

			if importFilter != nil && !importFilter.MatchString(logContent) {
				skipLines++
				continue
			}

			d := 0
			if !noDeltaCheck {
				if lastTime > 0 {
					d = int(t - lastTime)
				}
				lastTime = t
			}

			if timeRange != "" && (st > 0 && st > t || et > 0 && et < t) {
				skipLines++
				continue
			}

			logCh <- &LogEnt{
				Time:  t,
				Log:   logContent,
				Delta: d,
				Hash:  hash,
				Line:  readLines,
			}

			if readLines%2000 == 0 {
				sendImportMsg(ImportMsg{
					Done:  false,
					Path:  path,
					Bytes: readBytes,
					Lines: readLines,
					Skip:  skipLines,
				})
			}
		}

		sendImportMsg(ImportMsg{
			Done:  false,
			Path:  path,
			Bytes: readBytes,
			Lines: readLines,
			Skip:  skipLines,
		})

		if len(hits) < size {
			break
		}
	}

	sendImportMsg(ImportMsg{
		Done:  false,
		Path:  path,
		Bytes: readBytes,
		Lines: readLines,
		Skip:  skipLines,
	})
}

func parseESTimestamp(val interface{}) int64 {
	switch v := val.(type) {
	case string:
		if t, err := dateparse.ParseAny(v); err == nil {
			return t.UnixNano()
		}
	case float64:
		if v > 1e16 {
			// nanos
			return int64(v)
		} else if v > 1e11 {
			// millis
			return int64(v * 1e6)
		}
		// seconds
		return int64(v * 1e9)
	case int64:
		if v > 1e16 {
			return v
		} else if v > 1e11 {
			return v * 1e6
		}
		return v * 1e9
	case json.Number:
		if f, err := v.Float64(); err == nil {
			return parseESTimestamp(f)
		}
	}
	return 0
}
