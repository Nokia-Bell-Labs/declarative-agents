// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// Stats runs mage stats in each sub-module and outputs combined JSON to stdout.
// Example modules (exampleModules) are excluded on purpose: they expose no mage
// stats target, so there is nothing to combine. They still participate in the
// audit and Go-test gates (see Audit and Test).
func Stats() error {
	results := make(map[string]json.RawMessage)

	for _, mod := range subModules {
		mageDir := filepath.Join(mod, "magefiles")
		if _, err := os.Stat(mageDir); os.IsNotExist(err) {
			continue
		}

		raw, err := runMageStats(mod)
		if err != nil {
			return fmt.Errorf("stats in %s: %w", mod, err)
		}
		results[mod] = raw
	}

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(results)
}

func runMageStats(dir string) (json.RawMessage, error) {
	cmd := exec.Command("mage", "stats")
	cmd.Dir = dir
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return nil, err
	}
	raw := json.RawMessage(bytes.TrimSpace(stdout.Bytes()))
	if !json.Valid(raw) {
		return nil, fmt.Errorf("invalid JSON from %s: %s", dir, stdout.String())
	}
	return raw, nil
}
