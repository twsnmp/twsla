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
	"fmt"
	"log"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/table"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/dustin/go-humanize"
	"github.com/montanaflynn/stats"
	"github.com/spf13/cobra"
	"github.com/twsnmp/twsla/pkg/anomaly"
	"github.com/twsnmp/twsla/pkg/datastore"
)

var tfidfThreshold float64
var tfidfCount int
var tfidfTop int
var tfidfNoGPU bool
var tfidfAccelBackend string

// tfidfCmd represents the tfidf command
var tfidfCmd = &cobra.Command{
	Use:   "tfidf",
	Short: "Log analysis using TF-IDF",
	Long: `Use TF-IDF to find rare logs.
You can specify a similarity threshold and the number of times the threshold is allowed to be exceeded.
`,
	Run: func(cmd *cobra.Command, args []string) {
		setupFilter(args)
		tfidfMain()
	},
}

func init() {
	rootCmd.AddCommand(tfidfCmd)
	tfidfCmd.Flags().Float64VarP(&tfidfThreshold, "limit", "l", 0.5, "Similarity threshold between logs")
	tfidfCmd.Flags().IntVarP(&tfidfCount, "count", "c", 0, "Number of threshold crossings to exclude")
	tfidfCmd.Flags().IntVarP(&tfidfTop, "top", "n", 0, "Top N")
	tfidfCmd.Flags().BoolVar(&tfidfNoGPU, "noGPU", false, "Disable GPU acceleration and use CPU/SIMD only")
	tfidfCmd.Flags().BoolVar(&tfidfNoGPU, "no-gpu", false, "Disable GPU acceleration and use CPU/SIMD only")
}

type tfidfMsg struct {
	Done   bool
	Phase  string
	Lines  int
	Hit    int
	PLines int
	Dur    time.Duration
}

