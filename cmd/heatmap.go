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
	"github.com/spf13/cobra"
	"go.etcd.io/bbolt"
)

// heatmapCmd represents the heatmap command
var heatmapCmd = &cobra.Command{
	Use:   "heatmap",
	Short: "Command to tally log counts by day of the week and time of day",
	Long: `Command to tally log counts by day of the week and time of day
	Aggregate by date mode is also available.`,
	Run: func(cmd *cobra.Command, args []string) {
		setupFilter(args)
		heatmapMain()
	},
}

var week = false

func init() {
	rootCmd.AddCommand(heatmapCmd)
	heatmapCmd.Flags().BoolVarP(&week, "week", "w", false, "Week mode")
}

func heatmapMain() {
	st = time.Now()
	if err := openDB(); err != nil {
		log.Fatalln(err)
	}
	defer db.Close()
	teaProg = tea.NewProgram(initHeatmapModel())
	var wg sync.WaitGroup
	wg.Add(1)
	go heatmapSub(&wg)
	if _, err := teaProg.Run(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	wg.Wait()
}

type heapmapEnt struct {
	Key   string // Day of week or date
	TimeH int
	X     int
	Y     int
	Count int
}

var heatmapList = []*heapmapEnt{}
var dateList = []string{}

func heatmapSub(wg *sync.WaitGroup) {
	var heatmapMap = make(map[string]*heapmapEnt)
	var dateMap = make(map[string]bool)
	defer wg.Done()
	sti, eti := getTimeRange()
	sk := fmt.Sprintf("%016x:", sti)
	if timeMode && nameCount == "Key" {
		nameCount = "Time"
	}
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
				ih := t / (3600 * 1000 * 1000 * 1000)
				th := time.Unix(3600*ih, 0)
				k := ""
				x := 0
				if week {
					w := th.Weekday()
					k = w.String()
					x = int(w)
				} else {
					k = th.Format("2006/01/02")
					if _, ok := dateMap[k]; !ok {
						dateMap[k] = true
						dateList = append(dateList, k)
					}
					x = len(dateList) - 1
				}
				hit++
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
			if i%100 == 0 {
				teaProg.Send(SearchMsg{Lines: i, Hit: hit, Dur: time.Since(st)})
			}
			if stopSearch {
				break
			}
		}
		return nil
	})
	for _, v := range heatmapMap {
		heatmapList = append(heatmapList, v)
	}
	sort.Slice(heatmapList, func(i, j int) bool {
		if heatmapList[i].X == heatmapList[j].X {
			return heatmapList[i].Y < heatmapList[j].Y
		}
		return heatmapList[i].X < heatmapList[j].X
	})
	mean = 0
	if len(heatmapList) > 0 {
		mean = float64(hit) / float64(len(heatmapList))
	}
	teaProg.Send(SearchMsg{Done: true, Lines: i, Hit: hit, Dur: time.Since(st)})
}

type heatmapModel struct {
	spinner   spinner.Model
	table     table.Model
	done      bool
	quitting  bool
	msg       SearchMsg
	lastSort  string
	save      bool
	textInput textinput.Model
}

