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
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/table"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/dustin/go-humanize"
	"github.com/muesli/reflow/wordwrap"
	"github.com/spf13/cobra"
	"github.com/tmc/langchaingo/llms"
	"github.com/tmc/langchaingo/llms/anthropic"
	"github.com/tmc/langchaingo/llms/googleai"
	"github.com/tmc/langchaingo/llms/ollama"
	"github.com/tmc/langchaingo/llms/openai"
	"github.com/tmc/langchaingo/prompts"
	"go.etcd.io/bbolt"
)

var aiProvider = "ollama"
var aiBaseURL = ""
var aiModelName = ""
var aiErrorLevels = "error,fatal,fail,crit,alert"
var aiWarnLevels = "warn"
var aiLang = ""
var aiTopNError = 10
var aiSampleSize = 50

var aiTotalEntries int
var aiErrorCount int
var aiWarningCount int
var aiStartTime int64
var aiEndTime int64

type aiLogEntry struct {
	Time       int64
	Level      string
	Log        string
	AIResponce string
}

var aiLogs = []*aiLogEntry{}

type aiErrorPattern struct {
	Pattern string
	Count   int
	Example string
}

var aiErrorPatternList = []*aiErrorPattern{}

var errCheckList = []string{}
var warnCheckList = []string{}

// aiCmd represents the ai command
var aiCmd = &cobra.Command{
	Use:   "ai <filter>...",
	Short: "AI-powered log analysis",
	Long: `AI-powered log analysis
Using environment variable for API key.
 GOOGLE_API_KEY : gemini
 ANTHROPIC_API_KEY : claude
 OPENAI_API_KEY: openai
`,
	Run: func(cmd *cobra.Command, args []string) {
		setupFilter(args)
		if aiProvider == "" {
			aiProvider = findAIProvider()
		}
		if aiProvider == "ollama" {
			if aiBaseURL == "" {
				aiBaseURL = "http://localhost:11434"
			}
			if aiModelName == "" {
				aiModelName = "qwen3:latest"
			}
		}
		// Check LLM
		getLLM()
		aiMain()
	},
}

func init() {
	rootCmd.AddCommand(aiCmd)
	aiCmd.Flags().StringVar(&aiProvider, "aiProvider", "", "AI provider(ollama|gemini|openai|claude)")
	aiCmd.Flags().StringVar(&aiBaseURL, "aiBaseURL", "", "AI base URL")
	aiCmd.Flags().StringVar(&aiModelName, "aiModel", "", "LLM Model name")
	aiCmd.Flags().StringVar(&aiErrorLevels, "aiErrorLevels", "error,fatal,fail,crit,alert", "Words included in the error level log")
	aiCmd.Flags().StringVar(&aiWarnLevels, "aiWarnLevels", "warn", "Words included in the warning level log")
	aiCmd.Flags().IntVar(&aiTopNError, "aiTopNError", 10, "Number of error log patterns to be analyzed by AI")
	aiCmd.Flags().IntVar(&aiSampleSize, "aiSampleSize", 50, "Number of sample log to be analyzed by AI")
	aiCmd.Flags().StringVar(&aiLang, "aiLang", "", "Language of the response")
}

type aiImportMsg struct {
	Done  bool
	Lines int
	Hit   int
	Dur   time.Duration
}

func aiMain() {
	st = time.Now()
	if err := openDB(); err != nil {
		log.Fatalln(err)
	}
	defer db.Close()
	teaProg = tea.NewProgram(initAIModel())
	var wg sync.WaitGroup
	wg.Add(1)
	go aiSub(&wg)
	if _, err := teaProg.Run(); err != nil {
		log.Fatalln(err)
	}
	wg.Wait()
}

func aiSub(wg *sync.WaitGroup) {
	defer wg.Done()
	errorLogMap := make(map[string]*aiErrorPattern)
	aiStartTime = time.Now().Add(time.Hour * 24 * 365 * 100).UnixNano()
	aiEndTime = 0
	errCheckList = strings.Split(strings.ToLower(aiErrorLevels), ",")
	warnCheckList = strings.Split(strings.ToLower(aiWarnLevels), ",")
	sti, eti := getTimeRange()
	sk := fmt.Sprintf("%016x:", sti)
	setupTimeGrinder()
	i := 0
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
				hit++
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
				aiLogs = append(aiLogs, &aiLogEntry{
					Time:  t,
					Log:   l,
					Level: level,
				})
			}
			if i%100 == 0 {
				teaProg.Send(aiImportMsg{Lines: i, Hit: hit, Dur: time.Since(st)})
			}
			if stopSearch {
				break
			}
		}
		return nil
	})
	teaProg.Send(aiImportMsg{Done: true, Lines: i, Hit: hit, Dur: time.Since(st)})
}

