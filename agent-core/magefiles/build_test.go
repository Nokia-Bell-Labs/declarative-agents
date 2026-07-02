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

	got := envWithDefault([]string{"PATH=/bin"}, "TEST_OVERRIDE_PATH", "/repo")
	if !containsEnv(got, "TEST_OVERRIDE_PATH=/repo") {
		t.Fatalf("envWithDefault() = %v, want TEST_OVERRIDE_PATH default", got)
	}

	existing := []string{"TEST_OVERRIDE_PATH=/custom"}
	got = envWithDefault(existing, "TEST_OVERRIDE_PATH", "/repo")
	if len(got) != 1 || got[0] != "TEST_OVERRIDE_PATH=/custom" {
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
