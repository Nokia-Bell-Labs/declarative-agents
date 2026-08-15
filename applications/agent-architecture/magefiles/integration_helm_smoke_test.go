// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package main

import (
	"strings"
	"testing"
)

func TestSelectCuratorPod(t *testing.T) {
	t.Run("picks the running pod and its curator restart count", func(t *testing.T) {
		raw := []byte(`{"items":[
			{"metadata":{"name":"old","uid":"uid-old"},"status":{"phase":"Succeeded","containerStatuses":[{"name":"curator","restartCount":9}]}},
			{"metadata":{"name":"live","uid":"uid-live"},"status":{"phase":"Running","containerStatuses":[{"name":"curator","restartCount":3}]}}
		]}`)
		pod, err := selectCuratorPod(raw)
		if err != nil {
			t.Fatalf("selectCuratorPod: %v", err)
		}
		if pod.name != "live" || pod.uid != "uid-live" || pod.restarts != 3 {
			t.Fatalf("got %+v, want {live uid-live 3}", pod)
		}
	})

	t.Run("falls back to the first container when none is named", func(t *testing.T) {
		raw := []byte(`{"items":[
			{"metadata":{"name":"live","uid":"u"},"status":{"phase":"Running","containerStatuses":[{"name":"sidecar","restartCount":1}]}}
		]}`)
		pod, err := selectCuratorPod(raw)
		if err != nil {
			t.Fatalf("selectCuratorPod: %v", err)
		}
		if pod.restarts != 1 {
			t.Fatalf("restarts = %d, want 1", pod.restarts)
		}
	})

	t.Run("errors when no pod is running", func(t *testing.T) {
		raw := []byte(`{"items":[{"metadata":{"name":"p","uid":"u"},"status":{"phase":"Pending"}}]}`)
		if _, err := selectCuratorPod(raw); err == nil {
			t.Fatal("expected error for no running pod")
		}
	})

	t.Run("errors on malformed JSON", func(t *testing.T) {
		if _, err := selectCuratorPod([]byte("{not json")); err == nil {
			t.Fatal("expected decode error")
		}
	})
}

func TestCuratorShutdownObserved(t *testing.T) {
	prior := curatorPod{name: "live", uid: "uid-live", restarts: 3}

	tests := []struct {
		name     string
		raw      string
		wantDone bool
		wantErr  string
	}{
		{
			name:     "restart with Completed exit is clean",
			raw:      `{"metadata":{"name":"live","uid":"uid-live"},"status":{"phase":"Running","containerStatuses":[{"name":"curator","restartCount":4,"lastState":{"terminated":{"exitCode":0,"reason":"Completed"}}}]}}`,
			wantDone: true,
		},
		{
			name:     "restart with terminal-state exit code 2 is clean",
			raw:      `{"metadata":{"name":"live","uid":"uid-live"},"status":{"phase":"Running","containerStatuses":[{"name":"curator","restartCount":4,"lastState":{"terminated":{"exitCode":2,"reason":"Error"}}}]}}`,
			wantDone: true,
		},
		{
			name:    "restart with crash exit code fails",
			raw:     `{"metadata":{"name":"live","uid":"uid-live"},"status":{"phase":"Running","containerStatuses":[{"name":"curator","restartCount":4,"lastState":{"terminated":{"exitCode":137,"reason":"OOMKilled"}}}]}}`,
			wantErr: "uncleanly",
		},
		{
			name:     "restart without recorded last state is accepted as terminated",
			raw:      `{"metadata":{"name":"live","uid":"uid-live"},"status":{"phase":"Running","containerStatuses":[{"name":"curator","restartCount":4}]}}`,
			wantDone: true,
		},
		{
			name:     "no restart and still running is not done",
			raw:      `{"metadata":{"name":"live","uid":"uid-live"},"status":{"phase":"Running","containerStatuses":[{"name":"curator","restartCount":3,"state":{}}]}}`,
			wantDone: false,
		},
		{
			name:     "fully terminated container with clean exit is done",
			raw:      `{"metadata":{"name":"live","uid":"uid-live"},"status":{"phase":"Succeeded","containerStatuses":[{"name":"curator","restartCount":3,"state":{"terminated":{"exitCode":0,"reason":"Completed"}}}]}}`,
			wantDone: true,
		},
		{
			name:    "fully terminated container with crash exit fails",
			raw:     `{"metadata":{"name":"live","uid":"uid-live"},"status":{"phase":"Failed","containerStatuses":[{"name":"curator","restartCount":3,"state":{"terminated":{"exitCode":1,"reason":"Error"}}}]}}`,
			wantErr: "uncleanly",
		},
		{
			name:     "replaced pod with a new uid counts as gone",
			raw:      `{"metadata":{"name":"live","uid":"uid-new"},"status":{"phase":"Running","containerStatuses":[{"name":"curator","restartCount":0}]}}`,
			wantDone: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			done, err := curatorShutdownObserved([]byte(tt.raw), prior)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("err = %v, want containing %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if done != tt.wantDone {
				t.Fatalf("done = %v, want %v", done, tt.wantDone)
			}
		})
	}

	t.Run("malformed JSON errors", func(t *testing.T) {
		if _, err := curatorShutdownObserved([]byte("{bad"), prior); err == nil {
			t.Fatal("expected decode error")
		}
	})
}

func TestIsKubectlNotFound(t *testing.T) {
	if !isKubectlNotFound([]byte(`Error from server (NotFound): pods "live" not found`)) {
		t.Fatal("expected NotFound message to be recognized")
	}
	if isKubectlNotFound([]byte(`{"metadata":{"name":"live"}}`)) {
		t.Fatal("normal pod JSON must not be treated as NotFound")
	}
}
