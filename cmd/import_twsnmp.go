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
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/twsnmp/twsnmpfc/client"
	"github.com/viant/afs/scp"
	"golang.org/x/crypto/ssh"
)

func importFromTWSNMP() {
	if apiMode {
		importFromTWSNMPByAPI()
		return
	}
	importFromTWSNMPBySSH()
}

func importFromTWSNMPBySSH() {
	st, et := getTimeRange()
	for ct := st; ct >= 0 && ct < et && !stopImport; {
		ct = importFromTWSNMPSSHSub(ct, et)
	}
}

func importFromTWSNMPSSHSub(st, et int64) int64 {
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
	plog := false
	cmd := fmt.Sprintf("get %s %d 1000", logType, st)
	if strings.HasPrefix(logType, "polling:") {
		tmp := strings.SplitN(logType, ":", 2)
		if len(tmp) != 2 {
			teaProg.Send(fmt.Errorf(""))
			return 0
		}
		cmd = fmt.Sprintf("get plog %s csv", tmp[1])
		plog = true
	}
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
		var a []string
		var t int64
		if plog {
			if strings.HasPrefix(l, "Time,") {
				continue
			}
			ts, ok, _ := tg.Extract([]byte(l))
			if !ok {
				continue
			}
			t = ts.UnixNano()
			a = append(a, "")
			a = append(a, l)
		} else {
			a = strings.SplitN(l, "\t", 2)
			if len(a) != 2 {
				continue
			}
			t, err = strconv.ParseInt(a[0], 10, 64)
			if err != nil {
				continue
			}
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
		pl := a[1]
		if logType == "syslog" {
			pl = parseTWSNMPSyslog(t, a[1])
			if pl == "" {
				pl = a[1]
			}
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

func parseTWSNMPSyslog(t int64, l string) string {
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

var ErrorNotSupportedLogType = errors.New("not supported log type")

func importFromTWSNMPByAPI() {
	if apiTLS {
		source = strings.Replace(source, "twsnmp:", "https:", 1)
	} else {
		source = strings.Replace(source, "twsnmp:", "http:", 1)
	}
	u, err := url.Parse(source)
	if err != nil {
		teaProg.Send(err)
		return
	}
	c := client.NewClient(fmt.Sprintf("%s://%s", u.Scheme, u.Host))
	c.InsecureSkipVerify = apiSkip
	c.Timeout = 60
	userID := u.User.Username()
	if userID == "" {
		userID = "twsnmp"
	}
	passwd, ok := u.User.Password()
	if !ok || passwd == "" {
		passwd = "twsnmp"
	}
	err = c.Login(userID, passwd)
	if err != nil {
		teaProg.Send(err)
		return
	}
	switch logType {
	case "eventlog":
		importEventLogFromTWSNMPByAPI(c)
	case "trap":
		importTrapFromTWSNMPByAPI(c)
	case "syslog":
		importSyslogFromTWSNMPByAPI(c)
	case "netflow", "ipfix":
		importNetFlowFromTWSNMPByAPI(c)
	case "sflow":
		importSFlowFromTWSNMPByAPI(c)
	case "sflowCounter":
		importSFlowCounterFromTWSNMPByAPI(c)
	case "arp":
		importArpLogFromTWSNMPByAPI(c)
	default:
		if strings.HasPrefix(logType, "polling") {
			importPollingLogFromTWSNMPByAPI(c)
			return
		}
		teaProg.Send(ErrorNotSupportedLogType)
	}
}

func importEventLogFromTWSNMPByAPI(c *client.TWSNMPApi) {
	st, et := getTimeRange()
	f := &client.EventLogFilter{
		StartDate: time.Unix(0, st).Format("2006-01-02"),
		StartTime: time.Unix(0, st).Format("15:04"),
		EndDate:   time.Unix(0, et).Format("2006-01-02"),
		EndTime:   time.Unix(0, et).Format("15:04"),
	}

	hash := getSHA1(source)
	lastTime := int64(0)
	readBytes := int64(0)
	readLines := 0
	skipLines := 0
	path := fmt.Sprintf("%s %s", source, logType)
	r, err := c.GetEventLogs(f)
	if err != nil {
		teaProg.Send(err)
		return
	}
	totalFiles++
	for i, l := range r.EventLogs {
		sl := fmt.Sprintf("%s %s '%s' %s", time.Unix(0, l.Time).Format(time.RFC3339Nano), l.Type, l.NodeName, l.Event)
		readBytes += int64(len(sl))
		totalBytes += int64(len(sl))
		readLines++
		totalLines++
		if importFilter != nil && !importFilter.MatchString(sl) {
			skipLines++
			continue
		}
		d := 0
		if lastTime > 0 {
			d = int(l.Time - lastTime)
		}
		lastTime = l.Time
		if st > l.Time || et < l.Time {
			skipLines++
			continue
		}
		logCh <- &LogEnt{
			Time:  l.Time,
			Log:   sl,
			Hash:  hash,
			Line:  readLines,
			Delta: d,
		}
		if i%100 == 0 {
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
		Done:  true,
		Path:  path,
		Bytes: readBytes,
		Lines: readLines,
		Skip:  skipLines,
	})
}

func importTrapFromTWSNMPByAPI(c *client.TWSNMPApi) {
	st, et := getTimeRange()
	f := &client.SnmpTrapFilter{
		StartDate: time.Unix(0, st).Format("2006-01-02"),
		StartTime: time.Unix(0, st).Format("15:04"),
		EndDate:   time.Unix(0, et).Format("2006-01-02"),
		EndTime:   time.Unix(0, et).Format("15:04"),
	}

	hash := getSHA1(source)
	lastTime := int64(0)
	readBytes := int64(0)
	readLines := 0
	skipLines := 0
	path := fmt.Sprintf("%s %s", source, logType)
	traps, err := c.GetSnmpTraps(f)
	if err != nil {
		teaProg.Send(err)
		return
	}
	totalFiles++
	for i, l := range traps {
		sl := fmt.Sprintf("%s %s %s %s", time.Unix(0, l.Time).Format(time.RFC3339Nano), l.FromAddress, l.TrapType, l.Variables)
		readBytes += int64(len(sl))
		totalBytes += int64(len(sl))
		readLines++
		totalLines++
		if importFilter != nil && !importFilter.MatchString(sl) {
			skipLines++
			continue
		}
		d := 0
		if lastTime > 0 {
			d = int(l.Time - lastTime)
		}
		lastTime = l.Time
		if st > l.Time || et < l.Time {
			skipLines++
			continue
		}
		logCh <- &LogEnt{
			Time:  l.Time,
			Log:   sl,
			Hash:  hash,
			Line:  readLines,
			Delta: d,
		}
		if i%100 == 0 {
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
		Done:  true,
		Path:  path,
		Bytes: readBytes,
		Lines: readLines,
		Skip:  skipLines,
	})
}

func importSyslogFromTWSNMPByAPI(c *client.TWSNMPApi) {
	st, et := getTimeRange()
	f := &client.SyslogFilter{
		NextTime: st,
		Filter:   0,
	}
	hash := getSHA1(source)
	lastTime := int64(0)
	readBytes := int64(0)
	readLines := 0
	skipLines := 0
	path := fmt.Sprintf("%s %s", source, logType)
	for ct := st; ct >= 0 && ct < et; {
		f.NextTime = ct
		r, err := c.GetSyslogs(f)
		if err != nil {
			teaProg.Send(err)
			return
		}
		totalFiles++
		readBytes = int64(0)
		readLines = 0
		skipLines = 0
		for _, l := range r.Logs {
			sl := fmt.Sprintf("%s %s %s: %s", time.Unix(0, l.Time).Format(time.RFC3339Nano), l.Type, l.Tag, l.Message)
			readBytes += int64(len(sl))
			totalBytes += int64(len(sl))
			readLines++
			totalLines++
			if importFilter != nil && !importFilter.MatchString(sl) {
				skipLines++
				continue
			}
			d := 0
			if lastTime > 0 {
				d = int(l.Time - lastTime)
			}
			lastTime = l.Time
			if st > l.Time || et < l.Time {
				skipLines++
				continue
			}
			ct = l.Time
			logCh <- &LogEnt{
				Time:  l.Time,
				Log:   sl,
				Hash:  hash,
				Line:  readLines,
				Delta: d,
			}
		}
		teaProg.Send(ImportMsg{
			Done:  false,
			Path:  path,
			Bytes: readBytes,
			Lines: readLines,
			Skip:  skipLines,
		})
		if r.NextTime == 0 {
			break
		}
	}
	teaProg.Send(ImportMsg{
		Done:  true,
		Path:  path,
		Bytes: readBytes,
		Lines: readLines,
		Skip:  skipLines,
	})
}

func importNetFlowFromTWSNMPByAPI(c *client.TWSNMPApi) {
	st, et := getTimeRange()
	f := &client.NetflowFilter{
		NextTime: st,
		Filter:   0,
	}
	ipfix := logType == "ipfix"
	hash := getSHA1(source)
	lastTime := int64(0)
	readBytes := int64(0)
	readLines := 0
	skipLines := 0
	path := fmt.Sprintf("%s %s", source, logType)
	for ct := st; ct >= 0 && ct < et; {
		f.NextTime = ct
		var err error
		var r *client.NetflowWebAPI
		if ipfix {
			r, err = c.GetIPFIX(f)
		} else {
			r, err = c.GetNetFlow(f)
		}
		if err != nil {
			teaProg.Send(err)
			return
		}
		totalFiles++
		readBytes = int64(0)
		readLines = 0
		skipLines = 0
		for _, l := range r.Logs {
			j, err := json.Marshal(&l)
			if err != nil {
				skipLines++
				continue
			}
			sl := fmt.Sprintf("%s %s", time.Unix(0, l.Time).Format(time.RFC3339Nano), string(j))
			readBytes += int64(len(sl))
			totalBytes += int64(len(sl))
			readLines++
			totalLines++
			if importFilter != nil && !importFilter.MatchString(sl) {
				skipLines++
				continue
			}
			d := 0
			if lastTime > 0 {
				d = int(l.Time - lastTime)
			}
			lastTime = l.Time
			if st > l.Time || et < l.Time {
				skipLines++
				continue
			}
			ct = l.Time
			logCh <- &LogEnt{
				Time:  l.Time,
				Log:   sl,
				Hash:  hash,
				Line:  readLines,
				Delta: d,
			}
		}
		teaProg.Send(ImportMsg{
			Done:  false,
			Path:  path,
			Bytes: readBytes,
			Lines: readLines,
			Skip:  skipLines,
		})
		if r.NextTime == 0 {
			break
		}
	}
	teaProg.Send(ImportMsg{
		Done:  true,
		Path:  path,
		Bytes: readBytes,
		Lines: readLines,
		Skip:  skipLines,
	})
}

// TODO: clientパッケージ側の修正が必要
func importSFlowFromTWSNMPByAPI(c *client.TWSNMPApi) {
	st, et := getTimeRange()
	f := &client.SFlowFilter{
		NextTime: st,
		Filter:   0,
	}
	hash := getSHA1(source)
	lastTime := int64(0)
	readBytes := int64(0)
	readLines := 0
	skipLines := 0
	path := fmt.Sprintf("%s %s", source, logType)
	for ct := st; ct >= 0 && ct < et; {
		f.NextTime = ct
		r, err := c.GetSFlow(f)
		if err != nil {
			teaProg.Send(err)
			return
		}
		totalFiles++
		readBytes = int64(0)
		readLines = 0
		skipLines = 0
		for _, l := range r.Logs {
			j, err := json.Marshal(&l)
			if err != nil {
				skipLines++
				continue
			}
			sl := fmt.Sprintf("%s %s", time.Unix(0, l.Time).Format(time.RFC3339Nano), string(j))
			readBytes += int64(len(sl))
			totalBytes += int64(len(sl))
			readLines++
			totalLines++
			if importFilter != nil && !importFilter.MatchString(sl) {
				skipLines++
				continue
			}
			d := 0
			if lastTime > 0 {
				d = int(l.Time - lastTime)
			}
			lastTime = l.Time
			if st > l.Time || et < l.Time {
				skipLines++
				continue
			}
			ct = l.Time
			logCh <- &LogEnt{
				Time:  l.Time,
				Log:   sl,
				Hash:  hash,
				Line:  readLines,
				Delta: d,
			}
		}
		teaProg.Send(ImportMsg{
			Done:  false,
			Path:  path,
			Bytes: readBytes,
			Lines: readLines,
			Skip:  skipLines,
		})
		if r.NextTime == 0 {
			break
		}
	}
	teaProg.Send(ImportMsg{
		Done:  true,
		Path:  path,
		Bytes: readBytes,
		Lines: readLines,
		Skip:  skipLines,
	})
}

func importSFlowCounterFromTWSNMPByAPI(c *client.TWSNMPApi) {
	st, et := getTimeRange()
	f := &client.SFlowCounterFilter{
		NextTime: st,
		Filter:   0,
	}
	hash := getSHA1(source)
	lastTime := int64(0)
	readBytes := int64(0)
	readLines := 0
	skipLines := 0
	path := fmt.Sprintf("%s %s", source, logType)
	for ct := st; ct >= 0 && ct < et; {
		f.NextTime = ct
		r, err := c.GetSFlowCounter(f)
		if err != nil {
			teaProg.Send(err)
			return
		}
		totalFiles++
		readBytes = int64(0)
		readLines = 0
		skipLines = 0
		for _, l := range r.Logs {
			j, err := json.Marshal(&l)
			if err != nil {
				skipLines++
				continue
			}
			sl := fmt.Sprintf("%s %s", time.Unix(0, l.Time).Format(time.RFC3339Nano), string(j))
			readBytes += int64(len(sl))
			totalBytes += int64(len(sl))
			readLines++
			totalLines++
			if importFilter != nil && !importFilter.MatchString(sl) {
				skipLines++
				continue
			}
			d := 0
			if lastTime > 0 {
				d = int(l.Time - lastTime)
			}
			lastTime = l.Time
			if st > l.Time || et < l.Time {
				skipLines++
				continue
			}
			ct = l.Time
			logCh <- &LogEnt{
				Time:  l.Time,
				Log:   sl,
				Hash:  hash,
				Line:  readLines,
				Delta: d,
			}
		}
		teaProg.Send(ImportMsg{
			Done:  false,
			Path:  path,
			Bytes: readBytes,
			Lines: readLines,
			Skip:  skipLines,
		})
		if r.NextTime == 0 {
			break
		}
	}
	teaProg.Send(ImportMsg{
		Done:  true,
		Path:  path,
		Bytes: readBytes,
		Lines: readLines,
		Skip:  skipLines,
	})
}

func importArpLogFromTWSNMPByAPI(c *client.TWSNMPApi) {
	st, et := getTimeRange()
	f := &client.ArpFilter{
		StartDate: time.Unix(0, st).Format("2006-01-02"),
		StartTime: time.Unix(0, st).Format("15:04"),
		EndDate:   time.Unix(0, et).Format("2006-01-02"),
		EndTime:   time.Unix(0, et).Format("15:04"),
	}

	hash := getSHA1(source)
	lastTime := int64(0)
	readBytes := int64(0)
	readLines := 0
	skipLines := 0
	path := fmt.Sprintf("%s %s", source, logType)
	arpLogs, err := c.GetArpLogs(f)
	if err != nil {
		teaProg.Send(err)
		return
	}
	totalFiles++
	for i, l := range arpLogs {
		j, err := json.Marshal(&l)
		if err != nil {
			skipLines++
			continue
		}
		sl := fmt.Sprintf("%s %s", time.Unix(0, l.Time).Format(time.RFC3339Nano), string(j))
		readBytes += int64(len(sl))
		totalBytes += int64(len(sl))
		readLines++
		totalLines++
		if importFilter != nil && !importFilter.MatchString(sl) {
			skipLines++
			continue
		}
		d := 0
		if lastTime > 0 {
			d = int(l.Time - lastTime)
		}
		lastTime = l.Time
		if st > l.Time || et < l.Time {
			skipLines++
			continue
		}
		logCh <- &LogEnt{
			Time:  l.Time,
			Log:   sl,
			Hash:  hash,
			Line:  readLines,
			Delta: d,
		}
		if i%100 == 0 {
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
		Done:  true,
		Path:  path,
		Bytes: readBytes,
		Lines: readLines,
		Skip:  skipLines,
	})
}

func importPollingLogFromTWSNMPByAPI(c *client.TWSNMPApi) {
	a := strings.SplitN(logType, ":", 2)
	if len(a) != 2 {
		teaProg.Send(fmt.Errorf("no polling id"))
		return
	}
	st, et := getTimeRange()
	f := &client.TimeFilter{
		StartDate: time.Unix(0, st).Format("2006-01-02"),
		StartTime: time.Unix(0, st).Format("15:04"),
		EndDate:   time.Unix(0, et).Format("2006-01-02"),
		EndTime:   time.Unix(0, et).Format("15:04"),
	}

	hash := getSHA1(source)
	lastTime := int64(0)
	readBytes := int64(0)
	readLines := 0
	skipLines := 0
	path := fmt.Sprintf("%s %s", source, logType)
	logs, err := c.GetPollingLogs(a[1], f)
	if err != nil {
		teaProg.Send(err)
		return
	}
	totalFiles++
	for i, l := range logs {
		j, err := json.Marshal(&l)
		if err != nil {
			skipLines++
			continue
		}
		sl := fmt.Sprintf("%s %s", time.Unix(0, l.Time).Format(time.RFC3339Nano), string(j))
		readBytes += int64(len(sl))
		totalBytes += int64(len(sl))
		readLines++
		totalLines++
		if importFilter != nil && !importFilter.MatchString(sl) {
			skipLines++
			continue
		}
		d := 0
		if lastTime > 0 {
			d = int(l.Time - lastTime)
		}
		lastTime = l.Time
		if st > l.Time || et < l.Time {
			skipLines++
			continue
		}
		logCh <- &LogEnt{
			Time:  l.Time,
			Log:   sl,
			Hash:  hash,
			Line:  readLines,
			Delta: d,
		}
		if i%100 == 0 {
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
		Done:  true,
		Path:  path,
		Bytes: readBytes,
		Lines: readLines,
		Skip:  skipLines,
	})
}
