// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
// Package scrum-master holds the Agent Plugins 1.0 package of the skills the
// scrum-master binary embeds; scrum-master install writes them for the developer's agent
// (srd037).
package scrummaster

import "embed"

// Plugin is the package plugin/: plugin.json and skills/<name>/SKILL.md.
//
//go:embed plugin
var Plugin embed.FS

// SkillsDir is where the skills sit in Plugin.
const SkillsDir = "plugin/skills"

// AgentsDir is where the declarative-agents profiles sit in Plugin: one
// directory per skill whose workflow is a profile (GH-135, design.md D14).
// scrum-master install renders such a skill's SKILL.md from its machine.
const AgentsDir = "plugin/agents"

// Constitution is the design constitution every repository of a program
// carries (design.md D7); scrum-master init and onboard write it (srd043).
//
//go:embed docs/constitutions/design.md
var Constitution []byte

// Guides are the guides a profile routes into its agent's context with
// scrum-master backlog guide (GH-59), as this release wrote them.
//
//go:embed docs/guides/*.md
var Guides embed.FS

// GuidesDir is where the guides sit in Guides.
const GuidesDir = "docs/guides"

// Charter is the corpus charter this corpus gates on; the corpora scrum-master
// scaffolds gate on the same checks (srd043 R1.1).
//
//go:embed docs/corpus-charter.yaml
var Charter []byte
