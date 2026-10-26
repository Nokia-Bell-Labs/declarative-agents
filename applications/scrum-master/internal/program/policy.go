// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
package program

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"

	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/corpus"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/tracker"
)

// Policy is what admission reads from control/policy.md and
// control/labels.md (config formats policy and labels). Without control
// files, Defaults applies.
type Policy struct {
	Planes []string
	Types  []string
	Kinds  []string
	// Components are the components the control project's subsystem SRDs
	// declare (srd001 R21.1), read when ComponentsRead. Read and empty, the
	// program declares none yet, so no component:: label is required; not
	// read, any component:: value is accepted.
	Components     []string
	ComponentsRead bool
	// RequireAC lists the kinds whose issues must carry Requirements and
	// Acceptance Criteria; any issue with a Cites line must carry them too.
	RequireAC []string
	// AdmissionRules are the named admission rules that apply (config format
	// policy, admission.rules); nil applies every rule scrum-master implements, so a
	// program disables one by naming the rest (srd013 R1.1).
	AdmissionRules   []string
	RequireMilestone bool
	// RequireApproval makes scrum-master issue merge refuse a branch without another
	// person's approval of its tip (srd013 R5.5); off, merge records an
	// approval when one exists and merges without one.
	RequireApproval  bool
	ManagementFilers []string // usernames; empty or ["leads"]: not checked
	TriageLabel      string
	// AuditCommand is the corpus gate scrum-master issue merge runs, in the worktree, on a
	// branch that changes docs/ (srd013 R5.3).
	AuditCommand string
	// ArchitectureChecks is how the corpus gate treats an ARCHITECTURE
	// component the plan does not cover or that drifted: warn, the
	// default, or refuse (srd045 R6.3, GH-56).
	ArchitectureChecks string
	// Releases and DefinitionOfDone drive scrum-master release (srd015, srd008 R2).
	Releases         Releases
	DefinitionOfDone map[string][]Check
	Escalation       Escalation
}

// Escalation is policy.escalation: the thresholds past which aggregation
// reports a signal (srd018 R3.2).
type Escalation struct {
	BlockerAgeDays     int `yaml:"blocker_age_days" json:"blocker_age_days"`
	WaitingChainLength int `yaml:"waiting_chain_length" json:"waiting_chain_length"`
	SlipReleases       int `yaml:"slip_releases" json:"slip_releases"`
	EvidenceAgeDays    int `yaml:"evidence_age_days" json:"evidence_age_days"`
}

// Releases is policy.releases: the version scheme, which bumps are
// breaking, and the tag format with {artifact} and {version}.
type Releases struct {
	Scheme    string `yaml:"scheme" json:"scheme"`
	Breaking  string `yaml:"breaking" json:"breaking"`
	TagFormat string `yaml:"tag_format" json:"tag_format"`
}

// Check is one entry of a definition_of_done gate (srd008 R2.2). Command is
// what a local tests or custom check runs from the repository root.
type Check struct {
	Kind        string `yaml:"kind" json:"kind"`
	Environment string `yaml:"environment" json:"environment,omitempty"`
	Suite       string `yaml:"suite" json:"suite,omitempty"`
	Command     string `yaml:"command" json:"command,omitempty"`
	Consumers   string `yaml:"consumers" json:"consumers,omitempty"`
	Against     string `yaml:"against" json:"against,omitempty"`
	Threshold   string `yaml:"threshold" json:"threshold,omitempty"`
}

// Gate is the checks declared for artifact, or the default gate.
func (p Policy) Gate(artifact string) []Check {
	if g, ok := p.DefinitionOfDone[artifact]; ok {
		return g
	}
	return p.DefinitionOfDone["default"]
}

// typeRenames pairs a type value's earlier name with its current one. The
// earlier name is read as an alias, so issues and control files written
// before the rename keep their meaning (srd001 R6.6).
var typeRenames = [][2]string{{"epic", "tracking"}}

// CanonicalType is the current name of a type value.
func CanonicalType(v string) string {
	for _, r := range typeRenames {
		if v == r[0] {
			return r[1]
		}
	}
	return v
}

