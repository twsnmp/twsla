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
	go_iforest "github.com/codegaudi/go-iforest"
	tf_idf "github.com/dkgv/go-tf-idf"
	"github.com/dustin/go-humanize"
	"github.com/spf13/cobra"
	"go.etcd.io/bbolt"
)

// anomalyCmd represents the anomaly command
var anomalyCmd = &cobra.Command{
	Use:   "anomaly",
	Short: "Anomaly log detection",
	Long: `Anomaly log detection
	Detect anomaly logs using isolation forests.
	Detection modes include walu, SQL injection, OS command injections, and directory traverses.
	`,
	Run: func(cmd *cobra.Command, args []string) {
		setupFilter(args)
		anomalyMain()
	},
}

var anomalyMode string

func init() {
	rootCmd.AddCommand(anomalyCmd)
	anomalyCmd.Flags().StringVarP(&anomalyMode, "mode", "m", "tfidf", "Detection modes(tfidf|sql|os|dir|walu|number)")
	anomalyCmd.Flags().StringVarP(&extract, "extract", "e", "", "Extract pattern")
}

type anomalyMsg struct {
	Done   bool
	Phase  string
	Lines  int
	Hit    int
	PLines int
	Dur    time.Duration
}

func anomalyMain() {
	st = time.Now()
	if err := openDB(); err != nil {
		log.Fatalln(err)
	}
	defer db.Close()
	teaProg = tea.NewProgram(initAnomayModel())
	var wg sync.WaitGroup
	wg.Add(1)
	go anomalySub(&wg)
	if _, err := teaProg.Run(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	wg.Wait()
}

type anomalyEnt struct {
	Log   int
	Score float64
}

var anomalyList = []anomalyEnt{}
var lines int
var hit int
var vectors = [][]float64{}
var times = []int64{}

func anomalySub(wg *sync.WaitGroup) {
	defer wg.Done()
	results = []string{}
	if len(filterList) == 0 && anomalyMode == "number" && extract != "" {
		filterList = append(filterList, getSimpleFilter(extract))
	}
	sti, eti := getTimeRange()
	sk := fmt.Sprintf("%016x:", sti)
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
			lines++
			if matchFilter(&l) {
				hit++
				results = append(results, l)
				times = append(times, t)
			}
			if lines%100 == 0 {
				teaProg.Send(anomalyMsg{Phase: "Search", Lines: lines, Hit: hit, Dur: time.Since(st)})
			}
			if stopSearch {
				break
			}
		}
		return nil
	})
	switch anomalyMode {
	case "sql":
		anomalySQL()
	case "os":
		anomalyOS()
	case "dir":
		anomalyDir()
	case "walu":
		anomalyWalu()
	case "number":
		anomalyNumber()
	default:
		// TF-IDF
		anomalyTFIDF()
	}
	// iforest
	teaProg.Send(anomalyMsg{Phase: "Trainnig", PLines: 0, Lines: lines, Hit: hit, Dur: time.Since(st)})
	iforest, err := go_iforest.NewIForest(vectors, 1000, 256)
	if err != nil {
		log.Fatalf("iforest err=%v", err)
	}
	anomalyList = []anomalyEnt{}
	for i, v := range vectors {
		if i%100 == 0 {
			teaProg.Send(anomalyMsg{Phase: "Score", PLines: i, Lines: lines, Hit: hit, Dur: time.Since(st)})
		}
		if stopSearch {
			break
		}
		anomalyList = append(anomalyList,
			anomalyEnt{
				Log:   i,
				Score: iforest.CalculateAnomalyScore(v),
			},
		)
	}
	sort.Slice(anomalyList, func(a, b int) bool {
		return anomalyList[a].Score > anomalyList[b].Score
	})
	teaProg.Send(anomalyMsg{Phase: "Done", Done: true, PLines: hit, Lines: lines, Hit: hit, Dur: time.Since(st)})

}

type anomalyModel struct {
	spinner   spinner.Model
	table     table.Model
	done      bool
	log       string
	quitting  bool
	msg       anomalyMsg
	save      bool
	textInput textinput.Model
}

