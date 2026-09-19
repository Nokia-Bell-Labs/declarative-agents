// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package load

import (
	"fmt"
	"path/filepath"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/support/corepath"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/catalog"
)

// A closure load and a request-scoped declaration load run under one lock, so
// neither resolves a rooted path against the other's roots (srd056 R2.3;
// GH-2251). The registries are process-scoped, so this test does not run in
// parallel with others.
func TestConcurrentLoadsKeepTheirOwnLibraryRoots(t *testing.T) {
	t.Cleanup(func() { corepath.SetLibraryRoots(nil); corepath.SetLibraryOverrides(nil) })
	core := agentCoreRoot(t)
	ollama := filepath.Join(core, "tools", "providers", "ollama")
	fixture := filepath.Join(core, "testdata", "providers", "fixture")
	closureProfile := writeDialectProfile(t, ollama)
	requestProfile, err := catalog.LoadProfile(writeDialectProfile(t, fixture))
	require.NoError(t, err)

	var wg sync.WaitGroup
	errs := make(chan error, 200)
	for i := 0; i < 100; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			closure, err := LoadClosure(closureProfile, Options{})
			if err == nil && closure.Selected[0].Config["dialect"] != filepath.Join(ollama, "chat-dialect.yaml") {
				err = errUnexpectedDialect(closure.Selected[0].Config["dialect"])
			}
			errs <- err
		}()
		go func() {
			defer wg.Done()
			errs <- corepath.WithLibraryRoots(requestProfile.Libraries, func() error {
				defs, err := catalog.LoadToolDeclarations(requestProfile.ToolDeclarations)
				if err == nil && defs[0].Config["dialect"] != filepath.Join(fixture, "chat-dialect.yaml") {
					err = errUnexpectedDialect(defs[0].Config["dialect"])
				}
				return err
			})
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		require.NoError(t, err)
	}
	require.Empty(t, corepath.LibraryRoots(), "no load leaves its roots behind")
}

func errUnexpectedDialect(got interface{}) error {
	return fmt.Errorf("dialect %v resolved against another load's root", got)
}
