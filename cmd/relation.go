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
	"regexp"
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

type relationDataEnt struct {
	Name  string
	Reg   *regexp.Regexp
	Index int
}

var relationCheckList = []relationDataEnt{}

// relationCmd represents the relation command
var relationCmd = &cobra.Command{
	Use:   "relation <data1> <data2>...",
	Short: "Relation Analysis",
	Long: `Analyzes the relationship between two or more pieces of data extracted from a log, 
such as the relationship between an IP address and a MAC address.
data entry is ip | mac | email | url | regex/<pattern>/<color> 
`,
	Run: func(cmd *cobra.Command, args []string) {
		fargs := []string{}
		for _, e := range args {
			switch {
			case strings.HasPrefix(e, "ip"):
				relationCheckList = append(relationCheckList, relationDataEnt{
					Name:  e,
					Reg:   regexpIP,
					Index: getRelationEntIndex(e),
				})
			case strings.HasPrefix(e, "mac"):
				relationCheckList = append(relationCheckList, relationDataEnt{
					Name:  e,
					Reg:   regexpMAC,
					Index: getRelationEntIndex(e),
				})
			case strings.HasPrefix(e, "email"):
				relationCheckList = append(relationCheckList, relationDataEnt{
					Name:  e,
					Reg:   regexpEMail,
					Index: getRelationEntIndex(e),
				})
			case strings.HasPrefix(e, "url"):
				relationCheckList = append(relationCheckList, relationDataEnt{
					Name:  e,
					Reg:   regexpURL,
					Index: getRelationEntIndex(e),
				})
			case strings.HasPrefix(e, "kv"):
				relationCheckList = append(relationCheckList, relationDataEnt{
					Name:  e,
					Reg:   regexpKV,
					Index: getRelationEntIndex(e),
				})
			case strings.HasPrefix(e, "regex/") || strings.HasPrefix(e, "regexp/"):
				{
					a := strings.Split(e, "/")
					if len(a) > 2 {
						p := ""
						for i := 1; i < len(a)-1; i++ {
							if p != "" {
								p += "/"
							}
							p += a[i]
						}
						relationCheckList = append(relationCheckList, relationDataEnt{
							Name:  e,
							Reg:   regexp.MustCompile(p),
							Index: getRelationEntIndex(e),
						})
					}
				}
			default:
				fargs = append(fargs, e)
			}
		}
		setupFilter(fargs)
		if len(relationCheckList) < 2 {
			log.Fatalln("you have to specify data entry.")
		}
		relationMain()
	},
}

func getRelationEntIndex(s string) int {
	a := strings.Split(s, ":")
	if len(a) < 2 {
		return 0
	}
	i, err := strconv.Atoi(a[len(a)-1])
	if err != nil {
		log.Fatalln(err)
	}
	return i
}

func init() {
	rootCmd.AddCommand(relationCmd)
}

