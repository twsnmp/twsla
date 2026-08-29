/*
Copyright © 2026 Masayuki Yamai <twsnmp@gmail.com>

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
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/dustin/go-humanize"
	"github.com/spf13/cobra"
	"github.com/twsnmp/twsla/pkg/model"
)

var modelDirFlag string

// modelCmd represents the model command
var modelCmd = &cobra.Command{
	Use:   "model",
	Short: "Manage local LLM models",
	Long: `Manage local LLM models for embedded AI analysis.
Download, list, and remove models stored locally.`,
}

var modelListCmd = &cobra.Command{
	Use:   "list",
	Short: "List locally downloaded models",
	Run: func(cmd *cobra.Command, args []string) {
		models, err := model.ListModels(modelDirFlag)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error listing models: %v\n", err)
			os.Exit(1)
		}
		if len(models) == 0 {
			fmt.Printf("No models found in %s\n", getEffectiveModelDir())
			fmt.Println("You can download a model with: twsla model download <preset|url>")
			fmt.Println("Available presets:", strings.Join(model.GetPresetNames(), ", "))
			return
		}
		fmt.Printf("Models in %s:\n\n", getEffectiveModelDir())
		fmt.Printf("%-35s %-12s %-10s %s\n", "NAME", "SIZE", "TYPE", "PATH")
		fmt.Println(strings.Repeat("-", 80))
		for _, m := range models {
			fmt.Printf("%-35s %-12s %-10s %s\n", m.Name, m.SizeHuman, m.Type, m.Path)
		}
	},
}

var modelDownloadCmd = &cobra.Command{
	Use:   "download [model_preset_or_url]",
	Short: "Download a model from Hugging Face or URL",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		target := args[0]
		fmt.Printf("Downloading model: %s ...\n", target)
		ctx := context.Background()

		lastPct := -1
		progress := func(downloaded, total int64) {
			if total > 0 {
				pct := int(float64(downloaded) / float64(total) * 100)
				if pct != lastPct {
					fmt.Printf("\rDownloading: %3d%% (%s / %s)",
						pct, humanize.Bytes(uint64(downloaded)), humanize.Bytes(uint64(total)))
					lastPct = pct
				}
			} else {
				fmt.Printf("\rDownloading: %s", humanize.Bytes(uint64(downloaded)))
			}
		}

		path, err := model.DownloadModel(ctx, modelDirFlag, target, progress)
		fmt.Println()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error downloading model: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Model successfully downloaded to: %s\n", path)
	},
}

var modelRemoveCmd = &cobra.Command{
	Use:   "remove [model_name]",
	Short: "Remove a local model",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		name := args[0]
		err := model.RemoveModel(modelDirFlag, name)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error removing model: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Model %q removed successfully.\n", name)
	},
}

var modelPresetsCmd = &cobra.Command{
	Use:   "presets",
	Short: "List available preset models",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("Available Preset Models:")
		fmt.Println()
		presets := model.GetPresetNames()
		sort.Strings(presets)
		for _, name := range presets {
			url := model.PresetModels[name]
			fmt.Printf("  %-16s %s\n", name, url)
		}
		fmt.Println("\nTo download, run: twsla model download <preset_name>")
	},
}

func getEffectiveModelDir() string {
	if modelDirFlag != "" {
		return modelDirFlag
	}
	return model.DefaultModelDir()
}

func init() {
	rootCmd.AddCommand(modelCmd)
	modelCmd.PersistentFlags().StringVar(&modelDirFlag, "modelDir", "", "Directory to store models (default: ~/.twsla/models)")

	modelCmd.AddCommand(modelListCmd)
	modelCmd.AddCommand(modelDownloadCmd)
	modelCmd.AddCommand(modelRemoveCmd)
	modelCmd.AddCommand(modelPresetsCmd)
}