func tfidfMain() {
	st = time.Now()
	if err := openDB(); err != nil {
		log.Fatalln(err)
	}
	defer closeDB()
	teaProg = tea.NewProgram(initTfidfModel())
	var wg sync.WaitGroup
	wg.Add(1)
	go tfidfSub(&wg)
	if _, err := teaProg.Run(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	wg.Wait()
}

type tfidfEnt struct {
	Log  int
	Min  float64
	Mean float64
	Max  float64
}

var tfidfList = []tfidfEnt{}

func tfidfSub(wg *sync.WaitGroup) {
	defer wg.Done()
	results = []string{}
	if tfidfTop > 0 {
		tfidfThreshold = 1.0
		tfidfCount = 1
	}
	sti, eti := getTimeRange()
	lines := 0
	hit := 0
	_ = ds.ForEach(sti, eti, func(entry *datastore.LogEntry) bool {
		l := entry.Log
		lines++
		if matchFilter(&l) {
			hit++
			results = append(results, l)
		}
		if lines%100 == 0 {
			teaProg.Send(tfidfMsg{Phase: "Search", Lines: lines, Hit: hit, Dur: time.Since(st)})
		}
		if stopSearch {
			return false
		}
		return true
	})
	// TF-IDF Feature Vector Generation
	nDocs := len(results)
	docTokens := make([][]string, nDocs)
	vocab := make(map[string]int)
	df := make(map[string]int)

	for i, d := range results {
		tokens := strings.FieldsFunc(d, func(r rune) bool {
			return !unicode.IsLetter(r) && !unicode.IsNumber(r)
		})
		docTokens[i] = tokens
		seen := make(map[string]bool)
		for _, t := range tokens {
			t = strings.ToLower(t)
			if len(t) < 2 {
				continue
			}
			if !seen[t] {
				seen[t] = true
				df[t]++
				if _, ok := vocab[t]; !ok {
					vocab[t] = len(vocab)
				}
			}
		}
		if i%100 == 0 {
			teaProg.Send(tfidfMsg{Phase: "Tokenize", PLines: i, Lines: lines, Hit: hit, Dur: time.Since(st)})
		}
		if stopSearch {
			break
		}
	}

	dim := len(vocab)
	idf := make([]float64, dim)
	for term, idx := range vocab {
		docFreq := df[term]
		idf[idx] = math.Log(1.0 + float64(nDocs)/float64(docFreq+1))
	}

	vectors := make([][]float64, nDocs)
	for i, tokens := range docTokens {
		vec := make([]float64, dim)
		if len(tokens) > 0 {
			termCounts := make(map[int]int)
			for _, t := range tokens {
				t = strings.ToLower(t)
				if idx, ok := vocab[t]; ok {
					termCounts[idx]++
				}
			}
			var sumSq float64
			for idx, cnt := range termCounts {
				tf := float64(cnt) / float64(len(tokens))
				val := tf * idf[idx]
				vec[idx] = val
				sumSq += val * val
			}
			if sumSq > 0 {
				norm := math.Sqrt(sumSq)
				for j := range vec {
					vec[j] /= norm
				}
			}
		}
		vectors[i] = vec
	}

	tfidfAccelBackend = anomaly.GetActiveBackend(tfidfNoGPU)
	teaProg.Send(tfidfMsg{Phase: fmt.Sprintf("Matrix Sim (%s)", tfidfAccelBackend), PLines: 0, Lines: lines, Hit: hit, Dur: time.Since(st)})

	// Accelerated All-Pairs Cosine Similarity Matrix (GPU / SIMD / CPU)
	simMatrix := anomaly.ComputeCosineSimilarityMatrix(vectors, tfidfNoGPU)

	for i := 0; i < nDocs; i++ {
		sims := make([]float64, 0, nDocs-1)
		done := true
		c := 0
		row := simMatrix[i]
		for j := 0; j < nDocs; j++ {
			if i == j {
				continue
			}
			s := row[j]
			sims = append(sims, s)
			if tfidfThreshold < s {
				c++
				if c > tfidfCount {
					done = false
					break
				}
			}
		}
		if done && len(sims) > 0 {
			min, _ := stats.Min(sims)
			max, _ := stats.Max(sims)
			mean, _ := stats.Mean(sims)
			tfidfList = append(tfidfList, tfidfEnt{
				Log:  i,
				Min:  min,
				Mean: mean,
				Max:  max,
			})
			if tfidfTop > 0 && tfidfTop < len(tfidfList) {
				sort.Slice(tfidfList, func(a, b int) bool {
					return tfidfList[a].Max < tfidfList[b].Max
				})
				tfidfList = tfidfList[:tfidfTop]
				tfidfThreshold = tfidfList[len(tfidfList)-1].Max
			}
		}
		if i%100 == 0 {
			teaProg.Send(tfidfMsg{Phase: "Check", PLines: i, Lines: lines, Hit: hit, Dur: time.Since(st)})
		}
		if stopSearch {
			break
		}
	}
	teaProg.Send(tfidfMsg{Done: true, Phase: "Done", PLines: hit, Lines: lines, Hit: hit, Dur: time.Since(st)})
}

type tfidfModel struct {
	spinner   spinner.Model
	table     table.Model
	done      bool
	log       string
	quitting  bool
	msg       tfidfMsg
	lastSort  string
	save      bool
	textInput textinput.Model
}

func initTfidfModel() tfidfModel {
	columns := []table.Column{
		{Title: "Log"},
		{Title: "Min"},
		{Title: "Mean"},
		{Title: "Max"},
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
	ti := textinput.New()
	ti.Placeholder = "save file name"
	ti.Focus()
	ti.CharLimit = 156
	ti.Width = 20
	return tfidfModel{spinner: s, table: t, textInput: ti}
}

func (m tfidfModel) Init() tea.Cmd {
	return m.spinner.Tick
}

var tfidfRows = []table.Row{}

func (m tfidfModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
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
		case "s":
			if m.done {
				m.save = true
			}
			return m, nil
		case "i", "e", "a":
			if m.done {
				k := msg.String()
				if k == m.lastSort {
					// Reverse
					for i, j := 0, len(tfidfRows)-1; i < j; i, j = i+1, j-1 {
						tfidfRows[i], tfidfRows[j] = tfidfRows[j], tfidfRows[i]
					}
				} else {
					// Change sort key
					m.lastSort = k
					switch k {
					case "i":
						sort.Slice(tfidfList, func(i, j int) bool {
							return tfidfList[i].Min < tfidfList[j].Min
						})
					case "e":
						sort.Slice(tfidfList, func(i, j int) bool {
							return tfidfList[i].Mean < tfidfList[j].Mean
						})
					default:
						sort.Slice(tfidfList, func(i, j int) bool {
							return tfidfList[i].Max < tfidfList[j].Max
						})
					}
					tfidfRows = []table.Row{}
					for _, r := range tfidfList {
						tfidfRows = append(tfidfRows, []string{
							results[r.Log],
							fmt.Sprintf("%.3f", r.Min),
							fmt.Sprintf("%.3f", r.Mean),
							fmt.Sprintf("%.3f", r.Max),
						})
					}
				}
				m.table.SetRows(tfidfRows)
			}
			return m, nil
		case "enter":
			if m.done {
				if m.log == "" {
					w := m.table.Width()
					if sel := m.table.SelectedRow(); sel != nil {
						s := sel[0]
						m.log = wrapString(s, w)
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
		m.table.SetWidth(msg.Width - 6)
		m.table.SetHeight(msg.Height - 6)
		w := m.table.Width() - 6
		columns := []table.Column{
			{Title: "Log", Width: 7 * w / 10},
			{Title: "Min", Width: 1 * w / 10},
			{Title: "Mean", Width: 1 * w / 10},
			{Title: "Max", Width: 1 * w / 10},
		}
		m.table.SetColumns(columns)
	case tfidfMsg:
		if msg.Done {
			w := m.table.Width() - 6
			columns := []table.Column{
				{Title: "Log", Width: 7 * w / 10},
				{Title: "Min", Width: 1 * w / 10},
				{Title: "Mean", Width: 1 * w / 10},
				{Title: "Max", Width: 1 * w / 10},
			}
			m.table.SetColumns(columns)
			tfidfRows = []table.Row{}
			for _, r := range tfidfList {
				tfidfRows = append(tfidfRows, []string{
					results[r.Log],
					fmt.Sprintf("%.3f", r.Min),
					fmt.Sprintf("%.3f", r.Mean),
					fmt.Sprintf("%.3f", r.Max),
				})
			}
			m.table.SetRows(tfidfRows)
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
	var cmd tea.Cmd
	m.table, cmd = m.table.Update(msg)
	return m, cmd
}

func (m tfidfModel) SaveUpdate(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyEnter:
			saveTfidfFile(m.textInput.Value())
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
func (m tfidfModel) View() string {
	if m.save {
		return fmt.Sprintf("Save file name?\n\n%s\n\n%s", m.textInput.View(), "(esc to quit)") + "\n"
	}
	if m.done {
		if m.log != "" {
			return m.log
		}
		return fmt.Sprintf("%s\n%s\n", m.headerView(), baseStyle.Render(m.table.View()))
	}
	str := fmt.Sprintf("\n%s %s line=%s hit=%s process=%s time=%v",
		m.msg.Phase,
		m.spinner.View(),
		humanize.Comma(int64(m.msg.Lines)),
		humanize.Comma(int64(m.msg.Hit)),
		humanize.Comma(int64(m.msg.PLines)),
		m.msg.Dur,
	)
	if m.quitting {
		return str + "\n"
	}
	return str + "\n\n" + helpStyle("Press q to quit") + "\n"
}

func (m tfidfModel) headerView() string {
	titleText := fmt.Sprintf("Results %d/%d/%d s:%s", len(tfidfList), m.msg.Hit, m.msg.Lines, m.msg.Dur.Truncate(time.Millisecond))
	if tfidfAccelBackend != "" {
		titleText = fmt.Sprintf("Results %d/%d/%d [%s] s:%s", len(tfidfList), m.msg.Hit, m.msg.Lines, tfidfAccelBackend, m.msg.Dur.Truncate(time.Millisecond))
	}
	title := titleStyle.Render(titleText)
	help := helpStyle("enter: Show / s: Save / i,e,a: Sort / q : Quit") + "  "
	gap := strings.Repeat(" ", max(0, m.table.Width()-lipgloss.Width(title)-lipgloss.Width(help)))
	return lipgloss.JoinHorizontal(lipgloss.Center, title, gap, help)
}

func saveTfidfFile(path string) {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".png":
	case ".html", ".htm":
	default:
		saveTfidfTSVFile(path)
	}
}

func saveTfidfTSVFile(path string) {
	if path == "" {
		return
	}
	// TSV
	f, err := os.Create(path)
	if err != nil {
		log.Fatalln(err)
	}
	defer f.Close()
	f.WriteString(strings.Join([]string{"Log", "Min", "Mean", "Max"}, "\t") + "\n")
	for _, r := range tfidfList {
		f.WriteString(strings.Join([]string{
			results[r.Log],
			fmt.Sprintf("%.3f", r.Min),
			fmt.Sprintf("%.3f", r.Mean),
			fmt.Sprintf("%.3f", r.Max),
		}, "\t") + "\n")
	}

}
