// Copyright (c) 2026 Nokia. All rights reserved.

package main

// applierScenario is one leg of the applier's two request machines: what the
// fakes should do, what the endpoint must answer, and what the run must (and
// must not) have invoked. The coding applier has no state endpoint, so the
// apply and rollout machines are the whole surface (srd006 R2, R3).
type applierScenario struct {
	name string
	// applyBody drives the apply endpoint; empty drives the rollout read.
	applyBody   string
	exits       map[string]int    // verb -> exit code, unplanned verbs exit 0
	stdout      map[string]string // verb -> stdout
	wantStatus  int
	wantBody    []string
	wantCalls   []string
	absentCalls []string
}

// applyPatch is a values-plane document of the shape an operator or CI caller
// decides and posts to the apply endpoint (srd006 R1.3). It adjusts a coding
// role parameter and carries no host, URL, method, or credential -- the applier
// accepts none (R2.3). The fake helm never reads it; the real chart schema is
// proven separately in applier_schema_test.go.
const applyPatch = `{"schema_version":"1","content":"roles:\n  executor:\n    resources:\n      requests:\n        memory: 321Mi\n"}`

// countsJSON is what kubectl_get_rollout_counts renders off the planner
// Deployment: one compact object the rollout response maps by field (srd006
// R3.3).
const countsJSON = `{"ready":2,"desired":2,"revision":7}`

// applierScenarios walks every terminal of both request machines. The apply
// machine has four (Done, Rejected, RolledBack, Failed) and the rollout machine
// three (Complete, Progressing, Unavailable); each is reached by failing a
// different word, which is the only way an exec-word machine tells its outcomes
// apart.
func applierScenarios() []applierScenario {
	return []applierScenario{
		{
			name:       "a valid patch applies as a values file and verifies all three roles",
			applyBody:  applyPatch,
			wantStatus: 200,
			wantBody:   []string{`"status":"applied"`},
			wantCalls: []string{
				// The dry-run validates against the chart schema before anything applies.
				"helm upgrade", "--dry-run",
				// The apply is a values-file rollout, atomic and waited (R2.2).
				"--reuse-values", "--atomic", "--wait", "-f",
				// All three role Deployments are verified by kubectl, not by the
				// applier computing a phase.
				"deployment/coding-agent-planner",
				"deployment/coding-agent-executor",
				"deployment/coding-agent-critic",
			},
			absentCalls: []string{"helm rollback"},
		},
		{
			name:       "a non-conforming patch is rejected with no rollout",
			applyBody:  applyPatch,
			exits:      map[string]int{"dry-run": 1},
			wantStatus: 400,
			wantBody:   []string{`"error":"validate_rejected"`, `"status":"rejected"`},
			wantCalls:  []string{"helm upgrade", "--dry-run"},
			// Nothing may be applied after the schema rejects the patch: no
			// waited upgrade, no rollout read, no rollback.
			absentCalls: []string{"--wait", "kubectl rollout status", "helm rollback"},
		},
		{
			name:       "a stalled verify rolls the release back",
			applyBody:  applyPatch,
			exits:      map[string]int{"verify": 1},
			wantStatus: 500,
			wantBody:   []string{`"error":"rolled_back"`, `"status":"rolled_back"`},
			wantCalls:  []string{"--wait", "kubectl rollout status", "helm rollback"},
		},
		{
			name:       "a failed apply reports failure and does not double-roll-back",
			applyBody:  applyPatch,
			exits:      map[string]int{"upgrade": 1},
			wantStatus: 500,
			wantBody:   []string{`"error":"apply_failed"`, `"status":"failed"`},
			wantCalls:  []string{"helm upgrade", "--dry-run"},
			// --atomic already rolled the failed upgrade back, so an explicit
			// rollback here would roll the release back one revision too far.
			absentCalls: []string{"helm rollback", "kubectl rollout status"},
		},
		{
			name:       "a complete rollout reports the phase with the counts",
			exits:      map[string]int{"poll": 0},
			stdout:     map[string]string{"counts": countsJSON},
			wantStatus: 200,
			wantBody:   []string{`"phase":"complete"`, `"ready":2`, `"desired":2`, `"revision":7`},
			wantCalls:  []string{"kubectl rollout status", "kubectl get"},
		},
		{
			name:       "a progressing rollout reports the phase with the counts",
			exits:      map[string]int{"poll": 1},
			stdout:     map[string]string{"counts": countsJSON},
			wantStatus: 200,
			wantBody:   []string{`"phase":"progressing"`, `"ready":2`, `"desired":2`},
			wantCalls:  []string{"kubectl rollout status", "kubectl get"},
		},
		{
			name: "an unreadable Deployment is a gateway error, not a phase",
			// The counts read is what proves the cluster answered at all. When it
			// fails, reporting "progressing" would render a wholly broken read as
			// an ongoing rollout in a panel that polls every 3s (srd006 R3.3).
			exits:      map[string]int{"poll": 1, "counts": 1},
			wantStatus: 502,
			wantBody:   []string{`"error":"rollout_read_failed"`, `"status":"unavailable"`},
			wantCalls:  []string{"kubectl get"},
		},
	}
}
