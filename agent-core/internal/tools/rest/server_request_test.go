// Copyright (c) 2026 Nokia. All rights reserved.

package rest

import (
	"encoding/json"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
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
	defer func() { _ = resp.Body.Close() }()
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
	defer func() { _ = resp.Body.Close() }()
	require.Equal(t, http.StatusOK, resp.StatusCode)
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Contains(t, string(body), `"run"`)
}

func TestReadRequestBodyRequiresExactlyOneJSONObject(t *testing.T) {
	t.Parallel()
	schema := map[string]interface{}{
		"properties": map[string]interface{}{"value": map[string]interface{}{"type": "integer"}},
		"required":   []interface{}{"value"},
	}
	tests := []struct {
		name    string
		body    string
		wantErr string
	}{
		{name: "one object", body: `{"value":1}`},
		{name: "trailing object", body: `{"value":1} {"value":2}`, wantErr: "exactly one JSON object"},
		{name: "trailing scalar", body: `{"value":1} true`, wantErr: "exactly one JSON object"},
		{name: "malformed trailing data", body: `{"value":1} {`, wantErr: "invalid trailing"},
		{name: "top-level array", body: `[{"value":1}]`, wantErr: "cannot unmarshal array"},
		{name: "top-level primitive", body: `42`, wantErr: "cannot unmarshal number"},
		{name: "oversized number", body: `{"value":1e10000}`, wantErr: "cannot unmarshal number"},
		{name: "empty", body: ``, wantErr: "EOF"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(tt.body))
			_, err := readRequestBody(map[string]interface{}{}, req, schema)
			if tt.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.ErrorContains(t, err, tt.wantErr)
		})
	}
}

func TestValidateBodySchemaJSONTypeMatrix(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		want    string
		value   interface{}
		wantErr bool
	}{
		{name: "string", want: "string", value: "text"},
		{name: "string rejects null", want: "string", value: nil, wantErr: true},
		{name: "number", want: "number", value: 1.5},
		{name: "integer", want: "integer", value: float64(2)},
		{name: "integer rejects fraction", want: "integer", value: 2.5, wantErr: true},
		{name: "boolean", want: "boolean", value: true},
		{name: "array", want: "array", value: []interface{}{1.0}},
		{name: "object", want: "object", value: map[string]interface{}{"nested": true}},
		{name: "null", want: "null", value: nil},
		{name: "wrong primitive", want: "boolean", value: "true", wantErr: true},
		{name: "unsupported schema type", want: "date", value: "2026-07-21", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			schema := map[string]interface{}{
				"properties": map[string]interface{}{"value": map[string]interface{}{"type": tt.want}},
			}
			err := validateBodySchema(schema, map[string]interface{}{"value": tt.value})
			if tt.wantErr {
				require.ErrorContains(t, err, `body field "value" must be `+tt.want)
				return
			}
			require.NoError(t, err)
		})
	}

	schema := map[string]interface{}{
		"properties": map[string]interface{}{"optional": map[string]interface{}{"type": "string"}},
	}
	require.NoError(t, validateBodySchema(schema, map[string]interface{}{}), "an absent optional field remains valid")
}

func FuzzReadRequestBodyIntegerContract(f *testing.F) {
	for _, seed := range []string{
		`{"value":1}`,
		`{"value":1.5}`,
		`{"value":null}`,
		`{"value":1} {"value":2}`,
		`[]`,
		``,
	} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, raw string) {
		schema := map[string]interface{}{
			"properties": map[string]interface{}{"value": map[string]interface{}{"type": "integer"}},
			"required":   []interface{}{"value"},
		}
		req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(raw))
		if _, err := readRequestBody(map[string]interface{}{}, req, schema); err != nil {
			return
		}

		decoder := json.NewDecoder(strings.NewReader(raw))
		var body map[string]interface{}
		require.NoError(t, decoder.Decode(&body))
		var trailing interface{}
		require.ErrorIs(t, decoder.Decode(&trailing), io.EOF)
		number, ok := body["value"].(float64)
		require.True(t, ok)
		require.Equal(t, number, math.Trunc(number))
	})
}
