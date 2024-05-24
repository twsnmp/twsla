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
	"archive/tar"
	"archive/zip"
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/dustin/go-humanize"
	"github.com/gravwell/gravwell/v3/timegrinder"
	"github.com/spf13/cobra"
	"github.com/viant/afs/scp"
	"go.etcd.io/bbolt"
	"golang.org/x/crypto/ssh"
)

var source string
var command string
var filePat string
var sshKey string
var utc bool

var tg *timegrinder.TimeGrinder
var importFilter *regexp.Regexp

type LogEnt struct {
	Time  int64
	Log   string
	Hash  string
	Line  int
	Delta int
}

type ImportMsg struct {
	Done  bool
	Path  string
	Bytes int64
	Lines int
	Skip  int
}

var stopImport bool
var logCh chan *LogEnt
var totalFiles int
var totalLines int
var totalBytes int64

// importCmd represents the import command
var importCmd = &cobra.Command{
	Use:   "import",
	Short: "Import log from source",
	Long: `Import log from source
source is file | dir | scp | ssh | twsnmp
`,
	Run: func(cmd *cobra.Command, args []string) {
		importMain()
	},
}

func init() {
	rootCmd.AddCommand(importCmd)
	importCmd.Flags().BoolVar(&utc, "utc", false, "Force UTC")
	importCmd.Flags().StringVarP(&source, "source", "s", "", "Log source")
	importCmd.Flags().StringVarP(&command, "command", "c", "", "SSH Command")
	importCmd.Flags().StringVarP(&sshKey, "key", "k", "", "SSH Key")
	importCmd.Flags().StringVarP(&filePat, "filePat", "p", "", "File name pattern")
}

func importMain() {
	st = time.Now()
	if err := openDB(); err != nil {
		log.Fatalln(err)
	}
	defer db.Close()
	teaProg = tea.NewProgram(initImportModel())
	setupTimeGrinder()
	logCh = make(chan *LogEnt, 1000)
	var wg sync.WaitGroup
	wg.Add(1)
	go importSub(&wg)
	wg.Add(1)
	go logSaver(&wg)
	if _, err := teaProg.Run(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	close(logCh)
	wg.Wait()
}

func importSub(wg *sync.WaitGroup) {
	defer wg.Done()
	switch getSourceType() {
	case "file":
		importFromFile(source)
	case "dir":
		importFromDir()
	case "scp":
		importFromSCP()
	case "ssh":
		importFromSSH()
	case "twsnmp":
		importFromTWSNMP()
	default:
		teaProg.Send(fmt.Errorf("invalid source"))
		return
	}
	teaProg.Send(ImportMsg{Done: true})
}

func getSourceType() string {
	if strings.HasPrefix(source, "scp:") {
		return "scp"
	}
	if strings.HasPrefix(source, "ssh:") {
		return "ssh"
	}
	if strings.HasPrefix(source, "twsnmp:") {
		return "twsnmp"
	}
	s, err := os.Stat(source)
	if err != nil {
		return ""
	}
	if s.IsDir() {
		return "dir"
	}
	return "file"
}

func setupTimeGrinder() error {
	var err error
	tg, err = timegrinder.New(timegrinder.Config{
		EnableLeftMostSeed: true,
	})
	if err != nil {
		return err
	}
	if utc {
		tg.SetLocalTime()
	}
	// [Sun Oct 09 00:36:03 2022]
	if p, err := timegrinder.NewUserProcessor("custom01", `[JFMASOND][anebriyunlgpctov]+\s+\d+\s+\d\d:\d\d:\d\d\s+\d\d\d\d`, "Jan _2 15:04:05 2006"); err == nil && p != nil {
		if _, err := tg.AddProcessor(p); err != nil {
			return err
		}
	} else {
		return err
	}
	// 2022/12/26 5:48:00
	if p, err := timegrinder.NewUserProcessor("custom02", `\d\d\d\d/\d+/\d+\s+\d+:\d\d:\d\d`, "2006/1/2 3:04:05"); err == nil && p != nil {
		if _, err := tg.AddProcessor(p); err != nil {
			return err
		}
	} else {
		return err
	}
	return nil
}

func importFromFile(path string) {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".zip":
		imortFromZIPFile(path)
		return
	case ".tgz":
		importFormTarGZFile(path)
		return
	case ".gz":
		if strings.HasSuffix(path, ".tar.gz") {
			importFormTarGZFile(path)
			return
		}
	}
	r, err := os.Open(path)
	if err != nil {
		log.Panicln(err)
	}
	defer r.Close()
	if ext == ".gz" {
		if gzr, err := gzip.NewReader(r); err == nil {
			doImport(path, gzr)
		}
		return
	}
	doImport(path, r)
}

