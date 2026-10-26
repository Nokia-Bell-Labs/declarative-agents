// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
// Package cli is scrum-master's command line: subcommand groups registered by
// packages, each subcommand with --json and --dry-run, none reading stdin or
// prompting (srd010 R1, srd013 R2.1).
package cli

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/tracker"
)

// Command is one subcommand of one group. A command with an empty Name is
// the group's own command: it runs when no subcommand follows the group.
type Command struct {
	Group   string
	Name    string
	Summary string
	// Flags declares the subcommand's own flags; --json and --dry-run are
	// added for every subcommand.
	Flags func(fs *flag.FlagSet)
	Run   func(ctx *Context) error
}

// Context is what a subcommand runs with.
type Context struct {
	JSON   bool
	DryRun bool
	Flags  *flag.FlagSet
	Out    io.Writer
	// In is the command's input, empty when the caller gives none (GH-59).
	In      io.Reader
	Tracker func() (tracker.Tracker, error)
	// used is the tracker the command opened, so Emit can name what it read.
	used tracker.Tracker
}

// Emit writes v as JSON with --json, else the human text. When the command's
// tracker knows the tracker commit it read, the output names it: as
// tracker_read in a JSON object, and as a last line of the human text.
func (c *Context) Emit(v any, human string) error {
	read, ok := c.read()
	if c.JSON {
		if ok {
			v = withRead(v, read)
		}
		enc := json.NewEncoder(c.Out)
		enc.SetIndent("", "  ")
		// Output is read by programs and people, never embedded in HTML.
		enc.SetEscapeHTML(false)
		return enc.Encode(v)
	}
	human = strings.TrimRight(human, "\n")
	if ok {
		human += "\n" + readLine(read)
	}
	_, err := fmt.Fprintln(c.Out, human)
	return err
}

func (c *Context) read() (tracker.Read, bool) {
	if r, isReader := c.used.(tracker.Reader); isReader {
		return r.LastRead()
	}
	return tracker.Read{}, false
}

// withRead adds tracker_read to v when v encodes as a JSON object; any
// other shape is left as it is, so no output changes type.
func withRead(v any, read tracker.Read) any {
	b, err := json.Marshal(v)
	if err != nil {
		return v
	}
	var obj map[string]json.RawMessage
	if json.Unmarshal(b, &obj) != nil || obj == nil {
		return v
	}
	rb, _ := json.Marshal(read)
	obj["tracker_read"] = rb
	return obj
}

func readLine(r tracker.Read) string {
	commit := r.Commit
	if len(commit) > 12 {
		commit = commit[:12]
	}
	if r.Offline {
		return fmt.Sprintf("tracker %s, offline: read the cache last pulled at %s", commit, r.At)
	}
	return "tracker " + commit
}

// Registry holds the groups and their commands.
type Registry struct {
	commands map[string]map[string]*Command
	// retired maps a group's former name to its current one.
	retired map[string]string
}

// Default is the registry cmd/tpm runs; packages register into it in init.
var Default = &Registry{}

// Register adds a command. A duplicate group and name panics at start-up.
func (r *Registry) Register(c *Command) {
	if r.commands == nil {
		r.commands = map[string]map[string]*Command{}
	}
	if r.commands[c.Group] == nil {
		r.commands[c.Group] = map[string]*Command{}
	}
	if _, dup := r.commands[c.Group][c.Name]; dup {
		panic("scrum-master: duplicate command " + c.Group + " " + c.Name)
	}
	r.commands[c.Group][c.Name] = c
}

// Retire makes old a former name of the group now: running it runs nothing
// and fails naming the group that replaced it, so a skill written for an
// earlier release says how to fix itself (srd013 R2.2).
func (r *Registry) Retire(old, now string) {
	if r.retired == nil {
		r.retired = map[string]string{}
	}
	r.retired[old] = now
}

// Groups names every registered group, sorted; former names are not groups.
func (r *Registry) Groups() []string { return sortedKeys(r.commands) }

// Commands returns a group's commands sorted by name, the group's own
// command, if it has one, first.
func (r *Registry) Commands(group string) []*Command {
	var cs []*Command
	for _, n := range sortedKeys(r.commands[group]) {
		cs = append(cs, r.commands[group][n])
	}
	return cs
}

