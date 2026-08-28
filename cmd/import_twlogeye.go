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
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

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
var twLogEyeAnomalyReportType = "monitor"

func importFromTwLogEye() {
	if source != "" && strings.HasPrefix(source, "twlogeye:") {
		u, err := url.Parse(source)
		if err == nil {
			if u.Hostname() != "" {
				twLogEyeApiServer = u.Hostname()
			}
			if u.Port() != "" {
				if p, err := strconv.Atoi(u.Port()); err == nil {
					twLogEyeApiPort = p
				}
			}
			// Parse path if present: /<target>/<subTarget>/<anomaly>
			p := strings.Trim(u.Path, "/")
			if p != "" {
				parts := strings.Split(p, "/")
				if len(parts) > 0 && parts[0] != "" {
					twLogEyeTarget = parts[0]
				}
				if len(parts) > 1 && parts[1] != "" {
					twLogEyeSubTarget = parts[1]
				}
				if len(parts) > 2 && parts[2] != "" {
					twLogEyeAnomalyReportType = parts[2]
				}
			}
		}
	}
	if twLogEyeApiServer == "" {
		twLogEyeApiServer = "localhost"
	}
	if twLogEyeApiPort == 0 {
		twLogEyeApiPort = 8081
	}

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
		case "otel":
			getTwLogEyeOTelReport()
		case "mqtt":
			getTwLogEyeMqttReport()
		case "monitor":
			getTwLogEyeMonitorReport()
		case "anomaly":
			getTwLogEyeAnomalyReport()
		default:
			sendErrorMsg(fmt.Errorf("invalid report type: %s", twLogEyeSubTarget))
		}
	default:
		sendErrorMsg(fmt.Errorf("invalid target: %s", twLogEyeTarget))
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
	client, conn, err := getTwLogEyeClient()
	if err != nil {
		sendErrorMsg(err)
		return
	}
	if conn != nil {
		defer conn.Close()
	}
	st, et := getImportTimeRange()
	i := 0
	readBytes := int64(0)
	s, err := client.SearchNotify(context.Background(), &api.NofifyRequest{
		Start: st,
		End:   et,
		Level: twLogEyeLevel,
	})
	if err != nil {
		sendErrorMsg(err)
		return
	}
	for {
		if stopImport {
			return
		}
		r, err := s.Recv()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			sendErrorMsg(err)
			return
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
			sendImportMsg(ImportMsg{
				Done:  false,
				Path:  path,
				Bytes: readBytes,
				Lines: i,
				Skip:  0,
			})
		}
	}
	totalBytes += readBytes
	totalLines += i
	totalFiles++
	sendImportMsg(ImportMsg{Done: false, Path: path, Bytes: readBytes, Lines: i})
}

type twLogEyeLogEnt struct {
	Time string
	Src  string
	Log  string
}

func getTwLogEyeLogs() {
	if twLogEyeSubTarget == "" {
		sendErrorMsg(fmt.Errorf("log type is empty"))
		return
	}
	path := fmt.Sprintf("%s:%d/%s/%s", twLogEyeApiServer, twLogEyeApiPort, twLogEyeTarget, twLogEyeSubTarget)
	hash := getSHA1(path)
	client, conn, err := getTwLogEyeClient()
	if err != nil {
		sendErrorMsg(err)
		return
	}
	if conn != nil {
		defer conn.Close()
	}
	st, et := getImportTimeRange()
	i := 0
	readBytes := int64(0)
	s, err := client.SearchLog(context.Background(), &api.LogRequest{
		Logtype: twLogEyeSubTarget,
		Start:   st,
		End:     et,
		Search:  twLogEyeFilter,
	})
	if err != nil {
		sendErrorMsg(err)
		return
	}
	for {
		if stopImport {
			return
		}
		r, err := s.Recv()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			sendErrorMsg(err)
			return
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
			sendImportMsg(ImportMsg{
				Done:  false,
				Path:  path,
				Bytes: readBytes,
				Lines: i,
				Skip:  0,
			})
		}
	}
	totalBytes += readBytes
	totalLines += i
	totalFiles++
	sendImportMsg(ImportMsg{Done: false, Path: path, Bytes: readBytes, Lines: i})
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
	path := fmt.Sprintf("%s:%d/%s/syslog", twLogEyeApiServer, twLogEyeApiPort, twLogEyeTarget)
	hash := getSHA1(path)
	client, conn, err := getTwLogEyeClient()
	if err != nil {
		sendErrorMsg(err)
		return
	}
	if conn != nil {
		defer conn.Close()
	}
	st, et := getImportTimeRange()
	i := 0
	readBytes := int64(0)
	s, err := client.GetSyslogReport(context.Background(), &api.ReportRequest{
		Start: st,
		End:   et,
	})
	if err != nil {
		sendErrorMsg(err)
		return
	}
	for {
		if stopImport {
			return
		}
		r, err := s.Recv()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			sendErrorMsg(err)
			return
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
			sendImportMsg(ImportMsg{
				Done:  false,
				Path:  path,
				Bytes: readBytes,
				Lines: i,
				Skip:  0,
			})
		}
	}
	totalBytes += readBytes
	totalLines += i
	totalFiles++
	sendImportMsg(ImportMsg{Done: false, Path: path, Bytes: readBytes, Lines: i})
}

