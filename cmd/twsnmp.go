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
	"log"
	"net/url"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/twsnmp/twsnmpfc/client"
)

var jsonOut = false
var checkCert = false
var twsnmp string

// twsnmpCmd represents the twsnmp command
var twsnmpCmd = &cobra.Command{
	Use:   "twsnmp [target]",
	Short: "Get information and logs from TWSNMP FC",
	Long: `Get information adn logs from TWSNMP FC
[taget] is node|polling|eventlog|syslog|trap|netflow|ipfix|sflow|sflowCounter|arplog|pollingLog`,
	Args: func(cmd *cobra.Command, args []string) error {
		// Optionally run one of the validators provided by cobra
		if err := cobra.MinimumNArgs(1)(cmd, args); err != nil {
			return err
		}
		switch args[0] {
		case "node":
		case "polling":
		case "ai":
		case "eventlog":
		case "syslog":
		case "trap":
		case "netflow":
		case "ipfix":
		case "sflow":
		case "sflowCounter":
		case "arplog":
		case "pollingLog":
			if len(args) != 2 {
				return fmt.Errorf("polling id not specified")
			}
		default:
			return fmt.Errorf("invalid target specified: %s", args[0])
		}
		return nil
	},
	Run: func(cmd *cobra.Command, args []string) {
		if len(args) < 1 {
			log.Fatalln("you have to specify target.")
		}
		switch args[0] {
		case "node":
			twsnmpNode()
		case "polling":
			twsnmpPolling()
		case "eventlog":
			twsnmpEventLog()
		case "syslog":
			twsnmpSyslog()
		case "trap":
			twsnmpTrap()
		case "netflow":
			twsnmpNetFlow(false)
		case "ipfix":
			twsnmpNetFlow(true)
		case "sflow":
			twsnmpSFlow()
		case "sflowCounter":
			twsnmpSFlowCounter()
		case "arplog":
			twsnmpArpLog()
		case "pollingLog":
			if len(args) != 2 {
				fmt.Println("polling id not specified")
				return
			}
			twsnmpPollingLog(args[1])
		case "ai":
			twsnmpAI()
		}
	},
}

func init() {
	rootCmd.AddCommand(twsnmpCmd)
	twsnmpCmd.Flags().BoolVar(&jsonOut, "jsonOut", false, "output json format")
	twsnmpCmd.Flags().BoolVar(&checkCert, "checkCert", true, "TWSNMP FC API Skip Cert verify")
	twsnmpCmd.Flags().StringVar(&twsnmp, "twsnmp", "http://localhost:8080", "TWSNMP FC URL")

}

// getTWSNMPClient get client to TWSNMP FC
func getTWSNMPClient() *client.TWSNMPApi {
	u, err := url.Parse(twsnmp)
	if err != nil {
		log.Fatalf("parece source  err=%v", err)
	}
	c := client.NewClient(fmt.Sprintf("%s://%s", u.Scheme, u.Host))
	c.InsecureSkipVerify = !checkCert
	c.Timeout = 60
	userID := u.User.Username()
	if userID == "" {
		userID = "twsnmp"
	}
	passwd, ok := u.User.Password()
	if !ok || passwd == "" {
		passwd = "twsnmp"
	}
	err = c.Login(userID, passwd)
	if err != nil {
		log.Fatalf("login to TWSNMP FC  err=%v", err)
	}
	return c
}

func twsnmpNode() {
	c := getTWSNMPClient()
	r, err := c.GetNodes()
	if err != nil {
		log.Fatalf("get nodes err=%v", err)
	}
	for _, n := range r {
		if jsonOut {
			j, err := json.Marshal(n)
			if err != nil {
				continue
			}
			fmt.Println(string(j))
		} else {
			fmt.Printf("%s\t%s\t%s\t%s\t%s\n", n.ID, n.Name, n.State, n.IP, n.MAC)
		}
	}
}

