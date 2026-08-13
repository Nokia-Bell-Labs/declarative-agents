// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"os"
	"regexp"
	"testing"
	"time"
)

func TestCodingAgentMachineTimeoutEnvelopes(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		path      string
		want      time.Duration
		authority time.Duration
	}{
		{
			name: "role server control await", path: "../agents/role-server/machine.yaml",
			want: 11 * time.Minute, authority: 10 * time.Minute,
		},
		{
			name: "planner request endpoint", path: "../agents/planner/request-machine.yaml",
			want: 21 * time.Minute, authority: 20 * time.Minute,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			got := readCodingMachineCommandTimeout(t, test.path)
			if got != test.want {
				t.Errorf("%s command_timeout = %s, want %s", test.path, got, test.want)
			}
			if got <= test.authority {
				t.Errorf("%s command_timeout = %s, must exceed governing operation %s", test.path, got, test.authority)
			}
		})
	}
}

func readCodingMachineCommandTimeout(t *testing.T, path string) time.Duration {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	match := regexp.MustCompile(`command_timeout:\s*([0-9]+(?:ns|us|µs|ms|s|m|h))`).FindSubmatch(data)
	if len(match) != 2 {
		t.Fatalf("%s has no command_timeout", path)
	}
	timeout, err := time.ParseDuration(string(match[1]))
	if err != nil {
		t.Fatalf("%s command_timeout: %v", path, err)
	}
	return timeout
}
