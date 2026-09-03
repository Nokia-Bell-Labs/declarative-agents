// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestPrepareMonitoredQwenRunWritesRequestFile(t *testing.T) {
	rootDir, err := filepath.Abs("..")
	if err != nil {
		t.Fatal(err)
	}
	run, cleanup, err := prepareMonitoredQwenRun(rootDir, "qwen-test")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(cleanup)

	request, err := os.ReadFile(run.requestPath)
	if err != nil {
		t.Fatal(err)
	}
	if got := string(request); got != monitoredQwenPrompt {
		t.Errorf("request = %q, want %q", got, monitoredQwenPrompt)
	}
	if got, want := filepath.Dir(run.requestPath), filepath.Dir(run.profilePath); got != want {
		t.Errorf("request directory = %q, want profile directory %q", got, want)
	}
}

func TestMonitoredQwenArgsPassRequestFilePath(t *testing.T) {
	want := []string{"--profile", "/tmp/profile.yaml", "--request", "/tmp/request.txt"}
	if got := monitoredQwenArgs("/tmp/profile.yaml", "/tmp/request.txt"); !reflect.DeepEqual(got, want) {
		t.Errorf("args = %#v, want %#v", got, want)
	}
}

func TestWaitMonitorHTTPSurfacesAgentExitBeforeReadinessTimeout(t *testing.T) {
	exitErr := errors.New("read --request file: request.txt: no such file")
	resultCh := make(chan error, 1)
	resultCh <- exitErr
	output := bytes.NewBufferString("Error: " + exitErr.Error())
	started := time.Now()

	err := waitMonitoredQwenHTTP("http://127.0.0.1:1/monitor/state", resultCh, output)

	if !errors.Is(err, exitErr) {
		t.Errorf("wait error = %v, want exit error", err)
	}
	for _, want := range []string{"agent exited before monitor readiness", output.String()} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("wait error %q missing %q", err, want)
		}
	}
	if elapsed := time.Since(started); elapsed >= 250*time.Millisecond {
		t.Errorf("early exit surfaced after %s, want under 250ms", elapsed)
	}
}
