// Copyright (c) 2026 Nokia. All rights reserved.

package main

import "testing"

func TestAuditRunFailed(t *testing.T) {
	tests := []struct {
		name   string
		output string
		want   bool
	}{
		{
			name: "terminal state failed",
			output: `2026/06/16 20:57:05 run complete: status=failed iterations=1
terminal state: failed
`,
			want: true,
		},
		{
			name:   "run summary failed",
			output: "2026/06/16 20:57:05 run complete: status=failed iterations=1\n",
			want:   true,
		},
		{
			name:   "succeeded",
			output: "2026/06/16 20:57:05 run complete: status=succeeded iterations=1\nterminal state: succeeded\n",
			want:   false,
		},
		{
			name:   "empty",
			output: "",
			want:   false,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := auditRunFailed(tc.output)
			if got != tc.want {
				t.Fatalf("auditRunFailed() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestEnvWithDefault(t *testing.T) {
	t.Parallel()

	got := envWithDefault([]string{"PATH=/bin"}, "AGENT_CORE_HOME", "/repo")
	if !containsEnv(got, "AGENT_CORE_HOME=/repo") {
		t.Fatalf("envWithDefault() = %v, want AGENT_CORE_HOME default", got)
	}

	existing := []string{"AGENT_CORE_HOME=/custom"}
	got = envWithDefault(existing, "AGENT_CORE_HOME", "/repo")
	if len(got) != 1 || got[0] != "AGENT_CORE_HOME=/custom" {
		t.Fatalf("envWithDefault() = %v, want existing value preserved", got)
	}
}

func containsEnv(env []string, want string) bool {
	for _, entry := range env {
		if entry == want {
			return true
		}
	}
	return false
}