func twsnmpPolling() {
	c := getTWSNMPClient()
	r, err := c.GetPollings()
	if err != nil {
		log.Fatalf("get polling err=%v", err)
	}
	nm := make(map[string]string)
	for _, n := range r.NodeList {
		nm[n.Value] = n.Text
	}
	for _, p := range r.Pollings {
		if jsonOut {
			j, err := json.Marshal(p)
			if err != nil {
				continue
			}
			fmt.Println(string(j))
		} else {
			nodeName, ok := nm[p.NodeID]
			if !ok {
				nodeName = p.NodeID
			}
			fmt.Printf("%s\t%s\t%s\t%d\t%s\n", p.ID, p.Name, p.State, p.LogMode, nodeName)
		}
	}
}

func twsnmpAI() {
	c := getTWSNMPClient()
	r, err := c.GetAIList()
	if err != nil {
		log.Fatalf("get ai list err=%v", err)
	}
	for _, a := range r {
		if jsonOut {
			j, err := json.Marshal(a)
			if err != nil {
				continue
			}
			fmt.Println(string(j))
		} else {
			fmt.Printf("%s\t%s\t%s\t%d\t%.2f\t%s\n", a.ID, a.NodeName, a.PollingName, a.Count, a.Score, time.Unix(a.LastTime, 0).Format(time.RFC3339Nano))
		}
	}
}

func twsnmpEventLog() {
	c := getTWSNMPClient()
	st, et := getTimeRange()
	f := &client.EventLogFilter{
		StartDate: time.Unix(0, st).Format("2006-01-02"),
		StartTime: time.Unix(0, st).Format("15:04"),
		EndDate:   time.Unix(0, et).Format("2006-01-02"),
		EndTime:   time.Unix(0, et).Format("15:04"),
	}
	r, err := c.GetEventLogs(f)
	if err != nil {
		log.Fatalf("get eventlog err=%v", err)
	}
	for _, l := range r.EventLogs {
		if jsonOut {
			j, err := json.Marshal(l)
			if err != nil {
				continue
			}
			fmt.Println(string(j))
		} else {
			fmt.Printf("%s\t%s\t%s\t%s\n", time.Unix(0, l.Time).Format(time.RFC3339Nano), l.Type, l.NodeName, l.Event)
		}
	}
}

func twsnmpSyslog() {
	c := getTWSNMPClient()
	st, et := getTimeRange()
	f := &client.SyslogFilter{
		NextTime: st,
	}
	for ct := st; ct <= et; {
		r, err := c.GetSyslogs(f)
		if err != nil {
			log.Fatalf("get syslog err=%v", err)
		}
		for _, l := range r.Logs {
			if jsonOut {
				j, err := json.Marshal(l)
				if err != nil {
					continue
				}
				fmt.Println(string(j))
			} else {
				fmt.Printf("%s\t%s\t%s\t%s\n", time.Unix(0, l.Time).Format(time.RFC3339Nano), l.Type, l.Tag, l.Message)
			}
			if l.Time > ct {
				ct = l.Time
			}
		}
		if r.NextTime == 0 {
			break
		}
	}
}

func twsnmpTrap() {
	c := getTWSNMPClient()
	st, et := getTimeRange()
	f := &client.SnmpTrapFilter{
		StartDate: time.Unix(0, st).Format("2006-01-02"),
		StartTime: time.Unix(0, st).Format("15:04"),
		EndDate:   time.Unix(0, et).Format("2006-01-02"),
		EndTime:   time.Unix(0, et).Format("15:04"),
	}
	r, err := c.GetSnmpTraps(f)
	if err != nil {
		log.Fatalf("get snmp trap err=%v", err)
	}
	for _, l := range r {
		if jsonOut {
			j, err := json.Marshal(l)
			if err != nil {
				continue
			}
			fmt.Println(string(j))
		} else {
			vb := strings.ReplaceAll(l.Variables, "\r", "")
			vb = strings.ReplaceAll(vb, "\n", "\t")
			fmt.Printf("%s\t%s\t%s\t%s\n", time.Unix(0, l.Time).Format(time.RFC3339Nano), l.FromAddress, l.TrapType, vb)
		}
	}
}

