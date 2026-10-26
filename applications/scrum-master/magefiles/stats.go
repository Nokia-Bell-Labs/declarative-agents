// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package main

import (
	"encoding/json"
	"fmt"
	"path/filepath"

	"gopkg.in/yaml.v3"
	"os"
)

// tpmStats is what the root stats gate aggregates for this application: its
// machines (the profiles under plugin/agents) and the states and tools each
// declares (GH-190).
type tpmStats struct {
	Application struct {
		Machines int      `json:"machines"`
		States   int      `json:"states"`
		Tools    int      `json:"tools"`
		Names    []string `json:"names"`
	} `json:"application"`
}

// Stats prints the application's machine statistics as JSON for the root
// stats aggregation.
func Stats() error {
	root, err := repositoryRoot()
	if err != nil {
		return err
	}
	profiles, err := filepath.Glob(filepath.Join(root, "plugin", "agents", "*", "profile.yaml"))
	if err != nil {
		return err
	}
	var s tpmStats
	for _, p := range profiles {
		s.Application.Machines++
		s.Application.Names = append(s.Application.Names, filepath.Base(filepath.Dir(p)))
		b, err := os.ReadFile(filepath.Join(filepath.Dir(p), "machine.yaml"))
		if err != nil {
			return err
		}
		var m struct {
			States []struct {
				Name string `yaml:"name"`
			} `yaml:"states"`
		}
		if err := yaml.Unmarshal(b, &m); err != nil {
			return err
		}
		s.Application.States += len(m.States)
		t, err := os.ReadFile(filepath.Join(filepath.Dir(p), "tools.yaml"))
		if err != nil {
			return err
		}
		var tl struct {
			Tools []string `yaml:"tools"`
		}
		if err := yaml.Unmarshal(t, &tl); err != nil {
			return err
		}
		s.Application.Tools += len(tl.Tools)
	}
	b, err := json.Marshal(s)
	if err != nil {
		return err
	}
	fmt.Println(string(b))
	return nil
}
