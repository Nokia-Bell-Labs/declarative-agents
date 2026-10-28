// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
package scaffold

import (
	"errors"
	"fmt"
	"path"
	"strings"

	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/install"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/program"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/tracker"
)

// OnboardOptions are what scrum-master onboard is given.
type OnboardOptions struct {
	Dir     string
	Agents  []string
	Version string
	// System is the system repository the registration is filed in, read at
	// Ref; Subsystem is the subsystem SRD whose components table gains the
	// repository.
	System, Ref, Subsystem string
	// Repository is the joining repository's URL, Path the path of the
	// component in it.
	Repository, Path string
	// Role is what the repository is to its subsystem: a component's
	// repository (RoleComponent, the default), the team's scrum repository,
	// where it plans (RoleScrum; GH-2745), or an initiative's repository,
	// holding its epics and features (RoleInitiative; GH-2811).
	Role string
	// Initiative is the initiative document the repository holds the epics
	// and features of (in001), for RoleInitiative.
	Initiative string
	// Owner is the subsystem's owner and Components its components' names,
	// for an SRD the registration asks to create.
	Owner         string
	Components    []string
	Force, DryRun bool
}

// The roles a joining repository plays in its subsystem (GH-2745, GH-2811).
const (
	RoleComponent  = "component"
	RoleScrum      = "scrum"
	RoleInitiative = "initiative"
)

// ownerPlaceholder stands for an owner the run did not name, for the
// person merging the registration to replace.
const ownerPlaceholder = "<the team's Product Owner>"

// OnboardResult is what scrum-master onboard reports.
type OnboardResult struct {
	Dir    string           `json:"dir"`
	Files  []Result         `json:"files"`
	Skills []install.Result `json:"skills,omitempty"`
	// Exclude is what the install did to .git/info/exclude (GH-2781).
	Exclude      string       `json:"exclude,omitempty"`
	Registration Registration `json:"registration"`
	DryRun       bool         `json:"dry_run"`
}

// Registration is the issue onboard filed in the system repository, or why
// it filed none (srd043 R2.2, R2.3).
type Registration struct {
	Filed  bool                `json:"filed"`
	Issue  *program.FileResult `json:"issue,omitempty"`
	Reason string              `json:"reason,omitempty"`
}

// registrationMarker names the repository a registration issue registers, so
// a second onboard finds it.
func registrationMarker(repository string) string {
	return "<!-- scrum-master onboard registers " + program.ProjectPath(repository) + " -->"
}

// Onboard lays down a joining repository's corpus skeleton in o.Dir and
// files its registration in the system repository (srd043 R2): one issue
// asking the subsystem SRD's owner to list the repository, never a commit
// there.
func Onboard(t tracker.Tracker, o OnboardOptions) (OnboardResult, error) {
	r := OnboardResult{Dir: o.Dir, DryRun: o.DryRun}
	if err := requireRepository(o.Dir); err != nil {
		return r, err
	}
	if o.System == "" || o.Subsystem == "" || o.Repository == "" {
		return r, fmt.Errorf("onboard needs the system repository, the subsystem SRD, and the repository's URL")
	}
	if o.Path == "" {
		o.Path = "/"
	}
	switch o.Role {
	case "":
		o.Role = RoleComponent
	case RoleComponent, RoleScrum:
	case RoleInitiative:
		if o.Initiative == "" {
			return r, fmt.Errorf("onboard --role initiative needs --initiative, the initiative document the repository holds the epics and features of, such as in001-reliable-agents")
		}
	default:
		return r, fmt.Errorf("onboard's role is %s, %s, or %s, not %q", RoleComponent, RoleScrum, RoleInitiative, o.Role)
	}
	files, err := corpusFiles("subsystem", o.Dir)
	if err != nil {
		return r, err
	}
	if r.Files, err = write(o.Dir, files, o.Force, o.DryRun); err != nil {
		return r, err
	}
	if len(o.Agents) > 0 {
		skills, err := install.EmbeddedSkills()
		if err != nil {
			return r, err
		}
		results, err := install.Install(o.Dir, o.Agents, skills, o.Version, o.Force, o.DryRun)
		if err != nil {
			return r, err
		}
		r.Skills, r.Exclude = install.SplitExclude(results)
	}
	r.Registration, err = register(t, o)
	return r, err
}

