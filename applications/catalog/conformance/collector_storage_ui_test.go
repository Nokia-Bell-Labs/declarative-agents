// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package conformance

import (
	"os"
	"strings"
	"testing"
)

// srd020 R10.2/R10.4/AC12, GH-2490 AC8: the shipped trace UI must surface
// completeness from the query engine rather than presenting WAL-only or
// unreachable object history as complete.
func TestCollectorUIStorageStatusContract(t *testing.T) {
	client, err := os.ReadFile("../agents/collector/ui/src/api/client.ts")
	if err != nil {
		t.Fatal(err)
	}
	page, err := os.ReadFile("../agents/collector/ui/src/pages/Traces.tsx")
	if err != nil {
		t.Fatal(err)
	}
	rest, err := os.ReadFile("../agents/collector/rest.yaml")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"getTraceStorageStatus", "/traces?page_size=1", "storage_status",
		"'complete'", "'partial'", "'unavailable'",
	} {
		if !strings.Contains(string(client), want) {
			t.Errorf("collector query client misses %q", want)
		}
	}
	for _, want := range []string{
		"StorageStatusBanner", "Storage: {status}",
		"Results must not be treated as complete history",
		"some retained objects or pending WAL evidence could not be read",
	} {
		if !strings.Contains(string(page), want) {
			t.Errorf("collector trace UI misses %q", want)
		}
	}
	for _, body := range []string{
		"traces: $.traces, total: $.total, offset: $.offset, page_size: $.page_size, storage_status: $.storage_status",
		"trace_id: $.trace_id, spans: $.spans, span_count: $.span_count, storage_status: $.storage_status",
	} {
		if !strings.Contains(string(rest), body) {
			t.Errorf("collector REST response does not carry storage status: %q", body)
		}
	}
}
