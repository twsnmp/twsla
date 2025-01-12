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
	"github.com/gravwell/gravwell/v3/timegrinder"
	"github.com/spf13/cobra"
	"go.etcd.io/bbolt"
)

// delayCmd represents the delay command
var delayCmd = &cobra.Command{
	Use:   "delay",
	Short: "Search for delays in the access log",
	Long:  `Search for delays in the access log`,
	Run: func(cmd *cobra.Command, args []string) {
		setupFilter(args)
		delayMain()
	},
}

var posDelay = 0

func init() {
	rootCmd.AddCommand(delayCmd)
	delayCmd.Flags().IntVarP(&posDelay, "timePos", "q", 0, "Specify second time stamp position")
	delayCmd.Flags().BoolVar(&utc, "utc", false, "Force UTC")
}

type delayMsg struct {
	Done  bool
	Lines int
	Hit   int
	Dur   time.Duration
}

func delayMain() {
	st = time.Now()
	if err := openDB(); err != nil {
		log.Fatalln(err)
	}
	defer db.Close()
	teaProg = tea.NewProgram(initDelayModel())
	var wg sync.WaitGroup
	wg.Add(1)
	go delaySub(&wg)
	if _, err := teaProg.Run(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	wg.Wait()
}

type delayEnt struct {
	Log   int
	Time  int64
	Delay float64
}

var delayList = []delayEnt{}

func delaySub(wg *sync.WaitGroup) {
	defer wg.Done()
	results = []string{}
	sti, eti := getTimeRange()
	sk := fmt.Sprintf("%016x:", sti)
	var err error
	if posDelay > 0 {
		tg, err = getTimeGrinder()
		if err != nil || tg == nil {
			log.Fatalln(err)
		}
	}
	db.View(func(tx *bbolt.Tx) error {
		b := tx.Bucket([]byte("logs"))
		bd := tx.Bucket([]byte("delta"))
		c := bd.Cursor()
		if posDelay > 0 {
			c = b.Cursor()
		}
		for k, v := c.Seek([]byte(sk)); k != nil; k, v = c.Next() {
			a := strings.Split(string(k), ":")
			if len(a) < 1 {
				continue
			}
			t, err := strconv.ParseInt(a[0], 16, 64)
			if err == nil && t > eti {
				break
			}
			var d float64
			if posDelay < 1 {
				d, err = strconv.ParseFloat(string(v), 64)
				if err != nil {
					continue
				}
				v = b.Get(k)
				if v == nil {
					continue
				}
			} else {
				t2 := getTimestamp(v)
				if t2 == 0 {
					continue
				}
				d = float64(t2 - t)
			}
			l := string(v)
			lines++
			if matchFilter(&l) {
				results = append(results, l)
				delayList = append(delayList, delayEnt{
					Log:   hit,
					Time:  t,
					Delay: -d / (1000 * 1000 * 1000),
				})
				hit++
			}
			if lines%100 == 0 {
				teaProg.Send(delayMsg{Lines: lines, Hit: hit, Dur: time.Since(st)})
			}
			if stopSearch {
				break
			}
		}
		return nil
	})
	sort.Slice(delayList, func(a, b int) bool {
		return delayList[a].Delay > delayList[b].Delay
	})

	teaProg.Send(delayMsg{Done: true, Lines: lines, Hit: hit, Dur: time.Since(st)})
}

type delayModel struct {
	spinner   spinner.Model
	table     table.Model
	done      bool
	log       string
	lastSort  string
	quitting  bool
	msg       delayMsg
	save      bool
	textInput textinput.Model
	sixel     string
}

func initDelayModel() delayModel {
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
	return delayModel{spinner: s, table: t, textInput: ti}
}

func (m delayModel) Init() tea.Cmd {
	return m.spinner.Tick
}

var delayRows = []table.Row{}

func (m delayModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if m.save {
		return m.SaveUpdate(msg)
	}
	if m.sixel != "" {
		switch msg := msg.(type) {
		case tea.KeyMsg:
			k := msg.String()
			if k == "esc" || k == "q" {
				m.sixel = ""
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
				p := filepath.Join(chartTmp, "delayTime.html")
				SaveDelayTimeECharts(p)
				openChart(p)
			}
		case "g":
			if m.done {
				p := filepath.Join(chartTmp, "delayTime.png")
				SaveDelayTimeChart(p)
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
		case "d", "t":
			if m.done {
				k := msg.String()
				if k == m.lastSort {
					// Reverse
					for i, j := 0, len(delayRows)-1; i < j; i, j = i+1, j-1 {
						delayRows[i], delayRows[j] = delayRows[j], delayRows[i]
					}
				} else {
					m.lastSort = k
					if k == "d" {
						sort.Slice(delayList, func(i, j int) bool {
							return delayList[i].Delay < delayList[j].Delay
						})
					} else if k == "t" {
						sort.Slice(delayList, func(i, j int) bool {
							return delayList[i].Time < delayList[j].Time
						})
					}
					delayRows = []table.Row{}
					for _, r := range delayList {
						delayRows = append(delayRows, []string{
							results[r.Log],
							fmt.Sprintf("%.3f", r.Delay),
						})
					}

				}
				m.table.SetRows(delayRows)
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
			{Title: "Log", Width: 9 * w / 10},
			{Title: "Delay", Width: 1 * w / 10},
		}
		m.table.SetColumns(columns)
	case delayMsg:
		if msg.Done {
			w := m.table.Width() - 4
			columns := []table.Column{
				{Title: "Log", Width: 9 * w / 10},
				{Title: "Delay", Width: 1 * w / 10},
			}
			m.table.SetColumns(columns)
			delayRows = []table.Row{}
			for _, r := range delayList {
				delayRows = append(delayRows, []string{
					results[r.Log],
					fmt.Sprintf("%.3f", r.Delay),
				})
			}
			m.table.SetRows(delayRows)
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

func (m delayModel) SaveUpdate(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyEnter:
			saveDelayFile(m.textInput.Value())
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
func (m delayModel) View() string {
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
		return fmt.Sprintf("%s\n%s\n", m.headerView(), baseStyle.Render(m.table.View()))
	}
	str := fmt.Sprintf("\nSearch %s pos=%d line=%s hit=%s time=%v",
		m.spinner.View(),
		posDelay,
		humanize.Comma(int64(m.msg.Lines)),
		humanize.Comma(int64(m.msg.Hit)),
		m.msg.Dur,
	)
	if m.quitting {
		return str + "\n"
	}
	return str + "\n\n" + helpStyle("Press q to quit") + "\n"
}

func (m delayModel) headerView() string {
	title := titleStyle.Render(fmt.Sprintf("Results %d/%d s:%s", m.msg.Hit, m.msg.Lines, m.msg.Dur.Truncate(time.Millisecond)))
	help := helpStyle("enter: Show / s: Save / t|d: Sort / g|h: Chart / q : Quit") + "  "
	gap := strings.Repeat(" ", max(0, m.table.Width()-lipgloss.Width(title)-lipgloss.Width(help)))
	return lipgloss.JoinHorizontal(lipgloss.Center, title, gap, help)
}

func saveDelayFile(path string) {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".png", ".html", ".htm":
		sort.Slice(delayList, func(i, j int) bool {
			return delayList[i].Time < delayList[j].Time
		})
		if ext == ".png" {
			SaveDelayTimeChart(path)
		} else {
			SaveDelayTimeECharts(path)
		}
	default:
		saveDelayTSVFile(path)
	}
}

func saveDelayTSVFile(path string) {
	// TSV
	f, err := os.Create(path)
	if err != nil {
		log.Fatalln(err)
	}
	defer f.Close()
	f.WriteString(strings.Join([]string{"Log", "Delay"}, "\t") + "\n")
	for _, r := range delayList {
		f.WriteString(strings.Join([]string{
			results[r.Log],
			fmt.Sprintf("%.3f", r.Delay),
		}, "\t") + "\n")
	}
}

func getTimeGrinder() (*timegrinder.TimeGrinder, error) {
	var err error
	tg, err := timegrinder.New(timegrinder.Config{
		EnableLeftMostSeed: false,
	})
	if err != nil {
		return tg, err
	}
	if !utc {
		tg.SetLocalTime()
	}
	// [Sun Oct 09 00:36:03 2022]
	if p, err := timegrinder.NewUserProcessor("custom01", `[JFMASOND][anebriyunlgpctov]+\s+\d+\s+\d\d:\d\d:\d\d\s+\d\d\d\d`, "Jan _2 15:04:05 2006"); err == nil && p != nil {
		if _, err := tg.AddProcessor(p); err != nil {
			return tg, err
		}
	} else {
		return tg, err
	}
	// 2022/12/26 5:48:00
	if p, err := timegrinder.NewUserProcessor("custom02", `\d\d\d\d/\d+/\d+\s+\d+:\d\d:\d\d`, "2006/1/2 3:04:05"); err == nil && p != nil {
		if _, err := tg.AddProcessor(p); err != nil {
			return tg, err
		}
	} else {
		return tg, err
	}
	return tg, nil
}

func getTimestamp(v []byte) int64 {
	for i := 0; i <= posDelay; i++ {
		_, e, ok := tg.Match(v)
		if !ok {
			return 0
		}
		if i == posDelay {
			if t, ok, _ := tg.Extract(v); ok {
				return t.UnixNano()
			}
			return 0
		}
		if len(v) <= e {
			return 0
		}
		v = v[e:]
	}
	return 0
}