func register(t tracker.Tracker, o OnboardOptions) (Registration, error) {
	subs, err := program.LoadSubsystems(t, o.System, o.Ref)
	if err != nil {
		return Registration{}, err
	}
	if _, sub, ok := subs.ComponentFor(o.Repository); ok {
		switch {
		case o.Role == RoleInitiative && sub.Initiative != o.Initiative:
			// The SRD lists the repository but does not name the initiative:
			// the registration asks for the context item alone (GH-2811).
		case sub.Scrum != "" && program.ProjectPath(sub.Scrum) == program.ProjectPath(o.Repository):
			return Registration{Reason: fmt.Sprintf("%s already names %s as its scrum repository", sub.ID, program.ProjectPath(o.Repository))}, nil
		case o.Role == RoleInitiative:
			return Registration{Reason: fmt.Sprintf("%s already names %s as %s's repository", sub.ID, program.ProjectPath(o.Repository), o.Initiative)}, nil
		default:
			return Registration{Reason: fmt.Sprintf("%s already lists %s", sub.ID, program.ProjectPath(o.Repository))}, nil
		}
	}
	srdPath := program.SRDDir + "/" + o.Subsystem + ".md"
	_, err = t.GetFile(o.System, srdPath, o.Ref)
	missing := errors.Is(err, tracker.ErrNotFound)
	if err != nil && !missing {
		return Registration{}, fmt.Errorf("%s %s: %w", o.System, srdPath, err)
	}
	marker := registrationMarker(o.Repository)
	open, err := t.ListIssues(o.System, tracker.IssueQuery{State: "opened"})
	if err != nil {
		return Registration{}, err
	}
	for _, i := range open {
		if strings.Contains(i.Description, marker) {
			return Registration{Reason: fmt.Sprintf("%s#%d already asks to register %s", o.System, i.IID, program.ProjectPath(o.Repository))}, nil
		}
	}
	policy, err := program.LoadPolicy(t, o.System, o.Ref)
	if err != nil {
		return Registration{}, err
	}
	component, _, _ := subs.ComponentFor(o.System)
	title, body := fmt.Sprintf("Register %s in %s", program.ProjectPath(o.Repository), o.Subsystem), registrationBody(o, policy.AuditCommand)
	if missing {
		title, body = fmt.Sprintf("Create %s with %s", o.Subsystem, program.ProjectPath(o.Repository)), creationBody(o, policy.AuditCommand)
	}
	in := tracker.NewIssue{
		Title:       title,
		Description: body + "\n\n" + marker + "\n",
		Labels:      append(program.Labels("management", "", "", component.Name), program.Op(program.OpOnboard)),
	}
	res, err := program.File(t, o.System, in, policy, o.DryRun)
	if err != nil {
		return Registration{}, err
	}
	res.Plane, res.Component = "management", component.Name
	return Registration{Filed: !o.DryRun, Issue: &res}, nil
}

// registrationBody asks the subsystem SRD's owner for the components-table
// row, or, for a scrum repository, the SRD's scrum_repository, in do-work's
// input format.
func registrationBody(o OnboardOptions, audit string) string {
	repo := program.ProjectPath(o.Repository)
	if o.Role == RoleInitiative {
		return fmt.Sprintf(`%[1]s joins the program as %[4]s's initiative repository: it holds the initiative's epics and features, which the site draws under %[4]s (design.md subsystem_srd, GH-2811).

## Requirements

1. In docs/specs/software-requirements/%[2]s.md, add the context item initiative: %[4]s, and, where the implementation targets table does not list it yet, the row | <component name> | %[3]s | / |.

## Acceptance Criteria

- %[2]s names %[4]s as its initiative and lists %[1]s, and the corpus audit passes (test: %[5]s; environment: local)`, repo, o.Subsystem, o.Repository, o.Initiative, audit)
	}
	if o.Role == RoleScrum {
		return fmt.Sprintf(`%[1]s joins the program as the scrum repository of %[2]s, where the team plans and journals (design.md subsystem_srd, GH-2738).

## Requirements

1. In docs/specs/software-requirements/%[2]s.md, add the context item scrum_repository: %[3]s.

## Acceptance Criteria

- %[2]s names %[1]s as its scrum repository, and the corpus audit passes (test: %[4]s; environment: local)`, repo, o.Subsystem, o.Repository, audit)
	}
	return fmt.Sprintf(`%[1]s joins the program and asks to be listed in %[2]s (design.md D7: membership is declared, not inferred).

## Requirements

1. In docs/specs/software-requirements/%[2]s.md, add the row | <component name> | %[3]s | %[4]s | to the implementation targets table, naming the component.

## Acceptance Criteria

- %[2]s lists %[1]s, and the corpus audit passes (test: %[5]s; environment: local)`, repo, o.Subsystem, o.Repository, o.Path, audit)
}

