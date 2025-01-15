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
	"encoding/json"
	"fmt"
	"log"
	"os"
	"regexp"
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
	Use:   "search [simple filter...]",
	Short: "Search logs.",
	Long: `Search logs.
Simple filters, regular expression filters, and exclusion filters can be specified.
`,
	Run: func(cmd *cobra.Command, args []string) {
		setupFilter(args)
		searchMain()
	},
}

var colorMode string
var marker string
var colorList = []*colorMapEnt{}

func init() {
	rootCmd.AddCommand(searchCmd)
	searchCmd.Flags().StringVarP(&colorMode, "color", "c", "", "Color mode")
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

type colorMapEnt struct {
	Reg   *regexp.Regexp
	Style lipgloss.Style
}

func searchSub(wg *sync.WaitGroup) {
	defer wg.Done()
	makeColorList()
	results = []string{}
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
				results = append(results, l)
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

func makeColorList() {
	colorList = []*colorMapEnt{}
	for _, cm := range strings.Split(colorMode, ",") {
		switch {
		case cm == "filter":
			for i, f := range filterList {
				if i == 0 && regexpFilter != "" {
					colorList = append(colorList, &colorMapEnt{
						Reg:   f,
						Style: lipgloss.NewStyle().Foreground(lipgloss.Color("9")),
					})
				} else {
					colorList = append(colorList, &colorMapEnt{
						Reg:   f,
						Style: lipgloss.NewStyle().Foreground(lipgloss.Color("5")),
					})

				}
			}
		case cm == "ip":
			colorList = append(colorList, &colorMapEnt{
				Reg:   regexpIP,
				Style: lipgloss.NewStyle().Foreground(lipgloss.Color("10")),
			})
		case cm == "mac":
			colorList = append(colorList, &colorMapEnt{
				Reg:   regexpMAC,
				Style: lipgloss.NewStyle().Foreground(lipgloss.Color("11")),
			})
		case cm == "email":
			colorList = append(colorList, &colorMapEnt{
				Reg:   regexpEMail,
				Style: lipgloss.NewStyle().Foreground(lipgloss.Color("12")),
			})
		case cm == "url":
			colorList = append(colorList, &colorMapEnt{
				Reg:   regexpURL,
				Style: lipgloss.NewStyle().Foreground(lipgloss.Color("14")),
			})
		case cm == "kv":
			colorList = append(colorList, &colorMapEnt{
				Reg:   regexpKV,
				Style: lipgloss.NewStyle().Foreground(lipgloss.Color("3")),
			})
		case strings.HasPrefix(cm, "regex/"):
			{
				a := strings.Split(cm, "/")
				if len(a) > 2 {
					p := ""
					for i := 1; i < len(a)-1; i++ {
						if p != "" {
							p += "/"
						}
						p += a[i]
					}
					colorList = append(colorList, &colorMapEnt{
						Reg:   regexp.MustCompile(p),
						Style: lipgloss.NewStyle().Foreground(lipgloss.Color(a[len(a)-1])),
					})
				}
			}
		}
	}
}

func getColoredResults(pretty bool) []string {
	r := []string{}
	var markerReg *regexp.Regexp
	if strings.HasPrefix(marker, "regex:") {
		a := strings.SplitN(marker, ":", 2)
		markerReg = getFilter(a[1])
	} else {
		markerReg = getSimpleFilter(marker)
	}
	for _, l := range results {
		if pretty {
			l = prettyJSON(l)
		}
		for _, c := range colorList {
			l = c.Reg.ReplaceAllStringFunc(l, func(s string) string {
				return c.Style.Render(s)
			})
		}
		if markerReg != nil {
			l = markerReg.ReplaceAllStringFunc(l, func(s string) string {
				return markStyle.Render(s)
			})
		}
		r = append(r, l)
	}
	return r
}

func prettyJSON(l string) string {
	var data interface{}
	if err := json.Unmarshal([]byte(l), &data); err == nil {
		if b, err := json.MarshalIndent(data, "", "  "); err == nil {
			l = string(b)
		}
	}
	return l
}

type searchModel struct {
	spinner        spinner.Model
	viewport       viewport.Model
	done           bool
	ready          bool
	quitting       bool
	msg            SearchMsg
	save           bool
	saveInput      textinput.Model
	color          bool
	colorModeInput textinput.Model
	marker         bool
	markerInput    textinput.Model
}

func initSearchModel() searchModel {
	s := spinner.New()
	s.Spinner = spinner.Line
	s.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("#00efff"))
	sti := textinput.New()
	sti.Placeholder = "save file name"
	sti.Focus()
	sti.CharLimit = 156
	sti.Width = 20
	cti := textinput.New()
	cti.Placeholder = "color mode"
	cti.Focus()
	cti.CharLimit = 256
	cti.Width = 40
	mti := textinput.New()
	mti.Placeholder = "mark key word"
	mti.Focus()
	mti.CharLimit = 256
	mti.Width = 40
	return searchModel{spinner: s, saveInput: sti, colorModeInput: cti, markerInput: mti}
}

func (m searchModel) Init() tea.Cmd {
	return m.spinner.Tick
}

func (m searchModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if m.save {
		return m.SaveUpdate(msg)
	}
	if m.color {
		return m.ColorUpdate(msg)
	}
	if m.marker {
		return m.MarkerUpdate(msg)
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
		case "c", "C":
			if m.done {
				m.colorModeInput.SetValue(colorMode)
				m.color = true
			}
			return m, nil
		case "m", "M":
			if m.done {
				m.markerInput.SetValue(marker)
				m.marker = true
			}
			return m, nil
		case "r":
			if m.done {
				for i, j := 0, len(results)-1; i < j; i, j = i+1, j-1 {
					results[i], results[j] = results[j], results[i]
				}
				m.viewport.SetContent(strings.Join(getColoredResults(false), "\n"))
			}
			return m, nil
		case "d", "p":
			if m.done {
				m.viewport.SetContent(strings.Join(getColoredResults(msg.String() == "p"), "\n"))
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
			m.viewport.YPosition = headerHeight + 1
			m.viewport.SetContent(strings.Join(getColoredResults(false), "\n"))
		} else {
			m.viewport.Width = msg.Width
			m.viewport.Height = msg.Height - headerHeight
		}
	case SearchMsg:
		if msg.Done {
			if m.ready {
				m.viewport.SetContent(strings.Join(getColoredResults(false), "\n"))
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
			saveSearchFile(m.saveInput.Value())
			m.save = false
			return m, nil
		case tea.KeyCtrlC, tea.KeyEsc:
			m.save = false
			return m, nil
		}
	}
	m.saveInput, cmd = m.saveInput.Update(msg)
	return m, cmd
}

func (m searchModel) ColorUpdate(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyEnter:
			colorMode = m.colorModeInput.Value()
			m.color = false
			makeColorList()
			m.viewport.SetContent(strings.Join(getColoredResults(false), "\n"))
			return m, nil
		case tea.KeyCtrlC, tea.KeyEsc:
			m.color = false
			return m, nil
		}
	}
	m.colorModeInput, _ = m.colorModeInput.Update(msg)
	m.markerInput, cmd = m.markerInput.Update(msg)
	return m, cmd
}