func imortFromZIPFile(path string) {
	r, err := zip.OpenReader(path)
	if err != nil {
		teaProg.Send(err)
		return
	}
	defer r.Close()
	filter := getSimpleFilter(filePat)
	for _, f := range r.File {
		p := filepath.Base(f.Name)
		if filter != nil && !filter.MatchString(p) {
			continue
		}
		r, err := f.Open()
		if err != nil {
			continue
		}
		ext := strings.ToLower(filepath.Ext(f.Name))
		if ext == ".gz" {
			if gzr, err := gzip.NewReader(r); err == nil {
				doImport(path+":"+f.Name, gzr)
			}
		} else {
			doImport(path+":"+f.Name, r)
		}
	}
}

func importFormTarGZFile(path string) {
	r, err := os.Open(path)
	if err != nil {
		teaProg.Send(err)
		return
	}
	defer r.Close()
	gzr, err := gzip.NewReader(r)
	if err != nil {
		teaProg.Send(err)
		return
	}
	filter := getSimpleFilter(filePat)
	tgzr := tar.NewReader(gzr)
	for {
		f, err := tgzr.Next()
		if err != nil {
			return
		}
		if filter != nil && !filter.MatchString(f.Name) {
			continue
		}
		if strings.HasSuffix(f.Name, ".gz") {
			igzr, err := gzip.NewReader(tgzr)
			if err != nil {
				teaProg.Send(err)
				return
			}
			doImport(path+":"+f.Name, igzr)
		} else {
			doImport(path+":"+f.Name, tgzr)
		}
	}
}

func importFromDir() {
	pat := "*"
	if filePat != "" {
		pat = filePat
	}
	files, err := filepath.Glob(filepath.Join(source, pat))
	if err != nil {
		teaProg.Send(err)
		return
	}
	for _, f := range files {
		importFromFile(f)
	}

}

func importFromSCP() {
	if sshKey == "" {
		sshKey = filepath.Join(os.Getenv("HOME"), ".ssh", "id_rsa")
	} else if strings.HasPrefix(sshKey, "~/") {
		sshKey = strings.Replace(sshKey, "~/", os.Getenv("HOME"), 1)
	}
	u, err := url.Parse(source)
	if err != nil {
		teaProg.Send(err)
		return
	}
	pass, ok := u.User.Password()
	if !ok {
		pass = ""
	}
	auth := scp.NewKeyAuth(sshKey, u.User.Username(), pass)
	provider := scp.NewAuthProvider(auth, nil)
	config, err := provider.ClientConfig()
	if err != nil {
		teaProg.Send(err)
		return
	}
	sv := u.Host
	if !strings.Contains(sv, ":") {
		sv += ":22"
	}
	service, err := scp.NewStorager(sv, time.Duration(time.Second)*3, config)
	if err != nil {
		teaProg.Send(err)
		return
	}
	filter := getSimpleFilter(filePat)
	if filter != nil {
		files, err := service.List(context.Background(), u.Path)
		if err != nil {
			teaProg.Send(err)
			return
		}
		for _, file := range files {
			path := file.Name()
			if !filter.MatchString(path) {
				continue
			}
			r, err := service.Open(context.Background(), filepath.Join(u.Path, path))
			if err != nil {
				teaProg.Send(err)
				return
			}
			ext := strings.ToLower(filepath.Ext(u.Path))
			if ext == ".gz" {
				if gzr, err := gzip.NewReader(r); err == nil {
					doImport(source+path, gzr)
				}
			} else {
				doImport(source+path, r)
			}
			r.Close()
		}
	} else {
		r, err := service.Open(context.Background(), u.Path)
		if err != nil {
			teaProg.Send(err)
			return
		}
		ext := strings.ToLower(filepath.Ext(u.Path))
		if ext == ".gz" {
			if gzr, err := gzip.NewReader(r); err == nil {
				doImport(source, gzr)
			}
		} else {
			doImport(source, r)
		}
		r.Close()
	}
}