func canonicalTypes(vals []string) []string {
	var out []string
	for _, v := range vals {
		if c := CanonicalType(v); !slices.Contains(out, c) {
			out = append(out, c)
		}
	}
	return out
}

// BacklogAudit is the corpus gate a repository without mage runs: scrum-master's own
// hierarchy gate (srd045 R8.2).
const BacklogAudit = WordsBinary + " backlog check"

// AuditFor is the audit a docs/ merge in the worktree at dir runs under p:
// the policy's command, except that the built-in mage audit becomes scrum-master
// backlog check in a repository with no magefiles, which could not run it
// (srd013 R5.3).
func AuditFor(p Policy, dir string) string {
	if p.AuditCommand == Defaults().AuditCommand {
		if _, err := os.Stat(filepath.Join(dir, "magefiles")); err != nil {
			return BacklogAudit
		}
	}
	return p.AuditCommand
}

// Defaults are the values srd001 fixes and the example policy uses.
func Defaults() Policy {
	return Policy{
		Planes:             []string{"management", "control", "data"},
		Types:              []string{"tracking", "sprint", "recurring"},
		Kinds:              []string{"build", "experiment", "research", "spec", "review", "consume", "handoff", "release", "defect", "request", "question"},
		RequireAC:          []string{"build", "experiment", "research", "spec"},
		RequireMilestone:   true,
		TriageLabel:        "needs-triage",
		AuditCommand:       "mage audit",
		Releases:           Releases{Scheme: "semver", Breaking: "major-and-minor-before-1.0", TagFormat: "{artifact}/v{version}"},
		DefinitionOfDone:   map[string][]Check{"default": {{Kind: "tests", Environment: "local", Suite: "unit", Command: "go test ./..."}}},
		Escalation:         Escalation{BlockerAgeDays: 14, WaitingChainLength: 3, SlipReleases: 2, EvidenceAgeDays: 7},
		ArchitectureChecks: ArchitectureWarn,
	}
}

// The values of a policy's architecture_checks (srd045 R6.3).
const (
	ArchitectureWarn   = "warn"
	ArchitectureRefuse = "refuse"
)

// ArchitectureChecksIn is the architecture_checks of the policy document
// data, the default when it names none. The corpus gate reads the policy
// from disk this way, since it opens no tracker (srd045 R8.3).
func ArchitectureChecksIn(data []byte) (string, error) {
	var f policyFile
	if err := corpus.DecodeConfig(data, &f); err != nil {
		return "", err
	}
	switch f.ArchitectureChecks {
	case "":
		return ArchitectureWarn, nil
	case ArchitectureWarn, ArchitectureRefuse:
		return f.ArchitectureChecks, nil
	}
	return "", fmt.Errorf("architecture_checks %q is neither %s nor %s", f.ArchitectureChecks, ArchitectureWarn, ArchitectureRefuse)
}

// TagFormatIn is the releases.tag_format of the policy document data, the
// default when it names none; the corpus gate reads which tags are releases
// this way (GH-88).
func TagFormatIn(data []byte) (string, error) {
	var f policyFile
	if err := corpus.DecodeConfig(data, &f); err != nil {
		return "", err
	}
	if f.Releases.TagFormat == "" {
		return Defaults().Releases.TagFormat, nil
	}
	return f.Releases.TagFormat, nil
}

// The control files of a program repository, config documents (design D11;
// config formats policy and labels).
const (
	PolicyFile = "control/policy.md"
	LabelsFile = "control/labels.md"
)

type policyFile struct {
	Admission struct {
		RequireMilestone          *bool    `yaml:"require_milestone"`
		RequireAcceptanceCriteria *bool    `yaml:"require_acceptance_criteria"`
		RequireApproval           *bool    `yaml:"require_approval"`
		ManagementFilers          any      `yaml:"management_filers"`
		AuditCommand              string   `yaml:"audit_command"`
		Rules                     []string `yaml:"rules"`
	} `yaml:"admission"`
	Releases           Releases           `yaml:"releases"`
	ArchitectureChecks string             `yaml:"architecture_checks"`
	Escalation         Escalation         `yaml:"escalation"`
	DefinitionOfDone   map[string][]Check `yaml:"definition_of_done"`
}

