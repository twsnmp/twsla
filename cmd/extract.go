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

// extractCmd represents the extract command
var extractCmd = &cobra.Command{
	Use:   "extract",
	Short: "Extract data from log",
	Long:  `Extract data from log`,
	Run: func(cmd *cobra.Command, args []string) {
		extractMain()
	},
}

var name string

func init() {
	rootCmd.AddCommand(extractCmd)
	extractCmd.Flags().StringVarP(&extract, "extract", "e", "", "Extract pattern")
	extractCmd.Flags().IntVarP(&pos, "pos", "p", 1, "positon")
	extractCmd.Flags().StringVarP(&name, "name", "n", "Value", "Name of Value")
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
}

var extractList = []extractEnt{}

func extractSub(wg *sync.WaitGroup) {
	defer wg.Done()
	results = []string{}
	filter := getFilter(regexpFilter)
	if filter == nil {
		filter = getSimpleFilter(simpleFilter)
	}
	extPat := getExtPat()
	if extPat == nil {
		log.Fatalln("no extract pattern")
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
			if filter == nil || filter.MatchString(l) {
				a := extPat.ExtReg.FindAllStringSubmatch(l, -1)
				if len(a) >= extPat.Index {
					extractList = append(extractList, extractEnt{Time: t, Value: a[extPat.Index-1][1]})
					hit++
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
}

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
	return extractModel{spinner: s, table: t, textInput: ti}
}

func (m extractModel) Init() tea.Cmd {
	return m.spinner.Tick
}

var extractRows = []table.Row{}

func (m extractModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
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
		case "t", "v":
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
					} else {
						sort.Slice(extractList, func(i, j int) bool {
							return extractList[i].Value < extractList[j].Value
						})
					}
					extractRows = []table.Row{}
					for _, r := range extractList {
						extractRows = append(extractRows, []string{time.Unix(0, r.Time).Format("2006/01/02T15:04:05.999"), r.Value})
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
	case SearchMsg:
		if msg.Done {
			w := m.table.Width() - 4
			columns := []table.Column{
				{Title: "Time", Width: 4 * w / 10},
				{Title: name, Width: 6 * w / 10},
			}
			m.table.SetColumns(columns)
			extractRows = []table.Row{}
			for _, r := range extractList {
				extractRows = append(extractRows, []string{time.Unix(0, r.Time).Format("2006/01/02T15:04:05.999"), r.Value})
			}
			m.table.SetRows(extractRows)
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
			saveExtractFile(m.textInput.Value())
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
	title := titleStyle.Render(fmt.Sprintf("Results %d/%d %d %v", m.msg.Hit, m.msg.Lines, len(countList), m.msg.Dur))
	help := helpStyle("s: Save / t: Sort by time / v: Sort by value / q : Quit") + "  "
	gap := strings.Repeat(" ", max(0, m.table.Width()-lipgloss.Width(title)-lipgloss.Width(help)))
	return lipgloss.JoinHorizontal(lipgloss.Center, title, gap, help)
}

func saveExtractFile(path string) {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".png", ".html":
		// saveChart(path,ext)
	case ".xlsx":
		// saveExtractExcel(path)
	default:
		saveExtractCSVFile(path)
	}
}

func saveExtractCSVFile(path string) {
	// CSV
	f, err := os.Create(path)
	if err != nil {
		log.Fatalln(err)
	}
	defer f.Close()
	w := csv.NewWriter(f)
	w.Write([]string{"Time", name})
	for _, r := range extractList {
		wr := []string{time.Unix(0, r.Time).Format("2006/01/02T15:04:05.999"), r.Value}
		w.Write(wr)
	}
	w.Flush()

}