type aiModel struct {
	spinner   spinner.Model
	table     table.Model
	viewport  viewport.Model
	done      bool
	log       string
	answer    string
	quitting  bool
	wait      bool
	analize   bool
	importMsg aiImportMsg
}

type aiAnswerMsg struct {
	Done bool
	Text string
}

func initAIModel() aiModel {
	columns := []table.Column{
		{Title: "Log"},
	}
	s := spinner.New()
	s.Spinner = spinner.Line
	s.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("#00efff"))
	t := table.New(
		table.WithColumns(columns),
		table.WithFocused(true),
		table.WithHeight(7),
	)

	ts := table.DefaultStyles()
	ts.Header = ts.Header.
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(lipgloss.Color("240")).
		BorderBottom(true).
		Bold(false)
	ts.Selected = ts.Selected.
		Foreground(lipgloss.Color("229")).
		Background(lipgloss.Color("57")).
		Bold(false)
	t.SetStyles(ts)
	vp := viewport.New(100, 100)
	return aiModel{spinner: s, table: t, viewport: vp}
}

func (m aiModel) Init() tea.Cmd {
	return m.spinner.Tick
}

var lastAICursor = -1

func (m aiModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var (
		cmd  tea.Cmd
		cmds []tea.Cmd
	)
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "esc", "ctrl+c":
			if !m.wait && (m.answer != "" || m.log != "") {
				m.log = ""
				m.answer = ""
				m.viewport.SetContent(m.answer)
				return m, nil
			}
			if m.done {
				return m, tea.Quit
			}
			m.quitting = true
			stopSearch = true
			return m, nil
		case "a":
			if m.done && !m.wait {
				m.wait = true
				m.analize = true
				go aiAnalyze()
				return m, m.spinner.Tick
			}
		case "e":
			if m.done && !m.wait {
				lastAICursor = m.table.Cursor()
				if lastAICursor >= 0 && lastAICursor < len(aiLogs) {
					m.wait = true
					m.analize = false
					go askToAI()
					return m, m.spinner.Tick
				}
			}
		case "enter":
			if m.done {
				if m.log == "" && m.answer == "" {
					w := m.table.Width()
					if sel := m.table.SelectedRow(); sel != nil {
						s := sel[0]
						m.log = wrapString(s, w)
					}
				} else {
					m.log = ""
					m.answer = ""
					m.viewport.SetContent(m.answer)
					return m, nil
				}
			}
		default:
			if !m.done {
				return m, nil
			}
		}
	case tea.WindowSizeMsg:
		m.table.SetWidth(msg.Width - 6)
		m.table.SetHeight(msg.Height - 5)
		m.viewport.Height = msg.Height - 4
		m.viewport.Width = msg.Width - 2
		columns := []table.Column{
			{Title: "Log", Width: m.table.Width() - 2},
		}
		m.table.SetColumns(columns)
		if m.answer != "" {
			m.viewport.SetContent(wordwrap.String(m.answer, m.viewport.Width))
		}
	case aiImportMsg:
		if msg.Done {
			rows := []table.Row{}
			for _, l := range aiLogs {
				rows = append(rows, table.Row{l.Log})
			}
			m.table.SetRows(rows)
			m.done = true
		}
		m.importMsg = msg
		return m, nil
	case aiAnswerMsg:
		if msg.Done {
			m.wait = false
			m.answer = msg.Text
		} else {
			m.answer += msg.Text
		}
		m.viewport.SetContent(wordwrap.String(m.answer, m.viewport.Width))
		m.viewport.GotoBottom()
	default:
		if !m.done || m.wait {
			var cmd tea.Cmd
			m.spinner, cmd = m.spinner.Update(msg)
			return m, cmd
		}
	}
	if m.log != "" || m.answer != "" {
		m.viewport, cmd = m.viewport.Update(msg)
		cmds = append(cmds, cmd)
	} else {
		m.table, cmd = m.table.Update(msg)
		cmds = append(cmds, cmd)
	}
	return m, tea.Batch(cmds...)
}

