// Copyright (c) 2026 Nokia. All rights reserved.

package rest

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDocsRuntimeMonitorREST_StateEventsAndUI(t *testing.T) {
	t.Parallel()

	def, err := LoadDefinition(filepath.Join(docsRuntimeFixtureDir(t), "rest.yaml"))
	require.NoError(t, err)

	srv, limits := curatorMonitorServerForTest(t, def)
	uiEp, ok := srv.Endpoints["monitor_ui"]
	require.True(t, ok)
	_, statErr := os.Stat(filepath.Join(uiEp.StaticAssets.Root, "index.html"))
	require.NoError(t, statErr, "monitor UI dist root=%q", uiEp.StaticAssets.Root)

	state := NewServerState()
	_, baseURL := launchRESTServerDefinition(t, state, ServerDefinition{
		Name:    "monitor",
		Server:  srv,
		Limits:  limits,
		Monitor: seededMonitorState(),
	})
	defer stopRESTServer(t, state, "monitor")

	current := getJSON(t, baseURL+"/monitor/state")
	run := current["run"].(map[string]interface{})
	require.Equal(t, "running", run["status"])

	events := getJSON(t, baseURL+"/monitor/events")
	recent := events["recent_events"].([]interface{})
	require.NotEmpty(t, recent, "monitor events feed should list recorded transitions")
	first := recent[0].(map[string]interface{})
	require.Contains(t, first, "from_state")
	require.Contains(t, first, "to_state")

	uiBody := requestBody(t, "GET", baseURL+"/ui/index.html", "", 200)
	require.Contains(t, uiBody, `id="app"`)
	require.Contains(t, uiBody, "Docs Runtime Monitor")
	linkedAssets := regexp.MustCompile(`(src|href)="([^"]+)"`).FindAllStringSubmatch(uiBody, -1)
	require.NotEmpty(t, linkedAssets, "monitor UI index should link its executable/style assets")
	for _, match := range linkedAssets {
		assetPath := strings.TrimPrefix(match[2], "./")
		assetBody := requestBody(t, "GET", baseURL+"/ui/"+assetPath, "", 200)
		require.NotEmpty(t, assetBody, "linked monitor UI asset %q must be served", assetPath)
		require.NotEqual(t, uiBody, assetBody, "linked asset %q must not resolve through the SPA index fallback", assetPath)
	}

	spaFallback := requestBody(t, "GET", baseURL+"/ui/monitor-spa-fallback", "", 200)
	require.Contains(t, spaFallback, `id="app"`)
}

func curatorMonitorServerForTest(t *testing.T, def Definition) (Server, LimitProfile) {
	t.Helper()
	srv, ok := def.Servers["monitor"]
	require.True(t, ok, "docs runtime rest.yaml should define servers.monitor")
	srv.Address = "127.0.0.1:0"
	fixture := docsRuntimeFixtureDir(t)
	const fixturePrefix = "fixtures/docs-runtime/"
	for name, ep := range srv.Endpoints {
		if ep.StaticAssets == nil || ep.StaticAssets.Root == "" {
			continue
		}
		r := ep.StaticAssets.Root
		if filepath.IsAbs(r) {
			continue
		}
		if !strings.HasPrefix(r, fixturePrefix) {
			t.Fatalf("unexpected static_assets root %q", r)
		}
		rel := strings.TrimPrefix(r, fixturePrefix)
		ep.StaticAssets.Root = filepath.Join(fixture, filepath.FromSlash(rel))
		srv.Endpoints[name] = ep
	}
	lim, ok := def.Limits[srv.LimitsRef]
	require.True(t, ok, "missing limits profile %q for monitor server", srv.LimitsRef)
	return srv, lim
}
