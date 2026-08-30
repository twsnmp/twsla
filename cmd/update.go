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
	"bufio"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/blang/semver"
	"github.com/inconshreveable/go-update"
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

type ghAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

type ghRelease struct {
	TagName string    `json:"tag_name"`
	Body    string    `json:"body"`
	Assets  []ghAsset `json:"assets"`
}

func fetchGitHubRelease(tag string) (*ghRelease, error) {
	url := "https://api.github.com/repos/twsnmp/twsla/releases/latest"
	if tag != "" {
		if !strings.HasPrefix(tag, "v") {
			tag = "v" + tag
		}
		url = fmt.Sprintf("https://api.github.com/repos/twsnmp/twsla/releases/tags/%s", tag)
	}

	client := &http.Client{Timeout: 30 * time.Second}
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	req.Header.Set("User-Agent", "twsla-selfupdate")

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d from GitHub API", resp.StatusCode)
	}

	var rel ghRelease
	if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
		return nil, err
	}
	return &rel, nil
}

func findMatchingAsset(rel *ghRelease) (*ghAsset, error) {
	osName := runtime.GOOS
	arch := runtime.GOARCH

	for _, a := range rel.Assets {
		nameLower := strings.ToLower(a.Name)
		if !strings.Contains(nameLower, osName) {
			continue
		}

		matchArch := false
		switch arch {
		case "amd64":
			matchArch = strings.Contains(nameLower, "amd64") || strings.Contains(nameLower, "x86_64")
		case "arm64":
			matchArch = strings.Contains(nameLower, "arm64") || strings.Contains(nameLower, "aarch64")
		case "386":
			matchArch = strings.Contains(nameLower, "386") || strings.Contains(nameLower, "i386")
		case "arm":
			matchArch = strings.Contains(nameLower, "armv") || strings.Contains(nameLower, "arm")
		default:
			matchArch = strings.Contains(nameLower, arch)
		}

		if matchArch {
			return &a, nil
		}
	}
	return nil, fmt.Errorf("no matching asset found for OS=%s ARCH=%s in release %s", osName, arch, rel.TagName)
}

func downloadAndApplyUpdate(asset *ghAsset) error {
	client := &http.Client{Timeout: 5 * time.Minute}
	req, err := http.NewRequest("GET", asset.BrowserDownloadURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "twsla-selfupdate")

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d when downloading release asset", resp.StatusCode)
	}

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response body: %w", err)
	}

	assetName := strings.ToLower(asset.Name)
	var binaryReader io.Reader

	if strings.HasSuffix(assetName, ".tar.gz") || strings.HasSuffix(assetName, ".tgz") {
		gr, err := gzip.NewReader(bytes.NewReader(bodyBytes))
		if err != nil {
			return fmt.Errorf("failed to open gzip stream: %w", err)
		}
		defer gr.Close()

		tr := tar.NewReader(gr)
		var extractedBytes []byte
		for {
			hdr, err := tr.Next()
			if err == io.EOF {
				break
			}
			if err != nil {
				return fmt.Errorf("tar read error: %w", err)
			}
			baseName := strings.ToLower(hdr.Name)
			if baseName == "twsla" || baseName == "twsla.exe" || strings.HasSuffix(baseName, "/twsla") || strings.HasSuffix(baseName, "/twsla.exe") {
				extractedBytes, err = io.ReadAll(tr)
				if err != nil {
					return fmt.Errorf("failed reading binary from tar: %w", err)
				}
				break
			}
		}
		if len(extractedBytes) == 0 {
			return fmt.Errorf("twsla binary not found inside archive")
		}
		binaryReader = bytes.NewReader(extractedBytes)
	} else if strings.HasSuffix(assetName, ".zip") {
		zr, err := zip.NewReader(bytes.NewReader(bodyBytes), int64(len(bodyBytes)))
		if err != nil {
			return fmt.Errorf("failed to open zip archive: %w", err)
		}
		var extractedBytes []byte
		for _, f := range zr.File {
			baseName := strings.ToLower(f.Name)
			if baseName == "twsla" || baseName == "twsla.exe" || strings.HasSuffix(baseName, "/twsla") || strings.HasSuffix(baseName, "/twsla.exe") {
				rc, err := f.Open()
				if err != nil {
					return fmt.Errorf("failed opening zipped file: %w", err)
				}
				extractedBytes, err = io.ReadAll(rc)
				rc.Close()
				if err != nil {
					return fmt.Errorf("failed reading binary from zip: %w", err)
				}
				break
			}
		}
		if len(extractedBytes) == 0 {
			return fmt.Errorf("twsla binary not found inside zip archive")
		}
		binaryReader = bytes.NewReader(extractedBytes)
	} else {
		binaryReader = bytes.NewReader(bodyBytes)
	}

	return update.Apply(binaryReader, update.Options{})
}

func checkUpdate() {
	v, err := semver.Parse(strings.TrimPrefix(Version, "v"))
	if err != nil {
		fmt.Printf("Invalid current version format: %v\n", err)
		return
	}

	rel, err := fetchGitHubRelease("")
	if err != nil {
		fmt.Printf("Binary update check failed: %v\n", err)
		return
	}

	latestV, err := semver.Parse(strings.TrimPrefix(rel.TagName, "v"))
	if err != nil {
		fmt.Printf("Invalid latest version format: %v\n", err)
		return
	}

	if latestV.GT(v) {
		fmt.Printf("New version %s is available! (Current: v%s)\n", rel.TagName, v)
		fmt.Printf("Release notes:\n%s\n", rel.Body)
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

	rel, err := fetchGitHubRelease("")
	if err != nil {
		fmt.Printf("Binary update check failed: %v\n", err)
		return
	}

	latestV, err := semver.Parse(strings.TrimPrefix(rel.TagName, "v"))
	if err != nil {
		fmt.Printf("Invalid latest version format: %v\n", err)
		return
	}

	if latestV.LTE(v) {
		fmt.Printf("twsla is already up to date (v%s).\n", v)
		return
	}

	asset, err := findMatchingAsset(rel)
	if err != nil {
		fmt.Printf("Update check failed: %v\n", err)
		return
	}

	if !updateYes {
		fmt.Printf("New version %s is available.\n", rel.TagName)
		fmt.Printf("Release notes:\n%s\n", rel.Body)
		fmt.Printf("Do you want to update to %s? [y/N]: ", rel.TagName)
		input, _ := bufio.NewReader(os.Stdin).ReadString('\n')
		if strings.ToLower(strings.TrimSpace(input)) != "y" {
			fmt.Println("Update cancelled.")
			return
		}
	}

	fmt.Printf("Updating to %s...\n", rel.TagName)
	if err := downloadAndApplyUpdate(asset); err != nil {
		fmt.Printf("Update failed: %v\n", err)
		return
	}

	fmt.Printf("Successfully updated to %s\n", rel.TagName)
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

	rel, err := fetchGitHubRelease("v" + targetV.String())
	if err != nil {
		fmt.Printf("Target version check failed: %v\n", err)
		return
	}

	asset, err := findMatchingAsset(rel)
	if err != nil {
		fmt.Printf("Target version not available for your system: %v\n", err)
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
	if err := downloadAndApplyUpdate(asset); err != nil {
		fmt.Printf("Update failed: %v\n", err)
		return
	}

	fmt.Printf("Successfully updated to v%s\n", targetV)
}

