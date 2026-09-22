// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package main

import (
	"os/exec"
	"strings"
	"testing"
)

// The durable collector storage contract (srd042 R10/R11; GH-2484) is wired
// once in the shared agent-services library chart. These render the chatbot
// mesh chart through that template and assert the object backend wires the
// declared storage channel, the filesystem default stays byte-identical, and
// every misconfiguration fails at render time with its own named reason.

// renderExpectingFailure templates the chart with overrides that must be
// rejected and returns the combined output for the caller to match a reason in.
func renderExpectingFailure(t *testing.T, sets ...string) string {
	t.Helper()
	if _, err := exec.LookPath("helm"); err != nil {
		t.Skip("helm not on PATH")
	}
	args := []string{"template", "rel", findChartDir(t)}
	for _, set := range sets {
		args = append(args, "--set", set)
	}
	out, err := exec.Command("helm", args...).CombinedOutput()
	if err == nil {
		t.Fatalf("render accepted a misconfiguration it should reject:\n%s", out)
	}
	return string(out)
}

// srd042 R11.1, GH-2484 AC2: the object backend wires the declared storage
// channel and a persistent WAL from the library template.
func TestCollectorObjectBackendWiresStorageChannel(t *testing.T) {
	out := helmTemplateOutput(t,
		"collector.storage.backend=object",
		"collector.storage.bucketName=chatbot-mesh",
	)
	for _, want := range []string{
		`{name: COLLECTOR_STORAGE_BACKEND, value: "object"}`,
		`{name: COLLECTOR_OBJECT_BUCKET_URL, value: "gs://chatbot-mesh"}`,
		`{name: COLLECTOR_APPLICATION, value: "chatbot-mesh"}`,
		"kind: PersistentVolumeClaim",
		"-collector-wal",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("object-backend render missing %q", want)
		}
	}
}

// GH-2484 R7: the filesystem default preserves the pre-contract render — no
// storage env and no WAL claim — so the existing collector is unchanged.
func TestCollectorFilesystemDefaultOmitsStorageWiring(t *testing.T) {
	out := helmTemplateOutput(t)
	for _, absent := range []string{
		"COLLECTOR_STORAGE_BACKEND",
		"-collector-wal",
	} {
		if strings.Contains(out, absent) {
			t.Errorf("filesystem default render unexpectedly contains %q", absent)
		}
	}
}

// GH-2484 AC1: schema and semantic checks reject missing identity, malformed
// bucket URLs, endpoint-without-local-backend, unsafe prefixes, and durable
// mode with ephemeral WAL storage, each with its own reason.
func TestCollectorStorageValidationsRejectMisconfig(t *testing.T) {
	cases := []struct {
		name string
		sets []string
		want string
	}{
		{
			name: "object without bucket",
			sets: []string{"collector.storage.backend=object"},
			want: "object backend requires bucketURL or bucketName",
		},
		{
			name: "endpoint without object backend",
			sets: []string{"collector.storage.endpoint=http://fake-gcs"},
			want: "endpoint is only valid with the object backend",
		},
		{
			name: "malformed bucket scheme",
			sets: []string{"collector.storage.backend=object", "collector.storage.bucketURL=ftp://x"},
			want: "must use scheme gs, s3, file, or mem",
		},
		{
			name: "durable mode with ephemeral wal",
			sets: []string{"collector.storage.backend=object", "collector.storage.bucketName=b", "collector.storage.walMode=ephemeral"},
			want: "durable mode requires a persistent WAL",
		},
		{
			name: "unsafe prefix",
			sets: []string{"collector.storage.backend=object", "collector.storage.bucketName=b", "collector.storage.prefix=../escape"},
			want: "must be a safe relative in-bucket prefix",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out := renderExpectingFailure(t, tc.sets...)
			if !strings.Contains(out, tc.want) {
				t.Errorf("rejection did not name %q:\n%s", tc.want, out)
			}
		})
	}
}

// GH-2484 R3: an explicitly ephemeral test collector mounts an emptyDir WAL and
// renders no persistent claim.
func TestCollectorEphemeralModeUsesEmptyDirWAL(t *testing.T) {
	out := helmTemplateOutput(t,
		"collector.storage.backend=object",
		"collector.storage.bucketName=b",
		"collector.storage.mode=ephemeral",
	)
	if !strings.Contains(out, "name: wal-ephemeral") {
		t.Error("ephemeral test mode did not mount an emptyDir WAL")
	}
	if strings.Contains(out, "kind: PersistentVolumeClaim") {
		t.Error("ephemeral test mode rendered a persistent WAL claim")
	}
}
