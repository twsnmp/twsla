package sigma

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/bradleyjkemp/sigma-go"
	"github.com/bradleyjkemp/sigma-go/evaluator"
	"github.com/elastic/go-grok"
	"gopkg.in/yaml.v3"
)

// SigmaRuleEntry represents a loaded Sigma rule and its execution context
type SigmaRuleEntry struct {
	Evaluator   *evaluator.RuleEvaluator
	Source      string // "pack:<pack-name>" or "file:<path>"
	Path        string
	Correlation *CorrelationConfig
}

// Config specifies engine loading and parsing options
type Config struct {
	RulesPath     string
	Packs         []string // e.g. ["windows-essential", "linux-auth"] or ["all"]
	ConfigDir     string
	NamedCaptures string
	GrokPat       []string
	GrokDef       string
	KeyValParse   bool
	Strict        bool
	Debug         bool
}

var (
	regJSON    = regexp.MustCompile(`^\s*\{.*\}\s*$`)
	regSplunk  = regexp.MustCompile(`\s*([a-zA-Z_]+[a-zA-Z0-9_]*)=([^ ,;\r\n]+)`)
	regexpGrok = regexp.MustCompile(`%\{.+\}`)

	wazuhLinuxDecoders = []string{
		`Accepted \S+ for (?P<user>\S+) from (?P<client>\S+) port (?P<srcport>\d+)`,
		`Failed \S+ for (?:invalid user )?(?P<user>\S+) from (?P<client>\S+) port (?P<srcport>\d+)`,
		`(?P<user>\S+) : (?:TTY=\S+ ; )?(?:PWD=\S+ ; )?USER=(?P<dstuser>\S+) ; COMMAND=(?P<command>.+)`,
		`pam_\S+\(sshd:auth\): authentication failure; .*user=(?P<user>\S+)`,
	}
)

// Engine holds compiled rules, evaluators, configs, and correlation tracking
type Engine struct {
	sync.RWMutex
	Config              Config
	RuleEntries         []*SigmaRuleEntry
	Evaluators          []*evaluator.RuleEvaluator
	SigmaConfigMap      map[string]*sigma.Config
	NamedCaptureRegList []*regexp.Regexp
	GrokList            []*grok.Grok
	CorrelationTracker  *CorrelationTracker
}

// NewEngine creates and initializes a new Sigma engine with given config
func NewEngine(cfg Config) (*Engine, error) {
	// Enable key-val parse by default
	cfg.KeyValParse = true

	// Expand "all" or empty packs to all available packs if no external rules given
	if len(cfg.Packs) == 1 && strings.ToLower(cfg.Packs[0]) == "all" {
		cfg.Packs = GetAvailableSigmaPacks()
	} else if len(cfg.Packs) == 0 && cfg.RulesPath == "" {
		cfg.Packs = GetAvailableSigmaPacks()
	}

	eng := &Engine{
		Config:              cfg,
		SigmaConfigMap:      make(map[string]*sigma.Config),
		NamedCaptureRegList: []*regexp.Regexp{},
		GrokList:            []*grok.Grok{},
		CorrelationTracker:  NewCorrelationTracker(),
	}

	eng.loadConfigs()
	eng.loadNamedCaptures()
	eng.loadGrok()
	if err := eng.loadRules(); err != nil {
		return nil, err
	}

	return eng, nil
}

func (e *Engine) loadConfigs() {
	e.SigmaConfigMap = make(map[string]*sigma.Config)
	ForEachSigmaConfig(e.Config.ConfigDir, func(k string, d []byte) {
		c, err := sigma.ParseConfig(d)
		if err != nil {
			if e.Config.Strict {
				log.Fatalf("sigma config parse err=%v", err)
			}
			return
		}
		e.SigmaConfigMap[k] = &c
	})
}

