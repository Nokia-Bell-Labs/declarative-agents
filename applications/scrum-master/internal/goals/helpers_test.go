// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
package goals

import "github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/tracker"

func openIssues() tracker.IssueQuery { return tracker.IssueQuery{State: "opened"} }

func sprintMilestone(due string) tracker.Milestone {
	return tracker.Milestone{Title: "2026-W43..W45", DueDate: due}
}