type twLogEyeTrapReport struct {
	Time  string
	Count int32
	Types int32
}

func getTwLogEyeTrapReport() {
	path := fmt.Sprintf("%s:%d/%s/trap", twLogEyeApiServer, twLogEyeApiPort, twLogEyeTarget)
	hash := getSHA1(path)
	client, conn, err := getTwLogEyeClient()
	if err != nil {
		sendErrorMsg(err)
		return
	}
	if conn != nil {
		defer conn.Close()
	}
	st, et := getImportTimeRange()
	i := 0
	readBytes := int64(0)
	s, err := client.GetTrapReport(context.Background(), &api.ReportRequest{
		Start: st,
		End:   et,
	})
	if err != nil {
		sendErrorMsg(err)
		return
	}
	for {
		if stopImport {
			return
		}
		r, err := s.Recv()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			sendErrorMsg(err)
			return
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
			sendImportMsg(ImportMsg{
				Done:  false,
				Path:  path,
				Bytes: readBytes,
				Lines: i,
				Skip:  0,
			})
		}
	}
	totalBytes += readBytes
	totalLines += i
	totalFiles++
	sendImportMsg(ImportMsg{Done: false, Path: path, Bytes: readBytes, Lines: i})
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
	Hosts     int32
	Locs      int32
	Country   int32
}

func getTwLogEyeNetflowReport() {
	path := fmt.Sprintf("%s:%d/%s/netflow", twLogEyeApiServer, twLogEyeApiPort, twLogEyeTarget)
	hash := getSHA1(path)
	client, conn, err := getTwLogEyeClient()
	if err != nil {
		sendErrorMsg(err)
		return
	}
	if conn != nil {
		defer conn.Close()
	}
	st, et := getImportTimeRange()
	i := 0
	readBytes := int64(0)
	s, err := client.GetNetflowReport(context.Background(), &api.ReportRequest{
		Start: st,
		End:   et,
	})
	if err != nil {
		sendErrorMsg(err)
		return
	}
	for {
		if stopImport {
			return
		}
		r, err := s.Recv()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			sendErrorMsg(err)
			return
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
			Hosts:     r.GetHosts(),
			Locs:      r.GetLocs(),
			Country:   r.GetCountry(),
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
			sendImportMsg(ImportMsg{
				Done:  false,
				Path:  path,
				Bytes: readBytes,
				Lines: i,
				Skip:  0,
			})
		}
	}
	totalBytes += readBytes
	totalLines += i
	totalFiles++
	sendImportMsg(ImportMsg{Done: false, Path: path, Bytes: readBytes, Lines: i})
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
	path := fmt.Sprintf("%s:%d/%s/winevent", twLogEyeApiServer, twLogEyeApiPort, twLogEyeTarget)
	hash := getSHA1(path)
	client, conn, err := getTwLogEyeClient()
	if err != nil {
		sendErrorMsg(err)
		return
	}
	if conn != nil {
		defer conn.Close()
	}
	st, et := getImportTimeRange()
	i := 0
	readBytes := int64(0)
	s, err := client.GetWindowsEventReport(context.Background(), &api.ReportRequest{
		Start: st,
		End:   et,
	})
	if err != nil {
		sendErrorMsg(err)
		return
	}
	for {
		if stopImport {
			return
		}
		r, err := s.Recv()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			sendErrorMsg(err)
			return
		}
		t := r.GetTime()
		j, err := json.Marshal(&twLogEyeWindowsEventReport{
			Time:       getTimeStr(r.GetTime()),
			Normal:     r.GetNormal(),
			Warn:        r.GetWarn(),
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
			sendImportMsg(ImportMsg{
				Done:  false,
				Path:  path,
				Bytes: readBytes,
				Lines: i,
				Skip:  0,
			})
		}
	}
	totalBytes += readBytes
	totalLines += i
	totalFiles++
	sendImportMsg(ImportMsg{Done: false, Path: path, Bytes: readBytes, Lines: i})
}

