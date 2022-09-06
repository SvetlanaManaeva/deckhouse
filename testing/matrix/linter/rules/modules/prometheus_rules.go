/*
Copyright 2021 Flant JSC

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

package modules

import (
	e "errors"
	"fmt"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/model/rulefmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/deckhouse/deckhouse/testing/matrix/linter/rules/errors"
)

const (
	successExitCode = 0
	failureExitCode = 1
	// Exit code 3 is used for "one or more lint issues detected".
	lintErrExitCode = 3

	lintOptionAll            = "all"
	lintOptionDuplicateRules = "duplicate-rules"
	lintOptionNone           = "none"
)

var lintOptions = []string{lintOptionAll, lintOptionDuplicateRules, lintOptionNone}

// nolint:revive
var lintError = fmt.Errorf("lint error")

type lintConfig struct {
	all            bool
	duplicateRules bool
	fatal          bool
}

func (ls lintConfig) lintDuplicateRules() bool {
	return ls.all || ls.duplicateRules
}

// CheckRules validates rule files.
func CheckRules(ls lintConfig, files ...string) int {
	failed := false
	hasErrors := false

	for _, f := range files {
		if n, errs := checkRules(f, ls); errs != nil {
			fmt.Fprintln(os.Stderr, "  FAILED:")
			for _, e := range errs {
				fmt.Fprintln(os.Stderr, e.Error())
			}
			failed = true
			for _, err := range errs {
				hasErrors = hasErrors || !e.Is(err, lintError)
			}
		} else {
			fmt.Printf("  SUCCESS: %d rules found\n", n)
		}
		fmt.Println()
	}
	if failed && hasErrors {
		return failureExitCode
	}
	if failed && ls.fatal {
		return lintErrExitCode
	}
	return successExitCode
}

func checkRules(filename string, lintSettings lintConfig) (int, []error) {
	fmt.Println("Checking", filename)

	rgs, errs := rulefmt.ParseFile(filename)
	if errs != nil {
		return successExitCode, errs
	}

	numRules := 0
	for _, rg := range rgs.Groups {
		numRules += len(rg.Rules)
	}

	if lintSettings.lintDuplicateRules() {
		dRules := checkDuplicates(rgs.Groups)
		if len(dRules) != 0 {
			errMessage := fmt.Sprintf("%d duplicate rule(s) found.\n", len(dRules))
			for _, n := range dRules {
				errMessage += fmt.Sprintf("Metric: %s\nLabel(s):\n", n.metric)
				for _, l := range n.label {
					errMessage += fmt.Sprintf("\t%s: %s\n", l.Name, l.Value)
				}
			}
			errMessage += "Might cause inconsistency while recording expressions"
			return 0, []error{fmt.Errorf("%w %s", lintError, errMessage)}
		}
	}

	return numRules, nil
}

type compareRuleType struct {
	metric string
	label  labels.Labels
}

func (c compareRuleTypes) Len() int           { return len(c) }
func (c compareRuleTypes) Swap(i, j int)      { c[i], c[j] = c[j], c[i] }
func (c compareRuleTypes) Less(i, j int) bool { return compare(c[i], c[j]) < 0 }

type compareRuleTypes []compareRuleType

func ruleMetric(rule rulefmt.RuleNode) string {
	if rule.Alert.Value != "" {
		return rule.Alert.Value
	}
	return rule.Record.Value
}

func compare(a, b compareRuleType) int {
	if res := strings.Compare(a.metric, b.metric); res != 0 {
		return res
	}

	return labels.Compare(a.label, b.label)
}

func checkDuplicates(groups []rulefmt.RuleGroup) []compareRuleType {
	var duplicates []compareRuleType
	var rules compareRuleTypes

	for _, group := range groups {
		for _, rule := range group.Rules {
			rules = append(rules, compareRuleType{
				metric: ruleMetric(rule),
				label:  labels.FromMap(rule.Labels),
			})
		}
	}
	if len(rules) < 2 {
		return duplicates
	}
	sort.Sort(rules)

	last := rules[0]
	for i := 1; i < len(rules); i++ {
		if compare(last, rules[i]) == 0 {
			// Don't add a duplicated rule multiple times.
			if len(duplicates) == 0 || compare(last, duplicates[len(duplicates)-1]) != 0 {
				duplicates = append(duplicates, rules[i])
			}
		}
		last = rules[i]
	}

	return duplicates
}

func newLintConfig(stringVal string, fatal bool) lintConfig {
	items := strings.Split(stringVal, ",")
	ls := lintConfig{
		fatal: fatal,
	}
	for _, setting := range items {
		switch setting {
		case lintOptionAll:
			ls.all = true
		case lintOptionDuplicateRules:
			ls.duplicateRules = true
		case lintOptionNone:
		default:
			fmt.Printf("WARNING: unknown lint option %s\n", setting)
		}
	}
	return ls
}

func prometheusModuleRule(name, path string) errors.LintRuleErrorsList {
	var lintRuleErrorsList errors.LintRuleErrorsList
	_ = filepath.Walk(path, func(path string, info os.FileInfo, _ error) error {
		if filepath.Ext(path) != ".yaml" {
			return nil
		}

		// No error checking logic right now, never mind.
		CheckRules(newLintConfig("all", true), path)
		return nil
	})
	return lintRuleErrorsList
}
