// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"bufio"
	"encoding/json"
	"os"
)

type meshStatsOutput struct {
	Agents agentsSection `json:"agents"`
}

// Stats outputs per-agent state-machine and YAML metrics for the mesh agents
// as JSON to stdout. Unlike the platform sub-modules, the example reports no
// module-wide LOC breakdown: its Go and Helm code are deployment scaffolding,
// and the agents are what the root stats aggregation counts (GH-754).
func Stats() error {
	var rec meshStatsOutput
	var err error
	rec.Agents, err = scanAgents("agents", meshCountLines)
	if err != nil {
		return err
	}

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(rec)
}

func meshCountLines(path string) (int, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer f.Close()
	n := 0
	s := bufio.NewScanner(f)
	for s.Scan() {
		n++
	}
	return n, s.Err()
}
