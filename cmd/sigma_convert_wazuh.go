package cmd

import (
	"fmt"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/twsnmp/twsla/pkg/sigma"
)

var (
	wazuhOutputDir     string
	wazuhMinLevel      int
	wazuhSkipFrequency bool
	wazuhStdout        bool
	wazuhService       string
	wazuhProduct       string
)

var convertWazuhCmd = &cobra.Command{
	Use:   "convert-wazuh [flags] <xml-file-or-dir>",
	Short: "Convert Wazuh XML rules to Sigma YAML rules",
	Long: `Convert Wazuh XML rule files to Sigma YAML rule files with correlation support.
Example:
  twsla sigma convert-wazuh -o ./converted-rules ./ruleset/rules/0095-sshd_rules.xml
  twsla sigma convert-wazuh -o ./converted-rules ./ruleset/rules/
`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		inputPath := args[0]
		xmlFiles := []string{}

		info, err := os.Stat(inputPath)
		if err != nil {
			log.Fatalf("cannot access input path: %v", err)
		}

		if info.IsDir() {
			_ = filepath.WalkDir(inputPath, func(p string, d fs.DirEntry, err error) error {
				if err != nil || d.IsDir() {
					return nil
				}
				if strings.HasSuffix(strings.ToLower(p), ".xml") {
					xmlFiles = append(xmlFiles, p)
				}
				return nil
			})
		} else {
			xmlFiles = append(xmlFiles, inputPath)
		}

		if len(xmlFiles) == 0 {
			log.Fatalf("no XML rule files found in %s", inputPath)
		}

		// 1. Collect all Wazuh rules across files
		var allRules []sigma.WazuhRuleXML
		for _, f := range xmlFiles {
			data, err := os.ReadFile(f)
			if err != nil {
				log.Printf("failed to read %s: %v", f, err)
				continue
			}
			rules, err := sigma.ParseWazuhRulesXML(data)
			if err != nil {
				log.Printf("warning: parse %s failed: %v", f, err)
				continue
			}
			allRules = append(allRules, rules...)
		}

		fmt.Printf("Parsed %d raw rules from %d XML file(s)\n", len(allRules), len(xmlFiles))

		// 2. Resolve hierarchy
		resolved := sigma.ResolveRuleHierarchy(allRules, wazuhService)

		// 3. Convert to Sigma
		opts := sigma.WazuhConvertOptions{
			MinLevel:       wazuhMinLevel,
			SkipFrequency:  wazuhSkipFrequency,
			DefaultProduct: wazuhProduct,
			DefaultService: wazuhService,
		}

		if !wazuhStdout {
			if err := os.MkdirAll(wazuhOutputDir, 0755); err != nil {
				log.Fatalf("failed to create output directory %s: %v", wazuhOutputDir, err)
			}
		}

		convertedCount := 0
		correlationCount := 0

		for _, r := range resolved {
			sigmaRule, err := sigma.ConvertWazuhRuleToSigma(r, opts)
			if err != nil || sigmaRule == nil {
				continue
			}

			yamlBytes, err := sigma.FormatSigmaYAML(sigmaRule)
			if err != nil {
				log.Printf("failed to format rule %s to YAML: %v", sigmaRule.ID, err)
				continue
			}

			if sigmaRule.Correlation != nil {
				correlationCount++
			}

			if wazuhStdout {
				fmt.Println("---")
				fmt.Print(string(yamlBytes))
			} else {
				filename := fmt.Sprintf("%s.yaml", strings.ToLower(sigmaRule.ID))
				target := filepath.Join(wazuhOutputDir, filename)
				if err := os.WriteFile(target, yamlBytes, 0644); err != nil {
					log.Printf("failed to write %s: %v", target, err)
					continue
				}
			}
			convertedCount++
		}

		if !wazuhStdout {
			fmt.Printf("Successfully converted %d rules (%d with correlation) to %s\n",
				convertedCount, correlationCount, wazuhOutputDir)
		}
	},
}