type labelsFile struct {
	Axes map[string]struct {
		Values []struct {
			Name string `yaml:"name"`
		} `yaml:"values"`
	} `yaml:"axes"`
}

// LoadPolicy reads the control files of the program repository at ref and
// falls back to Defaults for each file that is absent.
func LoadPolicy(t tracker.Tracker, project, ref string) (Policy, error) {
	p := Defaults()
	if project == "" {
		return p, nil
	}
	if b, err := t.GetFile(project, PolicyFile, ref); err == nil {
		var f policyFile
		if err := corpus.DecodeConfig(b, &f); err != nil {
			return p, fmt.Errorf("%s %s: %w", project, PolicyFile, err)
		}
		if f.Admission.RequireMilestone != nil {
			p.RequireMilestone = *f.Admission.RequireMilestone
		}
		if f.Admission.RequireApproval != nil {
			p.RequireApproval = *f.Admission.RequireApproval
		}
		if f.Admission.RequireAcceptanceCriteria != nil && !*f.Admission.RequireAcceptanceCriteria {
			p.RequireAC = nil
		}
		if f.Admission.AuditCommand != "" {
			p.AuditCommand = f.Admission.AuditCommand
		}
		if f.Admission.Rules != nil {
			p.AdmissionRules = f.Admission.Rules
		}
		if f.Releases.Scheme != "" {
			p.Releases.Scheme = f.Releases.Scheme
		}
		if f.Releases.Breaking != "" {
			p.Releases.Breaking = f.Releases.Breaking
		}
		if f.Releases.TagFormat != "" {
			p.Releases.TagFormat = f.Releases.TagFormat
		}
		for _, v := range []struct{ from, to *int }{
			{&f.Escalation.BlockerAgeDays, &p.Escalation.BlockerAgeDays},
			{&f.Escalation.WaitingChainLength, &p.Escalation.WaitingChainLength},
			{&f.Escalation.SlipReleases, &p.Escalation.SlipReleases},
			{&f.Escalation.EvidenceAgeDays, &p.Escalation.EvidenceAgeDays},
		} {
			if *v.from > 0 {
				*v.to = *v.from
			}
		}
		if len(f.DefinitionOfDone) > 0 {
			p.DefinitionOfDone = f.DefinitionOfDone
		}
		if f.ArchitectureChecks != "" {
			p.ArchitectureChecks = f.ArchitectureChecks
		}
		switch v := f.Admission.ManagementFilers.(type) {
		case []any:
			for _, u := range v {
				p.ManagementFilers = append(p.ManagementFilers, fmt.Sprint(u))
			}
		case string:
			p.ManagementFilers = []string{v}
		}
	} else if !errors.Is(err, tracker.ErrNotFound) {
		return p, err
	}
	if b, err := t.GetFile(project, LabelsFile, ref); err == nil {
		var f labelsFile
		if err := corpus.DecodeConfig(b, &f); err != nil {
			return p, fmt.Errorf("%s %s: %w", project, LabelsFile, err)
		}
		pick := func(axis string, into *[]string) {
			if a, ok := f.Axes[axis]; ok && len(a.Values) > 0 {
				*into = nil
				for _, v := range a.Values {
					*into = append(*into, v.Name)
				}
			}
		}
		pick("plane", &p.Planes)
		pick("type", &p.Types)
		p.Types = canonicalTypes(p.Types)
		pick("kind", &p.Kinds)
	} else if !errors.Is(err, tracker.ErrNotFound) {
		return p, err
	}
	subs, err := LoadSubsystems(t, project, ref)
	if err != nil && !errors.Is(err, tracker.ErrNotFound) {
		return p, fmt.Errorf("%s subsystem SRDs: %w", project, err)
	}
	p.Components, p.ComponentsRead = subs.Components(), true
	return p, nil
}
