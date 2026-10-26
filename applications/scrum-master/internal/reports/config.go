// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
package reports

import (
	"errors"
	"fmt"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/corpus"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// ConfigFile is the reports repository's configuration (config format
// reports-config).
const ConfigFile = "reports.md"

// Member is one person who reports.
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
}

// Default values of reports.md.
const (
	DefaultReminderWeekday  = "wednesday"
	DefaultStatusIssueTitle = "Recurring: weekly status {week}"
)

func (c *Config) defaults() {
	if c.ReminderWeekday == "" {
		c.ReminderWeekday = DefaultReminderWeekday
	}
	c.ReminderWeekday = strings.ToLower(c.ReminderWeekday)
	if c.StatusIssueTitle == "" {
		c.StatusIssueTitle = DefaultStatusIssueTitle
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

// Title is the status issue's title for a week.
func (c Config) Title(week string) string {
	return strings.ReplaceAll(c.StatusIssueTitle, "{week}", week)
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
