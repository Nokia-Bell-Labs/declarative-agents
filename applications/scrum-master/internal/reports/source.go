// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
package reports

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/tracker"
)

// Source is where a word reads a program's weekly reports (GH-2775): in
// place, collected from the repositories the roster in the system
// repository names, or a central reports repository's checkout. Every
// word that reads reports chooses it by ChooseSource and says why.
type Source struct {
	InPlace bool
	System  string
	// Central is the central reports checkout the word was given, read
	// when the source is not in place.
	Central string
	Config  Config
	Reason  string
}

// IsCentral reports whether dir is a central reports repository's checkout:
// it holds ConfigFile.
func IsCentral(dir string) bool {
	if dir == "" {
		return false
	}
	_, err := os.Stat(filepath.Join(dir, ConfigFile))
	return err == nil
}

// ChooseSource is the one rule every word that reads reports follows: in
// place when the system repository's roster names a reporter, else the
// central reports checkout when the word was given one, else in place with
// the roster as it stands. init writes the roster into every system
// repository, so its existence alone says nothing; its reporters do.
func ChooseSource(t tracker.Forge, system, ref, central string) (Source, error) {
	// A checkout without reports.md, such as a reports repository that is
	// the site's home only, is no central one: it is named as not read,
	// never dropped in silence (GH-2791).
	given := central
	if !IsCentral(central) {
		central = ""
	}
	notRead := ""
	if given != "" && central == "" {
		notRead = fmt.Sprintf("; %s holds no %s, so it is no central reports checkout and is not read", given, ConfigFile)
	}
	if system != "" {
		cfg, err := LoadControl(t, system, ref)
		switch {
		case err == nil && len(cfg.Team) > 0:
			s := Source{InPlace: true, System: system, Central: central, Config: cfg,
				Reason: fmt.Sprintf("in place: the roster in %s %s names %d reporter(s)", system, ControlFile, len(cfg.Team))}
			if central != "" {
				s.Reason += fmt.Sprintf("; the central reports at %s are not read", central)
			}
			s.Reason += notRead
			return s, nil
		case err != nil && !errors.Is(err, tracker.ErrNotFound):
			return Source{}, err
		}
		if central == "" {
			if errors.Is(err, tracker.ErrNotFound) {
				cfg = Config{System: system}
				cfg.defaults()
			}
			return Source{InPlace: true, System: system, Config: cfg,
				Reason: fmt.Sprintf("in place: the roster in %s %s names no reporter, and no central reports checkout was given", system, ControlFile) + notRead}, nil
		}
	}
	if central == "" {
		return Source{}, fmt.Errorf("no reports to read: name the system repository holding %s, or a central reports checkout holding %s", ControlFile, ConfigFile)
	}
	cfg, err := LoadConfig(central)
	if err != nil {
		return Source{}, err
	}
	reason := "central: the reports at " + central
	if system != "" {
		reason = fmt.Sprintf("central: the roster in %s %s names no reporter, so the reports at %s", system, ControlFile, central)
	}
	return Source{System: system, Central: central, Config: cfg, Reason: reason}, nil
}

// Explain records the source on a, and, when the source in place held no
// report for the period while the central checkout the word was given
// holds some, says so rather than leaving every reporter missing.
func (s Source) Explain(t tracker.Tracker, a *Aggregation, in AggregateInput) {
	a.Source = s.Reason
	if !s.InPlace || s.Central == "" || a.Reports > 0 {
		return
	}
	cfg, err := LoadConfig(s.Central)
	if err != nil {
		return
	}
	in.Dir, in.Config, in.Collection = s.Central, cfg, nil
	other, err := Aggregate(t, in)
	if err != nil || other.Reports == 0 {
		return
	}
	a.Source += fmt.Sprintf("; no report was written in place for %s to %s, but the central reports at %s hold %d: move them into place, or empty the roster to read them there", a.From, a.To, s.Central, other.Reports)
}
