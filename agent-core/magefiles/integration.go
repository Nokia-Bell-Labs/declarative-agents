// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package main

import (
	"fmt"
	"net/http"
	"strings"
	"sync"

	"github.com/magefile/mage/mg"
)

// Integration contains focused runtime integrations that cross a live service
// boundary. Application workflows belong to the applications under applications/.
type Integration mg.Namespace

// All runs every base-service integration test and prints a summary.
func (i Integration) All() error {
	tests := []struct {
		name string
		fn   func() error
	}{
		{"monitor", i.Monitor},
		{"ollamaRest", i.OllamaRest},
		{"ollamaMonitor", i.OllamaMonitor},
		{"dolt", i.Dolt},
		{"doltWord", i.DoltWord},
	}

	var passed, failed, skipped int
	results := make([]string, 0, len(tests))

	for _, t := range tests {
		fmt.Printf("\n=== %s ===\n", t.name)
		beginUC(t.name)
		err := t.fn()
		switch {
		case err != nil:
			failed++
			results = append(results, fmt.Sprintf("  FAIL  %s  %v", t.name, err))
		case wasSkipped(t.name):
			skipped++
			results = append(results, fmt.Sprintf("  SKIP  %s", t.name))
		default:
			passed++
			results = append(results, fmt.Sprintf("  PASS  %s", t.name))
		}
	}

	fmt.Printf("\n%s\n", strings.Repeat("─", 40))
	for _, r := range results {
		fmt.Println(r)
	}
	fmt.Printf("%s\n", strings.Repeat("─", 40))
	fmt.Printf("Total: %d passed, %d failed, %d skipped\n", passed, failed, skipped)

	if failed > 0 {
		return fmt.Errorf("%d integration test(s) failed", failed)
	}
	return nil
}

var skippedIntegrations = struct {
	sync.Mutex
	items map[string]bool
}{items: make(map[string]bool)}

func skipUC(id, reason string) error {
	skippedIntegrations.Lock()
	skippedIntegrations.items[id] = true
	skippedIntegrations.Unlock()
	fmt.Printf("SKIP %s: %s\n", id, reason)
	return nil
}

func beginUC(id string) {
	skippedIntegrations.Lock()
	delete(skippedIntegrations.items, id)
	skippedIntegrations.Unlock()
}

func wasSkipped(id string) bool {
	skippedIntegrations.Lock()
	defer skippedIntegrations.Unlock()
	return skippedIntegrations.items[id]
}

func requireOllama() error {
	resp, err := integrationHTTPClient.Get("http://localhost:11434/api/version")
	if err != nil {
		return fmt.Errorf("ollama not reachable at localhost:11434: %w", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("ollama returned status %d", resp.StatusCode)
	}
	return nil
}
