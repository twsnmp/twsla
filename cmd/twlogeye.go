/*
Copyright © 2025 Masayuki Yamai <twsnmp@gmail.com>

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
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"sync"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"
	"github.com/twsnmp/twlogeye/api"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
)

var twLogEyeApiServer string
var twLogEyeApiPort int
var twLogEyeCaCert string
var twLogEyeClientCert string
var twLogEyeClientKey string
var twLogEyeTarget = "notify"
var twLogEyeFilter string

// twlogeyeCmd represents the twlogeye command
var twlogeyeCmd = &cobra.Command{
	Use:   "twlogeye",
	Short: "Inmport notify and log from twlogeye",
	Long: `Import notify and log from twlogeye
twsla twlogeye <target>
  taregt: notify | syslog | trap | netflow | windows 
`,
	Run: func(cmd *cobra.Command, args []string) {
		if len(args) > 0 {
			twLogEyeTarget = args[0]
		}
		twLogEyeMain()
	},
}

func init() {
	rootCmd.AddCommand(twlogeyeCmd)
	twlogeyeCmd.Flags().StringVar(&twLogEyeApiServer, "apiServer", "", "twlogeye api server ip address")
	twlogeyeCmd.Flags().IntVar(&twLogEyeApiPort, "apiPort", 8081, "twlogeye api port number")
	twlogeyeCmd.Flags().StringVar(&twLogEyeCaCert, "ca", "", "CA Cert file path")
	twlogeyeCmd.Flags().StringVar(&twLogEyeClientCert, "cert", "", "Client cert file path")
	twlogeyeCmd.Flags().StringVar(&twLogEyeClientKey, "key", "", "Client key file path")
	twlogeyeCmd.Flags().StringVar(&twLogEyeFilter, "filter", "", "Notfiy level or Log search text")
}

func twLogEyeMain() {
	st = time.Now()
	if err := openDB(); err != nil {
		log.Fatalln(err)
	}
	defer db.Close()
	teaProg = tea.NewProgram(initImportModel())
	logCh = make(chan *LogEnt, 10000)
	var wg sync.WaitGroup
	wg.Add(1)
	go twLogEyeSub(&wg)
	wg.Add(1)
	go logSaver(&wg)
	if _, err := teaProg.Run(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	close(logCh)
	wg.Wait()
}

func twLogEyeSub(wg *sync.WaitGroup) {
	defer wg.Done()
	path := fmt.Sprintf("%s:%d/%s", twLogEyeApiServer, twLogEyeApiPort, twLogEyeTarget)
	hash := getSHA1(path)
	client := getTwLogEyeClient()
	st, et := getTimeRange()
	if st == 0 {
		st = time.Now().Add(-24 * time.Hour).UnixNano()
	}
	i := 0
	readBytes := int64(0)
	if twLogEyeTarget == "notify" {
		s, err := client.SearchNotify(context.Background(), &api.NofifyRequest{
			Start: st,
			End:   et,
			Level: twLogEyeFilter,
		})
		if err != nil {
			log.Fatalf("search notify err=%v", err)
		}
		for {
			r, err := s.Recv()
			if errors.Is(err, io.EOF) {
				break
			}
			if err != nil {
				log.Fatalf("search notify err=%v", err)
			}
			t := r.GetTime()
			l := fmt.Sprintf("%s %s %s %s %s %s", getTimeStr(r.GetTime()), r.GetSrc(), r.GetLevel(), r.GetId(), r.GetTags(), r.GetTitle())
			logCh <- &LogEnt{
				Time:  t,
				Log:   l,
				Delta: 0,
				Hash:  hash,
				Line:  i,
			}
			i++
			readBytes += int64(len(l))
			if i%100 == 0 {
				teaProg.Send(ImportMsg{
					Done:  false,
					Path:  path,
					Bytes: readBytes,
					Lines: i,
					Skip:  0,
				})
			}
		}
	} else {
		s, err := client.SearchLog(context.Background(), &api.LogRequest{
			Logtype: twLogEyeTarget,
			Start:   st,
			End:     et,
			Search:  twLogEyeFilter,
		})
		if err != nil {
			log.Fatalf("search log err=%v", err)
		}
		for {
			r, err := s.Recv()
			if errors.Is(err, io.EOF) {
				break
			}
			if err != nil {
				log.Fatalf("search log err=%v", err)
			}
			t := r.GetTime()
			l := fmt.Sprintf("%s %s %s", getTimeStr(r.GetTime()), r.GetSrc(), r.GetLog())
			logCh <- &LogEnt{
				Time:  t,
				Log:   l,
				Delta: 0,
				Hash:  hash,
				Line:  i,
			}
			i++
			readBytes += int64(len(l))
			if i%100 == 0 {
				teaProg.Send(ImportMsg{
					Done:  false,
					Path:  path,
					Bytes: readBytes,
					Lines: i,
					Skip:  0,
				})
			}
		}
	}
	totalBytes = readBytes
	totalLines = i
	totalFiles = 1
	teaProg.Send(ImportMsg{Done: false, Path: path, Bytes: readBytes, Lines: i})
	teaProg.Send(ImportMsg{Done: true})
}

func getTwLogEyeClient() api.TWLogEyeServiceClient {
	var conn *grpc.ClientConn
	var err error
	address := fmt.Sprintf("%s:%d", twLogEyeApiServer, twLogEyeApiPort)
	if twLogEyeCaCert == "" {
		// not TLS
		conn, err = grpc.NewClient(
			address,
			grpc.WithTransportCredentials(insecure.NewCredentials()),
		)
		if err != nil {
			log.Fatalf("did not connect: %v", err)
		}
	} else {
		if twLogEyeClientCert != "" && twLogEyeClientKey != "" {
			// mTLS
			cert, err := tls.LoadX509KeyPair(twLogEyeClientCert, twLogEyeClientKey)
			if err != nil {
				log.Fatalf("failed to load client cert: %v", err)
			}
			ca := x509.NewCertPool()
			caBytes, err := os.ReadFile(twLogEyeCaCert)
			if err != nil {
				log.Fatalf("failed to read ca cert  err=%v", err)
			}
			if ok := ca.AppendCertsFromPEM(caBytes); !ok {
				log.Fatalf("failed to parse %q", twLogEyeCaCert)
			}
			tlsConfig := &tls.Config{
				ServerName:   "",
				Certificates: []tls.Certificate{cert},
				RootCAs:      ca,
			}
			conn, err = grpc.NewClient(address, grpc.WithTransportCredentials(credentials.NewTLS(tlsConfig)))
			if err != nil {
				log.Fatalf("failed to connect  err=%v", err)
			}
		} else {
			// TLS
			creds, err := credentials.NewClientTLSFromFile(twLogEyeCaCert, "")
			if err != nil {
				log.Fatalf("failed to load credentials: %v", err)
			}
			conn, err = grpc.NewClient(address, grpc.WithTransportCredentials(creds))
			if err != nil {
				log.Fatalf("failed to connect  err=%v", err)
			}
		}
	}
	return api.NewTWLogEyeServiceClient(conn)
}

func getTimeStr(t int64) string {
	return time.Unix(0, t).Format(time.RFC3339Nano)
}
