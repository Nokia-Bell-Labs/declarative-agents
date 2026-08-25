// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package client

import (
	"net/url"
	"testing"

	"github.com/stretchr/testify/require"
)

func renderQueryValue(t *testing.T, declared string, params map[string]interface{}) string {
	t.Helper()
	def := ClientOperationDefinition{
		Client: Client{BaseURL: "http://127.0.0.1:8001"},
		Operation: Operation{
			Method: "GET",
			Path:   "/api/v1/namespaces/default/pods",
			Params: RequestBinding{Query: map[string]interface{}{"labelSelector": declared}},
		},
	}
	rendered, err := renderURL(def, params, nil)
	require.NoError(t, err)
	parsed, err := url.Parse(rendered)
	require.NoError(t, err)
	return parsed.Query().Get("labelSelector")
}

// A declared query value is used when the operation takes no runtime params, so
// a body_source none operation keeps its configured value instead of "<nil>".
func TestRenderURL_DeclaredQueryValueUsedWhenParamsEmpty(t *testing.T) {
	t.Parallel()
	require.Equal(t, "app.kubernetes.io/instance=demo",
		renderQueryValue(t, "app.kubernetes.io/instance=demo", map[string]interface{}{}))
}

// A runtime param still overrides the declared query value.
func TestRenderURL_RuntimeQueryParamOverridesDeclared(t *testing.T) {
	t.Parallel()
	require.Equal(t, "app=chat",
		renderQueryValue(t, "app.kubernetes.io/instance=demo", map[string]interface{}{"labelSelector": "app=chat"}))
}

// An absent declared value with no runtime param renders empty, not "<nil>".
func TestRenderURL_EmptyDeclaredQueryRendersEmpty(t *testing.T) {
	t.Parallel()
	require.Equal(t, "", renderQueryValue(t, "", map[string]interface{}{}))
}
