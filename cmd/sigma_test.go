package cmd

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/bradleyjkemp/sigma-go"
	"github.com/bradleyjkemp/sigma-go/evaluator"
	sigmaPkg "github.com/twsnmp/twsla/pkg/sigma"
)

func captureStdout(f func()) string {
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	f()

	w.Close()
	os.Stdout = old
	var buf bytes.Buffer
	io.Copy(&buf, r)
	return buf.String()
}

func TestSigmaCLI_Packs(t *testing.T) {
	out := captureStdout(func() {
		showSigmaPacks()
	})
	if !strings.Contains(out, "Available Rule Packs") {
		t.Errorf("expected Available Rule Packs header, got: %s", out)
	}
	if !strings.Contains(out, "windows-essential") || !strings.Contains(out, "linux-auth") {
		t.Errorf("expected standard packs in output, got: %s", out)
	}
}

func TestSigmaCLI_Stat(t *testing.T) {
	out := captureStdout(func() {
		sigmaStat()
	})
	if !strings.Contains(out, "Total Rules: 75") {
		t.Errorf("expected Total Rules: 75, got: %s", out)
	}
	if !strings.Contains(out, "Top Tags") {
		t.Errorf("expected Top Tags in stat output, got: %s", out)
	}
	if !strings.Contains(out, "attack.") {
		t.Errorf("expected MITRE ATT&CK tags in stat output, got: %s", out)
	}
}

func TestSigmaCLI_RuleList(t *testing.T) {
	filterPack = "linux-auth"
	defer func() { filterPack = "" }()

	out := captureStdout(func() {
		sigmaRuleList()
	})
	if !strings.Contains(out, "pack:linux-auth") {
		t.Errorf("expected pack:linux-auth in list output, got: %s", out)
	}
	if !strings.Contains(out, "SSH Failed Password Authentication") {
		t.Errorf("expected SSH rule title, got: %s", out)
	}
}

func TestSigmaCLI_TestMatching(t *testing.T) {
	out := captureStdout(func() {
		sigmaTest([]string{
			"test",
			`{"Event":{"System":{"Channel":"Security","Computer":"WIN-DC","EventID":4625,"Level":0},"EventData":{"TargetUserName":"admin","WorkstationName":"DESKTOP-1"}}}`,
		})
	})
	if !strings.Contains(out, "HIT:") {
		t.Errorf("expected HIT in test output, got: %s", out)
	}
	if !strings.Contains(out, "018d9f10-ff31-419b-a36c-941cbfa0451a") {
		t.Errorf("expected Windows 4625 rule ID, got: %s", out)
	}
}

func TestSigmaModel_TagAggregationAndCycling(t *testing.T) {
	m := initSigmaModel()
	m.done = true
	if m.viewMode != viewModeEvents {
		t.Errorf("expected initial viewMode = Events (%d), got %d", viewModeEvents, m.viewMode)
	}

	// Cycle mode: Events -> RuleCount -> TagCount -> Events
	resM, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'c'}})
	m = resM.(sigmaModel)
	if m.viewMode != viewModeRuleCount {
		t.Errorf("expected viewMode = RuleCount (%d), got %d", viewModeRuleCount, m.viewMode)
	}

	resM, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'c'}})
	m = resM.(sigmaModel)
	if m.viewMode != viewModeTagCount {
		t.Errorf("expected viewMode = TagCount (%d), got %d", viewModeTagCount, m.viewMode)
	}

	resM, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'c'}})
	m = resM.(sigmaModel)
	if m.viewMode != viewModeEvents {
		t.Errorf("expected viewMode = Events (%d), got %d", viewModeEvents, m.viewMode)
	}
}

