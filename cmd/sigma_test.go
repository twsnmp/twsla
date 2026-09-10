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

	// Cycle mode: Events -> RuleCount -> TagCount -> Events using 'm'
	resM, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'m'}})
	m = resM.(sigmaModel)
	if m.viewMode != viewModeRuleCount {
		t.Errorf("expected viewMode = RuleCount (%d), got %d", viewModeRuleCount, m.viewMode)
	}

	resM, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'m'}})
	m = resM.(sigmaModel)
	if m.viewMode != viewModeTagCount {
		t.Errorf("expected viewMode = TagCount (%d), got %d", viewModeTagCount, m.viewMode)
	}

	resM, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'m'}})
	m = resM.(sigmaModel)
	if m.viewMode != viewModeEvents {
		t.Errorf("expected viewMode = Events (%d), got %d", viewModeEvents, m.viewMode)
	}
}

func TestSigmaModel_HeaderViewAndHelp(t *testing.T) {
	m := initSigmaModel()
	m.done = true
	m.table.SetWidth(120)

	// Events mode
	hEvents := m.headerView()
	if strings.Contains(hEvents, "[Events]") || strings.Contains(hEvents, "[Tag Count]") || strings.Contains(hEvents, "[Rule Count]") {
		t.Errorf("header should not contain [Events]/[Rule Count]/[Tag Count], got: %s", hEvents)
	}
	if !strings.Contains(hEvents, "Results") {
		t.Errorf("header should contain Results, got: %s", hEvents)
	}
	if !strings.Contains(hEvents, "enter: Show") {
		t.Errorf("Events view should contain 'enter: Show', got: %s", hEvents)
	}
	if !strings.Contains(hEvents, "l|t: Sort") || !strings.Contains(hEvents, "m: Mode") {
		t.Errorf("Events view should contain 'l|t: Sort' and 'm: Mode', got: %s", hEvents)
	}

	// Rule Count mode
	resM, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'m'}})
	m = resM.(sigmaModel)
	hRule := m.headerView()
	if strings.Contains(hRule, "[Rule Count]") {
		t.Errorf("header should not contain [Rule Count], got: %s", hRule)
	}
	if strings.Contains(hRule, "enter: Show") {
		t.Errorf("Rule Count view should NOT contain 'enter: Show', got: %s", hRule)
	}
	if !strings.Contains(hRule, "l|r|c: Sort") || !strings.Contains(hRule, "m: Mode") {
		t.Errorf("Rule Count view should contain 'l|r|c: Sort' and 'm: Mode', got: %s", hRule)
	}

	// Tag Count mode
	resM, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'m'}})
	m = resM.(sigmaModel)
	hTag := m.headerView()
	if strings.Contains(hTag, "[Tag Count]") {
		t.Errorf("header should not contain [Tag Count], got: %s", hTag)
	}
	if strings.Contains(hTag, "enter: Show") {
		t.Errorf("Tag Count view should NOT contain 'enter: Show', got: %s", hTag)
	}
	if !strings.Contains(hTag, "t|c: Sort") || !strings.Contains(hTag, "m: Mode") {
		t.Errorf("Tag Count view should contain 't|c: Sort' and 'm: Mode', got: %s", hTag)
	}
}

