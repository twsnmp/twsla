/*
Copyright © 2026 Masayuki Yamai <twsnmp@gmail.com>

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
	"io"
	"log"
	"mime"
	"net"
	"net/mail"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"blitiri.com.ar/go/spf"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/table"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/dustin/go-humanize"
	"github.com/muesli/reflow/wordwrap"
	"github.com/spf13/cobra"
	"go.etcd.io/bbolt"
	"golang.org/x/text/encoding/japanese"
	"golang.org/x/text/transform"
)

// Count by
var emailCountBy string
var checkSPF = false

var emailSPFMap = make(map[string]string)

// emailCmd represents the email command
var emailCmd = &cobra.Command{
	Use:   "email [search|count]",
	Short: "Search or count email logs",
	Long: `Search or count email logs from the database.
It provides subcommands to search for specific emails or count emails by various fields such as From, To, Subject, Sender IP, and SPF status.

Examples:
  twsla email search -t "last 1h"
  twsla email count --emailCountBy from -t "last 24h"`,
	Args: func(cmd *cobra.Command, args []string) error {
		// Optionally run one of the validators provided by cobra
		if err := cobra.MinimumNArgs(1)(cmd, args); err != nil {
			return err
		}
		switch args[0] {
		case "search":
		case "count":
		default:
			return fmt.Errorf("invalid subcommand specified: %s", args[0])
		}
		return nil
	},
	Run: func(cmd *cobra.Command, args []string) {
		setupFilter(args[1:])
		switch args[0] {
		case "search":
			emailSearchMain()
		case "count":
			emailCountMain()
		}
	},
}

func init() {
	rootCmd.AddCommand(emailCmd)
	emailCmd.Flags().StringVar(&emailCountBy, "emailCountBy", "time", "Count by field")
	emailCmd.Flags().BoolVar(&checkSPF, "checkSPF", false, "Check SPF")
}

type emailSearchDataEnt struct {
	Time       string
	From       string
	To         string
	Subject    string
	RetrunPath string
	SenderIP   string
	Domain     string
	SPF        string
	SPFList    string
	Relay      int
	Delay      int
	Msg        *mail.Message
	Log        *string
}

var emailSearchList = []emailSearchDataEnt{}

type emailSearchMsg struct {
	Done  bool
	Lines int
	Hit   int
	Dur   time.Duration
}

func emailSearchMain() {
	st = time.Now()
	if err := openDB(); err != nil {
		log.Fatalln(err)
	}
	defer db.Close()
	loadEmailSPFMap()
	teaProg = tea.NewProgram(initEmailSearchModel())
	var wg sync.WaitGroup
	wg.Add(1)
	go emailSearchSub(&wg)
	if _, err := teaProg.Run(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	wg.Wait()
	saveEmailSPFMap()
}

func emailSearchSub(wg *sync.WaitGroup) {
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
			i++
			l := string(v)
			email := getMailInfo(&l)
			if matchFilter(&l) {
				email.Time = time.Unix(0, t).Format("2006/01/02 15:04")
				emailSearchList = append(emailSearchList, *email)

			}
			teaProg.Send(emailSearchMsg{Lines: i, Hit: len(emailSearchList), Dur: time.Since(st)})
			if stopSearch {
				break
			}
		}
		return nil
	})
	teaProg.Send(emailSearchMsg{Done: true, Lines: i, Hit: hit, Dur: time.Since(st)})
}

type emailSearchModel struct {
	spinner  spinner.Model
	table    table.Model
	viewport viewport.Model
	done     bool
	log      string
	lastSort string
	quitting bool
	msg      emailSearchMsg
}

func initEmailSearchModel() emailSearchModel {
	columns := []table.Column{
		{Title: "Time"},
		{Title: "From"},
		{Title: "Subject"},
		{Title: "SPF"},
		{Title: "Log"},
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
	vp := viewport.New(0, 0)
	return emailSearchModel{spinner: s, table: t, viewport: vp}
}

func (m emailSearchModel) Init() tea.Cmd {
	return m.spinner.Tick
}

var emailSearchRows = []table.Row{}

func (m emailSearchModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.table.SetWidth(msg.Width - 6)
		m.table.SetHeight(msg.Height - 6)
		w := m.table.Width() - 4
		columns := getColumns(w)
		m.table.SetColumns(columns)
		m.viewport.Width = msg.Width
		m.viewport.Height = msg.Height
	}
	if m.log != "" {
		switch msg := msg.(type) {
		case tea.KeyMsg:
			switch msg.String() {
			case "q", "esc", "ctrl+c", "enter":
				m.log = ""
				return m, nil
			}
		}
		var cmd tea.Cmd
		m.viewport, cmd = m.viewport.Update(msg)
		return m, cmd
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
		case "s", "t", "d", "r":
			if m.done {
				k := msg.String()
				if k == m.lastSort {
					// Reverse
					for i, j := 0, len(emailSearchRows)-1; i < j; i, j = i+1, j-1 {
						emailSearchRows[i], emailSearchRows[j] = emailSearchRows[j], emailSearchRows[i]
					}
				} else {
					m.lastSort = k
					switch k {
					case "s":
						sort.Slice(emailSearchList, func(i, j int) bool {
							return emailSearchList[i].SPF < emailSearchList[j].SPF
						})
					case "t":
						sort.Slice(emailSearchList, func(i, j int) bool {
							return emailSearchList[i].Time < emailSearchList[j].Time
						})
					case "d":
						sort.Slice(emailSearchList, func(i, j int) bool {
							return emailSearchList[i].Delay < emailSearchList[j].Delay
						})
					case "r":
						sort.Slice(emailSearchList, func(i, j int) bool {
							return emailSearchList[i].Relay < emailSearchList[j].Relay
						})
					}
					emailSearchRows = []table.Row{}
					for _, r := range emailSearchList {
						emailSearchRows = append(emailSearchRows, []string{
							r.Time,
							r.From,
							r.Subject,
							time.Duration(time.Second * time.Duration(r.Delay)).String(),
							fmt.Sprintf("%10s", humanize.Comma(int64(r.Relay))),
							r.SPF,
							*r.Log,
						})
					}
				}
				m.table.SetRows(emailSearchRows)
			}
			return m, nil
		case "enter":
			if m.done {
				if sel := m.table.SelectedRow(); sel != nil {
					m.log = wordwrap.String(sel[6], m.viewport.Width)
					m.viewport.SetContent(m.log)
					m.viewport.GotoTop()
				}
			}
		default:
			if !m.done {
				return m, nil
			}
		}
	case emailSearchMsg:
		if msg.Done {
			w := m.table.Width() - 4
			columns := getColumns(w)
			m.table.SetColumns(columns)
			emailSearchRows = []table.Row{}
			for _, r := range emailSearchList {
				emailSearchRows = append(emailSearchRows, []string{
					r.Time,
					r.From,
					r.Subject,
					time.Duration(time.Second * time.Duration(r.Delay)).String(),
					fmt.Sprintf("%10s", humanize.Comma(int64(r.Relay))),
					r.SPF,
					*r.Log,
				})
			}
			m.table.SetRows(emailSearchRows)
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

func getColumns(w int) []table.Column {
	return []table.Column{
		{Title: "Time", Width: 15 * w / 100},
		{Title: "From", Width: 20 * w / 100},
		{Title: "Subject", Width: 30 * w / 100},
		{Title: "Delay", Width: 10 * w / 100},
		{Title: "Relay", Width: 10 * w / 100},
		{Title: "SPF", Width: 15 * w / 100},
		{Title: "Log", Width: 0},
	}
}

func (m emailSearchModel) View() string {
	if m.done {
		if m.log != "" {
			return m.viewport.View()
		}
		return fmt.Sprintf("%s\n%s\n", m.headerView(), baseStyle.Render(m.table.View()))
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

func (m emailSearchModel) headerView() string {
	title := titleStyle.Render(fmt.Sprintf("Results %d/%d s:%s", m.msg.Hit, m.msg.Lines, m.msg.Dur.Truncate(time.Millisecond)))
	help := helpStyle("enter: trace / t|s|d|r: Sort / q : Quit") + "  "
	gap := strings.Repeat(" ", max(0, m.table.Width()-lipgloss.Width(title)-lipgloss.Width(help)))
	return lipgloss.JoinHorizontal(lipgloss.Center, title, gap, help)
}

// email count command
func emailCountMain() {
	st = time.Now()
	if err := openDB(); err != nil {
		log.Fatalln(err)
	}
	defer db.Close()
	loadEmailSPFMap()
	teaProg = tea.NewProgram(initCountModel())
	var wg sync.WaitGroup
	wg.Add(1)
	go emailCountSub(&wg)
	if _, err := teaProg.Run(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	wg.Wait()
	saveEmailSPFMap()
}

func emailCountSub(wg *sync.WaitGroup) {
	defer wg.Done()
	var countMap = make(map[string]int)
	intv := int64(getInterval()) * 1000 * 1000 * 1000
	sti, eti := getTimeRange()
	sk := fmt.Sprintf("%016x:", sti)
	i := 0
	hit := 0
	mode := 0
	extract = "notTimemode"
	switch emailCountBy {
	case "time":
		extract = ""
		mode = 0
		name = "Time"
	case "to":
		mode = 1
		name = "To"
	case "from":
		mode = 2
		name = "From"
	case "matrix":
		mode = 3
		name = "From->To"
	case "subject":
		name = "Subject"
		mode = 4
	case "ip":
		mode = 5
		name = "Sender IP"
	case "domain":
		mode = 6
		name = "Sender Domain"
	case "spf":
		mode = 7
		name = "SPF"
		checkSPF = true
	case "spf.list":
		mode = 8
		name = "SPF List"
		checkSPF = true
	default:
		name = emailCountBy
		mode = 8
	}
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
			i++
			l := string(v)
			email := getMailInfo(&l)
			if email == nil {
				continue
			}
			if matchFilter(&l) {
				switch mode {
				case 1:
					countMap[email.To]++
				case 2:
					countMap[email.From]++
				case 3:
					countMap[email.From+"=>"+email.To]++
				case 4:
					countMap[email.Subject]++
				case 5:
					countMap[email.SenderIP]++
				case 6:
					countMap[email.Domain]++
				case 7:
					countMap[email.SPF]++
				case 8:
					countMap[email.SPFList]++
				case 9:
					k := email.Msg.Header.Get(emailCountBy)
					countMap[k]++
				case 0:
					// TIME
					d := t / intv
					ck := time.Unix(0, d*intv).Format("2006/01/02 15:04")
					countMap[ck]++
				}
				hit++
			}
			teaProg.Send(SearchMsg{Lines: i, Hit: hit, Dur: time.Since(st)})
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
	if extract == "" {
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

var mimeWordDec = &mime.WordDecoder{
	CharsetReader: func(charset string, input io.Reader) (io.Reader, error) {
		switch strings.ToLower(charset) {
		case "iso-2022-jp":
			return transform.NewReader(input, japanese.ISO2022JP.NewDecoder()), nil
		case "shift_jis":
			return transform.NewReader(input, japanese.ShiftJIS.NewDecoder()), nil
		case "euc-jp":
			return transform.NewReader(input, japanese.EUCJP.NewDecoder()), nil
		}
		return nil, fmt.Errorf("unhandled charset %q", charset)
	},
}

func getMimeDecodedWord(s string) string {
	r, err := mimeWordDec.DecodeHeader(s)
	if err != nil {
		return s
	}
	return r
}

func getMailInfo(l *string) *emailSearchDataEnt {
	r := &emailSearchDataEnt{}
	msg, err := mail.ReadMessage(strings.NewReader(*l))
	if err != nil {
		return nil
	}
	*l += "-------\r\n\r\n"
	r.Msg = msg
	subject := getMimeDecodedWord(msg.Header.Get("Subject"))
	r.Subject = subject
	*l += "Subject:" + subject + "\r\n"
	from := ""
	if list, err := msg.Header.AddressList("From"); err == nil {
		for i, a := range list {
			name := getMimeDecodedWord(a.Name)
			if i > 0 {
				from += ","
			}
			from += fmt.Sprintf("%s <%s>", name, a.Address)
		}
	}
	r.From = from
	*l += "From: " + from + "\r\n"
	to := ""
	if list, err := msg.Header.AddressList("To"); err == nil {
		for i, a := range list {
			name := getMimeDecodedWord(a.Name)
			if i > 0 {
				to += ","
			}
			to += fmt.Sprintf("%s <%s>", name, a.Address)
		}
	}
	*l += "To:" + to + "\r\n"
	r.To = to
	returnPath := msg.Header.Get("Return-Path")
	if returnPath == "" {
		if list, err := msg.Header.AddressList("From"); err == nil && len(list) > 0 {
			returnPath = list[0].Address
		}
	}
	r.RetrunPath = returnPath
	*l += "Return-Path: " + returnPath + "\r\n"
	domain := extractDomain(returnPath)
	r.Domain = domain
	*l += "Domain: " + domain + "\r\n"
	spfs := []string{}
	mt, errdate := msg.Header.Date()
	r.Delay = 0
	r.Relay = 0
	for i, v := range msg.Header["Received"] {
		if errdate == nil {
			if rt, err := extractTimestamp(v); err == nil {
				dt := int(rt.Unix() - mt.Unix())
				if dt > r.Delay {
					r.Delay = dt
				}
				*l += fmt.Sprintf("Received[%d]: %v; %s\n\r", i, rt.Sub(mt), v)
			}
		}
		if !strings.Contains(v, "qmail") && !strings.Contains(v, "invoked") && !strings.Contains(v, "with l") {
			//Count non-local transfers
			r.Relay++
		}
		senderIP := extractIP(v)
		if senderIP == nil {
			continue
		}
		r.SenderIP = senderIP.String()
		helo := extractHelo(v)
		if helo == "" {
			helo = domain
		}
		if checkSPF {
			result, e := getSPF(senderIP, helo, returnPath)
			if e != "" {
				if result == "none" || result == "" {
					spfs = append(spfs, e)
					if r.SPF == "" {
						r.SPF = string(result)
					}
				} else {
					r.SPF = fmt.Sprintf("%s:%s", result, e)
					spfs = append(spfs, fmt.Sprintf("%s(%s/%s/%s):%s", result, senderIP, helo, returnPath, e))
				}
				continue
			}
			r.SPF = fmt.Sprintf("%s", result)
			spfs = append(spfs, fmt.Sprintf("%s(%s/%s/%s)", result, senderIP, helo, returnPath))
		}
	}
	r.SPFList = strings.Join(spfs, ",")
	*l += "SPF-List: " + r.SPFList + "\r\n"
	*l += "SPF: " + r.SPF + "\r\n"
	r.Log = l
	return r
}

func extractDomain(addr string) string {
	if strings.Contains(addr, "@") {
		parts := strings.Split(addr, "@")
		domain := strings.Trim(parts[len(parts)-1], "> ")
		return domain
	}
	return addr
}

func extractIP(received string) net.IP {
	re := regexp.MustCompile(`(\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3})`)
	matches := re.FindAllStringSubmatch(received, -1)
	for _, match := range matches {
		if len(match) > 1 {
			ip := net.ParseIP(match[1])
			if ip != nil && ip.IsGlobalUnicast() {
				return ip
			}
		}
	}
	return nil
}

func extractHelo(recived string) string {
	re := regexp.MustCompile(`(?i)from\s+([^\s(]+)`)
	matches := re.FindStringSubmatch(recived)
	if len(matches) > 1 {
		return matches[1]
	}
	return ""
}

func getSPF(ip net.IP, helo, sender string) (string, string) {
	k := fmt.Sprintf("%s/%s/%s", ip, helo, sender)
	if v, ok := emailSPFMap[k]; ok {
		a := strings.SplitN(v, "\t", 2)
		if len(a) == 2 {
			return a[0], a[1]
		}
		return v, ""
	}
	r, err := spf.CheckHostWithSender(ip, helo, sender)
	if err != nil {
		emailSPFMap[k] = fmt.Sprintf("%s\t%v", r, err)
		return string(r), err.Error()
	}
	emailSPFMap[k] = string(r)
	return string(r), ""
}

func loadEmailSPFMap() {
	db.View(func(tx *bbolt.Tx) error {
		b := tx.Bucket([]byte("emailSPF"))
		if b == nil {
			return nil
		}
		b.ForEach(func(k []byte, v []byte) error {
			emailSPFMap[string(k)] = string(v)
			return nil
		})
		return nil
	})
}

func saveEmailSPFMap() {
	db.Batch(func(tx *bbolt.Tx) error {
		b, err := tx.CreateBucketIfNotExists([]byte("emailSPF"))
		if err != nil {
			log.Fatalln(err)
		}
		for k, v := range emailSPFMap {
			b.Put([]byte(k), []byte(v))
		}
		return nil
	})
}

func extractTimestamp(header string) (time.Time, error) {
	parts := strings.Split(header, ";")
	if len(parts) < 2 {
		return time.Time{}, fmt.Errorf("; not found")
	}
	dateStr := strings.TrimSpace(parts[len(parts)-1])

	// (JST) may contain comments, so remove unnecessary parts using regular expressions
	// *mail.ParseDate may fail if comments are included
	re := regexp.MustCompile(`\s*\(.*\)$`)
	dateStr = re.ReplaceAllString(dateStr, "")

	// Parsed as RFC 822 format (RFC 1123, 2822, etc.)

	t, err := mail.ParseDate(dateStr)
	if err != nil {
		return time.Time{}, fmt.Errorf("failed to parse date: %v", err)
	}
	return t, nil
}
