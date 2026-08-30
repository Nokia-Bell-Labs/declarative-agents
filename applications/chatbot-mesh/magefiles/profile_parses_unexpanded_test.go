// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// TestAgentProfilesParseUnexpanded keeps every agent YAML readable before the
// runtime rewrites its environment references.
//
// An unquoted ${...} inside a flow sequence is not valid YAML: `hosts:
// [${CHROMA_HOST:-127.0.0.1}]` fails to parse, because the brace opens a flow
// mapping the sequence never closes. The runtime never sees it -- expandEnv
// rewrites the bytes first -- but everything that reads a profile as a document
// does: the packaging checks, the declaration checks, and every test here that
// unmarshals one. Four of the mesh profiles failed a plain parse.
//
// Block sequences carry the same references without quoting, which matters for
// ports: a quoted "8000" would land as a string where the policy declares an
// int (GH-219).
func TestAgentProfilesParseUnexpanded(t *testing.T) {
	agentsRoot := filepath.Dir(agentDir(t, "chatbot"))
	var scanned int
	err := filepath.WalkDir(agentsRoot, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".yaml") {
			return nil
		}
		// Test fixtures under an agent's tests/ directory stand in for other
		// services and are not loaded as mesh profiles.
		if strings.Contains(path, string(filepath.Separator)+"tests"+string(filepath.Separator)) {
			return nil
		}
		body, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		scanned++
		var document any
		if parseErr := yaml.Unmarshal(body, &document); parseErr != nil {
			rel, _ := filepath.Rel(agentsRoot, path)
			t.Errorf("agents/%s does not parse as YAML before expansion: %v."+
				" An environment reference inside a flow sequence needs a block"+
				" sequence instead", rel, parseErr)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", agentsRoot, err)
	}
	if scanned == 0 {
		t.Fatal("no agent YAML scanned; the check would pass vacuously")
	}
}
