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
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

var lokiQuery string
var lokiOrgId string
var lokiToken string
var lokiHTTPClient *http.Client

type lokiQueryResponse struct {
	Status string `json:"status"`
	Data   struct {
		ResultType string `json:"resultType"`
		Result     []struct {
			Stream map[string]string `json:"stream"`
			Values [][]string        `json:"values"`
		} `json:"result"`
	} `json:"data"`
}

func importFromLoki() {
	u, err := url.Parse(source)
	if err != nil {
		sendErrorMsg(err)
		return
	}

	scheme := "http"
	if u.Scheme == "lokis" || strings.HasPrefix(source, "https:") {
		scheme = "https"
	}
	host := u.Host
	if host == "" {
		host = u.Path
	}

	lokiURL := fmt.Sprintf("%s://%s/loki/api/v1/query_range", scheme, host)
	baseURL := fmt.Sprintf("%s://%s", scheme, host)

	// If query specified in URL path like loki://host:port/{app="foo"}
	if lokiQuery == "" && u.Path != "" && strings.Contains(u.Path, "{") {
		p := strings.Trim(u.Path, "/")
		if strings.HasPrefix(p, "{") {
			lokiQuery = p
		}
	}

	client := lokiHTTPClient
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
	query := resolveLokiQuery(client, baseURL, u.User, st, et)
	path := fmt.Sprintf("%s (%s)", source, query)
	hash := getSHA1(path)
	const maxWindowNanos = int64(24 * time.Hour)
	limit := 5000

	lastTime := int64(0)
	readBytes := int64(0)
	readLines := 0
	skipLines := 0
	totalFiles++

	for currentStart := st; currentStart < et && !stopImport; {
		reqEnd := currentStart + maxWindowNanos
		if reqEnd > et {
			reqEnd = et
		}

		params := url.Values{}
		params.Set("query", query)
		params.Set("start", strconv.FormatInt(currentStart, 10))
		params.Set("end", strconv.FormatInt(reqEnd, 10))
		params.Set("limit", strconv.Itoa(limit))
		params.Set("direction", "FORWARD")

		reqURL := fmt.Sprintf("%s?%s", lokiURL, params.Encode())
		req, err := http.NewRequest("GET", reqURL, nil)
		if err != nil {
			sendErrorMsg(err)
			return
		}

		if u.User != nil {
			pass, _ := u.User.Password()
			req.SetBasicAuth(u.User.Username(), pass)
		}
		if lokiToken != "" {
			req.Header.Set("Authorization", "Bearer "+lokiToken)
		}
		if lokiOrgId != "" {
			req.Header.Set("X-Scope-OrgID", lokiOrgId)
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
			sendErrorMsg(fmt.Errorf("loki error response [%d]: %s", resp.StatusCode, string(body)))
			return
		}

		var lokiResp lokiQueryResponse
		if err := json.Unmarshal(body, &lokiResp); err != nil {
			sendErrorMsg(fmt.Errorf("failed to parse loki response: %w", err))
			return
		}

		// Flatten entries sorted by timestamp
		type valEnt struct {
			ts     int64
			log    string
			stream map[string]string
		}
		var batchEntries []valEnt

		for _, res := range lokiResp.Data.Result {
			for _, v := range res.Values {
				if len(v) < 2 {
					continue
				}
				ts, err := strconv.ParseInt(v[0], 10, 64)
				if err != nil {
					continue
				}
				batchEntries = append(batchEntries, valEnt{ts: ts, log: v[1], stream: res.Stream})
			}
		}

		if len(batchEntries) == 0 {
			currentStart = reqEnd
			if currentStart >= et {
				break
			}
			continue
		}

		maxTs := currentStart
		for _, ent := range batchEntries {
			if stopImport {
				return
			}
			t := ent.ts
			if t > maxTs {
				maxTs = t
			}

			// Format log as "TIMESTAMP JSON"
			logMap := make(map[string]interface{})
			for k, v := range ent.stream {
				logMap[k] = v
			}
			logMap["message"] = ent.log
			jsonBytes, err := json.Marshal(logMap)
			var l string
			if err != nil {
				l = fmt.Sprintf("%s %s", time.Unix(0, t).Format(time.RFC3339Nano), ent.log)
			} else {
				l = fmt.Sprintf("%s %s", time.Unix(0, t).Format(time.RFC3339Nano), string(jsonBytes))
			}

			readBytes += int64(len(l))
			totalBytes += int64(len(l))
			readLines++
			totalLines++

			if importFilter != nil && !importFilter.MatchString(l) {
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

			if st > t || et < t {
				skipLines++
				continue
			}

			logCh <- &LogEnt{
				Time:  t,
				Log:   l,
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

		// Advance next start time
		if len(batchEntries) >= limit {
			if maxTs <= currentStart {
				currentStart++
			} else {
				currentStart = maxTs + 1
			}
		} else {
			currentStart = reqEnd
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

func getLokiQuery() string {
	if lokiQuery != "" {
		return lokiQuery
	}
	return `{job=~".+"}`
}

func resolveLokiQuery(client *http.Client, baseURL string, authUser *url.Userinfo, st, et int64) string {
	if lokiQuery != "" {
		return lokiQuery
	}

	// Try auto-detecting labels from /loki/api/v1/labels
	labelsURL := fmt.Sprintf("%s/loki/api/v1/labels?start=%d&end=%d", baseURL, st, et)
	req, err := http.NewRequest("GET", labelsURL, nil)
	if err == nil {
		if authUser != nil {
			pass, _ := authUser.Password()
			req.SetBasicAuth(authUser.Username(), pass)
		}
		if lokiToken != "" {
			req.Header.Set("Authorization", "Bearer "+lokiToken)
		}
		if lokiOrgId != "" {
			req.Header.Set("X-Scope-OrgID", lokiOrgId)
		}

		resp, err := client.Do(req)
		if err == nil {
			defer resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				var lResp struct {
					Status string   `json:"status"`
					Data   []string `json:"data"`
				}
				if err := json.NewDecoder(resp.Body).Decode(&lResp); err == nil && len(lResp.Data) > 0 {
					// Prefer standard labels like job, app, service_name, container, filename, or any non-internal label
					preferred := []string{"job", "app", "service_name", "container", "filename", "stream"}
					for _, pref := range preferred {
						for _, l := range lResp.Data {
							if l == pref {
								return fmt.Sprintf("{%s=~\".+\"}", l)
							}
						}
					}
					// Fallback to first non-internal label
					for _, l := range lResp.Data {
						if l != "" && !strings.HasPrefix(l, "__") {
							return fmt.Sprintf("{%s=~\".+\"}", l)
						}
					}
				}
			}
		}
	}

	return `{job=~".+"}`
}

