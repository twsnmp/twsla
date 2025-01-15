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
	"encoding/csv"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/PaesslerAG/jsonpath"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/table"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/dustin/go-humanize"
	"github.com/montanaflynn/stats"
	"github.com/spf13/cobra"
	"go.etcd.io/bbolt"
)

// extractCmd represents the extract command
var extractCmd = &cobra.Command{
	Use:   "extract",
	Short: "Extract data from log",
	Long: `Extract data from the log.
Numeric data, IP addresses, MAC addresses, email addresses
words, etc. can be extracted.
`,
	Run: func(cmd *cobra.Command, args []string) {
		setupFilter(args)
		extractMain()
	},
}

func init() {
	rootCmd.AddCommand(extractCmd)
	extractCmd.Flags().StringVarP(&extract, "extract", "e", "", "Extract pattern")
	extractCmd.Flags().IntVarP(&pos, "pos", "p", 1, "Specify variable location")
	extractCmd.Flags().StringVarP(&name, "name", "n", "Value", "Name of value")
	extractCmd.Flags().StringVarP(&grokPat, "grokPat", "x", "", "grok pattern")
	extractCmd.Flags().StringVarP(&grokDef, "grok", "g", "", "grok pattern definitions")
	extractCmd.Flags().StringVar(&geoipDBPath, "geoip", "", "geo IP database file")
	extractCmd.Flags().StringVar(&ipInfoMode, "ip", "", "IP info mode(host|domain|loc|country)")
}

func extractMain() {
	st = time.Now()
	if err := openDB(); err != nil {
		log.Fatalln(err)
	}
	defer db.Close()
	teaProg = tea.NewProgram(initExtractModel())
	var wg sync.WaitGroup
	wg.Add(1)
	go extractSub(&wg)
	if _, err := teaProg.Run(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	wg.Wait()
}

type extractEnt struct {
	Time  int64
	Value string
	Val   float64
	Delta float64
	PS    float64
}

var extractList = []extractEnt{}

func extractSub(wg *sync.WaitGroup) {
	defer wg.Done()
	mode := 0
	ipm := getIPInfoMode()
	switch extract {
	case "json":
		mode = 1
	case "grok":
		mode = 2
		setGrok()
		if gr == nil {
			log.Fatalln("no grok")
		}
	default:
		setExtPat()
		if extPat == nil {
			log.Fatalln("no extract pattern")
		}
	}
	sti, eti := getTimeRange()
	sk := fmt.Sprintf("%016x:", sti)
	i := 0
	hit := 0
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
				switch mode {
				case 1:
					// JSON
					var data map[string]interface{}
					if ji := strings.IndexByte(string(v), '{'); ji >= 0 {
						if err := json.Unmarshal(v[ji:], &data); err == nil {
							if val, err := jsonpath.Get(name, data); err == nil && val != nil {
								if ipm > 0 {
									ip := fmt.Sprintf("%v", val)
									extractList = append(extractList, extractEnt{Time: t, Value: fmt.Sprintf("%s(%s)", ip, getIPInfo(ip, ipm))})
								} else {
									extractList = append(extractList, extractEnt{Time: t, Value: fmt.Sprintf("%v", val)})
								}
								hit++
							}
						}
					}
				case 2:
					// GROK
					if data, err := gr.ParseString(l); err == nil {
						if val, ok := data[name]; ok {
							if ipm > 0 {
								val = fmt.Sprintf("%s(%s)", val, getIPInfo(val, ipm))
							}
							extractList = append(extractList, extractEnt{Time: t, Value: val})
							hit++
						}
					}
				default:
					// TWSLA
					a := extPat.ExtReg.FindAllStringSubmatch(l, -1)
					if len(a) >= extPat.Index && len(a[extPat.Index-1]) > 1 {
						if ipm > 0 {
							ip := a[extPat.Index-1][1]
							extractList = append(extractList, extractEnt{Time: t, Value: fmt.Sprintf("%s(%s)", ip, getIPInfo(ip, ipm))})
						} else {
							extractList = append(extractList, extractEnt{Time: t, Value: a[extPat.Index-1][1]})
						}
						hit++
					}
				}
			}
			if i%100 == 0 {
				teaProg.Send(SearchMsg{Lines: i, Hit: hit, Dur: time.Since(st)})
			}
			if stopSearch {
				break
			}
		}
		return nil
	})
	for i := 0; i < len(extractList); i++ {
		if v, err := strconv.ParseFloat(extractList[i].Value, 64); err == nil {
			extractList[i].Val = v
			mean += v
		}
		if i > 0 {
			extractList[i].Delta = extractList[i].Val - extractList[i-1].Val
			dt := extractList[i].Time - extractList[i-1].Time
			if dt > 0 {
				extractList[i].PS = (extractList[i].Delta * 1000 * 1000 * 1000) / float64(dt)
			}
		}
	}
	if len(extractList) > 0 {
		mean /= float64(len(extractList))
	}
	teaProg.Send(SearchMsg{Done: true, Lines: i, Hit: hit, Dur: time.Since(st)})
}

