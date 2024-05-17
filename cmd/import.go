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
	"fmt"
	"io"
	"log"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

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

var stopImport bool
var logCh chan *LogEnt
var db *bbolt.DB

// importCmd represents the import command
var importCmd = &cobra.Command{
	Use:   "import",
	Short: "Import log from source",
	Long: `Import log from source
	source is file | dir | scp | ssh
	`,
	Run: func(cmd *cobra.Command, args []string) {
		importFunc()
	},
}

func init() {
	rootCmd.AddCommand(importCmd)
	importCmd.Flags().BoolVar(&utc, "utc", false, "Force UTC")
	importCmd.Flags().StringVarP(&source, "source", "s", "", "Log source")
	importCmd.Flags().StringVarP(&command, "command", "c", "", "SSH Command")
	importCmd.Flags().StringVarP(&sshKey, "key", "k", "", "SSH Key")
	importCmd.Flags().StringVarP(&filePat, "fileName", "f", "", "File name pattern")
}

func importFunc() {
	if err := openDB(); err != nil {
		log.Panicln(err)
	}
	defer db.Close()
	var logCh = make(chan *LogEnt, 1000)
	setupTimeGrinder()
	var wg sync.WaitGroup
	wg.Add(1)
	go logSaver(&wg)
	switch getSourceType() {
	case "file":
		importFromFile(source)
	case "dir":
		importFromDir()
	case "scp":
		importFromSCP()
	case "ssh":
		importFromSSH()
	default:
		fmt.Println("source type error")
	}
	close(logCh)
	wg.Wait()
}

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

func getSourceType() string {
	if strings.HasPrefix(source, "twsnmp:") {
		return "twsnmp"
	} else if strings.HasPrefix(source, "scp:") {
		return "scp"
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

func getFileNameFilter() *regexp.Regexp {
	if filePat == "" {
		return nil
	}
	pat := filePat
	pat = strings.ReplaceAll(pat, "*", ".*")
	pat = strings.ReplaceAll(pat, "?", ".")
	if f, err := regexp.Compile(pat); err == nil {
		return f
	}
	return nil
}

func importFromFile(path string) {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case "zip":
		imortFromZIPFile(path)
		return
	case "gz", "tgz":
		if strings.HasSuffix(path, "tar.gz") {
			importFormTarGZFile(path)
			return
		}
	}
	r, err := os.Open(path)
	if err != nil {
		log.Panicln(err)
	}
	defer r.Close()
	if ext == "gz" {
		if gzr, err := gzip.NewReader(r); err == nil {
			doImport(getSHA1(path), gzr)
		}
		return
	}
	doImport(getSHA1(path), r)
}

func imortFromZIPFile(path string) {
	r, err := zip.OpenReader(path)
	if err != nil {
		return
	}
	defer r.Close()
	filter := getFileNameFilter()
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
		if ext == "gz" {
			if gzr, err := gzip.NewReader(r); err == nil {
				doImport(getSHA1((path + f.Name)), gzr)
			}
		} else {
			doImport(getSHA1(path+f.Name), r)
		}
	}
}

func importFormTarGZFile(path string) {
	r, err := os.Open(path)
	if err != nil {
		return
	}
	defer r.Close()
	gzr, err := gzip.NewReader(r)
	if err != nil {
		return
	}
	filter := getFileNameFilter()
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
				continue
			}
			doImport(getSHA1(path+f.Name), igzr)
		} else {
			doImport(getSHA1(path+f.Name), tgzr)
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
		log.Panicln(err)
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
		return
	}
	sv := u.Host
	pt := u.Port()
	if pt == "" {
		pt = "22"
	}
	sv += ":" + pt
	service, err := scp.NewStorager(sv, time.Duration(time.Second)*3, config)
	if err != nil {
		return
	}
	files, err := service.List(context.Background(), u.Path)
	if err != nil {
		return
	}
	filter := getFileNameFilter()
	for _, file := range files {
		path := file.Name()
		if filter != nil && !filter.MatchString(path) {
			continue
		}
		r, err := service.Open(context.Background(), path)
		if err != nil {
			continue
		}
		doImport(source+path, r)
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
		return
	}
	sv := u.Host
	pt := u.Port()
	if pt == "" {
		pt = "22"
	}
	sv += ":" + pt
	conn, err := net.DialTimeout("tcp", sv, time.Duration(60)*time.Second)
	if err != nil {
		return
	}
	if err := conn.SetDeadline(time.Now().Add(time.Second * time.Duration(120))); err != nil {
		return
	}
	c, ch, req, err := ssh.NewClientConn(conn, sv, config)
	if err != nil {
		return
	}
	client := ssh.NewClient(c, ch, req)
	defer client.Close()
	session, err := client.NewSession()
	if err != nil {
		return
	}
	defer session.Close()
	stdout, err := session.Output(getCommand())
	if err != nil {
		return
	}
	r := bytes.NewReader(stdout)
	doImport(getSHA1(sv+command), r)
}

func doImport(hash string, r io.Reader) {
	lastTime := int64(0)
	readBytes := int64(0)
	readLines := 0
	skipLines := 0
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
		readLines++
		if importFilter != nil && !importFilter.MatchString(l) {
			skipLines++
			continue
		}
		d := 0
		if lastTime > 0 {
			d = int(ts.UnixNano() - lastTime)
		}
		logCh <- &LogEnt{
			Time:  t,
			Log:   l,
			Delta: d,
			Hash:  hash,
			Line:  readLines,
		}
	}
	if err := scanner.Err(); err != nil {
		log.Panicln(err)
	}
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
	return hex.EncodeToString(sha1.Sum(nil))
}

func getCommand() string {
	if command != "twsnmp" {
		return command
	}
	return ""
}