func (m aiModel) View() string {
	per := fmt.Sprintf(" %3.1f%%", m.viewport.ScrollPercent()*100)
	div := strings.Repeat("─", max(0, m.viewport.Width-len(per)))
	if m.wait {
		if m.analize {
			return fmt.Sprintf("%s AI is thinking... Press esc to quit.\n\n%s%s\n%s", m.spinner.View(), div, per, m.viewport.View())
		}
		return fmt.Sprintf("%s AI is thinking... Press esc to quit.\n%s\n%s%s\n%s", m.spinner.View(), aiLogs[lastAICursor].Log, div, per, m.viewport.View())
	}
	if m.answer != "" {
		if m.analize {
			return fmt.Sprintf("AI response Press enter | q | esc to back.\n\n%s%s\n%s", div, per, m.viewport.View())
		}
		return fmt.Sprintf("AI response Press enter | q | esc to back.\n%s\n%s%s\n%s", aiLogs[lastAICursor].Log, div, per, m.viewport.View())
	}
	if m.done {
		if m.log != "" {
			return m.log
		}
		return fmt.Sprintf("%s\n%s\n", m.headerView(), baseStyle.Render(m.table.View()))
	}
	str := fmt.Sprintf("\nSearch %s line=%s hit=%s time=%v",
		m.spinner.View(),
		humanize.Comma(int64(m.importMsg.Lines)),
		humanize.Comma(int64(m.importMsg.Hit)),
		m.importMsg.Dur,
	)
	if m.quitting {
		return str + "\n"
	}
	return str + "\n\n" + helpStyle("Press q to quit") + "\n"
}

func (m aiModel) headerView() string {
	title := titleStyle.Render(fmt.Sprintf("Results %d/%d s:%s", m.importMsg.Hit, m.importMsg.Lines, m.importMsg.Dur.Truncate(time.Millisecond)))
	help := helpStyle("enter: Show / a: Analyze / e: Explain  q | esc: Quit") + "  "
	gap := strings.Repeat(" ", max(0, m.table.Width()-lipgloss.Width(title)-lipgloss.Width(help)))
	return lipgloss.JoinHorizontal(lipgloss.Center, title, gap, help)
}

func askToAI() {
	le := aiLogs[lastAICursor]
	if le.AIResponce != "" {
		teaProg.Send(aiAnswerMsg{
			Text: string(le.AIResponce),
			Done: true,
		})
		return
	}
	template := prompts.NewPromptTemplate(`
You are an expert log analyst. Help me understand what this log message means and its implications.

Log Details:
- Timestamp: {{.timestamp}}
- Severity: {{.severity}}
- Message:  {{.message}}

Please provide:
1. What this log message indicates (what happened)
2. Whether this is normal/expected or indicates a problem
3. If it's a problem, what might be the root cause
4. Any recommended actions or things to investigate
5. Context about what this type of log typically means in applications

Keep your response concise but informative. Focus on practical insights that would help a developer or operator understand and respond to this log entry.
{{.add_prompt}}
`, []string{"message", "severity", "timestamp", "add_prompt"})

	addPrompt := ""
	if aiLang != "" {
		addPrompt = fmt.Sprintf("Responce in %s.", aiLang)
	}

	prompt, err := template.Format(map[string]any{
		"message":    le.Log,
		"severity":   le.Level,
		"timestamp":  time.Unix(0, le.Time).Format(time.RFC3339),
		"add_prompt": addPrompt,
	})
	if err != nil {
		log.Fatalf("formatting prompt: %v", err)
	}
	llm := getLLM()
	ctx := context.Background()
	response, err := llm.GenerateContent(ctx, []llms.MessageContent{
		llms.TextParts(llms.ChatMessageTypeHuman, prompt),
	},
		llms.WithStreamingFunc(func(ctx context.Context, chunk []byte) error {
			teaProg.Send(aiAnswerMsg{
				Text: string(chunk),
			})
			return nil
		}),
	)
	if err != nil {
		log.Fatalf("generating analysis: %v", err)
	}
	le.AIResponce = response.Choices[0].Content + "\n\n"
	teaProg.Send(aiAnswerMsg{
		Text: le.AIResponce,
		Done: true,
	})
}

