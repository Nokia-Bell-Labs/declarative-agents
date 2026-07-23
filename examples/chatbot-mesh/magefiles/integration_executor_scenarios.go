// Copyright (c) 2026 Nokia. All rights reserved.

package main

// executorScenario is one leg of the executor's two request machines: what the
// fakes should do, what the endpoint must answer, and what the run must (and
// must not) have invoked.
type executorScenario struct {
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

// applyPatch is a values-plane document of the shape the coordinator decides and
// the creator forwards (srd006 R1.4). It carries no host, URL, method, or
// credential -- the executor accepts none (R2.3).
const applyPatch = `{"schema_version":"1","content":"ragUnits:\n  - name: rag0\n    collection: corpus\n"}`

// countsJSON is what kubectl_get_rollout_counts renders off the Deployment: one
// compact object the rollout response maps by field (srd006 R3.3, GH-686).
const countsJSON = `{"ready":2,"desired":2,"revision":7}`

// executorScenarios walks every terminal of both machines. The apply machine has
// four (Done, Rejected, RolledBack, Failed) and the rollout machine three
// (Complete, Progressing, Unavailable); each is reached by failing a different
// word, which is the only way an exec-word machine tells its outcomes apart.
func executorScenarios() []executorScenario {
	return []executorScenario{
		{
			name:       "a valid patch applies as a values file and verifies",
			applyBody:  applyPatch,
			wantStatus: 200,
			wantBody:   []string{`"status":"applied"`},
			wantCalls: []string{
				// The dry-run validates against the chart schema before anything applies.
				"helm upgrade", "--dry-run",
				// The apply is a values-file rollout, atomic and waited (R2.2).
				"--reuse-values", "--atomic", "--wait", "-f",
				// The rollout is verified by kubectl, not by the executor computing a phase.
				"kubectl rollout status",
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
			// an ongoing rollout in a panel that polls every 3s (GH-686).
			exits:      map[string]int{"poll": 1, "counts": 1},
			wantStatus: 502,
			wantBody:   []string{`"error":"rollout_read_failed"`, `"status":"unavailable"`},
			wantCalls:  []string{"kubectl get"},
		},
	}
}
