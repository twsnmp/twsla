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
	"bufio"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/NimbleMarkets/ntcharts/sparkline"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/dustin/go-humanize"
	"github.com/gravwell/gravwell/v3/timegrinder"
	"github.com/spf13/cobra"
	"go.etcd.io/bbolt"
)

var source string
var sources []string
var command string
var filePat string
var sshKey string
var utc bool
var apiMode bool
var apiTLS bool
var apiSkip bool
var logType string
var noDeltaCheck bool
var noTimeStamp bool

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
		if source != "" {
			sources = append(sources, source)
		}
		sources = append(sources, args...)
		importMain()
	},
}

func init() {
	rootCmd.AddCommand(importCmd)
	importCmd.Flags().BoolVar(&utc, "utc", false, "Force UTC")
	importCmd.Flags().BoolVar(&noDeltaCheck, "noDelta", false, "Disable delta check")
	importCmd.Flags().BoolVar(&apiMode, "api", false, "TWSNMP FC API Mode")
	importCmd.Flags().BoolVar(&apiTLS, "tls", false, "TWSNMP FC API TLS")
	importCmd.Flags().BoolVar(&apiSkip, "skip", true, "TWSNMP FC API skip verify certificate")
	importCmd.Flags().BoolVar(&noTimeStamp, "noTS", false, "Import no time stamp file")
	importCmd.Flags().StringVarP(&source, "source", "s", "", "Log source")
	importCmd.Flags().StringVarP(&command, "command", "c", "", "SSH Command")
	importCmd.Flags().StringVarP(&sshKey, "key", "k", "", "SSH Key")
	importCmd.Flags().StringVarP(&filePat, "filePat", "p", "", "File name pattern")
	importCmd.Flags().StringVarP(&logType, "logType", "l", "syslog", "TWSNNP FC log type")
}

func importMain() {
	st = time.Now()
	if err := openDB(); err != nil {
		log.Fatalln(err)
	}
	defer db.Close()
	teaProg = tea.NewProgram(initImportModel())
	setupTimeGrinder()
	logCh = make(chan *LogEnt, 10000)
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
	for _, src := range sources {
		source = src
		importOne()
	}
	teaProg.Send(ImportMsg{Done: true})
}

func importOne() {
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
	if !utc {
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
		var t int64
		if noTimeStamp {
			t = time.Now().UnixNano()
		} else {
			ts, ok, _ := tg.Extract([]byte(l))
			if !ok {
				continue
			}
			t = ts.UnixNano()
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
		if !noDeltaCheck {
			if lastTime > 0 {
				d = int(t - lastTime)
			}
			lastTime = t
		}
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

	const batchSize = 10000 // 適切なバッチサイズに調整
	logsBuffer := make([]struct {
		ID    []byte
		Log   []byte
		Delta []byte // Deltaが存在する場合のみ使用
	}, 0, batchSize)

	for l := range logCh {
		id := []byte(fmt.Sprintf("%016x:%s:%x", l.Time, l.Hash, l.Line))
		logsBuffer = append(logsBuffer, struct {
			ID    []byte
			Log   []byte
			Delta []byte
		}{ID: id, Log: []byte(l.Log), Delta: nil})

		if l.Delta < 0 {
			logsBuffer[len(logsBuffer)-1].Delta = []byte(fmt.Sprintf("%d", l.Delta))
		}

		if len(logsBuffer) >= batchSize {
			if err := db.Batch(func(tx *bbolt.Tx) error {
				bl := tx.Bucket([]byte("logs"))
				bd := tx.Bucket([]byte("delta"))
				for _, data := range logsBuffer {
					if err := bl.Put(data.ID, data.Log); err != nil {
						return err
					}
					if data.Delta != nil {
						if err := bd.Put(data.ID, data.Delta); err != nil {
							return err
						}
					}
				}
				return nil
			}); err != nil {
				log.Printf("Error during batch commit: %v\n", err)
			}
			logsBuffer = logsBuffer[:0] // バッファをクリア
		}
	}

	// チャネルが閉じられた後に残っているログを処理
	if len(logsBuffer) > 0 {
		if err := db.Batch(func(tx *bbolt.Tx) error {
			bl := tx.Bucket([]byte("logs"))
			bd := tx.Bucket([]byte("delta"))
			for _, data := range logsBuffer {
				if err := bl.Put(data.ID, data.Log); err != nil {
					return err
				}
				if data.Delta != nil {
					if err := bd.Put(data.ID, data.Delta); err != nil {
						return err
					}
				}
			}
			return nil
		}); err != nil {
			log.Printf("Error during final batch commit: %v\n", err)
		}
	}
}

func getSHA1(str string) string {
	sha1 := sha1.New()
	io.WriteString(sha1, str)
	return hex.EncodeToString(sha1.Sum(nil))[:2]
}

func getCommand() string {
	if command != "twsnmp" {
		return command
	}
	return ""
}

type importModel struct {
	spinner  spinner.Model
	sl       sparkline.Model
	quitting bool
	err      error
	msg      ImportMsg
}

func initImportModel() importModel {
	s := spinner.New()
	s.Spinner = spinner.Line
	s.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("#00efff"))
	sl := sparkline.New(50, 1, sparkline.WithStyle(lipgloss.NewStyle().Foreground(lipgloss.Color("#0099ff"))))
	return importModel{spinner: s, sl: sl}
}

func (m importModel) Init() tea.Cmd {
	return m.spinner.Tick
}

func (m importModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.sl = sparkline.New(msg.Width-15, 1, sparkline.WithStyle(lipgloss.NewStyle().Foreground(lipgloss.Color("#0099ff"))))
		return m, nil
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
var importStatsStyle = lipgloss.NewStyle().Border(lipgloss.NormalBorder())

func (m importModel) View() string {
	if m.err != nil {
		return "\n" + errorStyle(m.err.Error()) + "\n"
	}
	d := time.Now().Unix() - st.Unix()
	if d > 0 {
		d = int64(totalBytes) / d
		m.sl.Push(float64(d))
		m.sl.Draw()
	}
	str := fmt.Sprintf("%s Loading path=%s line=%s byte=%s\n  Total file=%s line=%s byte=%s time=%v\n%s %s/Sec",
		m.spinner.View(),
		m.msg.Path,
		humanize.Comma(int64(m.msg.Lines)),
		humanize.Bytes(uint64(m.msg.Bytes)),
		humanize.Comma(int64(totalFiles)),
		humanize.Comma(int64(totalLines)),
		humanize.Bytes(uint64(totalBytes)),
		time.Since(st),
		m.sl.View(),
		humanize.Bytes(uint64(d)),
	)
	str = importStatsStyle.Render(str)
	if m.quitting {
		return str + "\n"
	}
	return str + "\n\n" + helpStyle("Press q to quit") + "\n"
}
