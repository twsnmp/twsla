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
	"regexp"
	"strings"
	"time"

	"github.com/araddon/dateparse"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/elastic/go-grok"
	"github.com/xhit/go-str2duration/v2"
	"go.etcd.io/bbolt"
)

var dataStore string
var timeRange string
var simpleFilter string
var regexpFilter string
var notFilter string
var extract string
var interval int
var pos int

// common data
type errMsg error

var db *bbolt.DB
var teaProg *tea.Program
var st time.Time
var gr *grok.Grok
var extPat *extPatEnt
var name string
var grokPat string
var grokDef string

// Style
var titleStyle = lipgloss.NewStyle().
	Bold(true).
	Foreground(lipgloss.Color("#FAFAFA")).
	Background(lipgloss.Color("#7D56F4")).
	PaddingLeft(2).
	PaddingRight(2)

var infoStyle = lipgloss.NewStyle().
	Foreground(lipgloss.Color("#FAFAFA")).
	Background(lipgloss.Color("#7D56F4")).
	PaddingLeft(2).
	PaddingRight(2)

var baseStyle = lipgloss.NewStyle().
	BorderStyle(lipgloss.NormalBorder()).
	BorderForeground(lipgloss.Color("240"))

var markStyle = lipgloss.NewStyle().
	Foreground(lipgloss.Color("#FAFAFA")).
	Background(lipgloss.Color("#c00000"))

// openDB : open bbolt DB
func openDB() error {
	var err error
	db, err = bbolt.Open(dataStore, 0600, &bbolt.Options{Timeout: 3 * time.Second})
	if err != nil {
		return err
	}
	return db.Update(func(tx *bbolt.Tx) error {
		if _, err := tx.CreateBucketIfNotExists([]byte("logs")); err != nil {
			return fmt.Errorf("create bucket: %s", err)
		}
		if _, err := tx.CreateBucketIfNotExists([]byte("delta")); err != nil {
			return fmt.Errorf("create bucket: %s", err)
		}
		return nil
	})
}

// getSimpleFilter : get filter from like test* test?k
func getSimpleFilter(f string) *regexp.Regexp {
	if f == "" {
		return nil
	}
	f = regexp.QuoteMeta(f)
	f = strings.ReplaceAll(f, "\\*", ".*")
	f = strings.ReplaceAll(f, "\\?", ".")
	if strings.HasSuffix(f, "\\$") {
		f = strings.TrimRight(f, "\\$")
		f += "$"
	}
	if r, err := regexp.Compile(f); err == nil {
		return r
	}
	return nil
}

func getFilter(f string) *regexp.Regexp {
	if f == "" {
		return nil
	}
	if r, err := regexp.Compile(f); err == nil {
		return r
	}
	return nil
}

func getTimeRange() (int64, int64) {
	st := time.Unix(0, 0)
	et := time.Now()
	a := strings.SplitN(timeRange, ",", 2)
	if len(a) == 1 && a[0] != "" {
		if d, err := str2duration.ParseDuration(a[0]); err == nil {
			st = et.Add(d * -1)
		} else if t, err := dateparse.ParseLocal(a[0]); err == nil {
			st = t
		}
	} else {
		if t, err := dateparse.ParseLocal(a[0]); err == nil {
			st = t
			if t, err := dateparse.ParseLocal(a[1]); err == nil {
				et = t
			} else if d, err := str2duration.ParseDuration(a[1]); err == nil {
				et = st.Add(d)
			}
		}
	}
	return st.UnixNano(), et.UnixNano()
}

func getInterval() int {
	if interval > 0 {
		return interval
	}
	st, et := getTimeRange()
	ds := (et - st) / (1000 * 1000 * 1000)
	for _, i := range []int{60, 300, 600} {
		if int(ds)/i < 1000 {
			return i
		}
	}
	return 3600
}

type extPatEnt struct {
	ExtReg *regexp.Regexp
	Index  int
}

