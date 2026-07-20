// Copyright (c) 2026 Nokia. All rights reserved.

package main

import "testing"

// TestJuristSucceeded pins the report classification to the jurist's observed
// output contract: a clean run ends "terminal state: succeeded"; a failing run
// ends "terminal state: failed" (with "status=failed" in the run-complete log);
// both exit zero, so the terminal state is the only signal. A report with neither
// marker is an indeterminate run and must be an error, not a silent pass.
func TestJuristSucceeded(t *testing.T) {
	cases := []struct {
		name    string
		report  string
		wantOK  bool
		wantErr bool
	}{
		{
			name:   "clean corpus",
			report: "validate: 3 SRDs ... — OK\nrun complete: status=succeeded\nterminal state: succeeded\n",
			wantOK: true,
		},
		{
			name:    "error finding fails",
			report:  "[error] builtin-spec-corpus/index-broken-path ...\nrun complete: status=failed\nterminal state: failed\n",
			wantOK:  false,
			wantErr: false,
		},
		{
			name:    "status=failed without terminal line still fails",
			report:  "run complete: status=failed iterations=3\n",
			wantOK:  false,
			wantErr: false,
		},
		{
			name:   "warnings only still succeed",
			report: "[warning] builtin-spec-corpus/orphaned-srd ...\nterminal state: succeeded\n",
			wantOK: true,
		},
		{
			name:    "indeterminate run is an error",
			report:  "building agent binary...\n",
			wantOK:  false,
			wantErr: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ok, err := juristSucceeded(tc.report)
			if ok != tc.wantOK {
				t.Errorf("ok = %v, want %v", ok, tc.wantOK)
			}
			if (err != nil) != tc.wantErr {
				t.Errorf("err = %v, wantErr %v", err, tc.wantErr)
			}
		})
	}
}
