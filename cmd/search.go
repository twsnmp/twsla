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
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/dustin/go-humanize"
	"go.etcd.io/bbolt"

	"github.com/spf13/cobra"
)

var stopSearch bool
var results []string

type SearchMsg struct {
	Done  bool
	Lines int
	Hit   int
	Dur   time.Duration
}

// searchCmd represents the search command
var searchCmd = &cobra.Command{
	Use:   "search",
	Short: "Search logs.",
	Long: `Search logs.
Simple filters, regular expression filters, and exclusion filters can be specified.
`,
	Run: func(cmd *cobra.Command, args []string) {
		searchMain()
	},
}

func init() {
	rootCmd.AddCommand(searchCmd)
}

func searchMain() {
	st = time.Now()
	if err := openDB(); err != nil {
		log.Fatalln(err)
	}
	defer db.Close()
	teaProg = tea.NewProgram(initSearchModel())
	var wg sync.WaitGroup
	wg.Add(1)
	go searchSub(&wg)
	if _, err := teaProg.Run(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	wg.Wait()
}

func searchSub(wg *sync.WaitGroup) {
	defer wg.Done()
	results = []string{}
	filter := getFilter(regexpFilter)
	if filter == nil {
		filter = getSimpleFilter(simpleFilter)
	}
	not := getFilter(notFilter)
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
			if filter == nil || filter.MatchString(l) {
				if not == nil || !not.MatchString(l) {
					results = append(results, l)
				}
			}
			if i%100 == 0 {
				teaProg.Send(SearchMsg{Lines: i, Hit: len(results), Dur: time.Since(st)})
			}
			if stopSearch {
				break
			}
		}
		return nil
	})
	teaProg.Send(SearchMsg{Done: true, Lines: i, Hit: len(results), Dur: time.Since(st)})
}

type searchModel struct {
	spinner   spinner.Model
	viewport  viewport.Model
	done      bool
	ready     bool
	quitting  bool
	msg       SearchMsg
	save      bool
	textInput textinput.Model
}

func initSearchModel() searchModel {
	s := spinner.New()
	s.Spinner = spinner.Line
	s.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("#00efff"))
	ti := textinput.New()
	ti.Placeholder = "save file name"
	ti.Focus()
	ti.CharLimit = 156
	ti.Width = 20
	return searchModel{spinner: s, textInput: ti}
}

func (m searchModel) Init() tea.Cmd {
	return m.spinner.Tick
}

func (m searchModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
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
		case "r":
			if m.done {
				for i, j := 0, len(results)-1; i < j; i, j = i+1, j-1 {
					results[i], results[j] = results[j], results[i]
				}
				m.viewport.SetContent(strings.Join(results, "\n"))
			}
			return m, nil
		default:
			if !m.done {
				return m, nil
			}
		}
	case tea.WindowSizeMsg:
		headerHeight := lipgloss.Height(m.headerView())

		if !m.ready {
			m.ready = true
			m.viewport = viewport.New(msg.Width, msg.Height-headerHeight)
			m.viewport.YPosition = headerHeight
			m.viewport.YPosition = headerHeight + 1
		} else {
			m.viewport.Width = msg.Width
			m.viewport.Height = msg.Height - headerHeight
		}
	case SearchMsg:
		if msg.Done {
			m.viewport.SetContent(strings.Join(results, "\n"))
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
	m.viewport, cmd = m.viewport.Update(msg)
	return m, cmd
}

func (m searchModel) SaveUpdate(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyEnter:
			saveSearchFile(m.textInput.Value())
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

func (m searchModel) View() string {
	if m.save {
		return fmt.Sprintf("Save file name?\n\n%s\n\n%s", m.textInput.View(), "(esc to quit)") + "\n"
	}
	if m.done {
		return fmt.Sprintf("%s\n%s", m.headerView(), m.viewport.View())
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

func (m searchModel) headerView() string {
	title := titleStyle.Render(fmt.Sprintf("Results %d/%d %v", m.msg.Hit, m.msg.Lines, m.msg.Dur))
	info := infoStyle.Render(fmt.Sprintf("%3.f%%", m.viewport.ScrollPercent()*100))
	help := helpStyle("s: Save / r: Reverse / q : Quit") + "  "
	gap := strings.Repeat(" ", max(0, m.viewport.Width-lipgloss.Width(title)-lipgloss.Width(info)-lipgloss.Width(help)))
	return lipgloss.JoinHorizontal(lipgloss.Center, title, gap, help, info)
}

func saveSearchFile(path string) {
	f, err := os.Create(path)
	if err != nil {
		log.Fatalln(err)
	}
	defer f.Close()
	for _, r := range results {
		f.WriteString(r + "\n")
	}
}