func initAnomayModel() anomalyModel {
	columns := []table.Column{
		{Title: "Log"},
		{Title: "Score"},
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
	return anomalyModel{spinner: s, table: t, textInput: ti}
}

func (m anomalyModel) Init() tea.Cmd {
	return m.spinner.Tick
}

var anomalyRows = []table.Row{}

func (m anomalyModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
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
				// Reverse
				for i, j := 0, len(anomalyRows)-1; i < j; i, j = i+1, j-1 {
					anomalyRows[i], anomalyRows[j] = anomalyRows[j], anomalyRows[i]
				}
				m.table.SetRows(anomalyRows)
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
			{Title: "Score", Width: 1 * w / 10},
		}
		m.table.SetColumns(columns)
	case anomalyMsg:
		if msg.Done {
			w := m.table.Width() - 4
			columns := []table.Column{
				{Title: "Log", Width: 9 * w / 10},
				{Title: "Score", Width: 1 * w / 10},
			}
			m.table.SetColumns(columns)
			anomalyRows = []table.Row{}
			for _, r := range anomalyList {
				anomalyRows = append(anomalyRows, []string{
					results[r.Log],
					fmt.Sprintf("%.3f", r.Score),
				})
			}
			m.table.SetRows(anomalyRows)
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

func (m anomalyModel) SaveUpdate(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyEnter:
			saveAnomalyFile(m.textInput.Value())
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
func (m anomalyModel) View() string {
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

func (m anomalyModel) headerView() string {
	title := titleStyle.Render(fmt.Sprintf("Results %d/%d %d s:%s", m.msg.Hit, m.msg.Lines, len(anomalyList), m.msg.Dur.Truncate(time.Millisecond)))
	help := helpStyle("enter: Show / s: Save / r: Sort / q : Quit") + "  "
	gap := strings.Repeat(" ", max(0, m.table.Width()-lipgloss.Width(title)-lipgloss.Width(help)))
	return lipgloss.JoinHorizontal(lipgloss.Center, title, gap, help)
}

func saveAnomalyFile(path string) {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".png":
	case ".html", ".htm":
	default:
		saveAnomalyTSVFile(path)
	}
}

func saveAnomalyTSVFile(path string) {
	if path == "" {
		return
	}
	// TSV
	f, err := os.Create(path)
	if err != nil {
		log.Fatalln(err)
	}
	defer f.Close()
	f.WriteString(strings.Join([]string{"Log", "Score"}, "\t") + "\n")
	for _, r := range anomalyList {
		f.WriteString(strings.Join([]string{
			results[r.Log],
			fmt.Sprintf("%.3f", r.Score),
		}, "\t") + "\n")
	}
}

func anomalyTFIDF() {
	// TF-IDF
	tfidf := tf_idf.New(
		tf_idf.WithDefaultStopWords(),
	)
	for i, l := range results {
		tfidf.AddDocument(l)
		if i%100 == 0 {
			teaProg.Send(anomalyMsg{Phase: "Add", PLines: i, Lines: lines, Hit: hit, Dur: time.Since(st)})
		}
		if stopSearch {
			break
		}
	}
	vectors = [][]float64{}
	for i, l := range results {
		vectors = append(vectors, tfidf.TermFrequencyInverseDocumentFrequencyForDocument(l))
		if i%100 == 0 {
			teaProg.Send(anomalyMsg{Phase: "TFIDF", PLines: i, Lines: lines, Hit: hit, Dur: time.Since(st)})
		}
	}
}

func anomalySQL() {
	var sqlKeys = []string{
		"&#039", "*", ";", "%20", "--",
		"select", "delete", "create", "drop", "alter",
		"insert", "update", "set", "from", "where",
		"union", "all", "like",
		"and", "&", "or", "|",
		"user", "username", "passwd", "id", "admin", "information_schema",
	}
	for i, l := range results {
		v := getKeywordsVector(&l, &sqlKeys)
		if len(v) == len(sqlKeys) {
			vectors = append(vectors, v)
		}
		if i%100 == 0 {
			teaProg.Send(anomalyMsg{Phase: "Add", PLines: i, Lines: lines, Hit: hit, Dur: time.Since(st)})
		}
	}
}

func anomalyOS() {
	var oscmdKeys = []string{
		"rm%20", "cat%20", "wget%20",
		"curl%20", "sudo%20", "ssh%20",
		"usermod%20", "useradd%20", "grep%20", "ls%20",
		";", "|", "&",
		"/bin", "/dev", "/home", "/lib", "/misc", "/opt",
		"/root", "/tftpboot", "/usr", "/boot", "/etc", "/initrd",
		"/lost+found", "/mnt", "/proc", "/sbin", "/tmp", "/var",
	}
	for i, l := range results {
		v := getKeywordsVector(&l, &oscmdKeys)
		if len(v) == len(oscmdKeys) {
			vectors = append(vectors, v)
		}
		if i%100 == 0 {
			teaProg.Send(anomalyMsg{Phase: "Add", PLines: i, Lines: lines, Hit: hit, Dur: time.Since(st)})
		}
	}
}

func anomalyDir() {
	var dirTraversalKeys = []string{
		"../", "..\\", ":\\",
		"/bin", "/dev", "/home", "/lib", "/misc", "/opt",
		"/root", "/tftpboot", "/usr", "/boot", "/etc/", "/initrd",
		"/lost+found", "/mnt", "/proc", "/sbin", "/tmp", "/var",
	}
	for i, l := range results {
		v := getKeywordsVector(&l, &dirTraversalKeys)
		if len(v) == len(dirTraversalKeys) {
			vectors = append(vectors, v)
		}
		if i%100 == 0 {
			teaProg.Send(anomalyMsg{Phase: "Add", PLines: i, Lines: lines, Hit: hit, Dur: time.Since(st)})
		}
	}
}

func anomalyNumber() {
	pre := ""
	suf := ""
	if extract != "" {
		a := strings.SplitN(extract, "*", 2)
		if len(a) > 1 {
			pre = a[0]
			suf = a[1]
		} else {
			pre = a[0]
		}
	}
	extPat := regexp.MustCompile(`[-+0-9.]+`)
	for i, l := range results {
		if pre != "" {
			a := strings.SplitN(l, pre, 2)
			if len(a) != 2 {
				continue
			}
			l = a[1]
		}
		if suf != "" {
			a := strings.SplitN(l, suf, 2)
			if len(a) != 2 {
				continue
			}
			l = a[0]
		}
		a := extPat.FindAllString(l, -1)
		v := []float64{}
		for _, vs := range a {
			if vf, err := strconv.ParseFloat(vs, 64); err == nil {
				v = append(v, vf)
			}
		}
		if len(vectors) > 0 && len(vectors[0]) != len(v) {
			log.Fatalf("extract number length mismatch %s", l)
		}
		vectors = append(vectors, v)
		if i%100 == 0 {
			teaProg.Send(anomalyMsg{Phase: "Add", PLines: i, Lines: lines, Hit: hit, Dur: time.Since(st)})
		}
	}
}

func anomalyWalu() {
	for i, l := range results {
		v := getWaluVector(&l)
		if len(v) > 20 {
			vectors = append(vectors, v)
		}
		if i%100 == 0 {
			teaProg.Send(anomalyMsg{Phase: "Add", PLines: i, Lines: lines, Hit: hit, Dur: time.Since(st)})
		}
	}
}

// getKeywordsVector : キーワードのりストから特徴ベクターを作成する
func getKeywordsVector(s *string, keys *[]string) []float64 {
	vector := []float64{}
	for _, k := range *keys {
		vector = append(vector, float64(strings.Count(*s, k)))
	}
	return vector
}

// https://github.com/Kanatoko/Walu
func getWaluVector(s *string) []float64 {
	vector := []float64{}
	a := strings.Split(*s, "\"")
	if len(a) < 2 {
		return vector
	}
	query := ""
	path := ""
	f := strings.Fields(a[1])
	if len(f) > 1 {
		ua := strings.SplitN(f[1], "?", 2)
		if len(ua) > 1 {
			path = ua[0]
			query = ua[1]
		}
	}

	ca := getCharCount(a[1])

	//findex_%
	vector = append(vector, float64(strings.Index(a[1], "%")))

	//findex_:
	vector = append(vector, float64(strings.Index(a[1], ":")))

	// countedCharArray
	for _, c := range []rune{':', '(', ';', '%', '/', '\'', '<', '?', '.', '#'} {
		vector = append(vector, float64(ca[c]))
	}

	//encoded =
	vector = append(vector, float64(strings.Count(a[1], "%3D")+strings.Count(a[1], "%3d")))

	//encoded /
	vector = append(vector, float64(strings.Count(a[1], "%2F")+strings.Count(a[1], "%2f")))

	//encoded \
	vector = append(vector, float64(strings.Count(a[1], "%5C")+strings.Count(a[1], "%5c")))

	//encoded %
	vector = append(vector, float64(strings.Count(a[1], "%25")))

	//%20
	vector = append(vector, float64(strings.Count(a[1], "%20")))

	//POST
	if strings.HasPrefix(a[1], "POST") {
		vector = append(vector, 1)
	} else {
		vector = append(vector, 0)
	}

	//path_nonalnum_count
	vector = append(vector, float64(len(path)-getAlphaNumCount(path)))

	//pvalue_nonalnum_avg
	vector = append(vector, float64(len(query)-getAlphaNumCount(query)))

	//non_alnum_len(max_len)
	vector = append(vector, float64(getMaxNonAlnumLength(a[1])))

	//non_alnum_count
	vector = append(vector, float64(getNonAlnumCount(a[1])))

	for _, p := range []string{"/%", "//", "/.", "..", "=/", "./", "/?"} {
		vector = append(vector, float64(strings.Count(a[1], p)))
	}
	return vector
}

func getCharCount(s string) []int {
	ret := []int{}
	for i := 0; i < 96; i++ {
		ret = append(ret, 0)
	}
	for _, c := range s {
		if 33 <= c && c <= 95 {
			ret[c] += 1
		}
	}
	return ret
}

func getAlphaNumCount(s string) int {
	ret := 0
	for _, c := range s {
		if 65 <= c && c <= 90 {
			ret++
		} else if 97 <= c && c <= 122 {
			ret++
		} else if 48 <= c && c <= 57 {
			ret++
		}
	}
	return ret
}

func getMaxNonAlnumLength(s string) int {
	max := 0
	length := 0
	for _, c := range s {
		if ('a' <= c && c <= 'z') || ('A' <= c && c <= 'Z') || ('0' <= c && c <= '9') {
			if length > max {
				max = length
			}
			length = 0
		} else {
			length++
		}
	}
	if max < length {
		max = length
	}
	return max
}

func getNonAlnumCount(s string) int {
	ret := 0
	for _, c := range s {
		if ('a' <= c && c <= 'z') || ('A' <= c && c <= 'Z') || ('0' <= c && c <= '9') {
		} else {
			ret++
		}
	}
	return ret
}
