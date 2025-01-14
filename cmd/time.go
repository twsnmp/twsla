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
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/dustin/go-humanize"
	"github.com/montanaflynn/stats"
	"go.etcd.io/bbolt"

	"github.com/spf13/cobra"
)

// timeCmd represents the time command
var timeCmd = &cobra.Command{
	Use:   "time",
	Short: "Time analysis",
	Long:  `Time analysis`,
	Run: func(cmd *cobra.Command, args []string) {
		setupFilter(args)
		timeMain()
	},
}

func init() {
	rootCmd.AddCommand(timeCmd)

}

type timeMsg struct {
	Done  bool
	Lines int
	Hit   int
	Dur   time.Duration
}

func timeMain() {
	st = time.Now()
	if err := openDB(); err != nil {
		log.Fatalln(err)
	}
	defer db.Close()
	teaProg = tea.NewProgram(initTimeModel())
	var wg sync.WaitGroup
	wg.Add(1)
	go timeSub(&wg)
	if _, err := teaProg.Run(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	wg.Wait()
}

type timeEnt struct {
	Log   string
	Time  int64
	Mark  bool
	Diff  float64
	Delta float64
}

var timeList = []timeEnt{}

func timeSub(wg *sync.WaitGroup) {
	defer wg.Done()
	sti, eti := getTimeRange()
	sk := fmt.Sprintf("%016x:", sti)
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
				timeList = append(timeList, timeEnt{
					Log:  l,
					Time: t,
				})
			}
			if i%100 == 0 {
				teaProg.Send(timeMsg{Lines: i, Hit: len(timeList), Dur: time.Since(st)})
			}
			if stopSearch {
				break
			}
		}
		return nil
	})
	teaProg.Send(timeMsg{Done: true, Lines: i, Hit: len(timeList), Dur: time.Since(st)})
}

type timeModel struct {
	spinner     spinner.Model
	table       table.Model
	done        bool
	log         string
	lastSort    string
	durString   string
	statsString string
	quitting    bool
	msg         timeMsg
	save        bool
	textInput   textinput.Model
	sixel       string
}

var lastCursor = -1