func importFromSSH() {
	if sshKey == "" {
		sshKey = filepath.Join(os.Getenv("HOME"), ".ssh", "id_rsa")
	} else if strings.HasPrefix(sshKey, "~/") {
		sshKey = strings.Replace(sshKey, "~/", os.Getenv("HOME"), 1)
	}
	u, err := url.Parse(source)
	if err != nil {
		teaProg.Send(err)
		return
	}
	pass, ok := u.User.Password()
	if !ok {
		pass = ""
	}
	auth := scp.NewKeyAuth(sshKey, u.User.Username(), pass)
	provider := scp.NewAuthProvider(auth, nil)
	config, err := provider.ClientConfig()
	if err != nil {
		teaProg.Send(err)
		return
	}
	sv := u.Host
	if !strings.Contains(sv, ":") {
		sv += ":22"
	}
	conn, err := net.DialTimeout("tcp", sv, time.Duration(60)*time.Second)
	if err != nil {
		teaProg.Send(err)
		return
	}
	if err := conn.SetDeadline(time.Now().Add(time.Second * time.Duration(120))); err != nil {
		teaProg.Send(err)
		return
	}
	c, ch, req, err := ssh.NewClientConn(conn, sv, config)
	if err != nil {
		teaProg.Send(err)
		return
	}
	client := ssh.NewClient(c, ch, req)
	defer client.Close()
	session, err := client.NewSession()
	if err != nil {
		teaProg.Send(err)
		return
	}
	defer session.Close()
	stdout, err := session.Output(getCommand())
	if err != nil {
		teaProg.Send(err)
		return
	}
	r := bytes.NewReader(stdout)
	doImport(sv+":"+command, r)
}

func importFromTWSNMP() {
	st, et := getTimeRange()
	for ct := st; ct > 0 && ct < et; {
		ct = importFromTWSNMPSub(ct, et)
	}
}

