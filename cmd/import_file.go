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
	"compress/gzip"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
)

func importFromFile(path string) {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".zip":
		imortFromZIPFile(path)
		return
	case ".evtx":
		importFromWindowsEvtx(path)
		return
	case ".tgz":
		importFormTarGZFile(path)
		return
	case ".gz":
		if strings.HasSuffix(path, ".tar.gz") {
			importFormTarGZFile(path)
			return
		}
	}
	r, err := os.Open(path)
	if err != nil {
		log.Panicln(err)
	}
	defer r.Close()
	if ext == ".gz" {
		if gzr, err := gzip.NewReader(r); err == nil {
			doImport(path, gzr)
		}
		return
	}
	doImport(path, r)
}

func imortFromZIPFile(path string) {
	r, err := zip.OpenReader(path)
	if err != nil {
		teaProg.Send(err)
		return
	}
	defer r.Close()
	filter := getSimpleFilter(filePat)
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
		if ext == ".gz" {
			if gzr, err := gzip.NewReader(r); err == nil {
				doImport(path+":"+f.Name, gzr)
			}
		} else if ext == ".evtx" {
			w, err := os.CreateTemp("", "winlog*.evtx")
			if err != nil {
				log.Fatalln(err)
			}
			defer os.Remove(w.Name())
			io.Copy(w, r)
			w.Close()
			importFromWindowsEvtx(w.Name())
		} else {
			doImport(path+":"+f.Name, r)
		}
	}
}

func importFormTarGZFile(path string) {
	r, err := os.Open(path)
	if err != nil {
		teaProg.Send(err)
		return
	}
	defer r.Close()
	gzr, err := gzip.NewReader(r)
	if err != nil {
		teaProg.Send(err)
		return
	}
	filter := getSimpleFilter(filePat)
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
				teaProg.Send(err)
				return
			}
			doImport(path+":"+f.Name, igzr)
		} else {
			doImport(path+":"+f.Name, tgzr)
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
		teaProg.Send(err)
		return
	}
	for _, f := range files {
		importFromFile(f)
	}

}
