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
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/viant/afs/scp"
	"golang.org/x/crypto/ssh"
)

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
