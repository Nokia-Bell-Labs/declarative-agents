// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package kindrig

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func readManifest(t *testing.T, dir string) EvidenceManifest {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, EvidenceManifestFile))
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	var manifest EvidenceManifest
	if err := yaml.Unmarshal(data, &manifest); err != nil {
		t.Fatalf("decode manifest: %v", err)
	}
	return manifest
}

func TestCaptureEvidenceManifestListsEveryFile(t *testing.T) {
	kindRun := func(args ...string) ([]byte, error) {
		return nil, os.MkdirAll(args[2], 0o755)
	}
	commandRun := func(_ string, args ...string) ([]byte, error) {
		if strings.Join(args[:2], " ") == "get pods" {
			return []byte("pod/api-0\n"), nil
		}
		return []byte("diagnostic\n"), nil
	}
	dir := t.TempDir()
	err := FailureEvidence{
		Directory: dir, Namespaces: []string{"da-helm-smoke"}, Run: commandRun,
		Revision: "0123456789ab",
	}.Capture(kindRun, PlatformClusterName)
	if err != nil {
		t.Fatalf("Capture: %v", err)
	}
	manifest := readManifest(t, dir)
	if manifest.Cluster != PlatformClusterName || manifest.Revision != "0123456789ab" ||
		len(manifest.Namespaces) != 1 || manifest.CapturedAt == "" {
		t.Fatalf("manifest header = %+v", manifest)
	}
	if len(manifest.CaptureErrors) != 0 {
		t.Fatalf("capture errors = %v, want none", manifest.CaptureErrors)
	}
	kinds := map[string]EvidenceFile{}
	for _, file := range manifest.Files {
		if _, err := os.Stat(filepath.Join(dir, file.Path)); err != nil {
			t.Errorf("manifest lists missing file %s: %v", file.Path, err)
		}
		kinds[file.Kind] = file
	}
	for _, kind := range []string{
		EvidenceKindLogs, EvidenceClusterEvents, EvidenceDescribe, EvidenceEvents,
		EvidenceRollout, EvidencePods, EvidenceLogs,
	} {
		if _, ok := kinds[kind]; !ok {
			t.Errorf("manifest has no %s file: %+v", kind, manifest.Files)
		}
	}
	if logs := kinds[EvidenceLogs]; logs.Namespace != "da-helm-smoke" || logs.Pod != "api-0" {
		t.Errorf("logs entry = %+v, want namespace and pod", logs)
	}
}

func TestCaptureEvidenceRecordsFailureAndContinues(t *testing.T) {
	kindRun := func(_ ...string) ([]byte, error) {
		return nil, errors.New("kind unavailable")
	}
	var commands []string
	commandRun := func(_ string, args ...string) ([]byte, error) {
		commands = append(commands, strings.Join(args, " "))
		if args[0] == "describe" {
			return []byte("partial"), fmt.Errorf("exit status 1")
		}
		return nil, nil
	}
	dir := t.TempDir()
	err := FailureEvidence{
		Directory: dir, Namespaces: []string{"da-helm-smoke"}, Run: commandRun,
	}.Capture(kindRun, PlatformClusterName)
	if err == nil {
		t.Fatal("Capture returned nil, want the joined capture errors")
	}
	if len(commands) != 5 {
		t.Fatalf("commands = %v, want capture to continue after the describe failure", commands)
	}
	manifest := readManifest(t, dir)
	if len(manifest.CaptureErrors) != 2 {
		t.Fatalf("capture errors = %v, want kind export and describe", manifest.CaptureErrors)
	}
	data, err := os.ReadFile(filepath.Join(dir, "namespace-da-helm-smoke-describe.txt"))
	if err != nil || !strings.Contains(string(data), "[command failed") {
		t.Fatalf("describe file = %q, %v; want the failure recorded in the file", data, err)
	}
	for _, file := range manifest.Files {
		if file.Kind == EvidenceKindLogs {
			t.Fatalf("manifest lists kind logs after a failed export: %+v", manifest.Files)
		}
	}
}

func TestCaptureEvidenceCopiesTraceTail(t *testing.T) {
	spool := filepath.Join(t.TempDir(), "collector.ndjson")
	var content strings.Builder
	line := `{"traceId":"` + strings.Repeat("a", 32) + `"}` + "\n"
	for content.Len() < traceTailBytes+len(line)*4 {
		content.WriteString(line)
	}
	if err := os.WriteFile(spool, []byte(content.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	kindRun := func(_ ...string) ([]byte, error) { return nil, nil }
	if err := (FailureEvidence{Directory: dir, TraceSpool: spool}).Capture(kindRun, "c"); err != nil {
		t.Fatalf("Capture: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(dir, traceTailFile))
	if err != nil {
		t.Fatal(err)
	}
	if len(data) > traceTailBytes || len(data)%len(line) != 0 || !strings.HasPrefix(string(data), `{"traceId"`) {
		t.Fatalf("trace tail has %d bytes, want whole lines within %d", len(data), traceTailBytes)
	}
	manifest := readManifest(t, dir)
	if manifest.Files[len(manifest.Files)-1] != (EvidenceFile{Path: traceTailFile, Kind: EvidenceTraces}) {
		t.Fatalf("manifest files = %+v, want the trace tail", manifest.Files)
	}
}

func TestCaptureEvidenceRecordsMissingTraceSpool(t *testing.T) {
	dir := t.TempDir()
	kindRun := func(_ ...string) ([]byte, error) { return nil, nil }
	err := FailureEvidence{
		Directory: dir, TraceSpool: filepath.Join(dir, "absent.ndjson"),
	}.Capture(kindRun, "c")
	if err == nil {
		t.Fatal("Capture returned nil for a missing spool")
	}
	if manifest := readManifest(t, dir); len(manifest.CaptureErrors) != 1 {
		t.Fatalf("capture errors = %v, want the missing spool", manifest.CaptureErrors)
	}
}
