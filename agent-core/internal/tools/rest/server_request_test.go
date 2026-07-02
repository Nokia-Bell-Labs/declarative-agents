// Copyright (c) 2026 Nokia. All rights reserved.

package rest

import (
	"io"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func browserLikeGETHeaders() map[string]string {
	return map[string]string{
		"Sec-Ch-Ua":                   `"Chromium";v="124", "Google Chrome";v="124"`,
		"Sec-Ch-Ua-Mobile":            "?0",
		"Sec-Ch-Ua-Platform":          `"macOS"`,
		"Sec-Fetch-Dest":              "empty",
		"Sec-Fetch-Mode":              "cors",
		"Sec-Fetch-Site":              "same-origin",
		"Sec-Fetch-User":              "?1",
		"Sec-Fetch-Storage-Access":    "active",
		"Referer":                     "http://127.0.0.1:18084/ui/",
		"Cache-Control":               "no-cache",
		"Pragma":                      "no-cache",
		"Accept-Language":             "en-US,en;q=0.9",
		"DNT":                         "1",
		"Upgrade-Insecure-Requests":   "1",
		"Priority":                    "u=0, i",
		"Cookie":                      "session=abc",
		"Sec-Ch-Prefers-Color-Scheme": "dark",
	}
}

func TestBrowserHeadersAllowedOnStaticAssetsEndpoint(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "index.html"), []byte("<html>ok</html>"), 0o644))
	srv := Server{
		Address: "127.0.0.1:0",
		Queue:   QueueConfig{Name: "browser_static", Capacity: 4, Timeout: "20ms"},
		Endpoints: map[string]Endpoint{
			"assets": {
				Method: "GET", Path: "/ui/{path...}",
				Binding: bindingStaticAssets,
				StaticAssets: &StaticAssetsConfig{
					Root: root,
				},
				Request: RequestBinding{Path: map[string]interface{}{
					"path": map[string]interface{}{"type": "string"},
				}},
			},
		},
	}
	state, baseURL := launchRESTServer(t, srv, LimitProfile{})
	defer stopRESTServer(t, state, "browser_static")

	req, err := http.NewRequest(http.MethodGet, baseURL+"/ui/index.html", nil)
	require.NoError(t, err)
	for k, v := range browserLikeGETHeaders() {
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)
	data, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Contains(t, string(data), "<html>ok</html>")
}

func TestBrowserHeadersAllowedOnMonitorReadState(t *testing.T) {
	t.Parallel()
	state, baseURL := launchMonitorRESTServer(t, "browser_monitor", seededMonitorState())
	defer stopRESTServer(t, state, "browser_monitor")

	req, err := http.NewRequest(http.MethodGet, baseURL+"/monitor/state", nil)
	require.NoError(t, err)
	for k, v := range browserLikeGETHeaders() {
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Contains(t, string(body), `"run"`)
}
