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
	"net"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/araddon/dateparse"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/domainr/dnsr"
	"github.com/elastic/go-grok"
	"github.com/oschwald/geoip2-golang"
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
var geoipDBPath string
var ipInfoMode string
var geoipDB *geoip2.Reader

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
	switch f {
	case "#LOCAL_IP":
		return regexpLocalIP
	case "#IP":
		return regexpIP
	case "#IPV6":
		return regexpIPv6
	case "#EMAIL":
		return regexpEMail
	case "#URL":
		return regexpURL
	case "#MAC":
		return regexpMAC
	case "#CREDITCARD":
		return regexpCreditCard
	case "#MYNUMBER":
		return regexpMyNumber
	case "#PHONE_JP":
		return regexpPhoneJP
	case "#PHONE_US":
		return regexpPhoneUS
	case "#PHONE_INTL":
		return regexpPhoneIntl
	case "#ZIP_JP":
		return regexpZipJP
	case "#UUID":
		return regexpUUID
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
	et := time.Now().AddDate(1, 0, 0)
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

func setExtPat() error {
	if extract == "" {
		return nil
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
	case "ipv6":
		e = `((?:[0-9a-fA-F]{1,4}:){7}[0-9a-fA-F]{1,4}|(?::{1,2}[0-9a-fA-F]{1,4}){1,7}|[0-9a-fA-F]{1,4}:(?::{1,2}[0-9a-fA-F]{1,4}){1,7})`
	case "mac":
		e = `([0-9a-fA-F]{2}:[0-9a-fA-F]{2}:[0-9a-fA-F]{2}:[0-9a-fA-F]{2}:[0-9a-fA-F]{2}:[0-9a-fA-F]{2})`
	case "email":
		e = `([a-zA-Z0-9_.+-]+@[a-zA-Z0-9_.+-]+)`
	case "creditcard":
		e = `(\b(?:4[0-9]{12}(?:[0-9]{3})?|5[1-5][0-9]{14}|3[47][0-9]{13}|3(?:0[0-5]|[68][0-9])[0-9]{11}|6(?:011|5[0-9]{2})[0-9]{12}|(?:2131|1800|35\d{3})\d{11})\b|\b(?:\d{4}[- ]){3}\d{4}\b)`
	case "mynumber":
		e = `(\b\d{12}\b)`
	case "phone_jp":
		e = `(\b(?:0\d{1,4}-\d{1,4}-\d{4}|0\d{9,10})\b)`
	case "phone_us":
		e = `(\b(?:\+?1[-. ]?)?\(?([0-9]{3})\)?[-. ]?([0-9]{3})[-. ]?([0-9]{4})\b)`
	case "phone_intl":
		e = `(\b\+(?:[0-9] ?){6,14}[0-9]\b)`
	case "zip_jp":
		e = `(\b\d{3}-\d{4}\b)`
	case "uuid":
		e = `([0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12})`
	default:
		e = strings.ReplaceAll(e, "%{number}", `([-+0-9.]+)`)
		e = strings.ReplaceAll(e, "%{ip}", `([0-9]{1,3}\.[0-9]{1,3}\.[0-9]{1,3}\.[0-9]{1,3})`)
		e = strings.ReplaceAll(e, "%{ipv6}", `((?:[0-9a-fA-F]{1,4}:){7}[0-9a-fA-F]{1,4}|(?::{1,2}[0-9a-fA-F]{1,4}){1,7}|[0-9a-fA-F]{1,4}:(?::{1,2}[0-9a-fA-F]{1,4}){1,7})`)
		e = strings.ReplaceAll(e, "%{mac}", `([0-9a-fA-F]{2}:[0-9a-fA-F]{2}:[0-9a-fA-F]{2}:[0-9a-fA-F]{2}:[0-9a-fA-F]{2}:[0-9a-fA-F]{2})`)
		e = strings.ReplaceAll(e, "%{email}", `([a-zA-Z0-9_.+-]+@[a-zA-Z0-9_.+-]+)`)
		e = strings.ReplaceAll(e, "%{creditcard}", `(\b(?:4[0-9]{12}(?:[0-9]{3})?|5[1-5][0-9]{14}|3[47][0-9]{13}|3(?:0[0-5]|[68][0-9])[0-9]{11}|6(?:011|5[0-9]{2})[0-9]{12}|(?:2131|1800|35\d{3})\d{11})\b|\b(?:\d{4}[- ]){3}\d{4}\b)`)
		e = strings.ReplaceAll(e, "%{mynumber}", `(\b\d{12}\b)`)
		e = strings.ReplaceAll(e, "%{phone_jp}", `(\b(?:0\d{1,4}-\d{1,4}-\d{4}|0\d{9,10})\b)`)
		e = strings.ReplaceAll(e, "%{phone_us}", `(\b(?:\+?1[-. ]?)?\(?([0-9]{3})\)?[-. ]?([0-9]{3})[-. ]?([0-9]{4})\b)`)
		e = strings.ReplaceAll(e, "%{phone_intl}", `(\b\+(?:[0-9] ?){6,14}[0-9]\b)`)
		e = strings.ReplaceAll(e, "%{zip_jp}", `(\b\d{3}-\d{4}\b)`)
		e = strings.ReplaceAll(e, "%{uuid}", `([0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12})`)
		e = strings.ReplaceAll(e, "%{word}", `(\S+)`)
	}

	r, err := regexp.Compile(e)
	if err != nil {
		return err
	}
	extPat = &extPatEnt{
		ExtReg: r,
		Index:  pos,
	}
	return nil
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
var regexpLocalIP = regexp.MustCompile(`\b(10\.\d{1,3}\.\d{1,3}\.\d{1,3}|172\.(1[6-9]|2\d|3[0-1])\.\d{1,3}\.\d{1,3}|192\.168\.\d{1,3}\.\d{1,3}|127\.0\.0\.1)\b`)
var regexpIPv6 = regexp.MustCompile(`([0-9a-fA-F]{1,4}:){7}[0-9a-fA-F]{1,4}|(:{1,2}[0-9a-fA-F]{1,4}){1,7}|[0-9a-fA-F]{1,4}:(:{1,2}[0-9a-fA-F]{1,4}){1,7}`)
var regexpCreditCard = regexp.MustCompile(`\b(?:4[0-9]{12}(?:[0-9]{3})?|5[1-5][0-9]{14}|3[47][0-9]{13}|3(?:0[0-5]|[68][0-9])[0-9]{11}|6(?:011|5[0-9]{2})[0-9]{12}|(?:2131|1800|35\d{3})\d{11})\b|\b(?:\d{4}[- ]){3}\d{4}\b`)
var regexpMyNumber = regexp.MustCompile(`\b\d{12}\b`)
var regexpPhoneJP = regexp.MustCompile(`\b(0\d{1,4}-\d{1,4}-\d{4}|0\d{9,10})\b`)
var regexpPhoneUS = regexp.MustCompile(`\b(?:\+?1[-. ]?)?\(?([0-9]{3})\)?[-. ]?([0-9]{3})[-. ]?([0-9]{4})\b`)
var regexpPhoneIntl = regexp.MustCompile(`\b\+(?:[0-9] ?){6,14}[0-9]\b`)
var regexpZipJP = regexp.MustCompile(`\b\d{3}-\d{4}\b`)
var regexpUUID = regexp.MustCompile(`[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}`)

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

func openGeoIPDB() error {
	if geoipDBPath == "" {
		return fmt.Errorf("no geoip path")
	}
	var err error
	geoipDB, err = geoip2.Open(geoipDBPath)
	return err
}

var ip2GeoMap = make(map[string]*geoip2.City)

func getIPLoc(sip string) *geoip2.City {
	if g, ok := ip2GeoMap[sip]; ok {
		return g
	}
	ip := net.ParseIP(sip)
	record, err := geoipDB.City(ip)
	if err != nil {
		return nil
	}
	ip2GeoMap[sip] = record
	return record
}

var dnsResolver *dnsr.Resolver

func getIPInfoMode() int {
	switch ipInfoMode {
	case "host":
		dnsResolver = dnsr.NewWithTimeout(10000, time.Millisecond*1000)
		return 1
	case "domain":
		dnsResolver = dnsr.NewWithTimeout(10000, time.Millisecond*1000)
		return 2
	case "loc":
		if err := openGeoIPDB(); err != nil {
			log.Fatalln(err)
		}
		return 3
	case "country":
		if err := openGeoIPDB(); err != nil {
			log.Fatalln(err)
		}
		return 4
	default:
		return 0
	}
}

var ip2Host = make(map[string]string)

func getHostByIP(ip string) string {
	if h, ok := ip2Host[ip]; ok {
		return h
	}
	a := strings.SplitN(ip, ".", 4)
	if len(a) == 4 {
		for _, rr := range dnsResolver.Resolve(fmt.Sprintf("%s.%s.%s.%s.in-addr.arpa", a[3], a[2], a[1], a[0]), "PTR") {
			if rr.Type == "PTR" {
				ip2Host[ip] = rr.Value
				return rr.Value
			}
		}
	}
	h := fmt.Sprintf("%s(unknown)", ip)
	ip2Host[ip] = h
	return h
}

func getIPInfo(ip string, mode int) string {
	switch mode {
	case 1:
		// host
		return getHostByIP(ip)
	case 2:
		// domain
		h := getHostByIP(ip)
		if !strings.HasSuffix(h, "wn)") {
			return h
		}
		a := strings.Split(h, ".")
		if len(a) < 4 {
			return h
		}
		return strings.Join(a[1:], ".")
	case 3:
		// loc
		if r := getIPLoc(ip); r != nil {
			return fmt.Sprintf("%s:%s:%0.3f,%0.3f", r.Country.IsoCode, r.City.Names["en"], r.Location.Latitude, r.Location.Longitude)
		}
		return "unknown"
	case 4:
		// country
		if r := getIPLoc(ip); r != nil {
			return r.Country.IsoCode
		}
		return "unknown"
	}
	return ip
}

func maskPII(s string) string {
	s = regexpIPv6.ReplaceAllString(s, "<IP>")
	s = regexpIP.ReplaceAllString(s, "<IP>")
	s = regexpEMail.ReplaceAllString(s, "<EMAIL>")
	s = regexpMAC.ReplaceAllString(s, "<MAC>")
	s = regexpCreditCard.ReplaceAllString(s, "<CARD>")
	s = regexpPhoneJP.ReplaceAllString(s, "<PHONE>")
	s = regexpPhoneUS.ReplaceAllString(s, "<PHONE>")
	s = regexpPhoneIntl.ReplaceAllString(s, "<PHONE>")
	s = regexpZipJP.ReplaceAllString(s, "<ZIP>")
	s = regexpUUID.ReplaceAllString(s, "<UUID>")
	return s
}
