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
	"embed"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/table"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/bradleyjkemp/sigma-go/evaluator"
	"github.com/dustin/go-humanize"
	"github.com/spf13/cobra"
	"github.com/twsnmp/twsla/pkg/datastore"
	sigmaPkg "github.com/twsnmp/twsla/pkg/sigma"
)

var (
	sigmaRules     string
	sigmaConfig    string
	sigmaPacksList []string
	filterPack     string
	filterCustom   bool
	namedCaptures  string
	sigmaEngine    *sigmaPkg.Engine
)

//go:embed sigma
var sigmaConfigs embed.FS

var (
	evaluators    []*evaluator.RuleEvaluator
	strict        = false
	skipRuleCount = 0
)

const (
	viewModeEvents    = 0
	viewModeRuleCount = 1
	viewModeTagCount  = 2
)

// sigmaCmd represents the sigma command
var sigmaCmd = &cobra.Command{
	Use:   "sigma",
	Short: "Detect threats using SIGMA rules",
	Long: `Detect threats using SIGMA rules.
Actions:
  (default)                Run interactive TUI log threat detector
  packs                    List available embedded rule packs
  list                     List active rules with metadata
  stat                     Show rule, source, and tag statistics
  logsrc                   List log sources
  field                    List log fields
  test <log1> [log2...]    Test log strings against active rules
  convert-wazuh            Convert Wazuh XML rules to Sigma YAML
  convert-wazuh-decoder    Convert Wazuh XML decoders to regex patterns
`,
	Run: func(cmd *cobra.Command, args []string) {
		if len(args) > 0 {
			switch args[0] {
			case "packs":
				showSigmaPacks()
				return
			case "list":
				sigmaRuleList()
				return
			case "stat":
				sigmaStat()
				return
			case "logsrc":
				sigmaLogSrc()
				return
			case "field":
				sigmaField()
				return
			case "test":
				sigmaTest(args)
				return
			}
		}
		setupFilter(args)
		sigmaMain()
	},
}

func init() {
	rootCmd.AddCommand(sigmaCmd)
	sigmaCmd.Flags().StringVarP(&sigmaRules, "rules", "s", "", "Sigma rules path (file or dir)")
	sigmaCmd.Flags().StringSliceVarP(&sigmaPacksList, "sigmaPacks", "p", nil, "Sigma rule packs (e.g. windows-essential,linux-auth or all)")
	sigmaCmd.Flags().StringVar(&filterPack, "pack", "", "Filter rules by pack name (for list)")
	sigmaCmd.Flags().BoolVar(&filterCustom, "custom", false, "Filter custom (file) rules only (for list)")
	sigmaCmd.Flags().StringVar(&namedCaptures, "namedCaptures", "", "Path to custom named captures regex file")
	sigmaCmd.Flags().BoolVar(&strict, "strict", false, "Strict rule check")
	sigmaCmd.Flags().StringVarP(&sigmaConfig, "sigmaConfig", "c", "", "config path")
	sigmaCmd.Flags().StringVarP(&grokPat, "grokPat", "x", "", "grok pattern if empty json mode")
	sigmaCmd.Flags().StringVarP(&grokDef, "grok", "g", "", "grok definitions")
}

func initSigmaEngine() (*sigmaPkg.Engine, error) {
	var grokPats []string
	if grokPat != "" {
		grokPats = []string{grokPat}
	}
	cfg := sigmaPkg.Config{
		RulesPath:     sigmaRules,
		Packs:         sigmaPacksList,
		ConfigDir:     sigmaConfig,
		NamedCaptures: namedCaptures,
		GrokPat:       grokPats,
		GrokDef:       grokDef,
		Strict:        strict,
	}
	return sigmaPkg.NewEngine(cfg)
}

