// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
// Package commands registers every scrum-master subcommand group in cli.Default, so
// cmd/tpm and the gates that read the dispatch table see the same groups.
package commands

import (
	_ "github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/backlog"
	_ "github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/goals"
	_ "github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/install"
	_ "github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/plan"
	_ "github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/program"
	_ "github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/release"
	_ "github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/reports"
	_ "github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/scaffold"
	_ "github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/site"
	_ "github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/sprintend"
	_ "github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/standup"
)
