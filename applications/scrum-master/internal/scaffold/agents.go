// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
package scaffold

import (
	"fmt"
	"strings"

	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/install"
)

// AgentsFile is the orientation file that tells an agent which skills work
// on the repository and the rules for changing it (srd043 R1.6).
const AgentsFile = "AGENTS.md"

// agentsFile renders AGENTS.md for a repository at level from the embedded
// skills, so it names what the vendored profiles run and cannot drift from
// them. It names no command: every workflow runs as a skill, the agent over
// a profile in .scrum-master (GH-36).
func agentsFile(level string) ([]byte, error) {
	skills, err := install.EmbeddedSkills()
	if err != nil {
		return nil, err
	}
	var b strings.Builder
	b.WriteString(BlockBegin + "\n# Agent instructions\n\n")
	fmt.Fprintf(&b, "This is a program's %s repository, run with the agent and the AI-native\n", level)
	b.WriteString("scrum process's profiles, vendored in .scrum-master. Read README.md first:\n" +
		"it says what the repository holds, how the planning hierarchy nests, and\n" +
		"which skill serves each step of the process.\n\n")
	b.WriteString("## Rules\n\n" +
		"- Every change starts as an issue: file it with the specify skill, take it with the pop or implement skill, and deliver it with the merge skill. Never commit to main directly.\n" +
		"- A change under docs/ is a change of intent: the corpus gate must pass before it merges.\n" +
		"- Run a workflow through its skill: each runs the agent over its profile in .scrum-master, and the profile's words fetch the facts and write the tracker. Never read or edit tracker files by hand. The words do the workflow, and the model writes only language (docs/constitutions/design.md D10).\n" +
		"- Every document follows docs/constitutions/design.md: Markdown with marker tags, in the format of the parent release docs/external-pins.yaml pins. After taking a new release, run the upgrade profile to vendor it, add missing files, repair the format, and refresh the skills; the check skill reports what drifted.\n" +
		"- Every figure is Mermaid in a fenced code block inside the Markdown document that shows it. Put no generated image file, and no other diagram source, under docs/.\n")
	if level == "system" {
		b.WriteString("- When control/trackers.md names a tracker_branch of this repository, it is a program of one repository: the tracker lives on that branch. Never check it out, merge it, or base a branch on it.\n")
	}
	b.WriteString("\n")
	b.WriteString("## Skills\n\nThe init or onboard profile installs them for your agent; the upgrade profile updates them to a new release.\n\n")
	// Every skill serves both levels: tasks files a release's Sprint Goals
	// at the system and a story's issues in a team's repository (#188).
	for _, s := range skills {
		desc, _, _ := strings.Cut(s.Description, " Use when")
		fmt.Fprintf(&b, "- %s: %s\n", s.Name, desc)
	}
	b.WriteString(BlockEnd + "\n")
	return []byte(b.String()), nil
}