func (m searchModel) MarkerUpdate(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyEnter:
			marker = m.markerInput.Value()
			m.marker = false
			makeColorList()
			m.viewport.SetContent(strings.Join(getColoredResults(false), "\n"))
			return m, nil
		case tea.KeyCtrlC, tea.KeyEsc:
			m.marker = false
			return m, nil
		}
	}
	m.colorModeInput, _ = m.colorModeInput.Update(msg)
	m.markerInput, cmd = m.markerInput.Update(msg)
	return m, cmd
}

func (m searchModel) View() string {
	if m.save {
		return fmt.Sprintf("Save file name?\n\n%s\n\n%s", m.saveInput.View(), "(esc to quit)") + "\n"
	}
	if m.color {
		return fmt.Sprintf("Color Map Mode?\n\n%s\n\n%s", m.colorModeInput.View(), "(esc to quit)") + "\n"
	}
	if m.marker {
		return fmt.Sprintf("Marker?\n\n%s\n\n%s", m.markerInput.View(), "(esc to quit)") + "\n"
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
	title := titleStyle.Render(fmt.Sprintf("Results %d/%d s:%s", m.msg.Hit, m.msg.Lines, m.msg.Dur.Truncate(time.Millisecond)))
	info := infoStyle.Render(fmt.Sprintf("%3.f%%", m.viewport.ScrollPercent()*100))
	help := helpStyle("s: Save / r: Reverse / m: Marker / c: Color / p/d: Format  / q : Quit") + "  "
	gap := strings.Repeat(" ", max(0, m.viewport.Width-lipgloss.Width(title)-lipgloss.Width(info)-lipgloss.Width(help)))
	return lipgloss.JoinHorizontal(lipgloss.Center, title, gap, help, info)
}

func saveSearchFile(path string) {
	if path == "" {
		return
	}
	f, err := os.Create(path)
	if err != nil {
		log.Fatalln(err)
	}
	defer f.Close()
	for _, r := range results {
		f.WriteString(r + "\n")
	}
}
