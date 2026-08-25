// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package resttest

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

// PostStatus POSTs JSON and requires the response status.
func PostStatus(t *testing.T, url, body string, want int) {
	t.Helper()
	RequestStatus(t, http.MethodPost, url, body, want)
}

// RequestStatus sends a JSON request and requires the response status.
func RequestStatus(t *testing.T, method, url, body string, want int) {
	t.Helper()
	RequestStatusWithHeaders(t, method, url, body, nil, want)
}

// RequestStatusWithHeaders sends a JSON request with extra headers.
func RequestStatusWithHeaders(t *testing.T, method, url, body string, headers map[string]string, want int) {
	t.Helper()
	req, err := http.NewRequest(method, url, bytes.NewBufferString(body))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	for name, value := range headers {
		req.Header.Set(name, value)
	}
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	_, _ = io.Copy(io.Discard, resp.Body)
	require.Equal(t, want, resp.StatusCode)
}

// RequestBody sends a JSON request and returns the response body.
func RequestBody(t *testing.T, method, url, body string, want int) string {
	t.Helper()
	req, err := http.NewRequest(method, url, bytes.NewBufferString(body))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	data, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Equal(t, want, resp.StatusCode, string(data))
	return string(data)
}

// PostJSON POSTs JSON and decodes the object response.
func PostJSON(t *testing.T, url, body string, want int) map[string]interface{} {
	t.Helper()
	data := RequestBody(t, http.MethodPost, url, body, want)
	var output map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(data), &output))
	return output
}

// GetJSON GETs a URL and requires HTTP 200 with a JSON object body.
func GetJSON(t *testing.T, url string) map[string]interface{} {
	t.Helper()
	resp, err := http.Get(url)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	var output map[string]interface{}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&output))
	require.Equal(t, http.StatusOK, resp.StatusCode)
	return output
}

func writeJSON(w http.ResponseWriter, status int, payload map[string]interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}