func initTimeModel() timeModel {
	columns := []table.Column{
		{Title: "Log"},
		{Title: "Delay"},
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
	return timeModel{spinner: s, table: t, textInput: ti}
}

func (m timeModel) Init() tea.Cmd {
	return m.spinner.Tick
}

var timeRows = []table.Row{}

func (m timeModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
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
				p := filepath.Join(chartTmp, "deltaTime.html")
				SaveDeltaTimeECharts(p)
				openChart(p)
			}
		case "g":
			if m.done {
				p := filepath.Join(chartTmp, "deltaTime.png")
				SaveDeltaTimeChart(p)
				if sixelChart {
					m.sixel = openChartSixel(p)
				} else {
					openChart(p)
				}
			}
		case "s":
			if m.done {
				m.save = true
			}
			return m, nil
		case "m", "M":
			if m.done {
				c := m.table.Cursor()
				if c >= 0 && c < len(timeList) {
					updateTimeRows(c)
					m.table.SetRows(timeRows)
					lastCursor = -1
					m.durString = ""
				}
			}
			return m, nil
		case "d", "t", "l":
			if m.done {
				k := msg.String()
				if k == m.lastSort {
					// Reverse
					for i, j := 0, len(timeRows)-1; i < j; i, j = i+1, j-1 {
						timeRows[i], timeRows[j] = timeRows[j], timeRows[i]
					}
				} else {
					m.lastSort = k
					if k == "d" {
						sort.Slice(timeList, func(i, j int) bool {
							return timeList[i].Diff < timeList[j].Diff
						})
					} else if k == "t" {
						sort.Slice(timeList, func(i, j int) bool {
							return timeList[i].Time < timeList[j].Time
						})
					} else if k == "l" {
						sort.Slice(timeList, func(i, j int) bool {
							return timeList[i].Delta < timeList[j].Delta
						})
					}
					c := m.table.Cursor()
					if c < 0 {
						c = 0
					}
					updateTimeRows(c)
				}
				m.table.SetRows(timeRows)
				lastCursor = -1
				m.durString = ""
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
		w := m.table.Width() - 4
		columns := []table.Column{
			{Title: "Log", Width: 8 * w / 10},
			{Title: "Diff", Width: 1 * w / 10},
			{Title: "Delta", Width: 1 * w / 10},
		}
		m.table.SetColumns(columns)
	case timeMsg:
		if msg.Done {
			w := m.table.Width() - 4
			columns := []table.Column{
				{Title: "Log", Width: 8 * w / 10},
				{Title: "Diff", Width: 1 * w / 10},
				{Title: "Delta", Width: 1 * w / 10},
			}
			m.table.SetColumns(columns)
			m.statsString = calcStats()
			updateTimeRows(0)
			m.table.SetRows(timeRows)
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
	if c := m.table.Cursor(); c != lastCursor {
		lastCursor = c
		if c >= 0 && c < len(timeList) {
			m.durString = " Diff:" + (time.Duration(int64(timeList[c].Diff*1000)) * time.Millisecond).String()
			m.durString += " Delta:" + (time.Duration(int64(timeList[c].Delta*1000)) * time.Millisecond).String()
		}
	}
	return m, cmd
}

func updateTimeRows(c int) {
	timeRows = []table.Row{}
	for i := 0; i < len(timeList); i++ {
		timeList[i].Mark = c == i
		timeList[i].Diff = float64(timeList[i].Time-timeList[c].Time) / (1000.0 * 1000.0 * 1000.0)
		l := timeList[i].Log
		if timeList[i].Mark {
			l = markStyle.Render(l)
		}
		timeRows = append(timeRows, []string{
			l,
			fmt.Sprintf("%.3f", timeList[i].Diff),
			fmt.Sprintf("%.3f", timeList[i].Delta),
		})
	}
}

func calcStats() string {
	data := []float64{}
	for i := 1; i < len(timeList); i++ {
		timeList[i].Diff = float64(timeList[i].Time-timeList[0].Time) / (1000.0 * 1000.0 * 1000.0)
		timeList[i].Delta = timeList[i].Diff - timeList[i-1].Diff
		data = append(data, timeList[i].Delta)
	}
	if len(data) < 1 {
		return ""
	}
	mean, _ := stats.Mean(data)
	median, _ := stats.Median(data)
	mode, _ := stats.Mode(data)
	if len(mode) < 1 {
		mode = []float64{0.0}
	}
	stddiv, _ := stats.StandardDeviation(data)

	return fmt.Sprintf("  Mean:%.3f Median:%.3f Mode:%v StdDiv:%.3f", mean, median, mode, stddiv)

}

func (m timeModel) SaveUpdate(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyEnter:
			saveTimeFile(m.textInput.Value())
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
func (m timeModel) View() string {
	if m.save {
		return fmt.Sprintf("Save file name?\n\n%s\n\n%s", m.textInput.View(), "(esc to quit)") + "\n"
	}
	if m.sixel != "" {
		return "\n\n" + m.sixel
	}
	if m.done {
		if m.log != "" {
			return m.log
		}
		return fmt.Sprintf("%s\n%s %s\n%s\n", m.headerView(), m.statsString, m.durString, baseStyle.Render(m.table.View()))
	}
	str := fmt.Sprintf("\nSearch %s line=%s hit=%s time=%v",
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

func (m timeModel) headerView() string {
	title := titleStyle.Render(fmt.Sprintf("Results %d/%d s:%s", m.msg.Hit, m.msg.Lines, m.msg.Dur.Truncate(time.Millisecond)))
	help := helpStyle("enter: Show / m: Mark / s: Save / t|d|l: Sort / g|h: Chart / q : Quit") + "  "
	gap := strings.Repeat(" ", max(0, m.table.Width()-lipgloss.Width(title)-lipgloss.Width(help)))
	return lipgloss.JoinHorizontal(lipgloss.Center, title, gap, help)
}

func saveTimeFile(path string) {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".png", ".html", ".htm":
		sort.Slice(timeList, func(i, j int) bool {
			return timeList[i].Time < timeList[j].Time
		})
		if ext == ".png" {
			SaveDeltaTimeChart(path)
		} else {
			SaveDeltaTimeECharts(path)
		}
	default:
		saveTimeTSVFile(path)
	}
}

func saveTimeTSVFile(path string) {
	// TSV
	f, err := os.Create(path)
	if err != nil {
		log.Fatalln(err)
	}
	defer f.Close()
	f.WriteString(strings.Join([]string{"Log", "Diff", "Delta"}, "\t") + "\n")
	for _, r := range timeList {
		f.WriteString(strings.Join([]string{
			r.Log,
			fmt.Sprintf("%.3f", r.Diff),
			fmt.Sprintf("%.3f", r.Delta),
		}, "\t") + "\n")
	}
}
