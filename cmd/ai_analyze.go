package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/dustin/go-humanize"
	"go.etcd.io/bbolt"

	"github.com/tmc/langchaingo/llms"
	"github.com/tmc/langchaingo/llms/ollama"
	"github.com/tmc/langchaingo/prompts"
)

var aiTotalEntries int
var aiErrorCount int
var aiWarningCount int
var aiStartTime int64
var aiEndTime int64

type aiSampleLogEntry struct {
	Timestamp int64
	Level     string
	Message   string
}

var aiSampleLogList = []*aiSampleLogEntry{}

type aiErrorPattern struct {
	Pattern string
	Count   int
	Example string
}

var aiErrorPatternList = []*aiErrorPattern{}

var errCheckList = []string{}
var warnCheckList = []string{}

type aiAnomaly struct {
	Type        string   `json:"type"`
	Description string   `json:"description"`
	Severity    string   `json:"severity"`
	Examples    []string `json:"examples"`
}

func aiAnalyze() {
	if ollamaURL == "" {
		log.Fatalln("you have to specify ollama url")
	}
	if generativeModel == "" {
		log.Fatalln("you have to specify generative model")
	}
	errCheckList = strings.Split(strings.ToLower(aiErrorLevels), ",")
	warnCheckList = strings.Split(strings.ToLower(aiWarnLevels), ",")
	st = time.Now()
	if err := openDB(); err != nil {
		log.Fatalln(err)
	}
	defer db.Close()
	teaProg = tea.NewProgram(initAIAnalyzeModel())
	var wg sync.WaitGroup
	wg.Add(1)
	go aiAnalyzeImport(&wg)
	if _, err := teaProg.Run(); err != nil {
		log.Fatalln(err)
	}
	wg.Wait()

	if stopAIAnalyze {
		return
	}
	fmt.Println()
	fmt.Println("AI thinking...")
	llm, err := ollama.New(
		ollama.WithModel(generativeModel),
		ollama.WithServerURL(ollamaURL),
	)
	if err != nil {
		log.Fatalf("create llm err=%v", err)
	}
	sampleSize := 50
	if len(aiSampleLogList) < sampleSize {
		sampleSize = len(aiSampleLogList)
	}

	sample := aiSampleLogList[len(aiSampleLogList)-sampleSize:] // Last N entries

	template := prompts.NewPromptTemplate(`
You are an expert system administrator analyzing application logs. Based on the log data provided, identify:

1. **Anomalies**: Unusual patterns, spikes, or unexpected behaviors
2. **Recommendations**: Specific actions to improve system reliability
3. **Critical Issues**: Problems requiring immediate attention

Log Summary:
- Total Entries: {{.total_entries}}
- Errors: {{.error_count}}
- Warnings: {{.warning_count}}
- Time Range: {{.time_range}}

Top Error Patterns:
{{range .top_errors}}
- {{.pattern}} ({{.count}} occurrences)
{{end}}

Recent Log Sample:
{{range .sample}}
{{.timestamp}} [{{.level}}] {{.message}}
{{end}}

Respond in JSON format:
{
  "anomalies": [
    {
      "type": "error_spike|performance|security|other",
      "description": "What was detected",
      "severity": "critical|high|medium|low",
      "examples": ["example log entries"]
    }
  ],
  "recommendations": [
    "Specific actionable recommendations"
  ]
}
{{.add_prompt}}
`, []string{"total_entries", "error_count", "warning_count", "time_range", "top_errors", "sample", "add_prompt"})

	sampleData := make([]map[string]string, len(sample))
	for i, entry := range sample {
		sampleData[i] = map[string]string{
			"timestamp": time.Unix(0, entry.Timestamp).Format(time.RFC3339),
			"level":     entry.Level,
			"message":   entry.Message,
		}
	}

	topErrors := make([]map[string]string, len(aiErrorPatternList))
	for i, entry := range aiErrorPatternList {
		topErrors[i] = map[string]string{
			"pattern": entry.Pattern,
			"count":   fmt.Sprintf("%d", entry.Count),
		}
	}
	if aiRfeportJA {
		addPrompt += "\nanomaliesのdescriptionとrecommendationsは日本語で回答してください。"
	}

	prompt, err := template.Format(map[string]any{
		"total_entries": aiTotalEntries,
		"error_count":   aiErrorCount,
		"warning_count": aiWarningCount,
		"time_range":    fmt.Sprintf("%s to %s", time.Unix(0, aiStartTime).Format(time.RFC3339), time.Unix(0, aiEndTime).Format(time.RFC3339)),
		"top_errors":    topErrors,
		"sample":        sampleData,
		"add_prompt":    addPrompt,
	})
	if err != nil {
		log.Fatalf("formatting prompt: %v", err)
	}

	ctx := context.Background()
	response, err := llm.GenerateContent(ctx, []llms.MessageContent{
		llms.TextParts(llms.ChatMessageTypeHuman, prompt),
	}, llms.WithJSONMode(),
		llms.WithStreamingFunc(func(ctx context.Context, chunk []byte) error {
			fmt.Print(".")
			return nil
		}),
	)
	if err != nil {
		log.Fatalf("generating analysis: %v", err)
	}

	var aiResult struct {
		Anomalies       []aiAnomaly `json:"anomalies"`
		Recommendations []string    `json:"recommendations"`
	}

	if err := json.Unmarshal([]byte(response.Choices[0].Content), &aiResult); err != nil {
		log.Fatalf("parsing AI response: %v", err)
	}
	if aiRfeportJA {
		fmt.Println()
		fmt.Printf("📊 ログ分析レポート\n")
		fmt.Printf("=====================\n\n")

		fmt.Printf("📈 概要:\n")
		fmt.Printf("  全ログ数: %d\n", aiTotalEntries)
		fmt.Printf("  エラー: %d\n", aiErrorCount)
		fmt.Printf("  警告: %d\n", aiWarningCount)
		fmt.Printf("  期間: %s to %s\n\n",
			time.Unix(0, aiStartTime).Format("2006-01-02 15:04:05"),
			time.Unix(0, aiEndTime).Format("2006-01-02 15:04:05"))

		if len(aiErrorPatternList) > 0 {
			fmt.Printf("🔴 件数の多いエラーパターン:\n")
			for i, pattern := range aiErrorPatternList {
				fmt.Printf("  %d. %s (%d 回)\n", i+1, pattern.Pattern, pattern.Count)
			}
			fmt.Println()
		}

		if len(aiResult.Anomalies) > 0 {
			fmt.Printf("⚠️  検知した異常:\n")
			for _, anomaly := range aiResult.Anomalies {
				fmt.Printf("  %s - %s (%s)\n", anomaly.Type, anomaly.Description, anomaly.Severity)
			}
			fmt.Println()
		}

		if len(aiResult.Recommendations) > 0 {
			fmt.Printf("💡 推奨事項:\n")
			for i, rec := range aiResult.Recommendations {
				fmt.Printf("  %d. %s\n", i+1, rec)
			}
			fmt.Println()
		}
	} else {

		fmt.Println()
		fmt.Printf("📊 Log Analysis Report\n")
		fmt.Printf("=====================\n\n")

		fmt.Printf("📈 Summary:\n")
		fmt.Printf("  Total Entries: %d\n", aiTotalEntries)
		fmt.Printf("  Errors: %d\n", aiErrorCount)
		fmt.Printf("  Warnings: %d\n", aiWarningCount)
		fmt.Printf("  Time Range: %s to %s\n\n",
			time.Unix(0, aiStartTime).Format("2006-01-02 15:04:05"),
			time.Unix(0, aiEndTime).Format("2006-01-02 15:04:05"))

		if len(aiErrorPatternList) > 0 {
			fmt.Printf("🔴 Top Error Patterns:\n")
			for i, pattern := range aiErrorPatternList {
				fmt.Printf("  %d. %s (%d occurrences)\n", i+1, pattern.Pattern, pattern.Count)
			}
			fmt.Println()
		}

		if len(aiResult.Anomalies) > 0 {
			fmt.Printf("⚠️  Detected Anomalies:\n")
			for _, anomaly := range aiResult.Anomalies {
				fmt.Printf("  %s - %s (%s)\n", anomaly.Type, anomaly.Description, anomaly.Severity)
			}
			fmt.Println()
		}

		if len(aiResult.Recommendations) > 0 {
			fmt.Printf("💡 Recommendations:\n")
			for i, rec := range aiResult.Recommendations {
				fmt.Printf("  %d. %s\n", i+1, rec)
			}
			fmt.Println()
		}
	}

}