func showSigmaPacks() {
	packs := sigmaPkg.GetAvailableSigmaPacks()
	fmt.Printf("Available Rule Packs (%d):\n", len(packs))
	for _, p := range packs {
		count := 0
		if info, err := sigmaPkg.GetSigmaPackInfo(p, false); err == nil {
			count = info.RuleCount
		}
		desc := sigmaPkg.GetSigmaPackDescription(p)
		fmt.Printf("  - %-20s (rules: %2d) : %s\n", p, count, desc)
	}
}

func sigmaRuleList() {
	eng, err := initSigmaEngine()
	if err != nil {
		log.Fatalf("failed to initialize sigma engine: %v", err)
	}
	for _, entry := range eng.RuleEntries {
		if filterPack != "" {
			expected := "pack:" + filterPack
			if entry.Source != expected {
				continue
			}
		}
		if filterCustom {
			if strings.HasPrefix(entry.Source, "pack:") {
				continue
			}
		}
		e := entry.Evaluator
		lsk := fmt.Sprintf("%s:%s:%s", e.Logsource.Product, e.Logsource.Category, e.Logsource.Service)
		tagsStr := strings.Join(e.Rule.Tags, ",")
		fmt.Printf("%-24s\t%s\t%-8s\t%-20s\t%-30s\t%s\n", entry.Source, e.ID, e.Level, lsk, tagsStr, e.Title)
	}
}

func sigmaStat() {
	eng, err := initSigmaEngine()
	if err != nil {
		log.Fatalf("failed to initialize sigma engine: %v", err)
	}
	logSrcMap := make(map[string]int)
	fieldMap := make(map[string]int)
	sourceMap := make(map[string]int)
	tagMap := make(map[string]int)
	for _, entry := range eng.RuleEntries {
		e := entry.Evaluator
		sourceMap[entry.Source]++
		lsk := fmt.Sprintf("%s:%s:%s", e.Logsource.Product, e.Logsource.Category, e.Logsource.Service)
		logSrcMap[lsk]++
		for _, tag := range e.Rule.Tags {
			tagMap[tag]++
		}
		for _, dsv := range e.Rule.Detection.Searches {
			for _, ems := range dsv.EventMatchers {
				for _, em := range ems {
					fk := lsk + ":" + em.Field
					fieldMap[fk]++
				}
			}
		}
	}
	fmt.Printf("Total Rules: %d\n", len(eng.RuleEntries))
	fmt.Printf("Log Sources: %d, Fields: %d, Unique Tags: %d\n\n", len(logSrcMap), len(fieldMap), len(tagMap))
	fmt.Println("Sources:")
	for s, c := range sourceMap {
		fmt.Printf("  %-24s : %d\n", s, c)
	}
	fmt.Println("\nTop Tags (MITRE ATT&CK / Compliance):")
	type kv struct {
		k string
		v int
	}
	var tagList []kv
	for k, v := range tagMap {
		tagList = append(tagList, kv{k, v})
	}
	sort.Slice(tagList, func(i, j int) bool {
		return tagList[i].v > tagList[j].v
	})
	limit := 25
	if len(tagList) < limit {
		limit = len(tagList)
	}
	for i := 0; i < limit; i++ {
		fmt.Printf("  %-35s : %d\n", tagList[i].k, tagList[i].v)
	}
}

func sigmaLogSrc() {
	eng, err := initSigmaEngine()
	if err != nil {
		log.Fatalf("failed to initialize sigma engine: %v", err)
	}
	logSrcMap := make(map[string]int)
	for _, entry := range eng.RuleEntries {
		e := entry.Evaluator
		lsk := fmt.Sprintf("%s_%s_%s", e.Logsource.Product, e.Logsource.Category, e.Logsource.Service)
		logSrcMap[lsk]++
	}
	for k, c := range logSrcMap {
		fmt.Printf("%s=%d\n", k, c)
	}
}

