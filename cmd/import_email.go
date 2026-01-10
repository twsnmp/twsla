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
	"bytes"
	"fmt"
	"io"
	"log"
	"net/mail"
	"net/url"
	"strconv"
	"strings"

	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapclient"
	"github.com/knadh/go-pop3"
)

func importEMailFile(path string, r io.Reader) {
	totalFiles++
	hash := getSHA1(path)
	st, et := getTimeRange()
	if stopImport {
		return
	}
	msg, err := mail.ReadMessage(r)
	if err != nil {
		teaProg.Send(ImportMsg{
			Done: false,
			Path: path,
			Skip: 1,
		})
		return
	}
	ts, err := msg.Header.Date()
	t := ts.UnixNano()
	if st > t || et < t {
		teaProg.Send(ImportMsg{
			Done: false,
			Path: path,
			Skip: 1,
		})
		return
	}
	a := []string{}
	for k, va := range msg.Header {
		for _, v := range va {
			a = append(a, fmt.Sprintf("%s: %s", k, v))
		}
	}
	l := strings.Join(a, "\r\n")
	l += "\r\n"
	if importFilter != nil && !importFilter.MatchString(l) {
		teaProg.Send(ImportMsg{
			Done: false,
			Path: path,
			Skip: 1,
		})
		return
	}
	totalLines += len(a)
	totalBytes += int64(len(l))
	logCh <- &LogEnt{
		Time: t,
		Log:  l,
		Hash: hash,
		Line: len(a),
	}
	teaProg.Send(ImportMsg{
		Done:  false,
		Path:  path,
		Bytes: int64(len(l)),
		Lines: len(a),
	})
}

func doListIMAPFolder() {
	c, err := getIMAPClient()
	if err != nil {
		log.Fatalf("IMAP failed to dial: %v", err)
	}
	defer c.Logout()
	if err := c.Login(emailUser, emailPassword).Wait(); err != nil {
		log.Fatalf("failed to login: %v", err)
	}
	mailboxes, err := c.List("", "*", nil).Collect()
	if err != nil {
		log.Fatalf("failed to list mailboxes: %v", err)
	}
	for _, mbox := range mailboxes {
		fmt.Printf("- %v\n", mbox.Mailbox)
	}
}

func importEMailIMAP() {
	c, err := getIMAPClient()
	if err != nil {
		log.Fatalln(err)
	}
	defer c.Logout()
	if err := c.Login(emailUser, emailPassword).Wait(); err != nil {
		log.Fatalln(err)
	}
	u, err := url.Parse(source)
	if err != nil {
		log.Fatalln(err)
	}
	mbox := "INBOX"
	if len(u.Path) > 1 {
		mbox = u.Path[1:]
	}
	status, err := c.Select(mbox, nil).Wait()
	if err != nil {
		log.Fatalln(err)
	}
	if status.NumMessages == 0 {
		log.Fatalf("mail box %s is empty", mbox)
	}
	var seqSet imap.SeqSet
	seqSet.AddRange(1, status.NumMessages)
	headerSection := imap.FetchItemBodySection{Specifier: imap.PartSpecifierHeader}
	fetchOptions := &imap.FetchOptions{
		Envelope:    true,
		BodySection: []*imap.FetchItemBodySection{&headerSection},
	}
	fetchCmd := c.Fetch(seqSet, fetchOptions)
	defer fetchCmd.Close()
	for {
		msg := fetchCmd.Next()
		if msg == nil {
			break
		}
		buf, err := msg.Collect()
		if err != nil {
			log.Printf("failed to collect message: %v", err)
			continue
		}
		headerData := buf.FindBodySection(&headerSection)
		if len(headerData) < 1 {
			continue
		}
		importEMailFile(fmt.Sprintf("%s#%d", source, msg.SeqNum), bytes.NewReader(headerData))
	}

}

func getIMAPClient() (*imapclient.Client, error) {
	u, err := url.Parse(source)
	if err != nil {
		return nil, err
	}
	server := u.Host
	if !strings.Contains(server, ":") {
		if u.Scheme == "imaps" {
			server += ":993"
		} else {
			server += ":143"
		}
	}
	if emailUser == "" && u.User != nil {
		emailUser = u.User.Username()
		if p, ok := u.User.Password(); ok {
			emailPassword = p
		}
	}
	if u.Scheme == "imaps" || strings.HasSuffix(server, ":993") {
		return imapclient.DialTLS(server, nil)
	}
	if emailTLS {
		return imapclient.DialStartTLS(server, nil)
	}
	return imapclient.DialInsecure(server, nil)
}

func importEMailPOP3() {
	conn, err := getPOP3Conn()
	if err != nil {
		log.Fatalln(err)
	}
	defer conn.Quit()

	if err := conn.Auth(emailUser, emailPassword); err != nil {
		log.Fatal(err)
	}
	count, _, _ := conn.Stat()
	if count == 0 {
		log.Fatalln("no messages on server")
	}
	for i := 1; i < count+1; i++ {
		msg, err := conn.Retr(count)
		if err != nil {
			log.Fatal(err)
		}
		a := []string{}
		for k, v := range msg.Header.Map() {
			for _, val := range v {
				a = append(a, fmt.Sprintf("%s: %s", k, val))
			}
		}
		importEMailFile(fmt.Sprintf("%s#%d", source, i), strings.NewReader(strings.Join(a, "\r\n")+"\r\n"))
	}
}

func getPOP3Conn() (*pop3.Conn, error) {
	u, err := url.Parse(source)
	if err != nil {
		return nil, err
	}
	server := u.Hostname()
	port := u.Port()
	if port == "" {
		if u.Scheme == "pop3s" {
			port = ":995"
		} else {
			port = ":110"
		}
	}
	nport, err := strconv.Atoi(port)
	if err != nil {
		return nil, err
	}
	if emailUser == "" && u.User != nil {
		emailUser = u.User.Username()
		if p, ok := u.User.Password(); ok {
			emailPassword = p
		}
	}
	p := pop3.New(pop3.Opt{
		Host:       server,
		Port:       nport,
		TLSEnabled: u.Scheme == "pop3s" || nport == 995,
	})
	return p.NewConn()
}