func relationMain() {
	st = time.Now()
	if err := openDB(); err != nil {
		log.Fatalln(err)
	}
	defer db.Close()
	teaProg = tea.NewProgram(initRelationModel())
	var wg sync.WaitGroup
	wg.Add(1)
	go relationSub(&wg)
	if _, err := teaProg.Run(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	wg.Wait()
}

type relationEnt struct {
	Key    string
	Values []string
	Count  int
}

var relationList = []*relationEnt{}

func relationSub(wg *sync.WaitGroup) {
	var relationMap = make(map[string]*relationEnt)
	defer wg.Done()
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
				var vals = []string{}
				for _, r := range relationCheckList {
					a := r.Reg.FindAllString(l, -1)
					if len(a) < r.Index+1 {
						break
					}
					vals = append(vals, a[r.Index])
				}
				if len(vals) != len(relationCheckList) {
					continue
				}
				hit++
				key := strings.Join(vals, "\t")
				if e, ok := relationMap[key]; ok {
					e.Count++
				} else {
					relationMap[key] = &relationEnt{
						Key:    key,
						Values: vals,
						Count:  1,
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
	for _, v := range relationMap {
		relationList = append(relationList, v)
	}
	sort.Slice(relationList, func(i, j int) bool {
		return relationList[i].Count > relationList[j].Count
	})
	if len(relationList) > 0 {
		mean = float64(hit) / float64(len(relationList))
	}
	teaProg.Send(SearchMsg{Done: true, Lines: i, Hit: hit, Dur: time.Since(st)})
}

type relationModel struct {
	spinner   spinner.Model
	table     table.Model
	done      bool
	quitting  bool
	msg       SearchMsg
	lastSort  string
	save      bool
	textInput textinput.Model
	sixel     string
}

func initRelationModel() relationModel {
	columns := []table.Column{}
	for _, e := range relationCheckList {
		columns = append(columns, table.Column{Title: e.Name})
	}
	columns = append(columns, table.Column{Title: "Count"})
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
	return relationModel{spinner: s, table: t, textInput: ti}
}

func (m relationModel) Init() tea.Cmd {
	return m.spinner.Tick
}

func (m relationModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
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
		case "g":
			if m.done {
				p := filepath.Join(chartTmp, "relation.png")
				SaveRelationChart(p)
				if sixelChart {
					m.sixel = openChartSixel(p)
				} else {
					openChart(p)
				}
			}
		case "h":
			if m.done {
				p := filepath.Join(chartTmp, "relation.html")
				SaveRelationECharts(p)
				openChart(p)
			}
		case "s":
			if m.done {
				m.save = true
			}
			return m, nil
		case "c", "0", "1", "2", "3", "4", "5", "6", "7", "8", "9":
			if m.done {
				k := msg.String()
				if k == m.lastSort {
					// Reverse
					for i, j := 0, len(rows)-1; i < j; i, j = i+1, j-1 {
						rows[i], rows[j] = rows[j], rows[i]
					}
				} else {
					// Change sort key
					if k == "c" {
						m.lastSort = k
						sort.Slice(relationList, func(i, j int) bool {
							return relationList[i].Count < relationList[j].Count
						})
					} else {
						if sk, err := strconv.Atoi(msg.String()); err == nil && sk >= 0 && sk < len(relationCheckList) {
							m.lastSort = k
							sort.Slice(relationList, func(i, j int) bool {
								return relationList[i].Values[sk] < relationList[j].Values[sk]
							})
						}
					}
					rows = []table.Row{}
					for _, r := range relationList {
						row := []string{}
						row = append(row, r.Values...)
						row = append(row, humanize.Comma(int64(r.Count)))
						rows = append(rows, row)
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
		columns := []table.Column{}
		for _, e := range relationCheckList {
			columns = append(columns, table.Column{Title: e.Name, Width: (w * 9) / (len(relationCheckList) * 10)})
		}
		columns = append(columns, table.Column{Title: "Count", Width: w / 10})
		m.table.SetColumns(columns)
	case SearchMsg:
		if msg.Done {
			w := m.table.Width() - 4
			columns := []table.Column{}
			for _, e := range relationCheckList {
				columns = append(columns, table.Column{Title: e.Name, Width: (w * 9) / (len(relationCheckList) * 10)})
			}
			columns = append(columns, table.Column{Title: "Count", Width: w / 10})
			m.table.SetColumns(columns)
			rows = []table.Row{}
			for _, r := range relationList {
				row := []string{}
				row = append(row, r.Values...)
				row = append(row, humanize.Comma(int64(r.Count)))
				rows = append(rows, row)
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

func (m relationModel) SaveUpdate(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyEnter:
			saveReleationFile(m.textInput.Value())
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
func (m relationModel) View() string {
	if m.save {
		return fmt.Sprintf("Save file name?\n\n%s\n\n%s", m.textInput.View(), "(esc to quit)") + "\n"
	}
	if m.sixel != "" {
		return "\n\n" + m.sixel
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

func (m relationModel) headerView() string {
	ms := fmt.Sprintf(" m:%.3f", mean)
	title := titleStyle.Render(fmt.Sprintf("Results %d/%d/%d s:%v%s", len(relationList), m.msg.Hit, m.msg.Lines, m.msg.Dur.Truncate(time.Millisecond), ms))
	help := helpStyle("s: Save / c,0-9: Sort / g|h: Chart / q : Quit") + "  "
	gap := strings.Repeat(" ", max(0, m.table.Width()-lipgloss.Width(title)-lipgloss.Width(help)))
	return lipgloss.JoinHorizontal(lipgloss.Center, title, gap, help)
}

func saveReleationFile(path string) {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".png":
		SaveRelationChart(path)
	case ".html":
		SaveRelationECharts(path)
	default:
		saveRelationCSVFile(path)
	}
}

func saveRelationCSVFile(path string) {
	f, err := os.Create(path)
	if err != nil {
		log.Fatalln(err)
	}
	defer f.Close()
	w := csv.NewWriter(f)
	row := []string{}
	for _, e := range relationCheckList {
		row = append(row, e.Name)
	}
	row = append(row, "Count")
	w.Write(row)
	for _, r := range relationList {
		wr := []string{}
		wr = append(wr, r.Values...)
		wr = append(wr, fmt.Sprintf("%d", r.Count))
		w.Write(wr)
	}
	w.Flush()
}
