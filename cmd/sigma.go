/*
Copyright © 2024 Masayuki Yamai <twsnmp@gmail.com>

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
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/table"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/bradleyjkemp/sigma-go"
	"github.com/bradleyjkemp/sigma-go/evaluator"
	"github.com/dustin/go-humanize"

	"github.com/spf13/cobra"
	"go.etcd.io/bbolt"
)

var sigmaRules string
var sigmaConfig string

//go:embed sigma
var sigmaConfigs embed.FS

var evaluators []*evaluator.RuleEvaluator
var strict = false
var skipRuleCount = 0

// sigmaCmd represents the sigma command
var sigmaCmd = &cobra.Command{
	Use:   "sigma",
	Short: "Detect threats using SIGMA rules",
	Long: `Detect threats using SIGMA rules.
	About SIGAMA
	https://sigmahq.io/
	`,
	Run: func(cmd *cobra.Command, args []string) {
		setupFilter(args)
		sigmaMain()
	},
}

func init() {
	rootCmd.AddCommand(sigmaCmd)
	sigmaCmd.Flags().StringVarP(&sigmaRules, "rules", "s", "", "Sigma rules path")
	sigmaCmd.Flags().BoolVar(&strict, "strict", false, "Strict rule check")
	sigmaCmd.Flags().StringVarP(&sigmaConfig, "config", "c", "", "config path")
	sigmaCmd.Flags().StringVarP(&grokPat, "grokPat", "x", "", "grok pattern if empty json mode")
	sigmaCmd.Flags().StringVarP(&grokDef, "grok", "g", "", "grok definitions")
}

type sigmaMsg struct {
	Done  bool
	Lines int
	Hit   int
	Match int
	Dur   time.Duration
}

func sigmaMain() {
	st = time.Now()
	if err := openDB(); err != nil {
		log.Fatalln(err)
	}
	defer db.Close()
	teaProg = tea.NewProgram(initSigmaModel())
	var wg sync.WaitGroup
	wg.Add(1)
	go sigmaSub(&wg)
	if _, err := teaProg.Run(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	wg.Wait()
}

type sigmaEnt struct {
	Log       int
	Evaluator *evaluator.RuleEvaluator
}

func getSigmaConfig() *sigma.Config {
	if sigmaConfig == "" {
		return nil
	}
	c, err := sigmaConfigs.ReadFile("sigma/" + sigmaConfig + ".yml")
	if err != nil {
		c, err = os.ReadFile(sigmaConfig)
		if err != nil {
			log.Fatalln("sigma config not found")
		}
	}
	ret, err := sigma.ParseConfig(c)
	if err != nil {
		log.Fatalf("sigma config parrse err=%v", err)
	}
	return &ret
}

func loadSigmaRules() {
	config := getSigmaConfig()
	filepath.WalkDir(sigmaRules, func(path string, info fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		ext := strings.ToLower(filepath.Ext(path))
		if ext != ".yaml" && ext != ".yml" {
			return nil
		}
		c, err := os.ReadFile(path)
		if err != nil {
			if !strict {
				log.Printf("invalid rule %s %s", path, err)
				skipRuleCount++
				return nil
			}
			log.Fatalf("invalid rule %s %s", path, err)
			return err
		}
		rule, err := sigma.ParseRule(c)
		if err != nil {
			if !strict {
				log.Printf("invalid rule %s %s", path, err)
				skipRuleCount++
				return nil
			}
			log.Fatalf("invalid rule %s %s", path, err)
			return err
		}
		if rule.ID == "" {
			rule.ID = path
		}
		if config != nil {
			evaluators = append(evaluators, evaluator.ForRule(rule, evaluator.WithConfig(*config), evaluator.CaseSensitive))
		} else {
			evaluators = append(evaluators, evaluator.ForRule(rule, evaluator.CaseSensitive))
		}
		return nil
	})
	if len(evaluators) < 1 {
		log.Fatalln("no sigma rule")
	}
}

func matchSigmaRule(l *string) *evaluator.RuleEvaluator {
	var data interface{}
	if gr != nil {
		var err error
		data, err = gr.ParseString(*l)
		if err != nil {
			return nil
		}
	} else {
		if err := json.Unmarshal([]byte(*l), &data); err != nil {
			return nil
		}
	}

	for _, ev := range evaluators {
		r, err := ev.Matches(context.Background(), data)
		if err != nil {
			return nil
		}
		if r.Match {
			return ev
		}
	}
	return nil
}

var sigmaList = []sigmaEnt{}

func sigmaSub(wg *sync.WaitGroup) {
	defer wg.Done()
	loadSigmaRules()
	setGrok()
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
			lines++
			if matchFilter(&l) {
				hit++
				if ev := matchSigmaRule(&l); ev != nil {
					results = append(results, l)
					times = append(times, t)
					sigmaList = append(sigmaList, sigmaEnt{
						Log:       len(sigmaList),
						Evaluator: ev,
					})
				}
			}
			if lines%100 == 0 {
				teaProg.Send(sigmaMsg{Lines: lines, Hit: hit, Match: len(sigmaList), Dur: time.Since(st)})
			}
			if stopSearch {
				break
			}
		}
		return nil
	})
	teaProg.Send(sigmaMsg{Done: true, Lines: lines, Hit: hit, Match: len(sigmaList), Dur: time.Since(st)})
}

type sigmaModel struct {
	spinner    spinner.Model
	table      table.Model
	countTable table.Model
	done       bool
	log        string
	quitting   bool
	msg        sigmaMsg
	save       bool
	textInput  textinput.Model
	viewport   viewport.Model
	showCount  bool
}

func initSigmaModel() sigmaModel {
	columns := []table.Column{
		{Title: "Level"},
		{Title: "Time"},
		{Title: "Rule"},
		{Title: "Log"},
	}
	countColumns := []table.Column{
		{Title: "Level"},
		{Title: "Rule"},
		{Title: "Count"},
	}
	s := spinner.New()
	s.Spinner = spinner.Line
	s.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("#00efff"))
	t := table.New(
		table.WithColumns(columns),
		table.WithFocused(true),
		table.WithHeight(7),
	)
	ct := table.New(
		table.WithColumns(countColumns),
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
	ct.SetStyles(ts)
	ti := textinput.New()
	ti.Placeholder = "save file name"
	ti.Focus()
	ti.CharLimit = 156
	ti.Width = 20
	vp := viewport.New(100, 100)
	return sigmaModel{spinner: s, table: t, textInput: ti, viewport: vp, countTable: ct}
}

func (m sigmaModel) Init() tea.Cmd {
	return m.spinner.Tick
}

var sigmaRows = []table.Row{}
var sigmaCountRows = []table.Row{}

type sigmaCountEnt struct {
	ID    string
	Level string
	Title string
	Tag   string
	Count int
}

var sigmaCountList = []*sigmaCountEnt{}

func (m sigmaModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if m.save {
		return m.SaveUpdate(msg)
	}
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "esc", "ctrl+c":
			if m.done {
				return m, tea.Quit
			}
			m.quitting = true
			stopSearch = true
			return m, nil
		case "h":
			if m.done {
				if m.showCount {
					p := filepath.Join(chartTmp, "sigmaCount.html")
					SaveCountECharts(p)
					openChart(p)
				} else {
					p := filepath.Join(chartTmp, "sigmaHeatmap.html")
					saveHeatmapECharts(p)
					openChart(p)
				}
			}
		case "g":
			if m.done {
				if m.showCount {
					p := filepath.Join(chartTmp, "sigmaCount.png")
					SaveCountChart(p)
					openChart(p)
				}
			}
		case "s":
			if m.done {
				m.save = true
			}
			return m, nil
		case "c":
			if m.done {
				m.showCount = !m.showCount
			}
		case "r":
			if m.done {
				// Reverse
				if m.showCount {
					for i, j := 0, len(sigmaCountRows)-1; i < j; i, j = i+1, j-1 {
						sigmaCountRows[i], sigmaCountRows[j] = sigmaCountRows[j], sigmaCountRows[i]
					}
					m.countTable.SetRows(sigmaCountRows)
				} else {
					for i, j := 0, len(sigmaRows)-1; i < j; i, j = i+1, j-1 {
						sigmaRows[i], sigmaRows[j] = sigmaRows[j], sigmaRows[i]
					}
					m.table.SetRows(sigmaRows)
				}
			}
			return m, nil
		case "enter":
			if m.done && !m.showCount {
				if m.log == "" {
					sel := m.table.SelectedRow()
					if len(sel) > 4 {
						m.log = sel[4]
						l := getColoredLevel(sel[0])
						s := fmt.Sprintf("Level:%s\nTime:%s\nRule:%s\nTag:%s\nLog:\n%s", l, sel[1], sel[2], sel[3], prettyJSON(m.log))
						m.viewport.SetContent(s)
					}
				} else {
					m.log = ""
				}
			}
		default:
			if !m.done {
				return m, nil
			}
		}
	case tea.WindowSizeMsg:
		m.viewport.Width = msg.Width
		m.viewport.Height = msg.Height
		m.table.SetWidth(msg.Width - 6)
		m.table.SetHeight(msg.Height - 6)
		w := m.table.Width() - 5
		columns := []table.Column{
			{Title: "Level", Width: 1 * w / 10},
			{Title: "Time", Width: 1 * w / 10},
			{Title: "Rule", Width: 5 * w / 10},
			{Title: "Tag", Width: 3 * w / 10},
			{Title: "Log", Width: -1},
		}
		m.table.SetColumns(columns)
		w += 1
		countColumns := []table.Column{
			{Title: "Level", Width: 1 * w / 10},
			{Title: "Rule", Width: 5 * w / 10},
			{Title: "Tag", Width: 3 * w / 10},
			{Title: "Count", Width: 1 * w / 10},
		}
		m.countTable.SetColumns(countColumns)
	case sigmaMsg:
		if msg.Done {
			w := m.table.Width() - 5
			columns := []table.Column{
				{Title: "Level", Width: 1 * w / 10},
				{Title: "Time", Width: 1 * w / 10},
				{Title: "Rule", Width: 5 * w / 10},
				{Title: "Tag", Width: 3 * w / 10},
				{Title: "Log", Width: -1},
			}
			m.table.SetColumns(columns)
			sigmaRows = []table.Row{}
			countMap := make(map[string]*sigmaCountEnt)
			heatmapMap := make(map[string]*heapmapEnt)
			dateMap := make(map[string]bool)
			for _, r := range sigmaList {
				sigmaRows = append(sigmaRows, []string{
					r.Evaluator.Level,
					time.Unix(0, times[r.Log]).Format("01/02 15:04"),
					r.Evaluator.Title,
					fmt.Sprintf("%v", r.Evaluator.Rule.Tags),
					results[r.Log],
				})
				if p, ok := countMap[r.Evaluator.ID]; ok {
					p.Count++
				} else {
					countMap[r.Evaluator.ID] = &sigmaCountEnt{
						ID:    r.Evaluator.ID,
						Level: r.Evaluator.Level,
						Title: r.Evaluator.Title,
						Tag:   fmt.Sprintf("%v", r.Evaluator.Rule.Tags),
						Count: 1,
					}
				}
				ih := times[r.Log] / (3600 * 1000 * 1000 * 1000)
				th := time.Unix(3600*ih, 0)
				x := 0
				k := th.Format("2006/01/02")
				if _, ok := dateMap[k]; !ok {
					dateMap[k] = true
					dateList = append(dateList, k)
				}
				x = len(dateList) - 1
				h := th.Hour()
				key := fmt.Sprintf("%s:%d", k, h)
				if e, ok := heatmapMap[key]; ok {
					e.Count++
				} else {
					heatmapMap[key] = &heapmapEnt{
						Key:   k,
						TimeH: h,
						X:     x,
						Y:     h,
						Count: 1,
					}
				}

			}
			m.table.SetRows(sigmaRows)
			sigmaCountList = []*sigmaCountEnt{}
			for _, e := range countMap {
				sigmaCountList = append(sigmaCountList, e)
			}
			sort.Slice(sigmaCountList, func(i, j int) bool {
				return sigmaCountList[i].Count > sigmaCountList[j].Count
			})
			w += 1
			countColumns := []table.Column{
				{Title: "Level", Width: 1 * w / 10},
				{Title: "Rule", Width: 5 * w / 10},
				{Title: "Tag", Width: 3 * w / 10},
				{Title: "Count", Width: 1 * w / 10},
			}
			m.countTable.SetColumns(countColumns)
			sigmaCountRows = []table.Row{}
			for _, r := range sigmaCountList {
				countList = append(countList, countEnt{
					Key:   fmt.Sprintf("%s:%s", r.Level, r.Title),
					Count: r.Count,
				})
				sigmaCountRows = append(sigmaCountRows, []string{
					r.Level,
					r.Title,
					r.Tag,
					fmt.Sprintf("%d", r.Count),
				})
			}
			m.countTable.SetRows(sigmaCountRows)
			for _, v := range heatmapMap {
				heatmapList = append(heatmapList, v)
			}
			m.done = true
		}
		m.msg = msg
		return m, nil
	default:
		if !m.done {
			var cmd tea.Cmd
			m.spinner, cmd = m.spinner.Update(msg)
			return m, cmd
		}
	}
	if m.log != "" {
		var cmd tea.Cmd
		m.viewport, cmd = m.viewport.Update(msg)
		return m, cmd
	}
	if m.showCount {
		var cmd tea.Cmd
		m.countTable, cmd = m.countTable.Update(msg)
		return m, cmd
	}
	var cmd tea.Cmd
	m.table, cmd = m.table.Update(msg)
	return m, cmd
}

func (m sigmaModel) SaveUpdate(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyEnter:
			saveSigmaFile(m.textInput.Value(), m.showCount)
			m.save = false
			return m, nil
		case tea.KeyCtrlC, tea.KeyEsc:
			m.save = false
			return m, nil
		}
	}
	m.textInput, cmd = m.textInput.Update(msg)
	return m, cmd
}

func (m sigmaModel) View() string {
	if m.save {
		return fmt.Sprintf("Save file name?\n\n%s\n\n%s", m.textInput.View(), "(esc to quit)") + "\n"
	}
	if m.done {
		if m.log != "" {
			return m.viewport.View()
		}
		if m.showCount {
			return fmt.Sprintf("%s\n%s\n", m.headerView(), baseStyle.Render(m.countTable.View()))
		}
		return fmt.Sprintf("%s\n%s\n", m.headerView(), baseStyle.Render(m.table.View()))
	}
	str := fmt.Sprintf("\n%s line=%s hit=%s match=%s time=%v",
		m.spinner.View(),
		humanize.Comma(int64(m.msg.Lines)),
		humanize.Comma(int64(m.msg.Hit)),
		humanize.Comma(int64(m.msg.Match)),
		m.msg.Dur,
	)
	if m.quitting {
		return str + "\n"
	}
	return str + "\n\n" + helpStyle("Press q to quit") + "\n"
}

func (m sigmaModel) headerView() string {
	title := titleStyle.Render(fmt.Sprintf("Results %d/%d/%d s:%s r:%d i:%d",
		len(sigmaList), m.msg.Hit, m.msg.Lines, m.msg.Dur.Truncate(time.Millisecond), len(evaluators), skipRuleCount))
	help := helpStyle("enter: Show / s: Save / r: Sort / c: Count / h: Chart / q : Quit") + "  "
	if m.showCount {
		help = helpStyle("s: Save / r: Sort / c: Exit count / g|h: Chart / q : Quit") + "  "
	}
	gap := strings.Repeat(" ", max(0, m.table.Width()-lipgloss.Width(title)-lipgloss.Width(help)))
	return lipgloss.JoinHorizontal(lipgloss.Center, title, gap, help)
}

func saveSigmaFile(path string, count bool) {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".png":
		if count {
			SaveCountChart(path)
		}
	case ".html", ".htm":
		if count {
			SaveCountECharts(path)
		} else {
			saveHeatmapECharts(path)
		}
	default:
		saveSigmaTSVFile(path, count)
	}
}

func saveSigmaTSVFile(path string, count bool) {
	// TSV
	f, err := os.Create(path)
	if err != nil {
		log.Fatalln(err)
	}
	defer f.Close()
	if count {
		f.WriteString(strings.Join([]string{"Level", "Rule", "Tags", "Count", "ID"}, "\t") + "\n")
		for _, r := range sigmaCountList {
			f.WriteString(strings.Join([]string{
				r.Level,
				r.Title,
				r.Tag,
				fmt.Sprintf("%d", r.Count),
				r.ID,
			}, "\t") + "\n")
		}
		return

	}
	f.WriteString(strings.Join([]string{"Level", "Time", "Rule", "Tags", "ID", "Log"}, "\t") + "\n")
	for _, r := range sigmaList {
		f.WriteString(strings.Join([]string{
			r.Evaluator.Level,
			time.Unix(0, times[r.Log]).Format("01/02 15:04"),
			r.Evaluator.Title,
			fmt.Sprintf("%v", r.Evaluator.Rule.Tags),
			r.Evaluator.ID,
			results[r.Log],
		}, "\t") + "\n")
	}
}

func getColoredLevel(l string) string {
	switch l {
	case "informational", "info":
	case "low":
		return lipgloss.NewStyle().Foreground(lipgloss.Color("3")).Render(l)
	case "medium":
		return lipgloss.NewStyle().Foreground(lipgloss.Color("11")).Render(l)
	case "high":
		return lipgloss.NewStyle().Foreground(lipgloss.Color("1")).Render(l)
	case "critical":
		return lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Render(l)
	}
	return l
}