func TestSigmaModel_Sorting(t *testing.T) {
	// Setup dummy data for events
	rule1 := sigma.Rule{Title: "B Rule", Level: "low"}
	rule2 := sigma.Rule{Title: "A Rule", Level: "critical"}
	rule3 := sigma.Rule{Title: "C Rule", Level: "medium"}
	ev1 := evaluator.ForRule(rule1)
	ev2 := evaluator.ForRule(rule2)
	ev3 := evaluator.ForRule(rule3)

	times = []int64{100, 300, 200}
	results = []string{"log1", "log2", "log3"}
	sigmaList = []sigmaEnt{
		{Log: 0, Evaluator: ev1},
		{Log: 1, Evaluator: ev2},
		{Log: 2, Evaluator: ev3},
	}
	updateSigmaRows()

	m := initSigmaModel()
	m.done = true

	// 1. Event view sort by level (l) -> critical, medium, low
	resM, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'l'}})
	m = resM.(sigmaModel)
	if sigmaList[0].Evaluator.Level != "critical" || sigmaList[1].Evaluator.Level != "medium" || sigmaList[2].Evaluator.Level != "low" {
		t.Errorf("expected level sort order critical, medium, low; got %v, %v, %v",
			sigmaList[0].Evaluator.Level, sigmaList[1].Evaluator.Level, sigmaList[2].Evaluator.Level)
	}

	// Press 'l' again -> reverse order
	resM, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'l'}})
	m = resM.(sigmaModel)
	if sigmaList[0].Evaluator.Level != "low" || sigmaList[2].Evaluator.Level != "critical" {
		t.Errorf("expected reversed level sort order low, medium, critical; got %v, %v, %v",
			sigmaList[0].Evaluator.Level, sigmaList[1].Evaluator.Level, sigmaList[2].Evaluator.Level)
	}

	// Sort by time (t) -> 100, 200, 300
	resM, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'t'}})
	m = resM.(sigmaModel)
	if times[sigmaList[0].Log] != 100 || times[sigmaList[1].Log] != 200 || times[sigmaList[2].Log] != 300 {
		t.Errorf("expected time sort order 100, 200, 300; got %d, %d, %d",
			times[sigmaList[0].Log], times[sigmaList[1].Log], times[sigmaList[2].Log])
	}

	// Press 't' again -> reverse time
	resM, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'t'}})
	m = resM.(sigmaModel)
	if times[sigmaList[0].Log] != 300 || times[sigmaList[2].Log] != 100 {
		t.Errorf("expected reversed time sort order 300, ..., 100; got %d, %d, %d",
			times[sigmaList[0].Log], times[sigmaList[1].Log], times[sigmaList[2].Log])
	}

	// Switch to RuleCount mode
	resM, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'m'}})
	m = resM.(sigmaModel)
	sigmaCountList = []*sigmaCountEnt{
		{ID: "1", Title: "Z Rule", Level: "low", Count: 5},
		{ID: "2", Title: "A Rule", Level: "high", Count: 20},
		{ID: "3", Title: "M Rule", Level: "critical", Count: 10},
	}
	updateSigmaCountRows()

	// Rule count: sort by count (c)
	resM, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'c'}})
	m = resM.(sigmaModel)
	if sigmaCountList[0].Count != 20 || sigmaCountList[1].Count != 10 || sigmaCountList[2].Count != 5 {
		t.Errorf("expected rule count sort by count 20, 10, 5; got %d, %d, %d",
			sigmaCountList[0].Count, sigmaCountList[1].Count, sigmaCountList[2].Count)
	}

	// Rule count: sort by rule title (r)
	resM, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
	m = resM.(sigmaModel)
	if sigmaCountList[0].Title != "A Rule" || sigmaCountList[1].Title != "M Rule" || sigmaCountList[2].Title != "Z Rule" {
		t.Errorf("expected rule count sort by title A, M, Z; got %s, %s, %s",
			sigmaCountList[0].Title, sigmaCountList[1].Title, sigmaCountList[2].Title)
	}

	// Rule count: sort by level (l)
	resM, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'l'}})
	m = resM.(sigmaModel)
	if sigmaCountList[0].Level != "critical" || sigmaCountList[1].Level != "high" || sigmaCountList[2].Level != "low" {
		t.Errorf("expected rule count sort by level critical, high, low; got %s, %s, %s",
			sigmaCountList[0].Level, sigmaCountList[1].Level, sigmaCountList[2].Level)
	}

	// Switch to TagCount mode
	resM, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'m'}})
	m = resM.(sigmaModel)
	sigmaTagCountList = []*sigmaTagCountEnt{
		{Tag: "zeta", Count: 15},
		{Tag: "alpha", Count: 30},
		{Tag: "beta", Count: 5},
	}
	updateSigmaTagCountRows()

	// Tag count: sort by tag (t)
	resM, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'t'}})
	m = resM.(sigmaModel)
	if sigmaTagCountList[0].Tag != "alpha" || sigmaTagCountList[1].Tag != "beta" || sigmaTagCountList[2].Tag != "zeta" {
		t.Errorf("expected tag count sort by tag alpha, beta, zeta; got %s, %s, %s",
			sigmaTagCountList[0].Tag, sigmaTagCountList[1].Tag, sigmaTagCountList[2].Tag)
	}

	// Tag count: sort by count (c)
	resM, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'c'}})
	m = resM.(sigmaModel)
	if sigmaTagCountList[0].Count != 30 || sigmaTagCountList[1].Count != 15 || sigmaTagCountList[2].Count != 5 {
		t.Errorf("expected tag count sort by count 30, 15, 5; got %d, %d, %d",
			sigmaTagCountList[0].Count, sigmaTagCountList[1].Count, sigmaTagCountList[2].Count)
	}

	// Press 'c' again -> reverse count
	resM, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'c'}})
	m = resM.(sigmaModel)
	if sigmaTagCountList[0].Count != 5 || sigmaTagCountList[2].Count != 30 {
		t.Errorf("expected reversed tag count 5, ..., 30; got %d, ..., %d",
			sigmaTagCountList[0].Count, sigmaTagCountList[2].Count)
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
