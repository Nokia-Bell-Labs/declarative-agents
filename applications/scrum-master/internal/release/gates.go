// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
// Package release cuts a tag only after a repository's declared gates pass:
// the gates file, the stage runner, and the v0.YYYYMMDD.N tag sequence
// (eng03). scrum-master's mage tag drives it; scrum-master release reuses the tagging.
package release

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/corpus"
)

// DefaultGatesFile is the gates file at the repository root, a config
// document (design D11): push is a field, and stages holds one stage record
// per stage whose gates field is a table of name, command, and directory.
const DefaultGatesFile = "release-gates.md"

// Whether a created tag is pushed without asking.
const (
	PushAsk  = "ask"
	PushAuto = "auto"
)

// Gate is one command a release must pass, run from Directory (relative to
// the repository root; empty is the root).
type Gate struct {
	Name      string `yaml:"name"`
	Command   string `yaml:"command"`
	Directory string `yaml:"directory"`
}

// Stage is gates that run at the same time; the next stage starts only when
// every gate in this one passed.
type Stage struct {
	Name  string `yaml:"name"`
	Gates []Gate `yaml:"gates"`
}

// Gates is the parsed gates file.
type Gates struct {
	Push   string  `yaml:"push"`
	Stages []Stage `yaml:"stages"`
}

// LoadGates reads and validates the gates file at path.
func LoadGates(path string) (Gates, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return Gates{}, err
	}
	var g Gates
	if err := corpus.DecodeConfig(b, &g); err != nil {
		return Gates{}, fmt.Errorf("%s: %w", path, err)
	}
	if err := g.validate(); err != nil {
		return Gates{}, fmt.Errorf("%s: %w", path, err)
	}
	return g, nil
}

func (g *Gates) validate() error {
	switch g.Push {
	case "":
		g.Push = PushAsk
	case PushAsk, PushAuto:
	default:
		return fmt.Errorf("push must be %s or %s, not %q", PushAsk, PushAuto, g.Push)
	}
	if len(g.Stages) == 0 {
		return errors.New("no stages: a release needs at least one gate")
	}
	for i, s := range g.Stages {
		if s.Name == "" {
			return fmt.Errorf("stage %d has no name", i+1)
		}
		if len(s.Gates) == 0 {
			return fmt.Errorf("stage %s has no gates", s.Name)
		}
		for _, gate := range s.Gates {
			if gate.Name == "" || strings.TrimSpace(gate.Command) == "" {
				return fmt.Errorf("stage %s: every gate needs a name and a command", s.Name)
			}
			if filepath.IsAbs(gate.Directory) || strings.HasPrefix(filepath.Clean(gate.Directory), "..") {
				return fmt.Errorf("gate %s: directory %q is not inside the repository", gate.Name, gate.Directory)
			}
		}
	}
	return nil
}
