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
	"os"
	"strings"

	"github.com/0xrawsec/golang-evtx/evtx"
)

type winLogEnt struct {
	Name   string
	String bool
	Path   evtx.GoEvtxPath
}

var winLogList = []winLogEnt{
	{Name: "EventID", Path: evtx.EventIDPath},
	{Name: "EventID", Path: evtx.EventIDPath2},
	{Name: "Level", Path: evtx.Path("/Event/System/Level")},
	{Name: "RecordID", Path: evtx.EventRecordIDPath},
	{Name: "Channel", Path: evtx.ChannelPath, String: true},
	{Name: "Provider", Path: evtx.Path("/Event/System/Provider/Name"), String: true},
	{Name: "Computer", Path: evtx.Path("/Event/System/Computer"), String: true},
	{Name: "UserID", Path: evtx.UserIDPath, String: true},
}

func importFromWindowsEvtx(path string) {
	r, err := os.Open(path)
	if err != nil {
		teaProg.Send(err)
		return
	}
	defer r.Close()
	ef, err := evtx.New(r)
	if err == nil {
		err = ef.Header.Verify()
	}
	if err != nil {
		err = ef.Header.Repair(r)
		if err != nil {
			teaProg.Send(err)
			return
		}
	}
	totalFiles++
	hash := getSHA1(path)
	readBytes := int64(0)
	st, et := getTimeRange()
	readLines := 0
	skipLines := 0
	i := 0
	for e := range ef.FastEvents() {
		if stopImport {
			return
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
		if e == nil {
			skipLines++
			continue
		}
		readLines++
		totalLines++
		syst, err := e.GetTime(&evtx.SystemTimePath)
		if err != nil {
			skipLines++
			continue
		}
		t := syst.UnixNano()
		l := ""
		if jsonMode {
			j, err := json.MarshalIndent(e, "", " ")
			if err != nil {
				skipLines++
				continue
			}
			l = string(j)
			l = strings.TrimSpace(l)
			l = strings.ReplaceAll(l, "\"", "")
			l = strings.ReplaceAll(l, "\\\\", "/")
		} else {
			a := []string{}
			a = append(a, syst.Format("2006-01-02T15:04:05.000"))
			for _, w := range winLogList {
				if w.String {
					if s, err := e.GetString(&w.Path); err == nil {
						a = append(a, fmt.Sprintf("%s=%s", w.Name, s))
					}
				} else {
					if v, err := e.GetInt(&w.Path); err == nil {
						a = append(a, fmt.Sprintf("%s=%d", w.Name, v))
					}
				}
			}
			l = strings.Join(a, " ")
		}
		readBytes += int64(len(l))
		totalBytes += int64(len(l))
		if importFilter != nil && !importFilter.MatchString(l) {
			skipLines++
			continue
		}
		if st > t || et < t {
			skipLines++
			continue
		}
		logCh <- &LogEnt{
			Time: t,
			Log:  l,
			Hash: hash,
			Line: readLines,
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
