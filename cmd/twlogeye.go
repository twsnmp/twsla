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
	"encoding/json"
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
var twLogEyeSubTarget string
var twLogEyeFilter string
var twLogEyeLevel string
var twLogEyeAnomalyReportType string

// twlogeyeCmd represents the twlogeye command
var twlogeyeCmd = &cobra.Command{
	Use:   "twlogeye",
	Short: "Inmport notify and log from twlogeye",
	Long: `Import notify and log from twlogeye
twsla twlogeye <target> [<sub target>] [<anomaly report type>]
  taregt: notify | logs | report 
	logs sub target: syslog | trap | netflow | winevent 
	report sub target: syslog | trap | netflow | winevent | monitor | anomaly 
	anomaly report type: syslog | trap | netflow | winevent | monitor | anomaly 
`,
	Run: func(cmd *cobra.Command, args []string) {
		if len(args) > 0 {
			twLogEyeTarget = args[0]
			if len(args) > 1 {
				twLogEyeSubTarget = args[1]
				if len(args) > 2 && twLogEyeSubTarget == "anomaly" {
					twLogEyeAnomalyReportType = args[2]
				}
			}
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
	twlogeyeCmd.Flags().StringVar(&twLogEyeFilter, "filter", "", "Log search text")
	twlogeyeCmd.Flags().StringVar(&twLogEyeLevel, "level", "", "Notfiy level")
	twlogeyeCmd.Flags().StringVar(&twLogEyeAnomalyReportType, "anomaly", "monitor", "Anomaly report type")
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
	switch twLogEyeTarget {
	case "notify":
		getTwLogEyeNotify()
	case "logs":
		getTwLogEyeLogs()
	case "report":
		switch twLogEyeSubTarget {
		case "syslog":
			getTwLogEyeSyslogReport()
		case "trap":
			getTwLogEyeTrapReport()
		case "netflow":
			getTwLogEyeNetflowReport()
		case "winevent":
			getTwLogEyeWindowsEventReport()
		case "monitor":
			getTwLogEyeMonitorReport()
		case "anomaly":
			getTwLogEyeAnomalyReport()
		default:
			log.Fatalln("invalid report type")
		}
	default:
		log.Fatalln("invalid target")
	}
}

type twLogEyeNotifyEnt struct {
	Time  string
	ID    string
	Level string
	Title string
	Tags  string
	Src   string
	Log   string
}

func getTwLogEyeNotify() {
	path := fmt.Sprintf("%s:%d/%s", twLogEyeApiServer, twLogEyeApiPort, twLogEyeTarget)
	hash := getSHA1(path)
	client := getTwLogEyeClient()
	st, et := getTimeRange()
	if st == 0 {
		st = time.Now().Add(-24 * time.Hour).UnixNano()
	}
	i := 0
	readBytes := int64(0)
	s, err := client.SearchNotify(context.Background(), &api.NofifyRequest{
		Start: st,
		End:   et,
		Level: twLogEyeLevel,
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
		j, err := json.Marshal(&twLogEyeNotifyEnt{
			Time:  getTimeStr(r.GetTime()),
			Src:   r.GetSrc(),
			Level: r.GetLevel(),
			ID:    r.GetId(),
			Tags:  r.GetTags(),
			Title: r.GetTitle(),
			Log:   r.GetLog(),
		})
		if err != nil {
			continue
		}
		logCh <- &LogEnt{
			Time:  t,
			Log:   string(j),
			Delta: 0,
			Hash:  hash,
			Line:  i,
		}
		i++
		readBytes += int64(len(j))
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
	totalBytes = readBytes
	totalLines = i
	totalFiles = 1
	teaProg.Send(ImportMsg{Done: false, Path: path, Bytes: readBytes, Lines: i})
	teaProg.Send(ImportMsg{Done: true})
}

type twLogEyeLogEnt struct {
	Time string
	Src  string
	Log  string
}

func getTwLogEyeLogs() {
	if twLogEyeSubTarget == "" {
		log.Fatalln("log type is empty")
	}
	path := fmt.Sprintf("%s:%d/%s", twLogEyeApiServer, twLogEyeApiPort, twLogEyeTarget)
	hash := getSHA1(path)
	client := getTwLogEyeClient()
	st, et := getTimeRange()
	if st == 0 {
		st = time.Now().Add(-24 * time.Hour).UnixNano()
	}
	i := 0
	readBytes := int64(0)
	s, err := client.SearchLog(context.Background(), &api.LogRequest{
		Logtype: twLogEyeSubTarget,
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
		j, err := json.Marshal(&twLogEyeLogEnt{
			Time: getTimeStr(r.GetTime()),
			Src:  r.GetSrc(),
			Log:  r.GetLog(),
		})
		if err != nil {
			continue
		}
		logCh <- &LogEnt{
			Time:  t,
			Log:   string(j),
			Delta: 0,
			Hash:  hash,
			Line:  i,
		}
		i++
		readBytes += int64(len(j))
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
	totalBytes = readBytes
	totalLines = i
	totalFiles = 1
	teaProg.Send(ImportMsg{Done: false, Path: path, Bytes: readBytes, Lines: i})
	teaProg.Send(ImportMsg{Done: true})
}

type twLogEyeSyslogReport struct {
	Time        string
	Normal      int32
	Warn        int32
	Error       int32
	Patterns    int32
	ErrPatterns int32
}

func getTwLogEyeSyslogReport() {
	path := fmt.Sprintf("%s:%d/%s", twLogEyeApiServer, twLogEyeApiPort, twLogEyeTarget)
	hash := getSHA1(path)
	client := getTwLogEyeClient()
	st, et := getTimeRange()
	if st == 0 {
		st = time.Now().Add(-24 * time.Hour).UnixNano()
	}
	i := 0
	readBytes := int64(0)
	s, err := client.GetSyslogReport(context.Background(), &api.ReportRequest{
		Start: st,
		End:   et,
	})
	if err != nil {
		log.Fatalf("get syslog report err=%v", err)
	}
	for {
		r, err := s.Recv()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			log.Fatalf("get syslog err=%v", err)
		}
		t := r.GetTime()
		j, err := json.Marshal(&twLogEyeSyslogReport{
			Time:        getTimeStr(r.GetTime()),
			Normal:      r.GetNormal(),
			Warn:        r.GetWarn(),
			Error:       r.GetError(),
			Patterns:    r.GetPatterns(),
			ErrPatterns: r.GetErrPatterns(),
		})
		if err != nil {
			continue
		}
		logCh <- &LogEnt{
			Time:  t,
			Log:   string(j),
			Delta: 0,
			Hash:  hash,
			Line:  i,
		}
		i++
		readBytes += int64(len(j))
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
	totalBytes = readBytes
	totalLines = i
	totalFiles = 1
	teaProg.Send(ImportMsg{Done: false, Path: path, Bytes: readBytes, Lines: i})
	teaProg.Send(ImportMsg{Done: true})
}

type twLogEyeTrapReport struct {
	Time  string
	Count int32
	Types int32
}

func getTwLogEyeTrapReport() {
	path := fmt.Sprintf("%s:%d/%s", twLogEyeApiServer, twLogEyeApiPort, twLogEyeTarget)
	hash := getSHA1(path)
	client := getTwLogEyeClient()
	st, et := getTimeRange()
	if st == 0 {
		st = time.Now().Add(-24 * time.Hour).UnixNano()
	}
	i := 0
	readBytes := int64(0)
	s, err := client.GetTrapReport(context.Background(), &api.ReportRequest{
		Start: st,
		End:   et,
	})
	if err != nil {
		log.Fatalf("get trap report err=%v", err)
	}
	for {
		r, err := s.Recv()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			log.Fatalf("get trap report err=%v", err)
		}
		t := r.GetTime()
		j, err := json.Marshal(&twLogEyeTrapReport{
			Time:  getTimeStr(r.GetTime()),
			Count: r.GetCount(),
			Types: r.GetTypes(),
		})
		if err != nil {
			continue
		}
		logCh <- &LogEnt{
			Time:  t,
			Log:   string(j),
			Delta: 0,
			Hash:  hash,
			Line:  i,
		}
		i++
		readBytes += int64(len(j))
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
	totalBytes = readBytes
	totalLines = i
	totalFiles = 1
	teaProg.Send(ImportMsg{Done: false, Path: path, Bytes: readBytes, Lines: i})
	teaProg.Send(ImportMsg{Done: true})
}

type twLogEyeNetflowReport struct {
	Time      string
	Packets   int64
	Bytes     int64
	MACs      int32
	IPs       int32
	Flows     int32
	Protocols int32
	Fumbles   int32
}

func getTwLogEyeNetflowReport() {
	path := fmt.Sprintf("%s:%d/%s", twLogEyeApiServer, twLogEyeApiPort, twLogEyeTarget)
	hash := getSHA1(path)
	client := getTwLogEyeClient()
	st, et := getTimeRange()
	if st == 0 {
		st = time.Now().Add(-24 * time.Hour).UnixNano()
	}
	i := 0
	readBytes := int64(0)
	s, err := client.GetNetflowReport(context.Background(), &api.ReportRequest{
		Start: st,
		End:   et,
	})
	if err != nil {
		log.Fatalf("get netflow report err=%v", err)
	}
	for {
		r, err := s.Recv()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			log.Fatalf("get netflow report err=%v", err)
		}
		t := r.GetTime()
		j, err := json.Marshal(&twLogEyeNetflowReport{
			Time:      getTimeStr(r.GetTime()),
			Packets:   r.GetPackets(),
			Bytes:     r.GetBytes(),
			MACs:      r.GetMacs(),
			IPs:       r.GetIps(),
			Flows:     r.GetFlows(),
			Protocols: r.GetProtocols(),
			Fumbles:   r.GetFumbles(),
		})
		if err != nil {
			continue
		}
		logCh <- &LogEnt{
			Time:  t,
			Log:   string(j),
			Delta: 0,
			Hash:  hash,
			Line:  i,
		}
		i++
		readBytes += int64(len(j))
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
	totalBytes = readBytes
	totalLines = i
	totalFiles = 1
	teaProg.Send(ImportMsg{Done: false, Path: path, Bytes: readBytes, Lines: i})
	teaProg.Send(ImportMsg{Done: true})
}

type twLogEyeWindowsEventReport struct {
	Time       string
	Normal     int32
	Warn       int32
	Error      int32
	Types      int32
	ErrorTypes int32
}

func getTwLogEyeWindowsEventReport() {
	path := fmt.Sprintf("%s:%d/%s", twLogEyeApiServer, twLogEyeApiPort, twLogEyeTarget)
	hash := getSHA1(path)
	client := getTwLogEyeClient()
	st, et := getTimeRange()
	if st == 0 {
		st = time.Now().Add(-24 * time.Hour).UnixNano()
	}
	i := 0
	readBytes := int64(0)
	s, err := client.GetWindowsEventReport(context.Background(), &api.ReportRequest{
		Start: st,
		End:   et,
	})
	if err != nil {
		log.Fatalf("get windows event report err=%v", err)
	}
	for {
		r, err := s.Recv()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			log.Fatalf("get windows event report err=%v", err)
		}
		t := r.GetTime()
		j, err := json.Marshal(&twLogEyeWindowsEventReport{
			Time:       getTimeStr(r.GetTime()),
			Normal:     r.GetNormal(),
			Warn:       r.GetWarn(),
			Error:      r.GetError(),
			Types:      r.GetTypes(),
			ErrorTypes: r.GetErrorTypes(),
		})
		if err != nil {
			continue
		}
		logCh <- &LogEnt{
			Time:  t,
			Log:   string(j),
			Delta: 0,
			Hash:  hash,
			Line:  i,
		}
		i++
		readBytes += int64(len(j))
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
	totalBytes = readBytes
	totalLines = i
	totalFiles = 1
	teaProg.Send(ImportMsg{Done: false, Path: path, Bytes: readBytes, Lines: i})
	teaProg.Send(ImportMsg{Done: true})
}

type twLogEyeMonitorReport struct {
	Time    string
	CPU     float64
	Memory  float64
	Load    float64
	Disk    float64
	Net     float64
	Bytes   int64
	DBSpeed float64
	DBSize  int64
}

func getTwLogEyeMonitorReport() {
	path := fmt.Sprintf("%s:%d/%s", twLogEyeApiServer, twLogEyeApiPort, twLogEyeTarget)
	hash := getSHA1(path)
	client := getTwLogEyeClient()
	st, et := getTimeRange()
	if st == 0 {
		st = time.Now().Add(-24 * time.Hour).UnixNano()
	}
	i := 0
	readBytes := int64(0)
	s, err := client.GetMonitorReport(context.Background(), &api.ReportRequest{
		Start: st,
		End:   et,
	})
	if err != nil {
		log.Fatalf("get monitor report err=%v", err)
	}
	for {
		r, err := s.Recv()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			log.Fatalf("get monitor report err=%v", err)
		}
		t := r.GetTime()
		j, err := json.Marshal(&twLogEyeMonitorReport{
			Time:    getTimeStr(r.GetTime()),
			CPU:     r.GetCpu(),
			Memory:  r.GetMemory(),
			Load:    r.GetLoad(),
			Disk:    r.GetDisk(),
			Net:     r.GetNet(),
			Bytes:   r.GetBytes(),
			DBSpeed: r.GetDbSpeed(),
			DBSize:  r.GetDbSize(),
		})
		if err != nil {
			continue
		}
		logCh <- &LogEnt{
			Time:  t,
			Log:   string(j),
			Delta: 0,
			Hash:  hash,
			Line:  i,
		}
		i++
		readBytes += int64(len(j))
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
	totalBytes = readBytes
	totalLines = i
	totalFiles = 1
	teaProg.Send(ImportMsg{Done: false, Path: path, Bytes: readBytes, Lines: i})
	teaProg.Send(ImportMsg{Done: true})
}

type twLogEyeAnomalyReport struct {
	Time  string
	Type  string
	Score float64
}

func getTwLogEyeAnomalyReport() {
	path := fmt.Sprintf("%s:%d/%s", twLogEyeApiServer, twLogEyeApiPort, twLogEyeTarget)
	hash := getSHA1(path)
	client := getTwLogEyeClient()
	st, et := getTimeRange()
	if st == 0 {
		st = time.Now().Add(-24 * time.Hour).UnixNano()
	}
	i := 0
	readBytes := int64(0)
	s, err := client.GetAnomalyReport(context.Background(), &api.AnomalyReportRequest{
		Start: st,
		End:   et,
		Type:  twLogEyeAnomalyReportType,
	})
	if err != nil {
		log.Fatalf("get anomaly report err=%v", err)
	}
	for {
		r, err := s.Recv()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			log.Fatalf("get anomaly report err=%v", err)
		}
		t := r.GetTime()
		j, err := json.Marshal(&twLogEyeAnomalyReport{
			Time:  getTimeStr(r.GetTime()),
			Type:  twLogEyeAnomalyReportType,
			Score: r.GetScore(),
		})
		if err != nil {
			continue
		}
		logCh <- &LogEnt{
			Time:  t,
			Log:   string(j),
			Delta: 0,
			Hash:  hash,
			Line:  i,
		}
		i++
		readBytes += int64(len(j))
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
