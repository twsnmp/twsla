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
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var cfgFile string
var chartTmp string
var sixelChart bool

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "twsla",
	Short: "Simple Log Analyzer",
	Long:  `Simple Log Analyzer by TWSNMP`,
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	td, err := os.MkdirTemp("", "twslaCharts")
	if err != nil {
		panic(err)
	}
	chartTmp = td
	defer os.RemoveAll(td)
	err = rootCmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}

func init() {
	cobra.OnInitialize(initConfig)
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is $HOME/.twsla.yaml)")
	rootCmd.PersistentFlags().StringVarP(&dataStore, "datastore", "d", "./twsla.db", "Bblot log db")
	rootCmd.PersistentFlags().StringVarP(&timeRange, "timeRange", "t", "", "Time range")
	rootCmd.PersistentFlags().StringVarP(&simpleFilter, "filter", "f", "", "Simple filter")
	rootCmd.PersistentFlags().StringVarP(&regexpFilter, "regex", "r", "", "Regexp filter")
	rootCmd.PersistentFlags().StringVarP(&notFilter, "not", "v", "", "Invert regexp filter")
	rootCmd.PersistentFlags().BoolVar(&sixelChart, "sixel", false, "show chart by sixel")
}

// initConfig reads in config file and ENV variables if set.
func initConfig() {
	if cfgFile != "" {
		// Use config file from the flag.
		viper.SetConfigFile(cfgFile)
	} else {
		// Find home directory.
		home, err := os.UserHomeDir()
		cobra.CheckErr(err)

		// Search config in home directory with name ".twsla" (without extension).
		viper.AddConfigPath(home)
		viper.SetConfigType("yaml")
		viper.SetConfigName(".twsla")
	}

	viper.AutomaticEnv() // read in environment variables that match
	viper.SetEnvPrefix("twsla")
	viper.BindEnv("datastore")
	viper.BindEnv("geoip")
	viper.BindEnv("grok")
	viper.BindEnv("sixel")
	viper.BindEnv("weaviate")
	viper.BindEnv("aiClass")

	// If a config file is found, read it in.
	if err := viper.ReadInConfig(); err == nil {
		fmt.Fprintln(os.Stderr, "Using config file:", viper.ConfigFileUsed())
		if v := viper.GetString("timeRange"); v != "" {
			fmt.Fprintln(os.Stderr, " timeRange:", v)
			timeRange = v
		}
		if v := viper.GetString("filter"); v != "" {
			fmt.Fprintln(os.Stderr, " filter:", v)
			simpleFilter = v
		}
		if v := viper.GetString("regex"); v != "" {
			fmt.Fprintln(os.Stderr, " regex:", v)
			regexpFilter = v
		}
		if v := viper.GetString("not"); v != "" {
			fmt.Fprintln(os.Stderr, " not:", v)
			notFilter = v
		}
		if v := viper.GetString("extract"); v != "" {
			fmt.Fprintln(os.Stderr, " extract:", v)
			extract = v
		}
		if v := viper.GetString("name"); v != "" {
			fmt.Fprintln(os.Stderr, " name:", v)
			name = v
		}
		if v := viper.GetString("grokPat"); v != "" {
			fmt.Fprintln(os.Stderr, " grokPat:", v)
			grokPat = v
		}
		if v := viper.GetString("ip"); v != "" {
			fmt.Fprintln(os.Stderr, " ip:", v)
			ipInfoMode = v
		}
		if v := viper.GetString("color"); v != "" {
			fmt.Fprintln(os.Stderr, " color:", v)
			colorMode = v
		}
		if v := viper.GetString("rules"); v != "" {
			fmt.Fprintln(os.Stderr, " rules:", v)
			sigmaRules = v
		}
		if v := viper.GetString("sigmaConfig"); v != "" {
			fmt.Fprintln(os.Stderr, " sigmaConfig:", v)
			sigmaConfig = v
		}
		if v := viper.GetString("twsnmp"); v != "" {
			fmt.Fprintln(os.Stderr, " twsnmp:", v)
			twsnmp = v
		}
		if v := viper.GetInt("interval"); v != 0 {
			fmt.Fprintln(os.Stderr, " interval:", v)
			interval = v
		}
		if v := viper.GetBool("jsonOut"); v {
			fmt.Fprintln(os.Stderr, " jsonOut:", v)
			jsonOut = v
		}
		if v := viper.GetBool("checkCert"); v {
			fmt.Fprintln(os.Stderr, " checkCert:", v)
			checkCert = v
		}
	}
	if v := viper.GetString("datastore"); v != "" {
		fmt.Fprintln(os.Stderr, " datastore:", v)
		dataStore = v
	}
	if v := viper.GetString("geoip"); v != "" {
		fmt.Fprintln(os.Stderr, " geoip:", v)
		geoipDBPath = v
	}
	if v := viper.GetString("grok"); v != "" {
		fmt.Fprintln(os.Stderr, " grok:", v)
		grokDef = v
	}
	if v := viper.GetBool("sixel"); v {
		fmt.Fprintln(os.Stderr, " sixel:", v)
		sixelChart = v
	}
}