func twsnmpNetFlow(ipfix bool) {
	c := getTWSNMPClient()
	st, et := getTimeRange()
	f := &client.NetflowFilter{
		NextTime: st,
	}
	for ct := st; ct <= et; {
		var r *client.NetflowWebAPI
		var err error
		if ipfix {
			r, err = c.GetIPFIX(f)
		} else {
			r, err = c.GetNetFlow(f)
		}
		if err != nil {
			log.Fatalf("get netflow err=%v", err)
		}
		for _, l := range r.Logs {
			j, err := json.Marshal(l)
			if err != nil {
				continue
			}
			if jsonOut {
				fmt.Println(string(j))
			} else {
				fmt.Printf("%s\t%s\n", time.Unix(0, l.Time).Format(time.RFC3339Nano), string(j))
			}
			if l.Time > ct {
				ct = l.Time
			}
		}
		if r.NextTime == 0 {
			break
		}
	}
}

func twsnmpSFlow() {
	c := getTWSNMPClient()
	st, et := getTimeRange()
	f := &client.SFlowFilter{
		NextTime: st,
	}
	for ct := st; ct <= et; {
		r, err := c.GetSFlow(f)
		if err != nil {
			log.Fatalf("get sflow err=%v", err)
		}
		for _, l := range r.Logs {
			j, err := json.Marshal(l)
			if err != nil {
				continue
			}
			if jsonOut {
				fmt.Println(string(j))
			} else {
				fmt.Printf("%s\t%s\n", time.Unix(0, l.Time).Format(time.RFC3339Nano), string(j))
			}
			if l.Time > ct {
				ct = l.Time
			}
		}
		if r.NextTime == 0 {
			break
		}
	}
}

func twsnmpSFlowCounter() {
	c := getTWSNMPClient()
	st, et := getTimeRange()
	f := &client.SFlowCounterFilter{
		NextTime: st,
	}
	for ct := st; ct <= et; {
		r, err := c.GetSFlowCounter(f)
		if err != nil {
			log.Fatalf("get sflow err=%v", err)
		}
		for _, l := range r.Logs {
			j, err := json.Marshal(l)
			if err != nil {
				continue
			}
			if jsonOut {
				fmt.Println(string(j))
			} else {
				fmt.Printf("%s\t%s\n", time.Unix(0, l.Time).Format(time.RFC3339Nano), string(j))
			}
			if l.Time > ct {
				ct = l.Time
			}
		}
		if r.NextTime == 0 {
			break
		}
	}
}

func twsnmpArpLog() {
	c := getTWSNMPClient()
	st, et := getTimeRange()
	f := &client.ArpFilter{
		StartDate: time.Unix(0, st).Format("2006-01-02"),
		StartTime: time.Unix(0, st).Format("15:04"),
		EndDate:   time.Unix(0, et).Format("2006-01-02"),
		EndTime:   time.Unix(0, et).Format("15:04"),
	}
	r, err := c.GetArpLogs(f)
	if err != nil {
		log.Fatalf("get arp logs err=%v", err)
	}
	for _, l := range r {
		if jsonOut {
			j, err := json.Marshal(l)
			if err != nil {
				continue
			}
			fmt.Println(string(j))
		} else {
			fmt.Printf("%s\t%s\t%s\t%s\t%s\t%s\t%s\n", time.Unix(0, l.Time).Format(time.RFC3339Nano), l.State, l.IP, l.MAC, l.Vendor, l.OldMAC, l.OldVendor)
		}
	}
}

func twsnmpPollingLog(id string) {
	c := getTWSNMPClient()
	st, et := getTimeRange()
	f := &client.TimeFilter{
		StartDate: time.Unix(0, st).Format("2006-01-02"),
		StartTime: time.Unix(0, st).Format("15:04"),
		EndDate:   time.Unix(0, et).Format("2006-01-02"),
		EndTime:   time.Unix(0, et).Format("15:04"),
	}
	r, err := c.GetPollingLogs(id, f)
	if err != nil {
		log.Fatalf("get polling log err=%v", err)
	}
	for _, l := range r {
		j, err := json.Marshal(l)
		if err != nil {
			continue
		}
		if jsonOut {
			fmt.Println(string(j))
		} else {
			fmt.Printf("%s\t%s\n", time.Unix(0, l.Time).Format(time.RFC3339Nano), string(j))
		}
	}
}
