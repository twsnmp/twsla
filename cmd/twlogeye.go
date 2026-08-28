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
	"fmt"

	"github.com/spf13/cobra"
)

// twlogeyeCmd represents the twlogeye command (wrapper to import)
var twlogeyeCmd = &cobra.Command{
	Use:   "twlogeye",
	Short: "Import notify,logs and report from twlogeye (deprecated: use 'import twlogeye://...' instead)",
	Long: `Import notify,logs and report from twlogeye
twsla twlogeye <target> [<sub target>] [<anomaly report type>]
  target: notify | logs | report 
	logs sub target: syslog | trap | netflow | winevent | otel | mqtt
	report sub target: syslog | trap | netflow | winevent | otel | mqtt | monitor | anomaly 
	anomaly report type: syslog | trap | netflow | winevent | otel | mqtt | monitor | anomaly 
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
		if twLogEyeApiServer == "" {
			twLogEyeApiServer = "localhost"
		}
		if twLogEyeApiPort == 0 {
			twLogEyeApiPort = 8081
		}
		src := fmt.Sprintf("twlogeye://%s:%d/%s", twLogEyeApiServer, twLogEyeApiPort, twLogEyeTarget)
		if twLogEyeSubTarget != "" {
			src += "/" + twLogEyeSubTarget
			if twLogEyeAnomalyReportType != "" && twLogEyeSubTarget == "anomaly" {
				src += "/" + twLogEyeAnomalyReportType
			}
		}
		sources = []string{src}
		importMain()
	},
}

func init() {
	rootCmd.AddCommand(twlogeyeCmd)
	twlogeyeCmd.Flags().StringVar(&twLogEyeApiServer, "apiServer", "", "twlogeye api server IP address")
	twlogeyeCmd.Flags().IntVar(&twLogEyeApiPort, "apiPort", 8081, "twlogeye api port number")
	twlogeyeCmd.Flags().StringVar(&twLogEyeCaCert, "ca", "", "CA Cert file path")
	twlogeyeCmd.Flags().StringVar(&twLogEyeClientCert, "cert", "", "Client cert file path")
	twlogeyeCmd.Flags().StringVar(&twLogEyeClientKey, "key", "", "Client key file path")
	twlogeyeCmd.Flags().StringVar(&twLogEyeFilter, "filter", "", "Log search text")
	twlogeyeCmd.Flags().StringVar(&twLogEyeLevel, "level", "", "Notify level")
	twlogeyeCmd.Flags().StringVar(&twLogEyeAnomalyReportType, "anomaly", "monitor", "Anomaly report type")
}
