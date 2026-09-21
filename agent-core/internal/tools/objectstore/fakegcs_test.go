// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package objectstore

import (
	"fmt"
	"net/http"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
)

// The live proof's backend, pinned by version and digest per ENG01 C2 and
// enumerated in magefiles/pinsurvey/pins.yaml so mage bump surveys it.
const (
	fakeGCSImageVersion = "1.56.1"
	fakeGCSImageDigest  = "sha256:797ce226d62f947c009dc40246b30cfb456b8473d8241407f9d6f2c04e4d69ef"
	fakeGCSImage        = "docker.io/fsouza/fake-gcs-server:" + fakeGCSImageVersion + "@" + fakeGCSImageDigest
	fakeGCSContainer    = "agent-core-objectstore-fake-gcs"
)

// startFakeGCS runs the pinned fake-gcs-server and returns the JSON API
// endpoint. It skips naming docker where docker is absent (ENG01: integration
// targets skip and name the missing dependency), replaces a leftover
// container from an interrupted run, and removes its own on every exit path.
func startFakeGCS(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("docker not on PATH: the fake-gcs live proof needs it")
	}
	// A leftover container from an interrupted run holds the name; replace
	// it rather than failing on the collision.
	_ = exec.Command("docker", "rm", "-f", fakeGCSContainer).Run()
	out, err := exec.Command("docker", "run", "-d", "--name", fakeGCSContainer,
		"-p", "127.0.0.1:0:4443", fakeGCSImage,
		"-scheme", "http", "-port", "4443").CombinedOutput()
	if err != nil {
		t.Fatalf("docker run fake-gcs-server: %v\n%s", err, out)
	}
	t.Cleanup(func() { _ = exec.Command("docker", "rm", "-f", fakeGCSContainer).Run() })

	portOut, err := exec.Command("docker", "port", fakeGCSContainer, "4443/tcp").Output()
	if err != nil {
		t.Fatalf("docker port: %v", err)
	}
	address := strings.TrimSpace(strings.Split(string(portOut), "\n")[0])
	endpoint := "http://" + address + "/storage/v1/"

	deadline := time.Now().Add(30 * time.Second)
	for {
		response, err := http.Get(endpoint + "b?project=test")
		if err == nil {
			_ = response.Body.Close()
			if response.StatusCode == http.StatusOK {
				break
			}
		}
		if time.Now().After(deadline) {
			logs, _ := exec.Command("docker", "logs", fakeGCSContainer).CombinedOutput()
			t.Fatalf("fake-gcs-server never answered at %s; container logs:\n%s", endpoint, logs)
		}
		time.Sleep(300 * time.Millisecond)
	}
	// The family opens buckets and never creates them; the fixture bucket is
	// the harness's to provide, as the platform provides a real one.
	body := strings.NewReader(`{"name":"agents"}`)
	response, err := http.Post(endpoint+"b?project=test", "application/json", body)
	if err != nil {
		t.Fatalf("create fixture bucket: %v", err)
	}
	_ = response.Body.Close()
	return endpoint
}

// srd059 R4.2, AC4: the family end to end over gs:// with the explicit
// endpoint, against a real server: write, read, list, and both undo
// directions.
func TestObjectstoreAgainstFakeGCS(t *testing.T) {
	endpoint := startFakeGCS(t)
	opener := NewOpener()
	connection := ConnectionConfig{BucketURL: "gs://agents", Endpoint: endpoint}

	executeWrite(t, opener, connection, "reports/live.txt", "first body")
	read := executeRead(t, opener, connection, "reports/live.txt")
	if read.Signal != core.ToolDone || read.Output != "first body" {
		t.Fatalf("read = %v %q", read.Signal, read.Output)
	}

	overwrite := executeWrite(t, opener, connection, "reports/live.txt", "second body")
	list := (&ListBuilder{Opener: opener, Connection: connection}).
		Build(paramsResult(map[string]string{"prefix": "reports/"})).Execute()
	if list.Signal != core.ToolDone || !strings.Contains(list.Output, "reports/live.txt") {
		t.Fatalf("list = %v %q", list.Signal, list.Output)
	}

	builder := &WriteBuilder{Opener: opener, Connection: connection}
	undo := builder.BuildReverser().Undo(core.Result{Receipt: overwrite.Receipt})
	if undo.Signal != core.ToolDone {
		t.Fatalf("undo overwrite: %v %s", undo.Signal, undo.Output)
	}
	restored := executeRead(t, opener, connection, "reports/live.txt")
	if restored.Output != "first body" {
		t.Fatalf("after undo, object = %q, want the prior bytes", restored.Output)
	}

	created := executeWrite(t, opener, connection, "reports/fresh.txt", "created")
	undo = builder.BuildReverser().Undo(core.Result{Receipt: created.Receipt})
	if undo.Signal != core.ToolDone {
		t.Fatalf("undo create: %v %s", undo.Signal, undo.Output)
	}
	gone := executeRead(t, opener, connection, "reports/fresh.txt")
	if gone.Signal == core.ToolDone {
		t.Fatalf("object survived the undo of its creation: %q", gone.Output)
	}
	fmt.Println("objectstore live proof PASS - gs:// against fake-gcs-server through the explicit endpoint")
}