func (e *Engine) getSigmaConfig(r *sigma.Rule) *sigma.Config {
	c := r.Logsource.Product
	if conf, ok := e.SigmaConfigMap[c]; ok {
		return conf
	}
	c += "_" + r.Logsource.Category
	if conf, ok := e.SigmaConfigMap[c]; ok {
		return conf
	}
	c += "_" + r.Logsource.Service
	if conf, ok := e.SigmaConfigMap[c]; ok {
		return conf
	}
	return nil
}

func getSourcePriority(src string) int {
	if strings.HasPrefix(src, "file:") {
		return 2
	}
	if strings.HasPrefix(src, "pack:") {
		return 1
	}
	return 0
}

func (e *Engine) loadRules() error {
	ruleMap := make(map[string]*SigmaRuleEntry)
	var orderedIDs []string

	ForEachSigmaRulesWithSource(e.Config.RulesPath, e.Config.Packs, func(c []byte, path, source string) {
		rule, err := sigma.ParseRule(c)
		if err != nil && strings.Contains(err.Error(), "'*'") {
			rule, err = autoFixSigmaRule(c, rule)
		}
		// check unsupported keywords
		for _, s := range rule.Detection.Searches {
			if len(s.Keywords) > 0 {
				err = fmt.Errorf("keywords not support")
				break
			}
		}
		if err != nil {
			if e.Config.Strict {
				log.Fatalf("invalid rule %s %v", path, err)
			}
			return
		}
		if rule.ID == "" {
			rule.ID = path
		}

		corr := ParseCorrelationConfig(rule.AdditionalFields["correlation"])

		if existing, exists := ruleMap[rule.ID]; exists {
			newPrio := getSourcePriority(source)
			existPrio := getSourcePriority(existing.Source)
			if newPrio > existPrio {
				conf := e.getSigmaConfig(&rule)
				var ev *evaluator.RuleEvaluator
				if conf != nil {
					ev = evaluator.ForRule(rule, evaluator.WithConfig(*conf), evaluator.CaseSensitive)
				} else {
					ev = evaluator.ForRule(rule, evaluator.CaseSensitive)
				}
				ruleMap[rule.ID] = &SigmaRuleEntry{
					Evaluator:   ev,
					Source:      source,
					Path:        path,
					Correlation: corr,
				}
			}
			return
		}

		conf := e.getSigmaConfig(&rule)
		var ev *evaluator.RuleEvaluator
		if conf != nil {
			ev = evaluator.ForRule(rule, evaluator.WithConfig(*conf), evaluator.CaseSensitive)
		} else {
			ev = evaluator.ForRule(rule, evaluator.CaseSensitive)
		}
		ruleMap[rule.ID] = &SigmaRuleEntry{
			Evaluator:   ev,
			Source:      source,
			Path:        path,
			Correlation: corr,
		}
		orderedIDs = append(orderedIDs, rule.ID)
	})

	e.RuleEntries = make([]*SigmaRuleEntry, 0, len(orderedIDs))
	e.Evaluators = make([]*evaluator.RuleEvaluator, 0, len(orderedIDs))
	for _, id := range orderedIDs {
		entry := ruleMap[id]
		e.RuleEntries = append(e.RuleEntries, entry)
		e.Evaluators = append(e.Evaluators, entry.Evaluator)
	}

	if len(e.Evaluators) == 0 && e.Config.Strict {
		return fmt.Errorf("no sigma rules loaded")
	}
	return nil
}

func (e *Engine) loadNamedCaptures() {
	e.NamedCaptureRegList = []*regexp.Regexp{}

	// Builtin decoders for wazuh-linux
	hasWazuhLinux := false
	for _, p := range e.Config.Packs {
		if p == "wazuh-linux" {
			hasWazuhLinux = true
			break
		}
	}
	if hasWazuhLinux {
		for _, pat := range wazuhLinuxDecoders {
			if r, err := regexp.Compile(pat); err == nil {
				e.NamedCaptureRegList = append(e.NamedCaptureRegList, r)
			}
		}
	}

	if e.Config.NamedCaptures == "" {
		return
	}
	c, err := os.ReadFile(e.Config.NamedCaptures)
	if err != nil {
		if e.Config.Strict {
			log.Fatalf("failed to read named captures %s: %v", e.Config.NamedCaptures, err)
		}
		return
	}
	for _, l := range strings.Split(string(c), "\n") {
		l = strings.TrimSpace(l)
		if l != "" && !strings.HasPrefix(l, "#") {
			if r, err := regexp.Compile(l); err == nil {
				e.NamedCaptureRegList = append(e.NamedCaptureRegList, r)
			}
		}
	}
}