func sigmaField() {
	eng, err := initSigmaEngine()
	if err != nil {
		log.Fatalf("failed to initialize sigma engine: %v", err)
	}
	fieldMap := make(map[string]int)
	for _, entry := range eng.RuleEntries {
		e := entry.Evaluator
		lsk := fmt.Sprintf("%s_%s_%s", e.Logsource.Product, e.Logsource.Category, e.Logsource.Service)
		for _, dsv := range e.Rule.Detection.Searches {
			for _, ems := range dsv.EventMatchers {
				for _, em := range ems {
					fk := lsk + ":" + em.Field
					fieldMap[fk]++
				}
			}
		}
	}
	for k, c := range fieldMap {
		fmt.Printf("%s=%d\n", k, c)
	}
}

func sigmaTest(args []string) {
	if len(args) < 2 {
		log.Fatalln("test data missing. Usage: twsla sigma test <log1> [log2...]")
	}
	eng, err := initSigmaEngine()
	if err != nil {
		log.Fatalf("failed to initialize sigma engine: %v", err)
	}
	hit := false
	now := time.Now().UnixNano()
	for _, l := range args[1:] {
		if entry := eng.Match(l, now); entry != nil {
			e := entry.Evaluator
			lsk := fmt.Sprintf("%s:%s:%s", e.Logsource.Product, e.Logsource.Category, e.Logsource.Service)
			tags := strings.Join(e.Rule.Tags, ", ")
			fmt.Printf("===\nHIT: %s\nRule ID : %s\nLevel   : %s\nSource  : %s\nLogSrc  : %s\nTags    : %s\nTitle   : %s\n",
				l, e.ID, e.Level, entry.Source, lsk, tags, e.Title)
			hit = true
		}
	}
	if !hit {
		fmt.Println("===\nno rule match")
	}
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
	defer closeDB()
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
	Entry     *sigmaPkg.SigmaRuleEntry
}

var sigmaList = []sigmaEnt{}

func sigmaSub(wg *sync.WaitGroup) {
	defer wg.Done()
	var err error
	sigmaEngine, err = initSigmaEngine()
	if err != nil {
		log.Fatalln(err)
	}
	evaluators = sigmaEngine.Evaluators
	if len(sigmaEngine.RuleEntries) < 1 {
		log.Fatalln("no sigma rule loaded")
	}
	results = []string{}
	times = []int64{}
	sigmaList = []sigmaEnt{}
	sti, eti := getTimeRange()
	_ = ds.ForEach(sti, eti, func(entry *datastore.LogEntry) bool {
		t := entry.Time
		l := entry.Log
		lines++
		if matchFilter(&l) {
			hit++
			if matched := sigmaEngine.Match(l, t); matched != nil {
				results = append(results, l)
				times = append(times, t)
				sigmaList = append(sigmaList, sigmaEnt{
					Log:       len(sigmaList),
					Evaluator: matched.Evaluator,
					Entry:     matched,
				})
			}
		}
		if lines%100 == 0 {
			teaProg.Send(sigmaMsg{Lines: lines, Hit: hit, Match: len(sigmaList), Dur: time.Since(st)})
		}
		if stopSearch {
			return false
		}
		return true
	})
	teaProg.Send(sigmaMsg{Done: true, Lines: lines, Hit: hit, Match: len(sigmaList), Dur: time.Since(st)})
}

type sigmaModel struct {
	spinner    spinner.Model
	table      table.Model
	countTable table.Model
	tagTable   table.Model
	done       bool
	log        string
	quitting   bool
	msg        sigmaMsg
	save       bool
	textInput  textinput.Model
	viewport   viewport.Model
	viewMode   int
	sixel      string
	lastSort   string
}

