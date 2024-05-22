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
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/araddon/dateparse"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/xhit/go-str2duration/v2"
	"go.etcd.io/bbolt"
)

var dataStore string
var timeRange string
var simpleFilter string
var regexpFilter string
var extract string
var delimiter string
var interval int

// common data
type errMsg error

var db *bbolt.DB
var teaProg *tea.Program
var st time.Time

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

func getExtPat() *extPatEnt {
	if extract == "" {
		return nil
	}
	p := ""
	s := ""
	e := extract
	i := 1
	a := strings.SplitN(extract, delimiter, 4)
	switch len(a) {
	case 4:
		p = a[0]
		s = a[1]
		e = a[2]
		i, _ = strconv.Atoi(a[3])
	case 3:
		if v, err := strconv.Atoi(a[2]); err != nil || v < 1 {
			p = a[0]
			e = a[1]
			s = a[2]
		} else {
			p = a[0]
			e = a[1]
			i = v
		}
	case 2:
		if v, err := strconv.Atoi(a[1]); err != nil || v < 1 {
			p = a[0]
			e = a[1]
		} else {
			e = a[0]
			i = v
		}
	}
	if i < 1 {
		i = 1
	}
	e = strings.ToLower(e)
	switch e {
	case "num", "number":
		p += `([-+0-9.]+)`
	case "ip":
		p += `([0-9]{1,3}\.[0-9]{1,3}\.[0-9]{1,3}\.[0-9]{1,3})`
	case "mac":
		p += `([0-9a-fA-F]{2}:[0-9a-fA-F]{2}:[0-9a-fA-F]{2}:[0-9a-fA-F]{2}:[0-9a-fA-F]{2}:[0-9a-fA-F]{2})`
	case "email":
		p += `([a-zA-Z0-9_.+-]+@[a-zA-Z0-9_.+-]+)`
	case "word":
		p += `(\S+)`
	default:
		return nil
	}
	r, err := regexp.Compile(p + s)
	if err != nil {
		log.Fatalln(err)
	}
	return &extPatEnt{
		ExtReg: r,
		Index:  i,
	}
}
