// Copyright (c) 2026 Nokia. All rights reserved.

package conformance

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveCoreRoot(t *testing.T) {
	tempRoot := t.TempDir()
	explicitRoot := filepath.Join(tempRoot, "configured-core")
	fallbackRoot := filepath.Join(tempRoot, "sibling-core")
	writeGoModule(t, explicitRoot)
	writeGoModule(t, fallbackRoot)

	tests := []struct {
		name         string
		explicitRoot string
		fallbackRoot string
		wantRoot     string
		wantFound    bool
		wantErr      string
	}{
		{
			name:         "environment path",
			explicitRoot: explicitRoot,
			fallbackRoot: filepath.Join(tempRoot, "missing-fallback"),
			wantRoot:     explicitRoot,
			wantFound:    true,
		},
		{
			name:         "sibling fallback",
			fallbackRoot: fallbackRoot,
			wantRoot:     fallbackRoot,
			wantFound:    true,
		},
		{
			name:         "invalid environment path",
			explicitRoot: filepath.Join(tempRoot, "missing-configured-core"),
			fallbackRoot: fallbackRoot,
			wantErr:      agentCoreRootEnv,
		},
		{
			name:         "absent prerequisite",
			fallbackRoot: filepath.Join(tempRoot, "missing-fallback"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root, found, err := resolveCoreRoot(tt.explicitRoot, tt.fallbackRoot)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("resolveCoreRoot() error = %v, want error containing %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("resolveCoreRoot() unexpected error: %v", err)
			}
			if found != tt.wantFound {
				t.Errorf("resolveCoreRoot() found = %t, want %t", found, tt.wantFound)
			}
			if root != tt.wantRoot {
				t.Errorf("resolveCoreRoot() root = %q, want %q", root, tt.wantRoot)
			}
		})
	}
}

func writeGoModule(t *testing.T, root string) {
	t.Helper()
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatalf("create checkout fixture: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/agent-core\n"), 0o600); err != nil {
		t.Fatalf("write checkout fixture: %v", err)
	}
}
