// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"bufio"
	"encoding/json"
	"os"
)

type meshStatsOutput struct {
	Agents      agentsSection      `json:"agents"`
	Composition compositionSection `json:"composition"`
}

// Stats outputs local implementation metrics and composition reuse separately.
// Unlike the platform sub-modules, the application reports no module-wide LOC
// breakdown: its Go and Helm code are deployment scaffolding, and only the
// locally owned agents feed root implementation totals (GH-754, GH-1000).
func Stats() error {
	ownership, err := scanAgentOwnership("agents", meshCountLines)
	if err != nil {
		return err
	}
	rec := meshStatsOutput(ownership)

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(rec)
}

func meshCountLines(path string) (int, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer func() { _ = f.Close() }()
	n := 0
	s := bufio.NewScanner(f)
	for s.Scan() {
		n++
	}
	return n, s.Err()
}