func (e *Engine) loadGrok() {
	if len(e.Config.GrokPat) == 0 {
		return
	}
	grokDefMap := make(map[string]string)
	if e.Config.GrokDef != "" {
		if c, err := os.ReadFile(e.Config.GrokDef); err == nil {
			for _, l := range strings.Split(string(c), "\n") {
				parts := strings.SplitN(l, " ", 2)
				if len(parts) == 2 {
					grokDefMap[parts[0]] = parts[1]
				}
			}
		}
	}
	for _, pat := range e.Config.GrokPat {
		gr, err := grok.NewComplete()
		if err != nil {
			continue
		}
		if len(grokDefMap) > 0 {
			gr.AddPatterns(grokDefMap)
		}
		if !regexpGrok.MatchString(pat) {
			pat = fmt.Sprintf("%%{%s}", pat)
		}
		if err := gr.Compile(pat, false); err == nil {
			e.GrokList = append(e.GrokList, gr)
		}
	}
}

// ParseLogData parses a log string into structured fields for Sigma evaluation
func (e *Engine) ParseLogData(logStr string) map[string]interface{} {
	var data map[string]interface{}

	// 1. Try direct JSON unmarshal
	if err := json.Unmarshal([]byte(logStr), &data); err != nil {
		// Non-JSON plain text log: wrap into content/message
		data = map[string]interface{}{
			"content": logStr,
			"message": logStr,
		}
	}

	if data == nil {
		data = make(map[string]interface{})
	}

	// 2. If content or message field exists, inspect and expand it
	rawText := ""
	if v, ok := data["content"]; ok {
		if s, ok := v.(string); ok {
			rawText = s
		}
	} else if v, ok := data["message"]; ok {
		if s, ok := v.(string); ok {
			rawText = s
		}
	}

	if rawText != "" {
		notJSON := true
		if regJSON.MatchString(rawText) {
			var tmpData map[string]interface{}
			if err := json.Unmarshal([]byte(rawText), &tmpData); err == nil {
				notJSON = false
				for k, v := range tmpData {
					data[k] = v
				}
			}
		}
		if notJSON {
			if e.Config.KeyValParse {
				for _, m := range regSplunk.FindAllStringSubmatch(rawText, -1) {
					if len(m) > 2 {
						if f, err := strconv.ParseFloat(m[2], 64); err == nil {
							data[m[1]] = f
						} else {
							data[m[1]] = m[2]
						}
					}
				}
			}
			for _, r := range e.NamedCaptureRegList {
				if match := r.FindStringSubmatch(rawText); match != nil {
					for i, k := range r.SubexpNames() {
						if i != 0 && k != "" {
							if f, err := strconv.ParseFloat(match[i], 64); err == nil {
								data[k] = f
							} else {
								data[k] = match[i]
							}
						}
					}
				}
			}
			for _, gr := range e.GrokList {
				if tmpData, err := gr.ParseString(rawText); err == nil {
					for k, v := range tmpData {
						data[k] = v
					}
				}
			}
		}
	}

	return data
}

// extractGroupKey builds group identifier from specified keys or default client
func extractGroupKey(data map[string]interface{}, groupBy []string) string {
	var parts []string
	for _, k := range groupBy {
		if v, ok := data[k]; ok && v != nil {
			parts = append(parts, fmt.Sprintf("%v", v))
		}
	}
	if len(parts) == 0 {
		if client, ok := data["client"]; ok && client != nil {
			return fmt.Sprintf("%v", client)
		}
		if srcip, ok := data["srcip"]; ok && srcip != nil {
			return fmt.Sprintf("%v", srcip)
		}
		return "all"
	}
	return strings.Join(parts, "/")
}

