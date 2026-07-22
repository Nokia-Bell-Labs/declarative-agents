// Copyright (c) 2026 Nokia. All rights reserved.

package docsapi

import (
	"context"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
	"github.com/stretchr/testify/require"
	"net/http"
	"path/filepath"
	"testing"
)

func TestLazyMachineRequestProxyOwnsBackendLifecycle(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	docsDir := filepath.Join(root, "docs")
	writeDocFixture(t, docsDir, "VISION.yaml", "title: Vision\n")
	proxy := NewLazyMachineRequestProxy(curatorProfilePath(t), docsDir)
	t.Cleanup(func() { _ = proxy.Close() })

	rec := getDocsRoute(t, proxy, "/api/v1/docs")
	require.Equal(t, http.StatusOK, rec.Code)
	addr := proxyBackendAddr(t, proxy)

	require.NoError(t, proxy.Close())
	requireAddressReleased(t, addr)
}

func TestLaunchDocumentationUndoStopsOwnedListener(t *testing.T) {
	t.Parallel()
	host := NewDocumentationHostLifecycle()
	cmd := newLaunchDocumentationBuilder(t, host).Build(core.Result{})
	res := cmd.Execute()
	require.Equal(t, core.Signal("ServerLaunched"), res.Signal)
	addr := requireResultAddr(t, res)
	t.Cleanup(func() { _, _ = host.Stop() })

	undo := cmd.Undo(core.Result{})

	require.Equal(t, core.Signal("ServerStopped"), undo.Signal, undo.Output)
	requireAddressReleased(t, addr)
}

func TestCuratorMachineExitStopsDocumentationHost(t *testing.T) {
	t.Parallel()
	host := NewDocumentationHostLifecycle()
	var launchedAddr string
	result, err := core.Loop(curatorExitLoopParams(t, host, &launchedAddr), context.Background())
	require.NoError(t, err)

	require.Equal(t, core.StatusSucceeded, result.Status)
	require.Equal(t, core.State("Done"), result.FinalState)
	require.NotEmpty(t, launchedAddr)
	requireDocsHostStoppedEvent(t, result)
	requireAddressReleased(t, launchedAddr)
}