type extractModel struct {
	spinner   spinner.Model
	table     table.Model
	done      bool
	quitting  bool
	msg       SearchMsg
	lastSort  string
	save      bool
	textInput textinput.Model
	sixel     string
	stats     bool
	statTable table.Model
}

var statsList []table.Row

func initExtractModel() extractModel {
	columns := []table.Column{
		{Title: "Time"},
		{Title: name},
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
	statColumns := []table.Column{
		{Title: "Stats"},
		{Title: name},
		{Title: "Delta"},
		{Title: "PS"},
	}
	st := table.New(
		table.WithColumns(statColumns),
		table.WithFocused(true),
		table.WithHeight(7),
	)
	st.SetStyles(ts)
	return extractModel{spinner: s, table: t, textInput: ti, statTable: st}
}

func (m extractModel) Init() tea.Cmd {
	return m.spinner.Tick
}

var extractRows = []table.Row{}

func (m extractModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
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
	if m.stats {
		switch msg := msg.(type) {
		case tea.KeyMsg:
			k := msg.String()
			if k == "esc" || k == "q" {
				m.stats = false
				return m, func() tea.Msg {
					return tea.ClearScreen()
				}
			} else if k == "s" {
				m.save = true
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
		case "s":
			if m.done {
				m.save = true
			}
			return m, nil
		case "i":
			if m.done {
				m.stats = true
			}
			return m, nil
		case "h":
			if m.done {
				p := filepath.Join(chartTmp, "extractTime.html")
				SaveExtractECharts(p)
				openChart(p)
			}
		case "g":
			if m.done {
				p := filepath.Join(chartTmp, "extractTime.png")
				SaveExtractChart(p)
				if sixelChart {
					m.sixel = openChartSixel(p)
				} else {
					openChart(p)
				}
			}
		case "t", "v", "d", "p":
			if m.done {
				k := msg.String()
				if k == m.lastSort {
					// Reverse
					for i, j := 0, len(extractRows)-1; i < j; i, j = i+1, j-1 {
						extractRows[i], extractRows[j] = extractRows[j], extractRows[i]
					}
				} else {
					// Change sort key
					m.lastSort = k
					if k == "t" {
						sort.Slice(extractList, func(i, j int) bool {
							return extractList[i].Time < extractList[j].Time
						})
					} else if k == "d" {
						sort.Slice(extractList, func(i, j int) bool {
							return extractList[i].Delta < extractList[j].Delta
						})
					} else if k == "p" {
						sort.Slice(extractList, func(i, j int) bool {
							return extractList[i].PS < extractList[j].PS
						})
					} else {
						sort.Slice(extractList, func(i, j int) bool {
							if v1, err := strconv.ParseFloat(extractList[i].Value, 64); err == nil {
								if v2, err := strconv.ParseFloat(extractList[j].Value, 64); err == nil {
									return v1 < v2
								}
							}
							return extractList[i].Value < extractList[j].Value
						})
					}
					extractRows = []table.Row{}
					for _, r := range extractList {
						extractRows = append(extractRows, []string{
							time.Unix(0, r.Time).Format("2006/01/02T15:04:05.999"),
							r.Value,
							humanize.FormatFloat("#,###.###", r.Delta),
							humanize.FormatFloat("#,###.###", r.PS),
						})
					}
				}
				m.table.SetRows(extractRows)
			}
			return m, nil
		default:
			if !m.done {
				return m, nil
			}
		}
	case tea.WindowSizeMsg:
		m.table.SetWidth(msg.Width - 6)
		m.table.SetHeight(msg.Height - 6)
		w := m.table.Width() - 8
		columns := []table.Column{
			{Title: "Time", Width: 3 * w / 10},
			{Title: name, Width: 5 * w / 10},
			{Title: "Delta", Width: 1 * w / 10},
			{Title: "PS", Width: 1 * w / 10},
		}
		m.table.SetColumns(columns)
		m.statTable.SetWidth(msg.Width - 6)
		m.statTable.SetHeight(msg.Height - 6)
		statColumns := []table.Column{
			{Title: "Stats", Width: 4 * w / 10},
			{Title: name, Width: 2 * w / 10},
			{Title: "Delta", Width: 2 * w / 10},
			{Title: "PS", Width: 2 * w / 10},
		}
		m.statTable.SetColumns(statColumns)
	case SearchMsg:
		if msg.Done {
			w := m.table.Width() - 8
			columns := []table.Column{
				{Title: "Time", Width: 3 * w / 10},
				{Title: name, Width: 5 * w / 10},
				{Title: "Delta", Width: 1 * w / 10},
				{Title: "PS", Width: 1 * w / 10},
			}
			m.table.SetColumns(columns)
			extractRows = []table.Row{}
			var vals []float64
			var deltas []float64
			var pss []float64
			for _, r := range extractList {
				extractRows = append(extractRows, []string{
					time.Unix(0, r.Time).Format("2006/01/02T15:04:05.999"),
					r.Value,
					humanize.FormatFloat("#,###.###", r.Delta),
					humanize.FormatFloat("#,###.###", r.PS),
				})
				vals = append(vals, r.Val)
				deltas = append(deltas, r.Delta)
				pss = append(pss, r.PS)
			}
			m.table.SetRows(extractRows)
			vMin, _ := stats.Min(vals)
			dMin, _ := stats.Min(deltas)
			psMin, _ := stats.Min(pss)

			vMax, _ := stats.Max(vals)
			dMax, _ := stats.Max(deltas)
			psMax, _ := stats.Max(pss)

			vMean, _ := stats.Mean(vals)
			dMean, _ := stats.Mean(deltas)
			psMean, _ := stats.Mean(pss)

			vMedian, _ := stats.Median(vals)
			dMedian, _ := stats.Median(deltas)
			psMedian, _ := stats.Median(pss)

			vMode, _ := stats.Mode(vals)
			dMode, _ := stats.Mode(deltas)
			psMode, _ := stats.Mode(pss)

			vVariance, _ := stats.Variance(vals)
			dVariance, _ := stats.Variance(deltas)
			psVariance, _ := stats.Variance(pss)

			statsList = []table.Row{
				[]string{
					"Min",
					humanize.FormatFloat("#,###.###", vMin),
					humanize.FormatFloat("#,###.###", dMin),
					humanize.FormatFloat("#,###.###", psMin),
				},
				[]string{
					"Max",
					humanize.FormatFloat("#,###.###", vMax),
					humanize.FormatFloat("#,###.###", dMax),
					humanize.FormatFloat("#,###.###", psMax),
				},
				[]string{
					"Mean",
					humanize.FormatFloat("#,###.###", vMean),
					humanize.FormatFloat("#,###.###", dMean),
					humanize.FormatFloat("#,###.###", psMean),
				},
				[]string{
					"Median",
					humanize.FormatFloat("#,###.###", vMedian),
					humanize.FormatFloat("#,###.###", dMedian),
					humanize.FormatFloat("#,###.###", psMedian),
				},
				[]string{
					"Mode",
					humanize.FormatFloat("#,###.###", vMode[0]),
					humanize.FormatFloat("#,###.###", dMode[0]),
					humanize.FormatFloat("#,###.###", psMode[0]),
				},
				[]string{
					"Variance",
					humanize.FormatFloat("#,###.###", vVariance),
					humanize.FormatFloat("#,###.###", dVariance),
					humanize.FormatFloat("#,###.###", psVariance),
				},
			}
			m.statTable.SetRows(statsList)
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

func (m extractModel) SaveUpdate(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyEnter:
			if m.stats {
				saveExtractStatsCSVFile(m.textInput.Value())
			} else {
				saveExtractFile(m.textInput.Value())
			}
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
func (m extractModel) View() string {
	if m.save {
		return fmt.Sprintf("Save file name?\n\n%s\n\n%s", m.textInput.View(), "(esc to quit)") + "\n"
	}
	if m.sixel != "" {
		return "\n\n" + m.sixel + "\n(esc to quit)"
	}
	if m.stats {
		return baseStyle.Render(m.statTable.View()) + "\n(esc to quit / s to save)"
	}
	if m.done {
		return fmt.Sprintf("%s\n%s\n", m.headerView(), baseStyle.Render(m.table.View()))
	}
	str := fmt.Sprintf("\n%s Searching line=%s hit=%s time=%v",
		m.spinner.View(),
		humanize.Comma(int64(m.msg.Lines)),
		humanize.Comma(int64(m.msg.Hit)),
		m.msg.Dur,
	)
	if m.quitting {
		return str + "\n"
	}
	return str + "\n\n" + helpStyle("Press q to quit") + "\n"
}

func (m extractModel) headerView() string {
	title := titleStyle.Render(fmt.Sprintf("Results %d/%d/%d s:%s m:%s",
		len(extractList), m.msg.Hit, m.msg.Lines, m.msg.Dur.Truncate(time.Millisecond), humanize.FormatFloat("#,###.###", mean)))
	help := helpStyle("s: Save / t,v,d,p: Sort / g|h: Chart i:Stats / q : Quit") + "  "
	gap := strings.Repeat(" ", max(0, m.table.Width()-lipgloss.Width(title)-lipgloss.Width(help)))
	return lipgloss.JoinHorizontal(lipgloss.Center, title, gap, help)
}

func saveExtractFile(path string) {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".png":
		SaveExtractChart(path)
	case ".html", "htm":
		SaveExtractECharts(path)
	default:
		saveExtractCSVFile(path)
	}
}

func saveExtractCSVFile(path string) {
	if path == "" {
		return
	}
	// CSV
	f, err := os.Create(path)
	if err != nil {
		log.Fatalln(err)
	}
	defer f.Close()
	w := csv.NewWriter(f)
	w.Write([]string{"Time", name, "Delta", "PS"})
	for _, r := range extractList {
		wr := []string{
			time.Unix(0, r.Time).Format("2006/01/02T15:04:05.999"),
			r.Value,
			fmt.Sprintf("%.3f", r.Delta),
			fmt.Sprintf("%.3f", r.PS),
		}
		w.Write(wr)
	}
	w.Flush()

}

func saveExtractStatsCSVFile(path string) {
	if path == "" {
		return
	}
	// CSV
	f, err := os.Create(path)
	if err != nil {
		log.Fatalln(err)
	}
	defer f.Close()
	w := csv.NewWriter(f)
	w.Write([]string{"Stats", name, "Delta", "PS"})
	for _, s := range statsList {
		w.Write(s)
	}
	w.Flush()

}
