package cmd

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/twsnmp/twsla/pkg/datastore"
)

func setupTestMCPDB(t *testing.T) (string, func()) {
	t.Helper()
	dir, err := os.MkdirTemp("", "mcp_test_*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}

	dbPath := filepath.Join(dir, "test.db")
	dataStore = dbPath

	ds, err := datastore.Open(dataStore)
	if err != nil {
		os.RemoveAll(dir)
		t.Fatalf("failed to open datastore: %v", err)
	}

	testLogs := []struct {
		offset time.Duration
		log    string
	}{
		{time.Minute * 10, `2026-08-30 01:00:00 [ERROR] 192.168.1.100 - user admin logged in with mac 00:11:22:33:44:55 admin@example.com`},
		{time.Minute * 8, `2026-08-30 01:02:00 [INFO] 192.168.1.100 - GET /index.html 200 OK`},
		{time.Minute * 6, `2026-08-30 01:04:00 [WARN] 192.168.1.105 - failed password for root from 192.168.1.105 port 22`},
		{time.Minute * 4, `2026-08-30 01:06:00 [ERROR] 192.168.1.105 - failed password for root from 192.168.1.105 port 22`},
		{time.Minute * 2, `2026-08-30 01:08:00 [INFO] 192.168.1.100 - GET /api/v1/data 200 OK user admin@example.com`},
	}

	entries := []*datastore.LogEntry{}
	baseTime := time.Now().Add(-time.Hour)
	for i, entry := range testLogs {
		logTime := baseTime.Add(entry.offset).UnixNano()
		entries = append(entries, &datastore.LogEntry{
			Time:  logTime,
			Log:   entry.log,
			Delta: 0,
			Hash:  "testhash",
			Line:  i + 1,
		})
	}
	if err := ds.SaveLogs(entries); err != nil {
		ds.Close()
		os.RemoveAll(dir)
		t.Fatalf("failed to save logs: %v", err)
	}
	ds.Close()

	cleanup := func() {
		os.RemoveAll(dir)
		dataStore = ""
		timeRange = ""
		regexpFilter = ""
	}

	return dbPath, cleanup
}

func TestMCPTools(t *testing.T) {
	_, cleanup := setupTestMCPDB(t)
	defer cleanup()

	ctx := context.Background()

	t.Run("get_db_info", func(t *testing.T) {
		res, _, err := getDBInfo(ctx, nil, getDBInfoParams{})
		if err != nil {
			t.Fatalf("getDBInfo failed: %v", err)
		}
		if len(res.Content) == 0 {
			t.Fatalf("expected content in result")
		}
		txt := res.Content[0].(*mcp.TextContent).Text
		var info map[string]interface{}
		if err := json.Unmarshal([]byte(txt), &info); err != nil {
			t.Fatalf("failed to unmarshal db info: %v", err)
		}
		if total, ok := info["total_logs"].(float64); !ok || total != 5 {
			t.Errorf("expected 5 total_logs, got %v", info["total_logs"])
		}
	})

	t.Run("search_log", func(t *testing.T) {
		res, _, err := searchLog(ctx, nil, searchLogParams{
			Filter: "ERROR",
			Limit:  100,
		})
		if err != nil {
			t.Fatalf("searchLog failed: %v", err)
		}
		txt := res.Content[0].(*mcp.TextContent).Text
		var results []string
		if err := json.Unmarshal([]byte(txt), &results); err != nil {
			t.Fatalf("failed to unmarshal search results: %v", err)
		}
		if len(results) != 2 {
			t.Errorf("expected 2 error logs, got %d", len(results))
		}
	})

	t.Run("count_log", func(t *testing.T) {
		res, _, err := countLog(ctx, nil, countLogParams{
			Unit: "ip",
			TopN: 10,
		})
		if err != nil {
			t.Fatalf("countLog failed: %v", err)
		}
		txt := res.Content[0].(*mcp.TextContent).Text
		var counts []mcpCountEnt
		if err := json.Unmarshal([]byte(txt), &counts); err != nil {
			t.Fatalf("failed to unmarshal count results: %v", err)
		}
		if len(counts) == 0 {
			t.Fatalf("expected counts, got 0")
		}
	})

	t.Run("extract_data_from_log", func(t *testing.T) {
		res, _, err := extractDataFromLog(ctx, nil, extractDataFromLogParams{
			Pattern: "email",
		})
		if err != nil {
			t.Fatalf("extractDataFromLog failed: %v", err)
		}
		txt := res.Content[0].(*mcp.TextContent).Text
		var extracts []mcpExtractEnt
		if err := json.Unmarshal([]byte(txt), &extracts); err != nil {
			t.Fatalf("failed to unmarshal extract results: %v", err)
		}
		if len(extracts) != 2 {
			t.Errorf("expected 2 email extracts, got %d", len(extracts))
		}
	})

	t.Run("get_log_summary", func(t *testing.T) {
		res, _, err := summaryLog(ctx, nil, summaryLogParams{
			TopN: 10,
		})
		if err != nil {
			t.Fatalf("summaryLog failed: %v", err)
		}
		txt := res.Content[0].(*mcp.TextContent).Text
		var summary mcpLogSummaryEnt
		if err := json.Unmarshal([]byte(txt), &summary); err != nil {
			t.Fatalf("failed to unmarshal summary: %v", err)
		}
		if summary.Total != 5 {
			t.Errorf("expected total 5, got %d", summary.Total)
		}
		if summary.Errors != 3 {
			t.Errorf("expected 3 errors, got %d", summary.Errors)
		}
	})

	t.Run("detect_anomalies", func(t *testing.T) {
		res, _, err := detectAnomalies(ctx, nil, detectAnomaliesParams{
			Mode: "tfidf",
			Algo: "iforest",
			TopN: 5,
		})
		if err != nil {
			t.Fatalf("detectAnomalies failed: %v", err)
		}
		txt := res.Content[0].(*mcp.TextContent).Text
		var hits []mcpAnomalyHit
		if err := json.Unmarshal([]byte(txt), &hits); err != nil {
			t.Fatalf("failed to unmarshal anomaly hits: %v", err)
		}
		if len(hits) == 0 {
			t.Errorf("expected anomaly hits, got 0")
		}
	})

	t.Run("analyze_relations", func(t *testing.T) {
		res, _, err := analyzeRelations(ctx, nil, analyzeRelationsParams{
			DataTypes: []string{"ip", "email"},
			TopN:      10,
		})
		if err != nil {
			t.Fatalf("analyzeRelations failed: %v", err)
		}
		txt := res.Content[0].(*mcp.TextContent).Text
		var hits []mcpRelationHit
		if err := json.Unmarshal([]byte(txt), &hits); err != nil {
			t.Fatalf("failed to unmarshal relation hits: %v", err)
		}
		if len(hits) == 0 {
			t.Errorf("expected relation hits, got 0")
		}
	})

	t.Run("empty_params_call", func(t *testing.T) {
		// search_log with empty params
		res, _, err := searchLog(ctx, nil, searchLogParams{})
		if err != nil {
			t.Fatalf("searchLog with empty params failed: %v", err)
		}
		if len(res.Content) == 0 {
			t.Fatalf("expected content for empty searchLog")
		}

		// count_log with empty params
		res, _, err = countLog(ctx, nil, countLogParams{})
		if err != nil {
			t.Fatalf("countLog with empty params failed: %v", err)
		}
		if len(res.Content) == 0 {
			t.Fatalf("expected content for empty countLog")
		}

		// summary_log with empty params
		res, _, err = summaryLog(ctx, nil, summaryLogParams{})
		if err != nil {
			t.Fatalf("summaryLog with empty params failed: %v", err)
		}
		if len(res.Content) == 0 {
			t.Fatalf("expected content for empty summaryLog")
		}

		// detect_anomalies with empty params
		res, _, err = detectAnomalies(ctx, nil, detectAnomaliesParams{})
		if err != nil {
			t.Fatalf("detectAnomalies with empty params failed: %v", err)
		}
		if len(res.Content) == 0 {
			t.Fatalf("expected content for empty detectAnomalies")
		}
	})
}

func TestMCPResources(t *testing.T) {
	_, cleanup := setupTestMCPDB(t)
	defer cleanup()

	ctx := context.Background()

	t.Run("twsla://db/status", func(t *testing.T) {
		res, err := dbStatusResourceHandler(ctx, &mcp.ReadResourceRequest{
			Params: &mcp.ReadResourceParams{URI: "twsla://db/status"},
		})
		if err != nil {
			t.Fatalf("dbStatusResourceHandler failed: %v", err)
		}
		if len(res.Contents) == 0 {
			t.Fatalf("expected contents in resource")
		}
		var status map[string]interface{}
		if err := json.Unmarshal([]byte(res.Contents[0].Text), &status); err != nil {
			t.Fatalf("failed to parse resource json: %v", err)
		}
		if total, ok := status["total_logs"].(float64); !ok || total != 5 {
			t.Errorf("expected 5 total_logs, got %v", status["total_logs"])
		}
	})

	t.Run("twsla://sigma/rules", func(t *testing.T) {
		res, err := sigmaRulesResourceHandler(ctx, &mcp.ReadResourceRequest{
			Params: &mcp.ReadResourceParams{URI: "twsla://sigma/rules"},
		})
		if err != nil {
			t.Fatalf("sigmaRulesResourceHandler failed: %v", err)
		}
		if len(res.Contents) == 0 {
			t.Fatalf("expected contents in resource")
		}
	})
}

func TestMCPPrompts(t *testing.T) {
	ctx := context.Background()

	t.Run("incident_investigation", func(t *testing.T) {
		res, err := incidentInvestigationPrompt(ctx, &mcp.GetPromptRequest{
			Params: &mcp.GetPromptParams{
				Arguments: map[string]string{
					"target": "192.168.1.100",
					"start":  "2026-08-30 00:00:00",
					"end":    "2026-08-30 02:00:00",
				},
			},
		})
		if err != nil {
			t.Fatalf("incidentInvestigationPrompt failed: %v", err)
		}
		if len(res.Messages) == 0 {
			t.Fatalf("expected prompt message")
		}
	})

	t.Run("security_threat_hunt", func(t *testing.T) {
		res, err := securityThreatHuntPrompt(ctx, &mcp.GetPromptRequest{
			Params: &mcp.GetPromptParams{
				Arguments: map[string]string{
					"config": "apache",
				},
			},
		})
		if err != nil {
			t.Fatalf("securityThreatHuntPrompt failed: %v", err)
		}
		if len(res.Messages) == 0 {
			t.Fatalf("expected prompt message")
		}
	})

	t.Run("anomaly_audit", func(t *testing.T) {
		res, err := anomalyAuditPrompt(ctx, &mcp.GetPromptRequest{
			Params: &mcp.GetPromptParams{
				Arguments: map[string]string{
					"filter": "error",
				},
			},
		})
		if err != nil {
			t.Fatalf("anomalyAuditPrompt failed: %v", err)
		}
		if len(res.Messages) == 0 {
			t.Fatalf("expected prompt message")
		}
	})
}