func aiAnalyzeImport(wg *sync.WaitGroup) {
	defer wg.Done()
	sti, eti := getTimeRange()
	sk := fmt.Sprintf("%016x:", sti)
	errorLogMap := make(map[string]*aiErrorPattern)
	setupTimeGrinder()
	i := 0
	aiStartTime = time.Now().Add(time.Hour * 24 * 365 * 100).UnixNano()
	aiEndTime = 0
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
			i++
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
				aiSampleLogList = append(aiSampleLogList,
					&aiSampleLogEntry{
						Level:     level,
						Message:   l,
						Timestamp: t,
					})
				if aiStartTime > t {
					aiStartTime = t
				}
				if aiEndTime < t {
					aiEndTime = t
				}
				aiTotalEntries++
			}
			if i%200 == 0 {
				teaProg.Send(aiMsg{Lines: i, Hit: len(aiSampleLogList), Dur: time.Since(st)})
			}
			if stopAIAnalyze {
				break
			}
		}
		if !stopAIAnalyze {
			aiErrorPatternList = []*aiErrorPattern{}
			for _, v := range errorLogMap {
				aiErrorPatternList = append(aiErrorPatternList, v)
			}
			sort.Slice(aiErrorPatternList, func(i, j int) bool {
				return aiErrorPatternList[i].Count > aiErrorPatternList[j].Count
			})
			if len(aiErrorPatternList) > 10 {
				aiErrorPatternList = aiErrorPatternList[:10]
			}
		}
		return nil
	})
	teaProg.Send(aiMsg{Done: true, Lines: i, Hit: len(aiSampleLogList), Dur: time.Since(st)})
}

