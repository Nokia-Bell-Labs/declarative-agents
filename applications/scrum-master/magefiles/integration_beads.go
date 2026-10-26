// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
package main

// The 03.0 targets build fixture tracker repositories on local disk and
// need no rig and no network (test-rel03.0-beads-tracker), so they are not
// among the integrationTargets mage regression runs against the rig.

// BeadsPopClose proves one issue's life on beads: pop assigns only the caller, the merge closes it with the commit cited, a second close changes nothing (test-rel03.0-beads-tracker).
func (Integration) BeadsPopClose() error {
	return beadsTest("TestPopAndCloseCitesTheMergeCommit")
}

// BeadsStates proves every srd041 table row fires on beads with its actor and assignee effect, and transitions outside the table are refused (test-rel03.0-beads-tracker).
func (Integration) BeadsStates() error {
	return beadsTest("TestStateTableReplaysOnBeads")
}

// BeadsSync proves two clones' disjoint writes both land and a contended pop keeps the earlier taker (test-rel03.0-beads-tracker).
func (Integration) BeadsSync() error {
	return beadsTest("TestContendedPopKeepsTheEarlierTaker")
}

// BeadsOffline proves an offline clone reads its cache and names the last pulled commit, and its provisional pops reconcile (test-rel03.0-beads-tracker).
func (Integration) BeadsOffline() error {
	return beadsTest("TestOfflineReadNamesTheLastPulledCommit")
}

func beadsTest(name string) error {
	root, err := repositoryRoot()
	if err != nil {
		return err
	}
	return runStreaming(root, "go", "test", "./internal/beads", "-run", "^"+name+"$", "-count=1", "-v")
}
