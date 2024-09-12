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

// countCmd represents the count command
var countCmd = &cobra.Command{
	Use:   "count",
	Short: "Count log",
	Long: `Count the number of logs.
Number of logs per specified time
Number of occurrences of items extracted from the log`,
	Run: func(cmd *cobra.Command, args []string) {
		countMain()
	},
}

var nameCount string

func init() {
	rootCmd.AddCommand(countCmd)
	countCmd.Flags().IntVarP(&interval, "interval", "i", 0, "Specify the aggregation interval in seconds.")
	countCmd.Flags().IntVarP(&pos, "pos", "p", 1, "Specify variable location")
	countCmd.Flags().StringVarP(&extract, "extract", "e", "", "Extract pattern")
	countCmd.Flags().StringVarP(&nameCount, "name", "n", "Key", "Name of key")
}

var timeMode = false
var mean float64

func countMain() {
	st = time.Now()
	extPat := getExtPat()
	timeMode = extPat == nil
	if err := openDB(); err != nil {
		log.Fatalln(err)
	}
	defer db.Close()
	teaProg = tea.NewProgram(initCountModel())
	var wg sync.WaitGroup
	wg.Add(1)
	go countSub(&wg)
	if _, err := teaProg.Run(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	wg.Wait()
}

type countEnt struct {
	Key   string
	Count int
	Delta int
}

var countList = []countEnt{}

func countSub(wg *sync.WaitGroup) {
	var countMap = make(map[string]int)
	defer wg.Done()
	filter := getFilter(regexpFilter)
	filterS := getSimpleFilter(simpleFilter)
	not := getFilter(notFilter)
	extPat := getExtPat()
	intv := int64(getInterval()) * 1000 * 1000 * 1000
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
			if filter == nil || filter.MatchString(l) {
				if filterS == nil || filterS.MatchString(l) {
					if not == nil || !not.MatchString(l) {
						if timeMode {
							d := t / intv
							ck := time.Unix(0, d*intv).Format("2006/01/02 15:04")
							countMap[ck]++
							hit++
						} else {
							a := extPat.ExtReg.FindAllStringSubmatch(l, -1)
							if len(a) >= extPat.Index {
								ck := a[extPat.Index-1][1]
								countMap[ck]++
								hit++
							}
						}
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
	for k, v := range countMap {
		countList = append(countList, countEnt{
			Key:   k,
			Count: v,
		})
	}
	if timeMode {
		sort.Slice(countList, func(i, j int) bool {
			return countList[i].Key < countList[j].Key
		})
		last := int64(0)
		mean = 0
		for i := 0; i < len(countList); i++ {
			if t, err := time.Parse("2006/01/02 15:04", countList[i].Key); err == nil {
				if i > 0 {
					countList[i].Delta = int(t.Unix() - last)
				}
				last = t.Unix()
			}
			mean += float64(countList[i].Delta)
		}
		if len(countList) > 1 {
			mean /= float64(len(countList) - 1)
		}
	} else {
		sort.Slice(countList, func(i, j int) bool {
			return countList[i].Count > countList[j].Count
		})
		if len(countList) > 0 {
			mean = float64(hit) / float64(len(countList))
		}
	}
	teaProg.Send(SearchMsg{Done: true, Lines: i, Hit: hit, Dur: time.Since(st)})
}

type countModel struct {
	spinner   spinner.Model
	table     table.Model
	done      bool
	quitting  bool
	msg       SearchMsg
	lastSort  string
	save      bool
	textInput textinput.Model
}

func initCountModel() countModel {
	columns := []table.Column{
		{Title: nameCount},
		{Title: "Count"},
	}
	if timeMode {
		columns = append(columns, table.Column{Title: "Delta"})
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
	return countModel{spinner: s, table: t, textInput: ti}
}

func (m countModel) Init() tea.Cmd {
	return m.spinner.Tick
}

var rows = []table.Row{}

func (m countModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
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
		case "c", "k", "d", "t":
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
					if k == "k" || k == "t" {
						sort.Slice(countList, func(i, j int) bool {
							return countList[i].Key < countList[j].Key
						})
					} else if k == "d" && timeMode {
						sort.Slice(countList, func(i, j int) bool {
							return countList[i].Delta < countList[j].Delta
						})
					} else {
						sort.Slice(countList, func(i, j int) bool {
							return countList[i].Count < countList[j].Count
						})
					}
					rows = []table.Row{}
					for _, r := range countList {
						if timeMode {
							rows = append(rows, []string{
								r.Key,
								fmt.Sprintf("%10s", humanize.Comma(int64(r.Count))),
								time.Duration(time.Second * time.Duration(r.Delta)).String(),
							})
						} else {
							rows = append(rows, []string{r.Key, fmt.Sprintf("%10s", humanize.Comma(int64(r.Count)))})
						}
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
	case SearchMsg:
		if msg.Done {
			w := m.table.Width() - 4
			if timeMode {
				w -= 2
				columns := []table.Column{
					{Title: nameCount, Width: 5 * w / 10},
					{Title: "Count", Width: 3 * w / 10},
					{Title: "Delta", Width: 2 * w / 10},
				}
				m.table.SetColumns(columns)
			} else {
				columns := []table.Column{
					{Title: nameCount, Width: 7 * w / 10},
					{Title: "Count", Width: 3 * w / 10},
				}
				m.table.SetColumns(columns)
			}
			rows = []table.Row{}
			for _, r := range countList {
				if timeMode {
					rows = append(rows, []string{
						r.Key,
						fmt.Sprintf("%10s", humanize.Comma(int64(r.Count))),
						time.Duration(time.Second * time.Duration(r.Delta)).String(),
					})
				} else {
					rows = append(rows, []string{r.Key, fmt.Sprintf("%10s", humanize.Comma(int64(r.Count)))})
				}
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

func (m countModel) SaveUpdate(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyEnter:
			saveCountFile(m.textInput.Value())
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
func (m countModel) View() string {
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

func (m countModel) headerView() string {
	ms := ""
	if timeMode {
		ms = fmt.Sprintf(" d:%s", time.Duration(time.Second*time.Duration(mean)).String())
	} else {
		ms = fmt.Sprintf(" m:%.3f", mean)
	}
	title := titleStyle.Render(fmt.Sprintf("Results %d/%d/%d s:%v%s", len(countList), m.msg.Hit, m.msg.Lines, m.msg.Dur.Truncate(time.Millisecond), ms))
	help := helpStyle("s: Save / c,k,d: Sort / q : Quit") + "  "
	gap := strings.Repeat(" ", max(0, m.table.Width()-lipgloss.Width(title)-lipgloss.Width(help)))
	return lipgloss.JoinHorizontal(lipgloss.Center, title, gap, help)
}

func saveCountFile(path string) {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".png":
		if timeMode {
			SaveCountTimeChart(path)
		} else {
			SaveCountChart(path)
		}
	case ".html", ".htm":
		if timeMode {
			SaveCountTimeECharts(path)
		} else {
			SaveCountECharts(path)
		}
	default:
		saveCountCSVFile(path)
	}
}

func saveCountCSVFile(path string) {
	f, err := os.Create(path)
	if err != nil {
		log.Fatalln(err)
	}
	defer f.Close()
	w := csv.NewWriter(f)
	if timeMode {
		w.Write([]string{nameCount, "Count", "Delta", "Delta(sec)"})
	} else {
		w.Write([]string{nameCount, "Count"})
	}
	for _, r := range countList {
		wr := []string{r.Key, fmt.Sprintf("%d", r.Count)}
		if timeMode {
			wr = append(wr, time.Duration(time.Second*time.Duration(r.Delta)).String())
			wr = append(wr, fmt.Sprintf("%d", r.Delta))
		}
		w.Write(wr)
	}
	w.Flush()
}