var analizeAnswer = ""

func aiAnalyze() {
	if analizeAnswer != "" {
		teaProg.Send(aiAnswerMsg{Done: true, Text: analizeAnswer})
		return
	}
	sampleSize := aiSampleSize
	if len(aiLogs) < sampleSize {
		sampleSize = len(aiLogs)
	}
	sample := aiLogs[len(aiLogs)-sampleSize:]
	template := prompts.NewPromptTemplate(`
You are an expert system administrator analyzing logs. Based on the log data provided, identify:

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


Please include the following information in your response. format in markdown.
- Summary:
  - Total log count
	- Error log count
	- Warning log count
	- Period
- anomalies: 
  - type: "error_spike|performance|security|other
  - description: What was detected,
  - severity: critical|high|medium|low,
  - examples: example log entries

- recommendations: Specific actionable recommendations

{{.add_prompt}}
`, []string{"total_entries", "error_count", "warning_count", "time_range", "top_errors", "sample", "add_prompt"})

	sampleData := make([]map[string]string, len(sample))
	for i, entry := range sample {
		sampleData[i] = map[string]string{
			"timestamp": time.Unix(0, entry.Time).Format(time.RFC3339),
			"level":     entry.Level,
			"message":   entry.Log,
		}
	}

	topErrors := make([]map[string]string, len(aiErrorPatternList))
	for i, entry := range aiErrorPatternList {
		topErrors[i] = map[string]string{
			"pattern": entry.Pattern,
			"count":   fmt.Sprintf("%d", entry.Count),
		}
	}
	addPrompt := ""
	if aiLang != "" {
		addPrompt = fmt.Sprintf("Responce in %s.", aiLang)
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
	llm := getLLM()
	ctx := context.Background()
	response, err := llm.GenerateContent(ctx, []llms.MessageContent{
		llms.TextParts(llms.ChatMessageTypeHuman, prompt),
	},
		llms.WithStreamingFunc(func(ctx context.Context, chunk []byte) error {
			teaProg.Send(aiAnswerMsg{
				Text: string(chunk),
			})
			return nil
		}),
	)
	if err != nil {
		log.Fatalf("generating analysis: %v", err)
	}
	analizeAnswer = response.Choices[0].Content + "\n\n"
	teaProg.Send(aiAnswerMsg{Text: analizeAnswer, Done: true})
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

func findAIProvider() string {
	if os.Getenv("OPENAI_API_KEY") != "" {
		return "openai"
	}
	if os.Getenv("ANTHROPIC_API_KEY") != "" {
		return "anthropic"
	}
	if os.Getenv("GOOGLE_API_KEY") != "" {
		return "gemini"
	}
	return "ollama"
}

func getLLM() llms.Model {
	switch aiProvider {
	case "ollama":
		llm, err := ollama.New(
			ollama.WithModel(aiModelName),
			ollama.WithServerURL(aiBaseURL),
		)
		if err != nil {
			log.Fatalf("get llm err=%v", err)
		}
		return llm
	case "gemini", "googleai":
		if aiModelName != "" {
			llm, err := googleai.New(context.Background(), googleai.WithDefaultModel(aiModelName))
			if err != nil {
				log.Fatalf("get llm err=%v", err)
			}
			return llm
		} else {
			llm, err := googleai.New(context.Background())
			if err != nil {
				log.Fatalf("get llm err=%v", err)
			}
			return llm
		}
	case "openai":
		if aiModelName != "" {
			llm, err := openai.New(openai.WithModel(aiModelName))
			if err != nil {
				log.Fatalf("get llm err=%v", err)
			}
			return llm
		} else {
			llm, err := openai.New()
			if err != nil {
				log.Fatalf("get llm err=%v", err)
			}
			return llm
		}
	case "anthropic", "claude":
		if aiModelName != "" {
			llm, err := anthropic.New(anthropic.WithModel(aiModelName))
			if err != nil {
				log.Fatalf("get llm err=%v", err)
			}
			return llm
		} else {
			llm, err := anthropic.New()
			if err != nil {
				log.Fatalf("get llm err=%v", err)
			}
			return llm
		}
	}
	log.Fatalln("llm not found")
	return nil
}
