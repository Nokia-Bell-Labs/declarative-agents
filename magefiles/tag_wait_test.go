// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package main

import (
	"io"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestBlockingReleaseResourcesNamesEachFullClassOnce(t *testing.T) {
	capacities := map[releaseResourceClass]int{
		releaseResourceDocker: 3, releaseResourceHostOllama: 1, releaseResourceCPU: 1,
	}
	inUse := map[releaseResourceClass]int{
		releaseResourceDocker: 3, releaseResourceHostOllama: 1,
	}
	got := blockingReleaseResources([]releaseResourceClass{
		releaseResourceHostOllama, releaseResourceCPU, releaseResourceDocker, releaseResourceDocker,
	}, capacities, inUse)
	want := []releaseResourceClass{releaseResourceHostOllama, releaseResourceDocker}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("blocking = %v, want %v", got, want)
	}
	if got := blockingReleaseResources([]releaseResourceClass{releaseResourceCPU}, capacities, inUse); len(got) != 0 {
		t.Fatalf("a free cpu slot reported blocking %v", got)
	}
}

func TestReleaseWaitNoticeNamesDurationAndSortedClasses(t *testing.T) {
	got := releaseWaitNotice("agent-core integration", 9*time.Minute+50*time.Second+1234*time.Microsecond,
		map[releaseResourceClass]bool{releaseResourceHostOllama: true, releaseResourceCPU: true})
	want := "=== release gate waited: agent-core integration (9m50.001s for cpu, host-ollama) ==="
	if got != want {
		t.Fatalf("notice = %q, want %q", got, want)
	}
}

// TestHostOllamaWaitIsReported is GH-2101: attempt 4 of the 2026-09-16 release
// showed agent-core idle for 5m52 behind chatbot-mesh, visible only by
// reconstructing timestamps. The scheduler now says so when agent-core launches,
// and says nothing for a gate that never waited.
func TestHostOllamaWaitIsReported(t *testing.T) {
	all := releaseGates("/release")
	gates := []releaseGate{all[0], all[3], all[6]}
	releaseChatbot := make(chan struct{})

	output := captureReleaseStdout(t, func() {
		done := make(chan error, 1)
		started := make(chan string, len(gates))
		go func() {
			done <- executeReleaseGates(gates, func(gate releaseGate) error {
				started <- gate.name
				if gate.name == "applications/chatbot-mesh integration" {
					<-releaseChatbot
				}
				return nil
			})
		}()
		receiveReleaseStart(t, started) // root audit
		receiveReleaseStart(t, started) // chatbot-mesh
		time.Sleep(20 * time.Millisecond)
		close(releaseChatbot)
		receiveReleaseStart(t, started) // agent-core
		if err := receiveReleaseResult(t, done); err != nil {
			t.Fatal(err)
		}
	})

	if !strings.Contains(output, "=== release gate waited: agent-core integration (") ||
		!strings.Contains(output, " for host-ollama) ===") {
		t.Fatalf("agent-core wait not reported:\n%s", output)
	}
	if strings.Contains(output, "waited: applications/chatbot-mesh") ||
		strings.Contains(output, "waited: root audit") {
		t.Fatalf("a gate that never waited reported a wait:\n%s", output)
	}
}

func captureReleaseStdout(t *testing.T, fn func()) string {
	t.Helper()
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	original := os.Stdout
	os.Stdout = writer
	collected := make(chan string, 1)
	go func() {
		data, _ := io.ReadAll(reader)
		collected <- string(data)
	}()
	defer func() {
		os.Stdout = original
	}()
	fn()
	os.Stdout = original
	_ = writer.Close()
	return <-collected
}