func getAILogLevel(l *string) string {
	ll := strings.ToLower(*l)
	for _, e := range errCheckList {
		if strings.Contains(ll, e) {
			return "ERROR"
		}
	}
	for _, w := range warnCheckList {
		if strings.Contains(ll, w) {
			return "WARN"
		}
	}
	if strings.Contains(ll, "debug") {
		return "DEBUG"
	}
	return "INFO"
}

// UI

var stopAIAnalyze bool

type aiAnalyzeModel struct {
	spinner  spinner.Model
	quitting bool
	err      error
	msg      aiMsg
}

func initAIAnalyzeModel() aiAnalyzeModel {
	s := spinner.New()
	s.Spinner = spinner.Line
	s.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("#00efff"))
	return aiAnalyzeModel{spinner: s}
}

func (m aiAnalyzeModel) Init() tea.Cmd {
	return m.spinner.Tick
}

func (m aiAnalyzeModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "esc", "ctrl+c":
			m.quitting = true
			stopAIAnalyze = true
			return m, nil
		default:
			return m, nil
		}
	case errMsg:
		m.err = msg
		m.quitting = true
		stopAIAnalyze = true
		return m, tea.Quit
	case aiMsg:
		if msg.Done {
			m.quitting = true
			return m, tea.Quit
		}
		m.msg = msg
		return m, nil
	default:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
	}
}

func (m aiAnalyzeModel) View() string {
	if m.err != nil {
		return "\n" + errorStyle(m.err.Error()) + "\n"
	}
	str := fmt.Sprintf("\n%s Loading line=%s hit=%s time=%v",
		m.spinner.View(),
		humanize.Comma(int64(m.msg.Lines)),
		humanize.Comma(int64(m.msg.Hit)),
		time.Since(st),
	)
	if m.quitting {
		return str + "\n"
	}
	return str + "\n\n" + helpStyle("Press q to quit") + "\n"
}