func initSigmaModel() sigmaModel {
	columns := []table.Column{
		{Title: "Level"},
		{Title: "Time"},
		{Title: "Rule"},
		{Title: "Tag"},
		{Title: "Log"},
	}
	countColumns := []table.Column{
		{Title: "Level"},
		{Title: "Rule"},
		{Title: "Tag"},
		{Title: "Count"},
	}
	tagColumns := []table.Column{
		{Title: "Tag"},
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
	tt := table.New(
		table.WithColumns(tagColumns),
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
	tt.SetStyles(ts)
	ti := textinput.New()
	ti.Placeholder = "save file name"
	ti.Focus()
	ti.CharLimit = 156
	ti.Width = 20
	vp := viewport.New(100, 100)
	vp.SetHorizontalStep(1)
	return sigmaModel{
		spinner:    s,
		table:      t,
		countTable: ct,
		tagTable:   tt,
		textInput:  ti,
		viewport:   vp,
		viewMode:   viewModeEvents,
	}
}

func (m sigmaModel) Init() tea.Cmd {
	return m.spinner.Tick
}

var sigmaRows = []table.Row{}
var sigmaCountRows = []table.Row{}
var sigmaTagCountRows = []table.Row{}

type sigmaCountEnt struct {
	ID    string
	Level string
	Title string
	Tag   string
	Count int
}

type sigmaTagCountEnt struct {
	Tag   string
	Count int
}

var sigmaCountList = []*sigmaCountEnt{}
var sigmaTagCountList = []*sigmaTagCountEnt{}

func getLevelScore(l string) int {
	switch strings.ToLower(l) {
	case "critical":
		return 5
	case "high":
		return 4
	case "medium":
		return 3
	case "low":
		return 2
	case "informational", "info":
		return 1
	default:
		return 0
	}
}

func updateSigmaRows() {
	sigmaRows = []table.Row{}
	for _, r := range sigmaList {
		tagsStr := strings.Join(r.Evaluator.Rule.Tags, ", ")
		sigmaRows = append(sigmaRows, []string{
			r.Evaluator.Level,
			time.Unix(0, times[r.Log]).Format("01/02 15:04"),
			r.Evaluator.Title,
			tagsStr,
			results[r.Log],
		})
	}
}

func updateSigmaCountRows() {
	sigmaCountRows = []table.Row{}
	for _, r := range sigmaCountList {
		sigmaCountRows = append(sigmaCountRows, []string{
			r.Level,
			r.Title,
			r.Tag,
			fmt.Sprintf("%d", r.Count),
		})
	}
}

func updateSigmaTagCountRows() {
	sigmaTagCountRows = []table.Row{}
	for _, t := range sigmaTagCountList {
		sigmaTagCountRows = append(sigmaTagCountRows, []string{
			t.Tag,
			fmt.Sprintf("%d", t.Count),
		})
	}
}

func (m sigmaModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if m.save {
		return m.SaveUpdate(msg)
	}
	if m.sixel != "" {
		switch msg := msg.(type) {
		case tea.KeyMsg:
			k := msg.String()
			if k == "esc" || k == "q" {
				m.sixel = ""
				return m, func() tea.Msg {
					return tea.ClearScreen()
				}
			}
		}
		return m, nil
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
				switch m.viewMode {
				case viewModeTagCount:
					p := filepath.Join(chartTmp, "sigmaTagCount.html")
					m.prepareTagChartList()
					SaveCountECharts(p)
					openChart(p)
				case viewModeRuleCount:
					p := filepath.Join(chartTmp, "sigmaCount.html")
					m.prepareRuleChartList()
					SaveCountECharts(p)
					openChart(p)
				default:
					p := filepath.Join(chartTmp, "sigmaHeatmap.html")
					saveHeatmapECharts(p)
					openChart(p)
				}
			}
		case "g":
			if m.done {
				switch m.viewMode {
				case viewModeTagCount:
					p := filepath.Join(chartTmp, "sigmaTagCount.png")
					m.prepareTagChartList()
					SaveCountChart(p)
					if sixelChart {
						m.sixel = openChartSixel(p)
					} else {
						openChart(p)
					}
				case viewModeRuleCount:
					p := filepath.Join(chartTmp, "sigmaCount.png")
					m.prepareRuleChartList()
					SaveCountChart(p)
					if sixelChart {
						m.sixel = openChartSixel(p)
					} else {
						openChart(p)
					}
				}
			}
		case "s":
			if m.done {
				m.save = true
			}
			return m, nil
		case "m":
			if m.done {
				m.viewMode = (m.viewMode + 1) % 3
				m.lastSort = ""
			}
			return m, nil
		case "l", "t", "r", "c":
			if m.done {
				k := msg.String()
				switch m.viewMode {
				case viewModeEvents:
					if k == "l" || k == "t" {
						if k == m.lastSort {
							for i, j := 0, len(sigmaRows)-1; i < j; i, j = i+1, j-1 {
								sigmaRows[i], sigmaRows[j] = sigmaRows[j], sigmaRows[i]
							}
							for i, j := 0, len(sigmaList)-1; i < j; i, j = i+1, j-1 {
								sigmaList[i], sigmaList[j] = sigmaList[j], sigmaList[i]
							}
						} else {
							m.lastSort = k
							if k == "l" {
								sort.Slice(sigmaList, func(i, j int) bool {
									si := getLevelScore(sigmaList[i].Evaluator.Level)
									sj := getLevelScore(sigmaList[j].Evaluator.Level)
									if si != sj {
										return si > sj
									}
									return times[sigmaList[i].Log] > times[sigmaList[j].Log]
								})
							} else {
								sort.Slice(sigmaList, func(i, j int) bool {
									return times[sigmaList[i].Log] < times[sigmaList[j].Log]
								})
							}
							updateSigmaRows()
						}
						m.table.SetRows(sigmaRows)
					}
				case viewModeRuleCount:
					if k == "l" || k == "r" || k == "c" {
						if k == m.lastSort {
							for i, j := 0, len(sigmaCountRows)-1; i < j; i, j = i+1, j-1 {
								sigmaCountRows[i], sigmaCountRows[j] = sigmaCountRows[j], sigmaCountRows[i]
							}
							for i, j := 0, len(sigmaCountList)-1; i < j; i, j = i+1, j-1 {
								sigmaCountList[i], sigmaCountList[j] = sigmaCountList[j], sigmaCountList[i]
							}
						} else {
							m.lastSort = k
							switch k {
							case "l":
								sort.Slice(sigmaCountList, func(i, j int) bool {
									si := getLevelScore(sigmaCountList[i].Level)
									sj := getLevelScore(sigmaCountList[j].Level)
									if si != sj {
										return si > sj
									}
									return sigmaCountList[i].Count > sigmaCountList[j].Count
								})
							case "r":
								sort.Slice(sigmaCountList, func(i, j int) bool {
									return sigmaCountList[i].Title < sigmaCountList[j].Title
								})
							case "c":
								sort.Slice(sigmaCountList, func(i, j int) bool {
									return sigmaCountList[i].Count > sigmaCountList[j].Count
								})
							}
							updateSigmaCountRows()
						}
						m.countTable.SetRows(sigmaCountRows)
					}
				case viewModeTagCount:
					if k == "t" || k == "c" {
						if k == m.lastSort {
							for i, j := 0, len(sigmaTagCountRows)-1; i < j; i, j = i+1, j-1 {
								sigmaTagCountRows[i], sigmaTagCountRows[j] = sigmaTagCountRows[j], sigmaTagCountRows[i]
							}
							for i, j := 0, len(sigmaTagCountList)-1; i < j; i, j = i+1, j-1 {
								sigmaTagCountList[i], sigmaTagCountList[j] = sigmaTagCountList[j], sigmaTagCountList[i]
							}
						} else {
							m.lastSort = k
							switch k {
							case "t":
								sort.Slice(sigmaTagCountList, func(i, j int) bool {
									return sigmaTagCountList[i].Tag < sigmaTagCountList[j].Tag
								})
							case "c":
								sort.Slice(sigmaTagCountList, func(i, j int) bool {
									return sigmaTagCountList[i].Count > sigmaTagCountList[j].Count
								})
							}
							updateSigmaTagCountRows()
						}
						m.tagTable.SetRows(sigmaTagCountRows)
					}
				}
			}
			return m, nil
		case "enter":
			if m.done && m.viewMode == viewModeEvents {
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
		m.countTable.SetWidth(msg.Width - 6)
		m.countTable.SetHeight(msg.Height - 6)
		m.tagTable.SetWidth(msg.Width - 6)
		m.tagTable.SetHeight(msg.Height - 6)

		w := m.table.Width() - 5
		columns := []table.Column{
			{Title: "Level", Width: 1 * w / 10},
			{Title: "Time", Width: 1 * w / 10},
			{Title: "Rule", Width: 4 * w / 10},
			{Title: "Tag", Width: 4 * w / 10},
			{Title: "Log", Width: -1},
		}
		m.table.SetColumns(columns)
		countColumns := []table.Column{
			{Title: "Level", Width: 1 * w / 10},
			{Title: "Rule", Width: 5 * w / 10},
			{Title: "Tag", Width: 3 * w / 10},
			{Title: "Count", Width: 1 * w / 10},
		}
		m.countTable.SetColumns(countColumns)
		tagColumns := []table.Column{
			{Title: "Tag", Width: 7 * w / 10},
			{Title: "Count", Width: 2 * w / 10},
		}
		m.tagTable.SetColumns(tagColumns)
	case sigmaMsg:
		if msg.Done {
			w := m.table.Width() - 5
			columns := []table.Column{
				{Title: "Level", Width: 1 * w / 10},
				{Title: "Time", Width: 1 * w / 10},
				{Title: "Rule", Width: 4 * w / 10},
				{Title: "Tag", Width: 4 * w / 10},
				{Title: "Log", Width: -1},
			}
			m.table.SetColumns(columns)
			sigmaRows = []table.Row{}
			countMap := make(map[string]*sigmaCountEnt)
			tagMap := make(map[string]int)
			heatmapMap := make(map[string]*heapmapEnt)
			dateMap := make(map[string]bool)
			for _, r := range sigmaList {
				tagsStr := strings.Join(r.Evaluator.Rule.Tags, ", ")
				sigmaRows = append(sigmaRows, []string{
					r.Evaluator.Level,
					time.Unix(0, times[r.Log]).Format("01/02 15:04"),
					r.Evaluator.Title,
					tagsStr,
					results[r.Log],
				})
				if p, ok := countMap[r.Evaluator.ID]; ok {
					p.Count++
				} else {
					countMap[r.Evaluator.ID] = &sigmaCountEnt{
						ID:    r.Evaluator.ID,
						Level: r.Evaluator.Level,
						Title: r.Evaluator.Title,
						Tag:   tagsStr,
						Count: 1,
					}
				}
				for _, tag := range r.Evaluator.Rule.Tags {
					tag = strings.TrimSpace(tag)
					if tag != "" {
						tagMap[tag]++
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

			// Populate Rule Count
			sigmaCountList = []*sigmaCountEnt{}
			for _, e := range countMap {
				sigmaCountList = append(sigmaCountList, e)
			}
			sort.Slice(sigmaCountList, func(i, j int) bool {
				return sigmaCountList[i].Count > sigmaCountList[j].Count
			})
			updateSigmaCountRows()
			m.countTable.SetRows(sigmaCountRows)

			// Populate Tag Count
			sigmaTagCountList = []*sigmaTagCountEnt{}
			for t, c := range tagMap {
				sigmaTagCountList = append(sigmaTagCountList, &sigmaTagCountEnt{
					Tag:   t,
					Count: c,
				})
			}
			sort.Slice(sigmaTagCountList, func(i, j int) bool {
				return sigmaTagCountList[i].Count > sigmaTagCountList[j].Count
			})
			updateSigmaTagCountRows()
			m.tagTable.SetRows(sigmaTagCountRows)

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
	switch m.viewMode {
	case viewModeTagCount:
		var cmd tea.Cmd
		m.tagTable, cmd = m.tagTable.Update(msg)
		return m, cmd
	case viewModeRuleCount:
		var cmd tea.Cmd
		m.countTable, cmd = m.countTable.Update(msg)
		return m, cmd
	default:
		var cmd tea.Cmd
		m.table, cmd = m.table.Update(msg)
		return m, cmd
	}
}

func (m *sigmaModel) prepareRuleChartList() {
	countList = []countEnt{}
	for _, r := range sigmaCountList {
		countList = append(countList, countEnt{
			Key:   fmt.Sprintf("%s:%s", r.Level, r.Title),
			Count: r.Count,
		})
	}
}

func (m *sigmaModel) prepareTagChartList() {
	countList = []countEnt{}
	for _, t := range sigmaTagCountList {
		countList = append(countList, countEnt{
			Key:   t.Tag,
			Count: t.Count,
		})
	}
}

func (m sigmaModel) SaveUpdate(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyEnter:
			saveSigmaFile(m.textInput.Value(), m.viewMode)
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
	if m.sixel != "" {
		return "\n\n" + m.sixel + "\n(esc to quit)"
	}
	if m.done {
		if m.log != "" {
			return m.viewport.View()
		}
		switch m.viewMode {
		case viewModeTagCount:
			return fmt.Sprintf("%s\n%s\n", m.headerView(), baseStyle.Render(m.tagTable.View()))
		case viewModeRuleCount:
			return fmt.Sprintf("%s\n%s\n", m.headerView(), baseStyle.Render(m.countTable.View()))
		default:
			return fmt.Sprintf("%s\n%s\n", m.headerView(), baseStyle.Render(m.table.View()))
		}
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
	ruleCount := 0
	if sigmaEngine != nil {
		ruleCount = len(sigmaEngine.RuleEntries)
	}
	title := titleStyle.Render(fmt.Sprintf("Results %d/%d/%d s:%s r:%d",
		len(sigmaList), m.msg.Hit, m.msg.Lines, m.msg.Dur.Truncate(time.Millisecond), ruleCount))
	var helpText string
	switch m.viewMode {
	case viewModeTagCount:
		helpText = "s: Save / t|c: Sort / m: Mode / g|h: Chart / q: Quit"
	case viewModeRuleCount:
		helpText = "s: Save / l|r|c: Sort / m: Mode / g|h: Chart / q: Quit"
	default:
		helpText = "enter: Show / s: Save / l|t: Sort / m: Mode / g|h: Chart / q: Quit"
	}
	help := helpStyle(helpText) + "  "
	gap := strings.Repeat(" ", max(0, m.table.Width()-lipgloss.Width(title)-lipgloss.Width(help)))
	return lipgloss.JoinHorizontal(lipgloss.Center, title, gap, help)
}

func saveSigmaFile(path string, mode int) {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".png":
		if mode != viewModeEvents {
			SaveCountChart(path)
		}
	case ".html", ".htm":
		if mode != viewModeEvents {
			SaveCountECharts(path)
		} else {
			saveHeatmapECharts(path)
		}
	default:
		saveSigmaTSVFile(path, mode)
	}
}

func saveSigmaTSVFile(path string, mode int) {
	if path == "" {
		return
	}
	f, err := os.Create(path)
	if err != nil {
		log.Fatalln(err)
	}
	defer f.Close()
	switch mode {
	case viewModeTagCount:
		f.WriteString(strings.Join([]string{"Tag", "Count"}, "\t") + "\n")
		for _, t := range sigmaTagCountList {
			f.WriteString(fmt.Sprintf("%s\t%d\n", t.Tag, t.Count))
		}
	case viewModeRuleCount:
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
	default:
		f.WriteString(strings.Join([]string{"Level", "Time", "Rule", "Tags", "ID", "Log"}, "\t") + "\n")
		for _, r := range sigmaList {
			f.WriteString(strings.Join([]string{
				r.Evaluator.Level,
				time.Unix(0, times[r.Log]).Format("01/02 15:04"),
				r.Evaluator.Title,
				strings.Join(r.Evaluator.Rule.Tags, ", "),
				r.Evaluator.ID,
				results[r.Log],
			}, "\t") + "\n")
		}
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