// ErrUsage marks a usage error; cmd/tpm exits 2 for it.
var ErrUsage = errors.New("usage")

// Run parses args (without the program name) and runs one subcommand.
func (r *Registry) Run(args []string, out io.Writer, open func() (tracker.Tracker, error)) error {
	return r.RunIn(args, nil, out, open)
}

// RunIn is Run with the command's input; nil is empty input.
func (r *Registry) RunIn(args []string, in io.Reader, out io.Writer, open func() (tracker.Tracker, error)) error {
	if in == nil {
		in = strings.NewReader("")
	}
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" {
		fmt.Fprint(out, r.usage())
		if len(args) == 0 {
			return ErrUsage
		}
		return nil
	}
	if now, ok := r.retired[args[0]]; ok {
		sub := ""
		if len(args) > 1 && !strings.HasPrefix(args[1], "-") {
			sub = " " + args[1]
		}
		return fmt.Errorf("scrum-master %s is now scrum-master %s: run scrum-master %s%s; a skill that called scrum-master %s was installed by an earlier release, so run the upgrade profile to update it",
			args[0], now, now, sub, args[0])
	}
	group, ok := r.commands[args[0]]
	if !ok {
		return fmt.Errorf("%w: unknown group %q\n\n%s", ErrUsage, args[0], r.usage())
	}
	cmd, rest := group[""], args[1:]
	if cmd == nil || (len(rest) > 0 && !strings.HasPrefix(rest[0], "-")) {
		if len(args) == 1 || args[1] == "-h" || args[1] == "--help" {
			fmt.Fprint(out, r.groupUsage(args[0]))
			if len(args) == 1 {
				return ErrUsage
			}
			return nil
		}
		if cmd, ok = group[args[1]]; !ok {
			return fmt.Errorf("%w: unknown subcommand %q\n\n%s", ErrUsage, args[1], r.groupUsage(args[0]))
		}
		rest = args[2:]
	}
	fs := flag.NewFlagSet(strings.TrimSpace("scrum-master "+cmd.Group+" "+cmd.Name), flag.ContinueOnError)
	fs.SetOutput(out)
	ctx := &Context{Flags: fs, Out: out, In: in}
	ctx.Tracker = func() (tracker.Tracker, error) {
		if ctx.used != nil {
			return ctx.used, nil
		}
		t, err := open()
		if err == nil {
			ctx.used = t
		}
		return t, err
	}
	fs.BoolVar(&ctx.JSON, "json", false, "write machine-readable JSON to stdout")
	fs.BoolVar(&ctx.DryRun, "dry-run", false, "report what would change and change nothing")
	if cmd.Flags != nil {
		cmd.Flags(fs)
	}
	if err := fs.Parse(rest); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return fmt.Errorf("%w: %v", ErrUsage, err)
	}
	return cmd.Run(ctx)
}

func (r *Registry) usage() string {
	var b strings.Builder
	b.WriteString("usage: scrum-master <group> <subcommand> [--json] [--dry-run] [flags]\n\ngroups:\n")
	for _, g := range sortedKeys(r.commands) {
		if c := r.commands[g][""]; c != nil && len(r.commands[g]) == 1 {
			fmt.Fprintf(&b, "  %-20s %s\n", g, c.Summary)
			continue
		}
		fmt.Fprintf(&b, "  %s\n", g)
	}
	if _, ok := r.commands["help"]; ok {
		b.WriteString("\nrun scrum-master help for the process, scrum-master help <group> for one group\n")
	}
	return b.String()
}

func (r *Registry) groupUsage(group string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "usage: scrum-master %s <subcommand> [--json] [--dry-run] [flags]\n\nsubcommands:\n", group)
	for _, n := range sortedKeys(r.commands[group]) {
		if n == "" {
			continue
		}
		fmt.Fprintf(&b, "  %-20s %s\n", n, r.commands[group][n].Summary)
	}
	return b.String()
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// Strings is a flag that repeats: each use adds one value.
type Strings []string

func (s *Strings) String() string { return strings.Join(*s, ",") }

// Set adds a value.
func (s *Strings) Set(v string) error {
	*s = append(*s, v)
	return nil
}
