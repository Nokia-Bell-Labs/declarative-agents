// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
package beads

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

// ConfigFile is where the tracker repository holds its settings: the issue
// ID prefix, one per program, set when scrum-master init seeds the tracker (srd042).
const ConfigFile = ".beads/config.yaml"

// DefaultPrefix names the issues of a tracker whose config file sets no
// prefix, as every tracker did before the prefix existed.
const DefaultPrefix = "sm"

// Config is the tracker repository's config file.
type Config struct {
	IssuePrefix string `yaml:"issue-prefix"`
	// Layout is how the tracker keeps its issues: LayoutFiles for one file
	// per issue, empty for bd's single JSONL file.
	Layout string `yaml:"layout,omitempty"`
}

var notPrefix = regexp.MustCompile(`[^a-z0-9-]+`)

// PrefixFor derives an issue prefix from a program group: its last path
// segment, lower-cased, keeping letters, digits, and hyphens.
func PrefixFor(group string) string {
	group = strings.TrimSuffix(group, "/")
	p := notPrefix.ReplaceAllString(strings.ToLower(group[strings.LastIndex(group, "/")+1:]), "")
	if p = strings.Trim(p, "-"); p == "" {
		return DefaultPrefix
	}
	return p
}

// WriteConfig writes c as the config file of the tracker clone at dir.
func WriteConfig(dir string, c Config) error {
	b, err := yaml.Marshal(c)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, ConfigFile), b, 0o644)
}

// prefix is the issue prefix the clone's config file sets, DefaultPrefix
// when it sets none.
func (s *Store) prefix() (string, error) {
	c, err := s.config()
	if err != nil {
		return "", err
	}
	if c.IssuePrefix == "" {
		return DefaultPrefix, nil
	}
	return c.IssuePrefix, nil
}

// config is the clone's config file; a tracker without one has the zero
// config.
func (s *Store) config() (Config, error) {
	var c Config
	b, err := os.ReadFile(filepath.Join(s.Dir, ConfigFile))
	if errors.Is(err, os.ErrNotExist) {
		return c, nil
	}
	if err != nil {
		return c, err
	}
	if err := yaml.Unmarshal(b, &c); err != nil {
		return c, fmt.Errorf("%s: %w", ConfigFile, err)
	}
	return c, nil
}
