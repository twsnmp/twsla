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
	"compress/gzip"
	"crypto/tls"
	"fmt"
	"net"
	"net/url"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/jlaffaye/ftp"
)

func importFromFTP() {
	u, err := url.Parse(source)
	if err != nil {
		sendErrorMsg(err)
		return
	}

	user := ftpUser
	pass := ftpPassword
	if u.User != nil {
		if u.User.Username() != "" {
			user = u.User.Username()
		}
		if p, ok := u.User.Password(); ok {
			pass = p
		}
	}
	if user == "" {
		user = "anonymous"
	}
	if pass == "" {
		pass = "anonymous@"
	}

	host := u.Hostname()
	port := u.Port()
	isTLS := strings.HasPrefix(source, "ftps:") || ftpTLS
	if port == "" {
		port = "21"
	}

	var dialOpts []ftp.DialOption
	dialOpts = append(dialOpts, ftp.DialWithTimeout(15*time.Second))

	if isTLS {
		tlsConfig := &tls.Config{
			InsecureSkipVerify: ftpSkip,
			ServerName:         host,
		}
		dialOpts = append(dialOpts, ftp.DialWithTLS(tlsConfig))
	}

	addr := net.JoinHostPort(host, port)
	client, err := ftp.Dial(addr, dialOpts...)
	if err != nil {
		sendErrorMsg(fmt.Errorf("FTP failed to dial %s: %w", addr, err))
		return
	}
	defer client.Quit()

	if err := client.Login(user, pass); err != nil {
		sendErrorMsg(fmt.Errorf("FTP failed to login as %s: %w", user, err))
		return
	}

	targetPath := u.Path
	if targetPath == "" {
		targetPath = "/"
	}

	filter := getSimpleFilter(filePat)

	// If file pattern is specified or path explicitly ends with "/", treat as directory
	if strings.HasSuffix(targetPath, "/") || filePat != "" {
		entries, err := client.List(targetPath)
		if err != nil {
			sendErrorMsg(fmt.Errorf("FTP failed to list directory %s: %w", targetPath, err))
			return
		}
		for _, entry := range entries {
			if stopImport {
				break
			}
			if entry.Type == ftp.EntryTypeFolder {
				continue
			}
			fileName := path.Base(entry.Name)
			if filter != nil && !filter.MatchString(fileName) {
				continue
			}
			var entryPath string
			if strings.HasPrefix(entry.Name, "/") {
				entryPath = entry.Name
			} else {
				entryPath = path.Join(targetPath, fileName)
			}
			if err := importSingleFTPFile(client, entryPath); err != nil {
				sendErrorMsg(err)
			}
		}
		return
	}

	// Try single file retrieve first
	if err := importSingleFTPFile(client, targetPath); err != nil {
		// If single file retrieve failed, check if targetPath is a directory without trailing slash
		entries, listErr := client.List(targetPath)
		if listErr == nil && len(entries) > 0 {
			firstBase := path.Base(entries[0].Name)
			if len(entries) > 1 || entries[0].Type == ftp.EntryTypeFolder || firstBase != path.Base(targetPath) {
				for _, entry := range entries {
					if stopImport {
						break
					}
					if entry.Type == ftp.EntryTypeFolder {
						continue
					}
					fileName := path.Base(entry.Name)
					if filter != nil && !filter.MatchString(fileName) {
						continue
					}
					var entryPath string
					if strings.HasPrefix(entry.Name, "/") {
						entryPath = entry.Name
					} else {
						entryPath = path.Join(targetPath, fileName)
					}
					if err := importSingleFTPFile(client, entryPath); err != nil {
						sendErrorMsg(err)
					}
				}
				return
			}
		}
		// If not a directory or list failed, report original retrieve error
		sendErrorMsg(err)
	}
}

func importSingleFTPFile(client *ftp.ServerConn, filePath string) error {
	resp, err := client.Retr(filePath)
	if err != nil {
		return fmt.Errorf("FTP failed to retrieve %s: %w", filePath, err)
	}
	defer resp.Close()

	ext := strings.ToLower(filepath.Ext(filePath))
	displayPath := source
	baseName := path.Base(filePath)
	if !strings.HasSuffix(displayPath, filePath) && !strings.HasSuffix(displayPath, baseName) {
		displayPath = strings.TrimRight(displayPath, "/") + "/" + baseName
	}

	if ext == ".gz" {
		gzr, err := gzip.NewReader(resp)
		if err != nil {
			return fmt.Errorf("failed to decompress gzip stream for %s: %w", filePath, err)
		}
		defer gzr.Close()
		doImport(displayPath, gzr)
	} else {
		doImport(displayPath, resp)
	}
	return nil
}