// Match evaluates a log entry against rules with correlation tracking
func (e *Engine) Match(logStr string, timeNano int64) *SigmaRuleEntry {
	data := e.ParseLogData(logStr)
	if data == nil {
		return nil
	}

	ctx := context.Background()
	for _, entry := range e.RuleEntries {
		res, err := entry.Evaluator.Matches(ctx, data)
		if err != nil || !res.Match {
			continue
		}
		if entry.Correlation != nil {
			groupVal := extractGroupKey(data, entry.Correlation.GroupBy)
			if !e.CorrelationTracker.RecordAndCheck(entry.Evaluator.Rule.ID, groupVal, timeNano, entry.Correlation) {
				continue
			}
		}
		return entry
	}
	return nil
}

// MatchAll evaluates a log entry and returns all matching rules that satisfy correlation
func (e *Engine) MatchAll(logStr string, timeNano int64) []*SigmaRuleEntry {
	data := e.ParseLogData(logStr)
	if data == nil {
		return nil
	}

	var matched []*SigmaRuleEntry
	ctx := context.Background()
	for _, entry := range e.RuleEntries {
		res, err := entry.Evaluator.Matches(ctx, data)
		if err != nil || !res.Match {
			continue
		}
		if entry.Correlation != nil {
			groupVal := extractGroupKey(data, entry.Correlation.GroupBy)
			if !e.CorrelationTracker.RecordAndCheck(entry.Evaluator.Rule.ID, groupVal, timeNano, entry.Correlation) {
				continue
			}
		}
		matched = append(matched, entry)
	}
	return matched
}

// ResetCorrelation resets in-memory sliding windows
func (e *Engine) ResetCorrelation() {
	if e.CorrelationTracker != nil {
		e.CorrelationTracker.Reset()
	}
}

func autoFixSigmaRule(c []byte, r sigma.Rule) (sigma.Rule, error) {
	var rule map[string]interface{}
	if err := yaml.Unmarshal(c, &rule); err != nil {
		return r, err
	}
	numReg := regexp.MustCompile(`\d+`)
	replaceMap := make(map[string]string)
	keys := []string{}
	for k, v := range rule {
		if k == "detection" {
			if m, ok := v.(map[string]interface{}); ok {
				for dk, dv := range m {
					if dk == "condition" {
						if cs, ok := dv.(string); ok {
							a := []string{}
							for _, f := range strings.Fields(cs) {
								if f != "1" && numReg.MatchString(f) {
									f = convertNumberToAlpha(f)
								}
								a = append(a, f)
							}
							replaceMap[cs] = strings.Join(a, " ")
							keys = append(keys, cs)
						}
					} else if numReg.MatchString(dk) {
						replaceMap[dk] = convertNumberToAlpha(dk)
						keys = append(keys, dk)
					}
				}
			}
		}
	}
	sort.Slice(keys, func(i, j int) bool {
		return len(keys[i]) > len(keys[j])
	})
	s := string(c)
	for _, k := range keys {
		s = strings.ReplaceAll(s, k, replaceMap[k])
	}
	return sigma.ParseRule([]byte(s))
}

func convertNumberToAlpha(input string) string {
	numToAlpha := map[rune]rune{
		'0': 'a',
		'1': 'b',
		'2': 'c',
		'3': 'd',
		'4': 'e',
		'5': 'f',
		'6': 'g',
		'7': 'h',
		'8': 'i',
		'9': 'j',
	}
	var builder strings.Builder
	for _, ch := range input {
		if replacement, exists := numToAlpha[ch]; exists {
			builder.WriteRune(replacement)
		} else {
			builder.WriteRune(ch)
		}
	}
	return builder.String()
}
