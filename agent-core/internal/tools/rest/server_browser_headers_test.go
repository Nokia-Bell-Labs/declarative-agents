// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package rest

import (
	"net/http"
	"strings"
	"testing"
)

// The undeclared-header refusal tolerates the headers a browser attaches on
// its own -- transport metadata, client hints, and privacy signals -- because
// nothing a deployment declares can stop a browser from sending them, and
// refusing them refuses the browser (GH-1935). These tests pin both edges of
// the allowlist: a browser privacy signal passes, an unknown header is still
// refused with the existing message.

func TestBrowserPrivacyHeadersAreTolerated(t *testing.T) {
	t.Parallel()
	for _, header := range []string{"Sec-GPC", "DNT", "Sec-Fetch-Mode"} {
		payload := map[string]interface{}{}
		values := http.Header{header: []string{"1"}}
		if err := addHeaderValues(payload, nil, values); err != nil {
			t.Errorf("a browser-attached %s header was refused: %v", header, err)
		}
	}
}

func TestUnknownHeadersAreStillRefused(t *testing.T) {
	t.Parallel()
	payload := map[string]interface{}{}
	values := http.Header{"X-Undeclared-Anything": []string{"1"}}
	err := addHeaderValues(payload, nil, values)
	if err == nil || !strings.Contains(err.Error(), `header "x-undeclared-anything" is not declared`) {
		t.Errorf("an unknown header must keep the declared refusal, got %v", err)
	}
}
