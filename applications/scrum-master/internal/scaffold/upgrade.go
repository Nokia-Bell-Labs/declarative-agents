// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
package scaffold

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"

	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/corpus"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/program"
)

// systemContext is the context field scrum-master init writes first in the system
// SRD: the system's owner, so the SRD declares the system repository a
// component of the program (srd043 R1.1).
const systemContext = "<!-- field: context | form: \"tree\" -->\n## Context\n\n- owner: The technical program manager\n"

var documentField = regexp.MustCompile(`(?m)^<!-- field: ([a-z_]+)[^\n]*-->\n## `)

// UpgradeSystemSRD adds to the system SRD text doc the fields scrum-master init now
// writes there and doc lacks: context as its first field, and
// implementation_targets, from the origin of the repository at dir, as its
// last. It returns the text and the fields it added; every other line is
// the owner's and stays.
func UpgradeSystemSRD(doc []byte, dir string) ([]byte, []string) {
	text := string(doc)
	have := map[string]bool{}
	for _, m := range documentField.FindAllStringSubmatch(text, -1) {
		have[m[1]] = true
	}
	var added []string
	if !have["context"] {
		if loc := documentField.FindStringIndex(text); loc != nil {
			text = text[:loc[0]] + systemContext + "\n" + text[loc[0]:]
			added = append(added, "context")
		}
	}
	if !have["implementation_targets"] {
		text = strings.TrimRight(text, "\n") + "\n\n" + implementationTargets(dir)
		added = append(added, "implementation_targets")
	}
	return []byte(text), added
}

var policySection = regexp.MustCompile(`(?m)^<!-- field: ([a-z_]+)[^\n]*-->\n(#+) `)

// AddCorpusPeerRead adds to the control/policy.md text doc the corpus
// release's peer-read gate scrum-master init now writes (srd015 R1.2), when its
// definition of done names no corpus gate. A definition of done that names
// one is the program's and stays. A policy with no definition of done gets
// init's whole one, so the defaults it read before still apply.
func AddCorpusPeerRead(doc []byte) ([]byte, bool, error) {
	var f struct {
		DefinitionOfDone map[string]any `yaml:"definition_of_done"`
	}
	if err := corpus.DecodeConfig(doc, &f); err != nil {
		return doc, false, err
	}
	if _, ok := f.DefinitionOfDone["corpus"]; ok {
		return doc, false, nil
	}
	text := string(doc)
	gate := InitPolicy().DefinitionOfDone
	if f.DefinitionOfDone == nil {
		var b strings.Builder
		b.WriteString("<!-- field: definition_of_done -->\n## Definition of done\n")
		artifacts := make([]string, 0, len(gate))
		for a := range gate {
			artifacts = append(artifacts, a)
		}
		sort.Strings(artifacts)
		for _, a := range artifacts {
			b.WriteString(gateTable(a, gate[a]))
		}
		return []byte(strings.TrimRight(text, "\n") + "\n\n" + b.String()), true, nil
	}
	table := gateTable("corpus", gate["corpus"])
	marks := policySection.FindAllStringSubmatchIndex(text, -1)
	in := false
	for _, m := range marks {
		name, level := text[m[2]:m[3]], len(text[m[4]:m[5]])
		switch {
		case level == 2 && name == "definition_of_done":
			in = true
		case level == 2 && in:
			return []byte(strings.TrimRight(text[:m[0]], "\n") + "\n" + table + "\n" + text[m[0]:]), true, nil
		case level == 3 && in && name > "corpus":
			return []byte(strings.TrimRight(text[:m[0]], "\n") + "\n" + table + "\n" + text[m[0]:]), true, nil
		}
	}
	return []byte(strings.TrimRight(text, "\n") + "\n" + table), true, nil
}

// gateTable renders one artifact's definition of done as PolicyDocument
// writes it.
func gateTable(artifact string, checks []program.Check) string {
	var b strings.Builder
	fmt.Fprintf(&b, "\n<!-- field: %s -->\n### %s\n\n| kind | environment | suite | command |\n| --- | --- | --- | --- |\n", artifact, heading(artifact))
	for _, c := range checks {
		fmt.Fprintf(&b, "| %s | %s | %s | %s |\n", c.Kind, c.Environment, c.Suite, c.Command)
	}
	return b.String()
}

// upgradeSystem applies UpgradeSystemSRD and AddCorpusPeerRead to the kept
// system SRD and policy at dir, reporting each file it changes as extended.
func upgradeSystem(dir string, files []Result, dryRun bool) error {
	edit := func(path string, change func([]byte) ([]byte, string, error)) error {
		i := slices.IndexFunc(files, func(f Result) bool { return f.Path == path })
		if i < 0 || files[i].Action != "kept" {
			return nil
		}
		p := filepath.Join(dir, filepath.FromSlash(path))
		doc, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		out, reason, err := change(doc)
		if err != nil {
			files[i].Reason = "does not parse, so doctor cannot add what scrum-master init now writes there: " + err.Error()
			return nil
		}
		if bytes.Equal(out, doc) {
			return nil
		}
		files[i].Action, files[i].Reason = "extended", reason
		if dryRun {
			return nil
		}
		return os.WriteFile(p, out, 0o644)
	}
	if err := edit(systemSRDPath, func(doc []byte) ([]byte, string, error) {
		out, added := UpgradeSystemSRD(doc, dir)
		return out, "adds " + strings.Join(added, " and ") + ", declaring the system repository a component of the program", nil
	}); err != nil {
		return err
	}
	return edit(program.PolicyFile, func(doc []byte) ([]byte, string, error) {
		out, _, err := AddCorpusPeerRead(doc)
		return out, "adds the corpus release's peer-read gate", err
	})
}
