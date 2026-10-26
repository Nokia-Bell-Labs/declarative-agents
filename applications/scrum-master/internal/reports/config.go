// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
package reports

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/corpus"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/tracker"
)

// ConfigFile is the reports repository's configuration (config format
// reports-config), read where no system repository holds ControlFile.
const ConfigFile = "reports.md"

// ReportsEnv names a central reports repository's checkout where a word
// is given none, as the scheduled profiles run.
const ReportsEnv = "SCRUM_MASTER_REPORTS"

// ControlFile is the roster in the system repository: who reports, in which
// repository, and when; the technical program manager owns it, and a
// report is written where its author worked, never in a central
// repository (GH-2725).
const ControlFile = "control/reports.md"

// Member is one person who reports, and the repository they report in.
type Member struct {
	Username string `yaml:"username" json:"username"`
	Project  string `yaml:"project" json:"project"`
}

// Config is reports.md, a config document (design D11), with its defaults
// applied.
type Config struct {
	Team             []Member `yaml:"team" json:"team"`
	ReminderWeekday  string   `yaml:"reminder_weekday" json:"reminder_weekday"`
	StatusIssueTitle string   `yaml:"status_issue_title" json:"status_issue_title"`
	System           string   `yaml:"system" json:"system"`
	// The rollups' issue titles, {week} and {subsystem} replaced, and the
	// person the program rollup issue is for; empty is whoever files it.
	TeamRollupTitle    string `yaml:"team_rollup_title,omitempty" json:"team_rollup_title,omitempty"`
	ProgramRollupTitle string `yaml:"program_rollup_title,omitempty" json:"program_rollup_title,omitempty"`
	ProgramLead        string `yaml:"program_lead,omitempty" json:"program_lead,omitempty"`
	// The due offsets: a report is due ReportDueDays days after the
	// reminder weekday, a team rollup TeamRollupDueDays working days after
	// that, and the program rollup ProgramRollupDueDays working days after
	// the team's; each is at least one.
	ReportDueDays        int `yaml:"report_due_days,omitempty" json:"report_due_days,omitempty"`
	TeamRollupDueDays    int `yaml:"team_rollup_due_days,omitempty" json:"team_rollup_due_days,omitempty"`
	ProgramRollupDueDays int `yaml:"program_rollup_due_days,omitempty" json:"program_rollup_due_days,omitempty"`
	// Reports names a central reports repository whose reports collect
	// still reads, so a program's history stays readable for the release
	// that moves its reports into place (GH-2725).
	Reports string `yaml:"reports,omitempty" json:"reports,omitempty"`
}

// Default values of reports.md.
const (
	DefaultReminderWeekday    = "wednesday"
	DefaultStatusIssueTitle   = "Recurring: weekly status {week}"
	DefaultTeamRollupTitle    = "Recurring: team rollup {subsystem} {week}"
	DefaultProgramRollupTitle = "Recurring: program rollup {week}"
)

func (c *Config) defaults() {
	if c.ReminderWeekday == "" {
		c.ReminderWeekday = DefaultReminderWeekday
	}
	c.ReminderWeekday = strings.ToLower(c.ReminderWeekday)
	if c.StatusIssueTitle == "" {
		c.StatusIssueTitle = DefaultStatusIssueTitle
	}
	if c.TeamRollupTitle == "" {
		c.TeamRollupTitle = DefaultTeamRollupTitle
	}
	if c.ProgramRollupTitle == "" {
		c.ProgramRollupTitle = DefaultProgramRollupTitle
	}
	for _, d := range []*int{&c.ReportDueDays, &c.TeamRollupDueDays, &c.ProgramRollupDueDays} {
		if *d < 1 {
			*d = 1
		}
	}
}

// ParseConfig reads reports.md content.
func ParseConfig(b []byte) (Config, error) {
	var c Config
	if err := corpus.DecodeConfig(b, &c); err != nil {
		return c, err
	}
	c.defaults()
	if _, err := Weekday(c.ReminderWeekday); err != nil {
		return c, err
	}
	return c, nil
}

// LoadConfig reads reports.md from a reports checkout.
func LoadConfig(dir string) (Config, error) {
	b, err := os.ReadFile(filepath.Join(dir, ConfigFile))
	if errors.Is(err, fs.ErrNotExist) {
		return Config{}, errors.New(dir + " has no " + ConfigFile + "; run scrum-master reports init")
	}
	if err != nil {
		return Config{}, err
	}
	return ParseConfig(b)
}

// LoadControl reads the roster from the system repository at ref, with
// the system named; tracker.ErrNotFound when the program keeps no roster
// there yet.
func LoadControl(t tracker.Forge, system, ref string) (Config, error) {
	b, err := t.GetFile(system, ControlFile, ref)
	if err != nil {
		return Config{}, err
	}
	c, err := ParseConfig(b)
	if err != nil {
		return c, fmt.Errorf("%s: %w", ControlFile, err)
	}
	c.System = system
	return c, nil
}

// Title is the status issue's title for a week.
func (c Config) Title(week string) string {
	return strings.ReplaceAll(c.StatusIssueTitle, "{week}", week)
}

// RollupTitle is a rollup issue's title for a week: the team's for a
// subsystem, the program's for none.
func (c Config) RollupTitle(subsystem, week string) string {
	t := c.ProgramRollupTitle
	if subsystem != "" {
		t = strings.ReplaceAll(c.TeamRollupTitle, "{subsystem}", subsystem)
	}
	return strings.ReplaceAll(t, "{week}", week)
}

// Weekday reads a weekday name.
func Weekday(name string) (time.Weekday, error) {
	for d := time.Sunday; d <= time.Saturday; d++ {
		if strings.EqualFold(d.String(), name) {
			return d, nil
		}
	}
	return 0, fmt.Errorf("reminder_weekday %q is not a weekday", name)
}

// RosterDocument is cfg as a roster document (config format
// reports-config), its defaults written out, for control/reports.md.
func RosterDocument(cfg Config) ([]byte, error) {
	cfg.defaults()
	return corpus.FormatConfig("reports", "Reports configuration", cfg)
}
