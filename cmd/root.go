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
	"regexp"
	"strings"
	"time"

	"github.com/araddon/dateparse"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/xhit/go-str2duration/v2"
	"go.etcd.io/bbolt"
)

var cfgFile string
var dataStore string
var timeRange string
var simpleFilter string
var regexpFilter string

// common data
type errMsg error

var db *bbolt.DB
var teaProg *tea.Program
var st time.Time

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "twsla",
	Short: "Simple Log Analyzer",
	Long:  `Simple Log Analyzer by TWSNMP`,
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	err := rootCmd.Execute()
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

	// If a config file is found, read it in.
	if err := viper.ReadInConfig(); err == nil {
		fmt.Fprintln(os.Stderr, "Using config file:", viper.ConfigFileUsed())
	}
}

// openDB : open bbolt DB
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

// getSimpleFilter : get filter from like test* test?k
func getSimpleFilter(f string) *regexp.Regexp {
	if f == "" {
		return nil
	}
	f = regexp.QuoteMeta(f)
	f = strings.ReplaceAll(f, "\\*", ".*")
	f = strings.ReplaceAll(f, "\\?", ".")
	if r, err := regexp.Compile(f); err == nil {
		return r
	}
	return nil
}

func getFilter(f string) *regexp.Regexp {
	if f == "" {
		return nil
	}
	if r, err := regexp.Compile(f); err == nil {
		return r
	}
	return nil
}

func getTimeRange() (int64, int64) {
	st := time.Unix(0, 0)
	et := time.Now()
	a := strings.SplitN(timeRange, ",", 2)
	if len(a) == 1 && a[0] != "" {
		if d, err := str2duration.ParseDuration(a[0]); err == nil {
			st = et.Add(d * -1)
		} else if t, err := dateparse.ParseLocal(a[0]); err == nil {
			st = t
		}
	} else {
		if t, err := dateparse.ParseLocal(a[0]); err == nil {
			st = t
			if t, err := dateparse.ParseLocal(a[1]); err == nil {
				et = t
			} else if d, err := str2duration.ParseDuration(a[1]); err == nil {
				et = st.Add(d)
			}
		}
	}
	return st.UnixNano(), et.UnixNano()
}
