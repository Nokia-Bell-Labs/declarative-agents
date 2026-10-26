// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
package cli

import (
	"bytes"
	"errors"
	"flag"
	"strings"
	"testing"

	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/tracker"
)

func registry(got *Context, name *string) *Registry {
	r := &Registry{}
	r.Register(&Command{Group: "program", Name: "file", Summary: "file an issue",
		Flags: func(fs *flag.FlagSet) { fs.StringVar(name, "title", "", "title") },
		Run: func(c *Context) error {
			*got = *c
			return c.Emit(map[string]string{"title": *name}, "filed "+*name)
		}})
	return r
}

func noTracker() (tracker.Tracker, error) { return nil, errors.New("no tracker in tests") }

func TestJSONAndDryRunReachTheSubcommand(t *testing.T) {
	var ctx Context
	var title string
	var out bytes.Buffer
	if err := registry(&ctx, &title).Run([]string{"program", "file", "--json", "--dry-run", "--title", "x"}, &out, noTracker); err != nil {
		t.Fatal(err)
	}
	if !ctx.JSON || !ctx.DryRun || title != "x" || strings.TrimSpace(out.String()) != "{\n  \"title\": \"x\"\n}" {
		t.Fatalf("ctx=%+v out=%q", ctx, out.String())
	}
}

func TestHumanOutputWithoutJSON(t *testing.T) {
	var ctx Context
	var title string
	var out bytes.Buffer
	if err := registry(&ctx, &title).Run([]string{"program", "file", "--title", "y"}, &out, noTracker); err != nil {
		t.Fatal(err)
	}
	if ctx.JSON || ctx.DryRun || out.String() != "filed y\n" {
		t.Fatalf("out=%q", out.String())
	}
}

func TestUsageListsGroupsAndSubcommands(t *testing.T) {
	var ctx Context
	var title string
	r := registry(&ctx, &title)
	var out bytes.Buffer
	if err := r.Run([]string{"--help"}, &out, noTracker); err != nil || !strings.Contains(out.String(), "  program") {
		t.Fatalf("err=%v out=%q", err, out.String())
	}
	out.Reset()
	if err := r.Run([]string{"program", "--help"}, &out, noTracker); err != nil || !strings.Contains(out.String(), "file                 file an issue") {
		t.Fatalf("err=%v out=%q", err, out.String())
	}
	for _, args := range [][]string{nil, {"nope"}, {"program", "nope"}, {"program", "file", "--bogus"}} {
		if err := r.Run(args, &bytes.Buffer{}, noTracker); !errors.Is(err, ErrUsage) {
			t.Errorf("%v: err = %v, want usage", args, err)
		}
	}
}

func TestGroupCommandRunsWithoutSubcommand(t *testing.T) {
	r := &Registry{}
	var force bool
	ran := 0
	r.Register(&Command{Group: "install", Summary: "install skills",
		Flags: func(fs *flag.FlagSet) { fs.BoolVar(&force, "force", false, "force") },
		Run:   func(*Context) error { ran++; return nil }})
	var out bytes.Buffer
	for _, args := range [][]string{{"install"}, {"install", "--force"}} {
		if err := r.Run(args, &out, noTracker); err != nil {
			t.Fatalf("%v: %v", args, err)
		}
	}
	if ran != 2 || !force {
		t.Fatalf("ran=%d force=%v", ran, force)
	}
	if err := r.Run([]string{"install", "extra"}, &out, noTracker); !errors.Is(err, ErrUsage) {
		t.Fatalf("a subcommand the group lacks is a usage error, got %v", err)
	}
	out.Reset()
	if err := r.Run([]string{"--help"}, &out, noTracker); err != nil || !strings.Contains(out.String(), "install              install skills") {
		t.Fatalf("err=%v out=%q", err, out.String())
	}
}

func TestRetiredGroupRunsNothingAndNamesItsReplacement(t *testing.T) {
	var ctx Context
	var title string
	r := registry(&ctx, &title)
	r.Retire("old", "program")
	var out bytes.Buffer
	err := r.Run([]string{"old", "file", "--title", "x"}, &out, noTracker)
	if err == nil || !strings.Contains(err.Error(), "scrum-master old is now scrum-master program: run scrum-master program file") || title != "" {
		t.Fatalf("retired group: err %v, ran with title %q", err, title)
	}
	if strings.Contains(r.usage(), "old") {
		t.Fatalf("usage lists a retired group:\n%s", r.usage())
	}
}