func initHeatmapModel() heatmapModel {
	columns := []table.Column{}
	if week {
		columns = append(columns, table.Column{Title: "Weekday"})
		columns = append(columns, table.Column{Title: "Hour"})
		columns = append(columns, table.Column{Title: "Count"})
	} else {
		columns = append(columns, table.Column{Title: "Date"})
		columns = append(columns, table.Column{Title: "Hour"})
		columns = append(columns, table.Column{Title: "Count"})
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
	return heatmapModel{spinner: s, table: t, textInput: ti}
}

func (m heatmapModel) Init() tea.Cmd {
	return m.spinner.Tick
}

func (m heatmapModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
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
		case "k", "c":
			if m.done {
				k := msg.String()
				if k == m.lastSort {
					// Reverse
					for i, j := 0, len(rows)-1; i < j; i, j = i+1, j-1 {
						rows[i], rows[j] = rows[j], rows[i]
					}
				} else {
					// Change sort key
					m.lastSort = k
					if k == "k" {
						sort.Slice(heatmapList, func(i, j int) bool {
							if heatmapList[i].X == heatmapList[j].X {
								return heatmapList[i].Y < heatmapList[j].Y
							}
							return heatmapList[i].X < heatmapList[j].X
						})

					} else {
						sort.Slice(heatmapList, func(i, j int) bool {
							return heatmapList[i].Count < heatmapList[j].Count
						})
					}
					rows = []table.Row{}
					for _, r := range heatmapList {
						rows = append(rows, []string{
							r.Key,
							fmt.Sprintf("%d", r.TimeH),
							fmt.Sprintf("%10s", humanize.Comma(int64(r.Count))),
						})
					}
				}
				m.table.SetRows(rows)
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
		w := m.table.Width() - 4
		if week {
			columns := []table.Column{
				{Title: "Weekday", Width: 6 * w / 10},
				{Title: "Hour", Width: 2 * w / 10},
				{Title: "Count", Width: 2 * w / 10},
			}
			m.table.SetColumns(columns)
		} else {
			columns := []table.Column{
				{Title: "Date", Width: 6 * w / 10},
				{Title: "Hour", Width: 2 * w / 10},
				{Title: "Count", Width: 2 * w / 10},
			}
			m.table.SetColumns(columns)
		}
	case SearchMsg:
		if msg.Done {
			w := m.table.Width() - 4
			if week {
				columns := []table.Column{
					{Title: "Weekday", Width: 6 * w / 10},
					{Title: "Hour", Width: 2 * w / 10},
					{Title: "Count", Width: 2 * w / 10},
				}
				m.table.SetColumns(columns)
			} else {
				columns := []table.Column{
					{Title: "Date", Width: 6 * w / 10},
					{Title: "Hour", Width: 2 * w / 10},
					{Title: "Count", Width: 2 * w / 10},
				}
				m.table.SetColumns(columns)
			}
			rows = []table.Row{}
			for _, r := range heatmapList {
				rows = append(rows, []string{
					r.Key,
					fmt.Sprintf("%d", r.TimeH),
					fmt.Sprintf("%10s", humanize.Comma(int64(r.Count))),
				})
			}
			m.table.SetRows(rows)
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

func (m heatmapModel) SaveUpdate(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyEnter:
			saveHeatmapFile(m.textInput.Value())
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

func (m heatmapModel) View() string {
	if m.save {
		return fmt.Sprintf("Save file name?\n\n%s\n\n%s", m.textInput.View(), "(esc to quit)") + "\n"
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

func (m heatmapModel) headerView() string {
	ms := ""
	if timeMode {
		ms = fmt.Sprintf(" d:%s", time.Duration(time.Second*time.Duration(mean)).String())
	} else {
		ms = fmt.Sprintf(" m:%.3f", mean)
	}
	title := titleStyle.Render(fmt.Sprintf("Results %d/%d/%d s:%v%s", len(heatmapList), m.msg.Hit, m.msg.Lines, m.msg.Dur.Truncate(time.Millisecond), ms))
	help := helpStyle("s: Save / k,c: Sort / q : Quit") + "  "
	gap := strings.Repeat(" ", max(0, m.table.Width()-lipgloss.Width(title)-lipgloss.Width(help)))
	return lipgloss.JoinHorizontal(lipgloss.Center, title, gap, help)
}

func saveHeatmapFile(path string) {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".png":
	case ".html", ".htm":
		saveHeatmapECharts(path)
	default:
		saveHeatmapCSVFile(path)
	}
}

func saveHeatmapCSVFile(path string) {
	f, err := os.Create(path)
	if err != nil {
		log.Fatalln(err)
	}
	defer f.Close()
	w := csv.NewWriter(f)
	if week {
		w.Write([]string{"Weekday", "Hour", "Count"})
	} else {
		w.Write([]string{"Date", "Hour", "Count"})
	}
	for _, r := range heatmapList {
		wr := []string{r.Key, fmt.Sprintf("%d", r.TimeH), fmt.Sprintf("%d", r.Count)}
		w.Write(wr)
	}
	w.Flush()
}