func setExtPat() {
	if extract == "" {
		return
	}
	if pos < 1 {
		pos = 1
	}
	e := extract
	switch e {
	case "num", "number":
		e = `([-+0-9.]+)`
	case "ip":
		e = `([0-9]{1,3}\.[0-9]{1,3}\.[0-9]{1,3}\.[0-9]{1,3})`
	case "mac":
		e = `([0-9a-fA-F]{2}:[0-9a-fA-F]{2}:[0-9a-fA-F]{2}:[0-9a-fA-F]{2}:[0-9a-fA-F]{2}:[0-9a-fA-F]{2})`
	case "email":
		e = `([a-zA-Z0-9_.+-]+@[a-zA-Z0-9_.+-]+)`
	default:
		e = strings.ReplaceAll(e, "%{number}", `([-+0-9.]+)`)
		e = strings.ReplaceAll(e, "%{ip}", `([0-9]{1,3}\.[0-9]{1,3}\.[0-9]{1,3}\.[0-9]{1,3})`)
		e = strings.ReplaceAll(e, "%{mac}", `([0-9a-fA-F]{2}:[0-9a-fA-F]{2}:[0-9a-fA-F]{2}:[0-9a-fA-F]{2}:[0-9a-fA-F]{2}:[0-9a-fA-F]{2})`)
		e = strings.ReplaceAll(e, "%{email}", `([a-zA-Z0-9_.+-]+@[a-zA-Z0-9_.+-]+)`)
		e = strings.ReplaceAll(e, "%{word}", `(\S+)`)
	}

	r, err := regexp.Compile(e)
	if err != nil {
		log.Fatalln(err)
	}
	extPat = &extPatEnt{
		ExtReg: r,
		Index:  pos,
	}
}

func wrapString(s string, w int) string {
	r := ""
	a := strings.Split(s, "")
	ln := 0
	for _, ss := range a {
		if w < len(ss)+ln {
			r += "\n"
			ln = 0
		}
		ln += len(ss)
		r += ss
	}
	return r
}

// regexp patterns

var regexpIP = regexp.MustCompile(`[0-9]{1,3}\.[0-9]{1,3}\.[0-9]{1,3}\.[0-9]{1,3}`)
var regexpMAC = regexp.MustCompile(`[0-9a-fA-F]{2}:[0-9a-fA-F]{2}:[0-9a-fA-F]{2}:[0-9a-fA-F]{2}:[0-9a-fA-F]{2}:[0-9a-fA-F]{2}`)
var regexpEMail = regexp.MustCompile(`[a-zA-Z0-9_.+-]+@[a-zA-Z0-9_.+-]+`)
var regexpURL = regexp.MustCompile(`https?://[\w!?/+\-_~;.,*&@#$%()'[\]]+`)
var regexpKV = regexp.MustCompile(`\w+=\w+[ ,]?`)
var regexpGrok = regexp.MustCompile(`%\{.+\}`)

// Filters

var filterList []*regexp.Regexp
var notFilterList []*regexp.Regexp

func setupFilter(args []string) {
	filterList = []*regexp.Regexp{}
	notFilterList = []*regexp.Regexp{}
	if regexpFilter != "" {
		filterList = append(filterList, getFilter(regexpFilter))
	}
	if simpleFilter != "" {
		filterList = append(filterList, getSimpleFilter(simpleFilter))
	}
	for _, s := range args {
		if s != "" {
			if strings.HasPrefix(s, "^") {
				notFilterList = append(notFilterList, getSimpleFilter(s[1:]))
			} else {
				filterList = append(filterList, getSimpleFilter(s))
			}
		}
	}
	if notFilter != "" {
		notFilterList = append(notFilterList, getFilter(notFilter))
	}
}

func matchFilter(l *string) bool {
	for _, f := range filterList {
		if !f.MatchString(*l) {
			return false
		}
	}
	for _, f := range notFilterList {
		if f.MatchString(*l) {
			return false
		}
	}
	return true
}

// GROK

func setGrok() {
	if grokPat == "" {
		return
	}
	var err error
	switch grokDef {
	case "full":
		gr, err = grok.NewComplete()
		if err != nil {
			log.Fatalln(err)
		}
	case "":
		gr = grok.New()
	default:
		if c, err := os.ReadFile(grokDef); err != nil {
			log.Fatalln(err)
		} else {
			gr = grok.New()
			for _, l := range strings.Split(string(c), "\n") {
				a := strings.SplitN(l, " ", 2)
				if len(a) != 2 {
					continue
				}
				gr.AddPattern(strings.TrimSpace(a[0]), strings.TrimSpace(a[1]))
			}
		}
	}
	pat := grokPat
	if !regexpGrok.MatchString(pat) {
		pat = fmt.Sprintf("%%{%s}", pat)
	}
	err = gr.Compile(pat, false)
	if err != nil {
		log.Fatalln(err)
	}
}