func importFromTWSNMPSub(st, et int64) int64 {
	if sshKey == "" {
		sshKey = filepath.Join(os.Getenv("HOME"), ".ssh", "id_rsa")
	} else if strings.HasPrefix(sshKey, "~/") {
		sshKey = strings.Replace(sshKey, "~/", os.Getenv("HOME"), 1)
	}
	u, err := url.Parse(strings.Replace(source, "twsnmp:", "ssh:", 1))
	if err != nil {
		teaProg.Send(err)
		return 0
	}
	pass, ok := u.User.Password()
	if !ok {
		pass = ""
	}
	auth := scp.NewKeyAuth(sshKey, u.User.Username(), pass)
	provider := scp.NewAuthProvider(auth, nil)
	config, err := provider.ClientConfig()
	if err != nil {
		teaProg.Send(err)
		return 0
	}
	sv := u.Host
	if !strings.Contains(sv, ":") {
		sv += ":22"
	}
	conn, err := net.DialTimeout("tcp", sv, time.Duration(60)*time.Second)
	if err != nil {
		teaProg.Send(err)
		return 0
	}
	if err := conn.SetDeadline(time.Now().Add(time.Second * time.Duration(120))); err != nil {
		teaProg.Send(err)
		return 0
	}
	c, ch, req, err := ssh.NewClientConn(conn, sv, config)
	if err != nil {
		teaProg.Send(err)
		return 0
	}
	client := ssh.NewClient(c, ch, req)
	defer client.Close()
	session, err := client.NewSession()
	if err != nil {
		teaProg.Send(err)
		return 0
	}
	defer session.Close()
	cmd := fmt.Sprintf("get syslog %d 1000", st)
	stdout, err := session.Output(cmd)
	if err != nil {
		teaProg.Send(err)
		return 0
	}
	r := bytes.NewReader(stdout)
	totalFiles++
	hash := getSHA1(sv + ":" + cmd)
	lastTime := int64(0)
	readBytes := int64(0)
	readLines := 0
	skipLines := 0
	i := 0
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		if stopImport {
			return 0
		}
		l := scanner.Text()
		a := strings.SplitN(l, "\t", 2)
		if len(a) != 2 {
			continue
		}
		t, err := strconv.ParseInt(a[0], 10, 64)
		if err != nil {
			continue
		}
		readBytes += int64(len(l))
		totalBytes += int64(len(l))
		readLines++
		totalLines++
		if importFilter != nil && !importFilter.MatchString(l) {
			skipLines++
			continue
		}
		d := 0
		if lastTime > 0 {
			d = int(t - lastTime)
		}
		lastTime = t
		if st > t || et < t {
			skipLines++
			continue
		}
		pl := parseTWSNMPLog(t, a[1])
		if pl == "" {
			pl = a[1]
		}
		logCh <- &LogEnt{
			Time:  t,
			Log:   pl,
			Delta: d,
			Hash:  hash,
			Line:  readLines,
		}
		i++
		if i%2000 == 0 {
			teaProg.Send(ImportMsg{
				Done:  false,
				Path:  sv + ":" + cmd,
				Bytes: readBytes,
				Lines: readLines,
				Skip:  skipLines,
			})
		}
	}
	teaProg.Send(ImportMsg{
		Done:  false,
		Path:  sv + ":" + cmd,
		Bytes: readBytes,
		Lines: readLines,
		Skip:  skipLines,
	})
	return lastTime
}

func parseTWSNMPLog(t int64, l string) string {
	var sl = make(map[string]interface{})
	if err := json.Unmarshal([]byte(l), &sl); err != nil {
		return ""
	}
	var ok bool
	var sv float64
	var fac float64
	var host string
	var tag string
	var message string
	if sv, ok = sl["severity"].(float64); !ok {
		return ""
	}
	if fac, ok = sl["facility"].(float64); !ok {
		return ""
	}
	if host, ok = sl["hostname"].(string); !ok {
		return ""
	}
	if tag, ok = sl["tag"].(string); !ok {
		if tag, ok = sl["app_name"].(string); !ok {
			return ""
		}
		message = ""
		for i, k := range []string{"proc_id", "msg_id", "message", "structured_data"} {
			if m, ok := sl[k].(string); ok && m != "" {
				if i > 0 {
					message += " "
				}
				message += m
			}
		}
	} else {
		if message, ok = sl["content"].(string); !ok {
			return ""
		}
	}
	return fmt.Sprintf("%s %s %s %s %s", time.Unix(0, t).Format(time.RFC3339Nano), host, getSyslogType(int(sv), int(fac)), tag, message)
}

var severityNames = []string{
	"emerg",
	"alert",
	"crit",
	"err",
	"warning",
	"notice",
	"info",
	"debug",
}

var facilityNames = []string{
	"kern",
	"user",
	"mail",
	"daemon",
	"auth",
	"syslog",
	"lpr",
	"news",
	"uucp",
	"cron",
	"authpriv",
	"ftp",
	"ntp",
	"logaudit",
	"logalert",
	"clock",
	"local0",
	"local1",
	"local2",
	"local3",
	"local4",
	"local5",
	"local6",
	"local7",
}

func getSyslogType(sv, fac int) string {
	r := ""
	if sv >= 0 && sv < len(severityNames) {
		r += severityNames[sv]
	} else {
		r += "unknown"
	}
	r += ":"
	if fac >= 0 && fac < len(facilityNames) {
		r += facilityNames[fac]
	} else {
		r += "unknown"
	}
	return r
}

