// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package main

// applierScenario is one leg of the applier's two request machines: what the fakes
// should do, what the endpoint must answer, and what the run must (and must not)
// have invoked. The applier has no state endpoint, so the apply and rollout machines
// are the whole surface (srd002-applier R2, R3).
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

// applyPatch is a values-plane document of the shape an operator or CI caller decides
// and posts to the apply endpoint (srd002-applier R1.3). It adjusts a collector
// resource request and carries no host, URL, method, or credential -- the applier
// accepts none (R2.3). The fake helm never reads it; the real chart schema is proven
// separately in applier_schema_test.go.
const applyPatch = `{"schema_version":"1","content":"collector:\n  resources:\n    requests:\n      memory: 96Mi\n"}`

// countsJSON is what kubectl_get_rollout_counts renders off the collector Deployment:
// one compact object the rollout response maps by field (srd002-applier R3.3).
const countsJSON = `{"ready":1,"desired":1,"revision":7}`

// applierScenarios walks every terminal of both request machines. The apply machine
// has four (Done, Rejected, RolledBack, Failed) and the rollout machine three
// (Complete, Progressing, Unavailable); each is reached by failing a different word,
// which is the only way an exec-word machine tells its outcomes apart.
func applierScenarios() []applierScenario {
	return []applierScenario{
		{
			name:       "a valid patch applies as a values file and verifies the collector",
			applyBody:  applyPatch,
			wantStatus: 200,
			wantBody:   []string{`"status":"applied"`},
			wantCalls: []string{
				// The dry-run validates against the chart schema before anything applies.
				"helm upgrade", "--dry-run",
				// The apply is a values-file rollout that returns without waiting (R2.2).
				"--reuse-values", "-f",
				// The collector Deployment -- the application's stable server -- is the
				// verify target, read by kubectl rather than the applier computing a phase.
				"deployment/agent-architecture-collector",
			},
			// helm_upgrade neither waits nor self-rolls-back: the verify step observes a
			// stall and helm_rollback compensates it (applier_helm_flags_test.go pins the
			// argv). The bounded curator is never a verify target, so no invocation names
			// it.
			absentCalls: []string{"--wait", "helm rollback", "deployment/agent-architecture-curator"},
		},
		{
			name:       "a non-conforming patch is rejected with no rollout",
			applyBody:  applyPatch,
			exits:      map[string]int{"dry-run": 1},
			wantStatus: 400,
			wantBody:   []string{`"error":"validate_rejected"`, `"status":"rejected"`},
			wantCalls:  []string{"helm upgrade", "--dry-run"},
			// Nothing may be applied after the schema rejects the patch: no waited
			// upgrade, no rollout read, no rollback.
			absentCalls: []string{"--wait", "kubectl rollout status", "helm rollback"},
		},
		{
			// The apply command succeeds, then the collector verify fails: the machine
			// reaches Applying -> VerifyingCollector(stall) -> RollingBack -> RolledBack,
			// the path a waited --atomic upgrade used to make unreachable. Reaching
			// kubectl rollout status proves the apply itself ran (the machine only
			// verifies after a successful Applying).
			name:       "a stalled collector verify rolls the release back",
			applyBody:  applyPatch,
			exits:      map[string]int{"verify": 1},
			wantStatus: 500,
			wantBody:   []string{`"error":"rolled_back"`, `"status":"rolled_back"`},
			wantCalls:  []string{"--reuse-values", "kubectl rollout status", "helm rollback"},
		},
		{
			name:       "a failed apply reports failure and does not roll back",
			applyBody:  applyPatch,
			exits:      map[string]int{"upgrade": 1},
			wantStatus: 500,
			wantBody:   []string{`"error":"apply_failed"`, `"status":"failed"`},
			wantCalls:  []string{"helm upgrade", "--dry-run"},
			// A hard apply-command failure lands in Failed with no compensating rollback;
			// helm rollback is reserved for a post-apply verify stall, and the machine
			// never reaches the verify step when Applying fails.
			absentCalls: []string{"helm rollback", "kubectl rollout status"},
		},
		{
			name:       "a complete rollout reports the phase with the counts",
			exits:      map[string]int{"poll": 0},
			stdout:     map[string]string{"counts": countsJSON},
			wantStatus: 200,
			wantBody:   []string{`"phase":"complete"`, `"ready":1`, `"desired":1`, `"revision":7`},
			wantCalls:  []string{"kubectl rollout status", "kubectl get"},
		},
		{
			name:       "a progressing rollout reports the phase with the counts",
			exits:      map[string]int{"poll": 1},
			stdout:     map[string]string{"counts": countsJSON},
			wantStatus: 200,
			wantBody:   []string{`"phase":"progressing"`, `"ready":1`, `"desired":1`},
			wantCalls:  []string{"kubectl rollout status", "kubectl get"},
		},
		{
			name: "an unreadable Deployment is a gateway error, not a phase",
			// The counts read is what proves the cluster answered at all. When it fails,
			// reporting "progressing" would render a wholly broken read as an ongoing
			// rollout in a panel that polls every 3s (srd002-applier R3.3).
			exits:      map[string]int{"poll": 1, "counts": 1},
			wantStatus: 502,
			wantBody:   []string{`"error":"rollout_read_failed"`, `"status":"unavailable"`},
			wantCalls:  []string{"kubectl get"},
		},
	}
}