// creationBody asks for the subsystem SRD that does not exist yet, whole:
// the system's SRD template with its owner, the repository as its scrum
// repository or as its components' repository, and a row per component
// (GH-2745). A team with a scrum repository and no code repository yet
// declares its components on the scrum repository, until a code
// repository takes each row.
func creationBody(o OnboardOptions, audit string) string {
	repo := program.ProjectPath(o.Repository)
	return fmt.Sprintf(`%[1]s joins the program, and its subsystem %[2]s has no SRD yet (design.md D7: membership is declared, not inferred).

## Requirements

1. Create docs/specs/software-requirements/%[2]s.md as below, %[3]s, then write its problem, goals, requirements, and acceptance criteria.

`+"```markdown\n%[4]s```"+`

## Acceptance Criteria

- %[2]s exists, %[5]s, and the corpus audit passes (test: %[6]s; environment: local)`,
		repo, o.Subsystem, ownerNote(o), newSubsystemSRD(o), roleNote(o), audit)
}

func ownerNote(o OnboardOptions) string {
	if o.Owner == "" {
		return "naming its owner where it says " + ownerPlaceholder
	}
	return "owned by " + o.Owner
}

func roleNote(o OnboardOptions) string {
	switch o.Role {
	case RoleScrum:
		return "names " + program.ProjectPath(o.Repository) + " as its scrum repository"
	case RoleInitiative:
		return "names " + o.Initiative + " as its initiative, whose epics and features " + program.ProjectPath(o.Repository) + " holds"
	}
	return "lists " + program.ProjectPath(o.Repository)
}

// shortName is a subsystem SRD's name without its number: srd002-platform
// is platform.
func shortName(subsystem string) string {
	if _, rest, ok := strings.Cut(subsystem, "-"); ok && strings.HasPrefix(subsystem, "srd") {
		return rest
	}
	return subsystem
}

// newSubsystemSRD is a new subsystem SRD from the system SRD's template:
// its id, its owner, its scrum repository for a scrum, and its components,
// each in the joining repository at its path.
func newSubsystemSRD(o OnboardOptions) string {
	b, err := templates.ReadFile(path.Join("templates", "system", "docs", "specs", "software-requirements", "srd001-{{level}}.md.tmpl"))
	if err != nil {
		return ""
	}
	short := shortName(o.Subsystem)
	owner := o.Owner
	if owner == "" {
		owner = ownerPlaceholder
	}
	components := o.Components
	if len(components) == 0 {
		components = []string{short}
	}
	targets := "<!-- field: implementation_targets -->\n## Implementation targets\n\n| name | repository | path |\n| --- | --- | --- |\n"
	for _, c := range components {
		targets += fmt.Sprintf("| %s | %s | %s |\n", c, o.Repository, o.Path)
	}
	context := "- owner: " + owner
	switch o.Role {
	case RoleScrum:
		context += "\n- scrum_repository: " + o.Repository
	case RoleInitiative:
		context += "\n- initiative: " + o.Initiative
	}
	return strings.NewReplacer("srd001-{{level}}", o.Subsystem, "{{Level}}", strings.ToUpper(short[:1])+short[1:], "{{level}}", short,
		"- owner: {{Owner}}", context, "{{Owner}}", owner, "{{implementation_targets}}", targets).Replace(string(b))
}