func doImport(path string, r io.Reader) {
	totalFiles++
	hash := getSHA1(path)
	lastTime := int64(0)
	readBytes := int64(0)
	st, et := getTimeRange()
	readLines := 0
	skipLines := 0
	i := 0
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		if stopImport {
			return
		}
		l := scanner.Text()
		ts, ok, _ := tg.Extract([]byte(l))
		if !ok {
			continue
		}
		t := ts.UnixNano()
		readBytes += int64(len(l))
		totalBytes += int64(len(l))
		readLines++
		totalLines++
		if importFilter != nil && !importFilter.MatchString(l) {
			skipLines++
			continue
		}
		d := 0
		if lastTime > 0 {
			d = int(t - lastTime)
		}
		lastTime = t
		if st > t || et < t {
			skipLines++
			continue
		}
		logCh <- &LogEnt{
			Time:  t,
			Log:   l,
			Delta: d,
			Hash:  hash,
			Line:  readLines,
		}
		i++
		if i%2000 == 0 {
			teaProg.Send(ImportMsg{
				Done:  false,
				Path:  path,
				Bytes: readBytes,
				Lines: readLines,
				Skip:  skipLines,
			})
		}
	}
	if err := scanner.Err(); err != nil {
		log.Panicln(err)
	}
	teaProg.Send(ImportMsg{
		Done:  false,
		Path:  path,
		Bytes: readBytes,
		Lines: readLines,
		Skip:  skipLines,
	})
}

func logSaver(wg *sync.WaitGroup) {
	defer wg.Done()
	logList := []*LogEnt{}
	for l := range logCh {
		logList = append(logList, l)
		if len(logList) > 1000 {
			saveLog(logList)
			logList = []*LogEnt{}
		}
	}
	if len(logList) > 0 {
		saveLog(logList)
	}
}

func saveLog(logList []*LogEnt) {
	db.Batch(func(tx *bbolt.Tx) error {
		bl := tx.Bucket([]byte("logs"))
		bd := tx.Bucket([]byte("delta"))
		for _, l := range logList {
			id := fmt.Sprintf("%016x:%s:%d", l.Time, l.Hash, l.Line)
			bl.Put([]byte(id), []byte(l.Log))
			if l.Delta < 0 {
				bd.Put([]byte(id), []byte(fmt.Sprintf("%d", l.Delta)))
			}
		}
		return nil
	})
}

func getSHA1(str string) string {
	sha1 := sha1.New()
	io.WriteString(sha1, str)
	return hex.EncodeToString(sha1.Sum(nil))[:8]
}

func getCommand() string {
	if command != "twsnmp" {
		return command
	}
	return ""
}

type importModel struct {
	spinner  spinner.Model
	quitting bool
	err      error
	msg      ImportMsg
}

func initImportModel() importModel {
	s := spinner.New()
	s.Spinner = spinner.Line
	s.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("#00efff"))
	return importModel{spinner: s}
}

func (m importModel) Init() tea.Cmd {
	return m.spinner.Tick
}

func (m importModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "esc", "ctrl+c":
			m.quitting = true
			stopImport = true
			return m, nil
		default:
			return m, nil
		}
	case errMsg:
		m.err = msg
		m.quitting = true
		stopImport = true
		return m, tea.Quit
	case ImportMsg:
		if msg.Done {
			m.quitting = true
			return m, tea.Quit
		}
		m.msg = msg
		return m, nil
	default:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
	}
}

var helpStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#626262")).Render
var errorStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#c00000")).Render

func (m importModel) View() string {
	if m.err != nil {
		return "\n" + errorStyle(m.err.Error()) + "\n"
	}
	str := fmt.Sprintf("\n%s Loading path=%s line=%s byte=%s\n  Total file=%s line=%s byte=%s time=%v",
		m.spinner.View(),
		m.msg.Path,
		humanize.Comma(int64(m.msg.Lines)),
		humanize.Bytes(uint64(m.msg.Bytes)),
		humanize.Comma(int64(totalFiles)),
		humanize.Comma(int64(totalLines)),
		humanize.Bytes(uint64(totalBytes)),
		time.Since(st),
	)
	if m.quitting {
		return str + "\n"
	}
	return str + "\n\n" + helpStyle("Press q to quit") + "\n"
}
