// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package kindrig

import (
	"errors"
	"strings"
	"testing"
)

func TestApplicationBucketNameAcceptsOnlyWholeGSBucket(t *testing.T) {
	if got, err := applicationBucketName("gs://chatbot-mesh-telemetry"); err != nil ||
		got != "chatbot-mesh-telemetry" {
		t.Fatalf("bucket = %q, %v", got, err)
	}
	for _, invalid := range []string{
		"s3://bucket", "gs://", "gs://bucket/prefix", "gs://bucket?project=other",
	} {
		if _, err := applicationBucketName(invalid); err == nil {
			t.Errorf("accepted invalid local bucket URL %q", invalid)
		}
	}
}

func TestFakeGCSBucketEnsureCreatesOnlyWhenAbsent(t *testing.T) {
	var commands []string
	run := func(name string, args ...string) ([]byte, error) {
		command := strings.Join(append([]string{name}, args...), " ")
		commands = append(commands, command)
		if strings.Contains(command, "--post-data=") {
			return []byte(`{"name":"fixture-telemetry"}`), nil
		}
		return []byte("HTTP/1.1 404 Not Found"), errors.New("exit status 8")
	}
	created, err := ensureFakeGCSBucketWithRunner(run, "fixture-telemetry")
	if err != nil || !created {
		t.Fatalf("ensure absent bucket = created %t, err %v", created, err)
	}
	got := strings.Join(commands, "\n")
	if !strings.Contains(got, `--post-data={"name":"fixture-telemetry"}`) {
		t.Fatalf("ensure did not create the exact bucket:\n%s", got)
	}

	commands = nil
	existing := func(name string, args ...string) ([]byte, error) {
		commands = append(commands, strings.Join(append([]string{name}, args...), " "))
		return []byte(`{"name":"fixture-telemetry"}`), nil
	}
	created, err = ensureFakeGCSBucketWithRunner(existing, "fixture-telemetry")
	if err != nil || created {
		t.Fatalf("ensure existing bucket = created %t, err %v", created, err)
	}
	if strings.Contains(strings.Join(commands, "\n"), "--post-data=") {
		t.Fatal("existing bucket was mutated")
	}
}

func TestFakeGCSBucketObjectsAreReadOnly(t *testing.T) {
	output := []byte(`{"items":[{"name":"traces/a.json"},{"name":"metrics/b.json"}]}`)
	var command string
	run := func(name string, args ...string) ([]byte, error) {
		command = strings.Join(append([]string{name}, args...), " ")
		return output, nil
	}
	keys, err := fakeGCSBucketObjectsWithRunner(run, "fixture-telemetry")
	if err != nil || strings.Join(keys, ",") != "traces/a.json,metrics/b.json" {
		t.Fatalf("objects = %v, %v", keys, err)
	}
	if strings.Contains(command, "--post-data") || strings.Contains(command, "DELETE") {
		t.Fatalf("read-only listing mutated storage: %s", command)
	}
}