func TestSigmaModel_DoneMessagePopulatesTagCounts(t *testing.T) {
	// Create dummy sigmaList
	dummyRule := sigma.Rule{
		Title: "Test Rule",
		ID:    "dummy-id-1",
		Tags:  []string{"attack.t1110", "pci_dss_10.2.4"},
	}
	ev := evaluator.ForRule(dummyRule)

	sigmaList = []sigmaEnt{
		{
			Log:       0,
			Evaluator: ev,
			Entry: &sigmaPkg.SigmaRuleEntry{
				Evaluator: ev,
				Source:    "pack:test",
			},
		},
	}
	times = []int64{time.Now().UnixNano()}
	results = []string{`{"test":"log"}`}

	m := initSigmaModel()
	newM, _ := m.Update(sigmaMsg{Done: true, Lines: 1, Hit: 1, Match: 1, Dur: time.Second})
	sm, ok := newM.(sigmaModel)
	if !ok {
		t.Fatalf("expected sigmaModel type")
	}
	if !sm.done {
		t.Errorf("expected sm.done = true")
	}

	// Verify Tag counts populated
	if len(sigmaTagCountList) != 2 {
		t.Fatalf("expected 2 tags in sigmaTagCountList, got %d", len(sigmaTagCountList))
	}
	tagCountMap := make(map[string]int)
	for _, tc := range sigmaTagCountList {
		tagCountMap[tc.Tag] = tc.Count
	}
	if tagCountMap["attack.t1110"] != 1 || tagCountMap["pci_dss_10.2.4"] != 1 {
		t.Errorf("unexpected tag counts: %+v", tagCountMap)
	}

	// Verify Rule counts populated
	if len(sigmaCountList) != 1 || sigmaCountList[0].ID != "dummy-id-1" {
		t.Errorf("unexpected rule counts: %+v", sigmaCountList)
	}
}

func TestWazuhConvertCmd(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "wazuh_convert_test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	xmlContent := `
<group name="syslog,sshd,">
  <rule id="5710" level="5">
    <match>illegal user|invalid user</match>
    <description>sshd: Attempt to login using a non-existent user</description>
    <mitre><id>T1110</id></mitre>
  </rule>
</group>
`
	xmlFile := filepath.Join(tmpDir, "rule.xml")
	if err := os.WriteFile(xmlFile, []byte(xmlContent), 0644); err != nil {
		t.Fatal(err)
	}

	outDir := filepath.Join(tmpDir, "out")
	rootCmd.SetArgs([]string{"sigma", "convert-wazuh", "-o", outDir, xmlFile})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("convert-wazuh command execute failed: %v", err)
	}

	targetFile := filepath.Join(outDir, "wazuh-5710.yaml")
	if _, err := os.Stat(targetFile); os.IsNotExist(err) {
		t.Errorf("expected converted file %s to exist", targetFile)
	}
}

func TestWazuhConvertDecoderCmd(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "wazuh_decoder_test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	xmlContent := `
<decoder name="sshd-success">
  <prematch>^Accepted</prematch>
  <regex offset="after_prematch">^ \S+ for (\S+) from (\S+) port (\d+)</regex>
  <order>user, srcip, srcport</order>
</decoder>
`
	xmlFile := filepath.Join(tmpDir, "decoder.xml")
	if err := os.WriteFile(xmlFile, []byte(xmlContent), 0644); err != nil {
		t.Fatal(err)
	}

	outDir := filepath.Join(tmpDir, "out")
	rootCmd.SetArgs([]string{"sigma", "convert-wazuh-decoder", "-o", outDir, xmlFile})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("convert-wazuh-decoder command execute failed: %v", err)
	}

	targetFile := filepath.Join(outDir, "wazuh_named_captures.txt")
	if _, err := os.Stat(targetFile); os.IsNotExist(err) {
		t.Errorf("expected converted file %s to exist", targetFile)
	}
	content, _ := os.ReadFile(targetFile)
	if !strings.Contains(string(content), "(?P<user>\\S+)") {
		t.Errorf("expected named capture (?P<user>\\S+), got: %s", string(content))
	}
}
