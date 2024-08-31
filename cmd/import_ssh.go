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
	"bytes"
	"compress/gzip"
	"context"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/viant/afs/scp"
	"golang.org/x/crypto/ssh"
)

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