var convertWazuhDecoderCmd = &cobra.Command{
	Use:   "convert-wazuh-decoder [flags] <xml-file-or-dir>",
	Short: "Convert Wazuh XML decoders to named-capture regex patterns",
	Long: `Convert Wazuh XML decoder files to Go named-capture regex patterns for namedCaptures.
Example:
  twsla sigma convert-wazuh-decoder ./decoders/0310-ssh_decoders.xml
`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		inputPath := args[0]
		xmlFiles := []string{}

		info, err := os.Stat(inputPath)
		if err != nil {
			log.Fatalf("cannot access input path: %v", err)
		}

		if info.IsDir() {
			_ = filepath.WalkDir(inputPath, func(p string, d fs.DirEntry, err error) error {
				if err != nil || d.IsDir() {
					return nil
				}
				if strings.HasSuffix(strings.ToLower(p), ".xml") {
					xmlFiles = append(xmlFiles, p)
				}
				return nil
			})
		} else {
			xmlFiles = append(xmlFiles, inputPath)
		}

		var allDecoders []sigma.WazuhDecoderXML
		for _, f := range xmlFiles {
			data, err := os.ReadFile(f)
			if err != nil {
				log.Printf("failed to read %s: %v", f, err)
				continue
			}
			decs, err := sigma.ParseWazuhDecodersXML(data)
			if err != nil {
				log.Printf("warning: parse %s failed: %v", f, err)
				continue
			}
			allDecoders = append(allDecoders, decs...)
		}

		fmt.Printf("Found %d decoders in %d XML file(s)\n", len(allDecoders), len(xmlFiles))

		patterns := []string{}
		for _, d := range allDecoders {
			if d.Regex == "" || d.Order == "" {
				continue
			}
			re, err := sigma.ConvertDecoderToNamedRegex(d)
			if err != nil {
				continue
			}
			patterns = append(patterns, re)
			if wazuhStdout {
				fmt.Printf("# Decoder: %s (parent: %s)\n%s\n\n", d.Name, d.Parent, re)
			}
		}

		if !wazuhStdout && wazuhOutputDir != "" {
			_ = os.MkdirAll(wazuhOutputDir, 0755)
			target := filepath.Join(wazuhOutputDir, "wazuh_named_captures.txt")
			content := strings.Join(patterns, "\n") + "\n"
			if err := os.WriteFile(target, []byte(content), 0644); err != nil {
				log.Fatalf("failed to write %s: %v", target, err)
			}
			fmt.Printf("Generated %d named-capture patterns to %s\n", len(patterns), target)
		} else if !wazuhStdout {
			fmt.Printf("Generated %d valid named-capture patterns:\n", len(patterns))
			for _, p := range patterns {
				fmt.Println(p)
			}
		}
	},
}

func init() {
	sigmaCmd.AddCommand(convertWazuhCmd)
	sigmaCmd.AddCommand(convertWazuhDecoderCmd)

	// Flags for convert-wazuh
	convertWazuhCmd.Flags().StringVarP(&wazuhOutputDir, "output", "o", "./converted-rules", "Output directory for Sigma YAML rules")
	convertWazuhCmd.Flags().IntVar(&wazuhMinLevel, "min-level", 3, "Minimum Wazuh rule level to convert (0-16)")
	convertWazuhCmd.Flags().BoolVar(&wazuhSkipFrequency, "skip-frequency", false, "Skip rules with frequency correlation")
	convertWazuhCmd.Flags().BoolVar(&wazuhStdout, "stdout", false, "Output converted rules to stdout")
	convertWazuhCmd.Flags().StringVar(&wazuhService, "service", "", "Override logsource.service")
	convertWazuhCmd.Flags().StringVar(&wazuhProduct, "product", "linux", "Override logsource.product")

	// Flags for convert-wazuh-decoder
	convertWazuhDecoderCmd.Flags().StringVarP(&wazuhOutputDir, "output", "o", "", "Output file/dir for named-capture patterns")
	convertWazuhDecoderCmd.Flags().BoolVar(&wazuhStdout, "stdout", false, "Output patterns with comments to stdout")
}
