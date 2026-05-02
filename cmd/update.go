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
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/blang/semver"
	"github.com/rhysd/go-github-selfupdate/selfupdate"
	"github.com/spf13/cobra"
)

var updateCheck bool
var updateVersion string
var updateYes bool

// updateCmd represents the update command
var updateCmd = &cobra.Command{
	Use:   "update",
	Short: "Update twsla to the latest or specified version",
	Long:  `Update twsla to the latest or specified version from GitHub releases.`,
	Run: func(cmd *cobra.Command, args []string) {
		if updateCheck {
			checkUpdate()
			return
		}
		if updateVersion != "" {
			updateToVersion(updateVersion)
			return
		}
		updateToLatest()
	},
}

func init() {
	rootCmd.AddCommand(updateCmd)
	updateCmd.Flags().BoolVarP(&updateCheck, "check", "c", false, "Check for updates only")
	updateCmd.Flags().StringVar(&updateVersion, "version", "", "Update to specified version")
	updateCmd.Flags().BoolVarP(&updateYes, "yes", "y", false, "Update without confirmation")
}

func checkUpdate() {
	v, err := semver.Parse(strings.TrimPrefix(Version, "v"))
	if err != nil {
		fmt.Printf("Invalid current version format: %v\n", err)
		return
	}

	latest, ok, err := selfupdate.DetectLatest("twsnmp/twsla")
	if err != nil {
		fmt.Printf("Binary update check failed: %v\n", err)
		return
	}

	if !ok {
		fmt.Println("No release found on GitHub for your OS/Arch.")
		return
	}

	if latest.Version.GT(v) {
		fmt.Printf("New version v%s is available! (Current: v%s)\n", latest.Version, v)
		fmt.Printf("Release notes:\n%s\n", latest.ReleaseNotes)
		fmt.Println("Run 'twsla update' to upgrade.")
	} else {
		fmt.Printf("twsla is up to date (v%s).\n", v)
	}
}

func updateToLatest() {
	v, err := semver.Parse(strings.TrimPrefix(Version, "v"))
	if err != nil {
		fmt.Printf("Invalid current version format: %v\n", err)
		return
	}

	latest, ok, err := selfupdate.DetectLatest("twsnmp/twsla")
	if err != nil {
		fmt.Printf("Binary update check failed: %v\n", err)
		return
	}

	if !ok || latest.Version.LTE(v) {
		fmt.Printf("twsla is already up to date (v%s).\n", v)
		return
	}

	if !updateYes {
		fmt.Printf("New version v%s is available.\n", latest.Version)
		fmt.Printf("Release notes:\n%s\n", latest.ReleaseNotes)
		fmt.Printf("Do you want to update to v%s? [y/N]: ", latest.Version)
		input, _ := bufio.NewReader(os.Stdin).ReadString('\n')
		if strings.ToLower(strings.TrimSpace(input)) != "y" {
			fmt.Println("Update cancelled.")
			return
		}
	}

	fmt.Printf("Updating to v%s...\n", latest.Version)
	if err := selfupdate.UpdateTo(latest.AssetURL, os.Args[0]); err != nil {
		fmt.Printf("Update failed: %v\n", err)
		return
	}

	fmt.Printf("Successfully updated to v%s\n", latest.Version)
}

func updateToVersion(target string) {
	_, err := semver.Parse(strings.TrimPrefix(Version, "v"))
	if err != nil {
		fmt.Printf("Invalid current version format: %v\n", err)
		return
	}

	targetV, err := semver.Parse(strings.TrimPrefix(target, "v"))
	if err != nil {
		fmt.Printf("Invalid target version format: %v\n", err)
		return
	}

	rel, ok, err := selfupdate.DetectVersion("twsnmp/twsla", "v"+targetV.String())
	if err != nil {
		fmt.Printf("Target version check failed: %v\n", err)
		return
	}
	if !ok {
		fmt.Printf("Version v%s not found on GitHub or no suitable asset found.\n", targetV)
		return
	}

	if !updateYes {
		fmt.Printf("Do you want to update to v%s? [y/N]: ", targetV)
		input, _ := bufio.NewReader(os.Stdin).ReadString('\n')
		if strings.ToLower(strings.TrimSpace(input)) != "y" {
			fmt.Println("Update cancelled.")
			return
		}
	}

	fmt.Printf("Updating to v%s...\n", targetV)
	if err := selfupdate.UpdateTo(rel.AssetURL, os.Args[0]); err != nil {
		fmt.Printf("Update failed: %v\n", err)
		return
	}

	fmt.Printf("Successfully updated to v%s\n", targetV)
}