type twLogEyeOTelReport struct {
	Time        string
	Normal      int32
	Warn        int32
	Error       int32
	Types       int32
	ErrorTypes  int32
	Hosts       int32
	TraceIds    int32
	TraceCount  int32
	MericsCount int32
}

func getTwLogEyeOTelReport() {
	path := fmt.Sprintf("%s:%d/%s/otel", twLogEyeApiServer, twLogEyeApiPort, twLogEyeTarget)
	hash := getSHA1(path)
	client, conn, err := getTwLogEyeClient()
	if err != nil {
		sendErrorMsg(err)
		return
	}
	if conn != nil {
		defer conn.Close()
	}
	st, et := getImportTimeRange()
	i := 0
	readBytes := int64(0)
	s, err := client.GetOTelReport(context.Background(), &api.ReportRequest{
		Start: st,
		End:   et,
	})
	if err != nil {
		sendErrorMsg(err)
		return
	}
	for {
		if stopImport {
			return
		}
		r, err := s.Recv()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			sendErrorMsg(err)
			return
		}
		t := r.GetTime()
		j, err := json.Marshal(&twLogEyeOTelReport{
			Time:        getTimeStr(r.GetTime()),
			Normal:      r.GetNormal(),
			Warn:        r.GetWarn(),
			Error:       r.GetError(),
			Types:       r.GetTypes(),
			ErrorTypes:  r.GetErrorTypes(),
			Hosts:       r.GetHosts(),
			TraceIds:    r.GetTraceIds(),
			TraceCount:  r.GetTraceCount(),
			MericsCount: r.GetMericsCount(),
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
			sendImportMsg(ImportMsg{
				Done:  false,
				Path:  path,
				Bytes: readBytes,
				Lines: i,
				Skip:  0,
			})
		}
	}
	totalBytes += readBytes
	totalLines += i
	totalFiles++
	sendImportMsg(ImportMsg{Done: false, Path: path, Bytes: readBytes, Lines: i})
}

type twLogEyeMqttReport struct {
	Time  string
	Count int32
	Types int32
}

func getTwLogEyeMqttReport() {
	path := fmt.Sprintf("%s:%d/%s/mqtt", twLogEyeApiServer, twLogEyeApiPort, twLogEyeTarget)
	hash := getSHA1(path)
	client, conn, err := getTwLogEyeClient()
	if err != nil {
		sendErrorMsg(err)
		return
	}
	if conn != nil {
		defer conn.Close()
	}
	st, et := getImportTimeRange()
	i := 0
	readBytes := int64(0)
	s, err := client.GetMqttReport(context.Background(), &api.ReportRequest{
		Start: st,
		End:   et,
	})
	if err != nil {
		sendErrorMsg(err)
		return
	}
	for {
		if stopImport {
			return
		}
		r, err := s.Recv()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			sendErrorMsg(err)
			return
		}
		t := r.GetTime()
		j, err := json.Marshal(&twLogEyeMqttReport{
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
			sendImportMsg(ImportMsg{
				Done:  false,
				Path:  path,
				Bytes: readBytes,
				Lines: i,
				Skip:  0,
			})
		}
	}
	totalBytes += readBytes
	totalLines += i
	totalFiles++
	sendImportMsg(ImportMsg{Done: false, Path: path, Bytes: readBytes, Lines: i})
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
	path := fmt.Sprintf("%s:%d/%s/monitor", twLogEyeApiServer, twLogEyeApiPort, twLogEyeTarget)
	hash := getSHA1(path)
	client, conn, err := getTwLogEyeClient()
	if err != nil {
		sendErrorMsg(err)
		return
	}
	if conn != nil {
		defer conn.Close()
	}
	st, et := getImportTimeRange()
	i := 0
	readBytes := int64(0)
	s, err := client.GetMonitorReport(context.Background(), &api.ReportRequest{
		Start: st,
		End:   et,
	})
	if err != nil {
		sendErrorMsg(err)
		return
	}
	for {
		if stopImport {
			return
		}
		r, err := s.Recv()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			sendErrorMsg(err)
			return
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
			sendImportMsg(ImportMsg{
				Done:  false,
				Path:  path,
				Bytes: readBytes,
				Lines: i,
				Skip:  0,
			})
		}
	}
	totalBytes += readBytes
	totalLines += i
	totalFiles++
	sendImportMsg(ImportMsg{Done: false, Path: path, Bytes: readBytes, Lines: i})
}

type twLogEyeAnomalyReport struct {
	Time  string
	Type  string
	Score float64
}

func getTwLogEyeAnomalyReport() {
	path := fmt.Sprintf("%s:%d/%s/anomaly/%s", twLogEyeApiServer, twLogEyeApiPort, twLogEyeTarget, twLogEyeAnomalyReportType)
	hash := getSHA1(path)
	client, conn, err := getTwLogEyeClient()
	if err != nil {
		sendErrorMsg(err)
		return
	}
	if conn != nil {
		defer conn.Close()
	}
	st, et := getImportTimeRange()
	i := 0
	readBytes := int64(0)
	s, err := client.GetAnomalyReport(context.Background(), &api.AnomalyReportRequest{
		Start: st,
		End:   et,
		Type:  twLogEyeAnomalyReportType,
	})
	if err != nil {
		sendErrorMsg(err)
		return
	}
	for {
		if stopImport {
			return
		}
		r, err := s.Recv()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			sendErrorMsg(err)
			return
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
			sendImportMsg(ImportMsg{
				Done:  false,
				Path:  path,
				Bytes: readBytes,
				Lines: i,
				Skip:  0,
			})
		}
	}
	totalBytes += readBytes
	totalLines += i
	totalFiles++
	sendImportMsg(ImportMsg{Done: false, Path: path, Bytes: readBytes, Lines: i})
}

func getTwLogEyeClient() (api.TWLogEyeServiceClient, *grpc.ClientConn, error) {
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
			return nil, nil, fmt.Errorf("did not connect: %w", err)
		}
	} else {
		if twLogEyeClientCert != "" && twLogEyeClientKey != "" {
			// mTLS
			cert, err := tls.LoadX509KeyPair(twLogEyeClientCert, twLogEyeClientKey)
			if err != nil {
				return nil, nil, fmt.Errorf("failed to load client cert: %w", err)
			}
			ca := x509.NewCertPool()
			caBytes, err := os.ReadFile(twLogEyeCaCert)
			if err != nil {
				return nil, nil, fmt.Errorf("failed to read ca cert err=%w", err)
			}
			if ok := ca.AppendCertsFromPEM(caBytes); !ok {
				return nil, nil, fmt.Errorf("failed to parse %q", twLogEyeCaCert)
			}
			tlsConfig := &tls.Config{
				ServerName:   "",
				Certificates: []tls.Certificate{cert},
				RootCAs:      ca,
			}
			conn, err = grpc.NewClient(address, grpc.WithTransportCredentials(credentials.NewTLS(tlsConfig)))
			if err != nil {
				return nil, nil, fmt.Errorf("failed to connect err=%w", err)
			}
		} else {
			// TLS
			creds, err := credentials.NewClientTLSFromFile(twLogEyeCaCert, "")
			if err != nil {
				return nil, nil, fmt.Errorf("failed to load credentials: %w", err)
			}
			conn, err = grpc.NewClient(address, grpc.WithTransportCredentials(creds))
			if err != nil {
				return nil, nil, fmt.Errorf("failed to connect err=%w", err)
			}
		}
	}
	return api.NewTWLogEyeServiceClient(conn), conn, nil
}

func getTimeStr(t int64) string {
	return time.Unix(0, t).Format(time.RFC3339Nano)
}
