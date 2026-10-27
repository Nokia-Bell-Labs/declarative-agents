// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
// Package spectool builds and runs the spec tool: the pinned parent
// release's own Markdown library, compiled from that release's source, which
// converts, formats, and checks a corpus's documents (srd040 R1). mage
// convert, fmt, and specCheck run it in this repository, and scrum-master doctor runs
// it in a program repository at that repository's pin (srd043 R4.4). The
// library is built at run time, so go.mod depends on no parent release.
package spectool

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// ReleaseURL is where the parent publishes each tag's binaries and source;
// tests point it at a local server.
var ReleaseURL = "https://github.com/Nokia-Bell-Labs/declarative-agents"

// ParentRepository is the parent a corpus's docs/external-pins.yaml pins.
const ParentRepository = "github.com/Nokia-Bell-Labs/declarative-agents"

// PinsFile is where a corpus pins its parent release; CheckoutsFile, which
// git ignores, names a local checkout of the parent to use instead.
const (
	PinsFile      = "docs/external-pins.yaml"
	CheckoutsFile = "docs/external-checkouts.yaml"
)

// Checkout is the local checkout of the parent that root's checkouts file
// names, relative paths read from root; ok is false when there is none.
func Checkout(root string) (dir string, ok bool, err error) {
	data, err := os.ReadFile(filepath.Join(root, CheckoutsFile))
	if errors.Is(err, os.ErrNotExist) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	var f struct {
		Checkouts map[string]string `yaml:"checkouts"`
	}
	if err := yaml.Unmarshal(data, &f); err != nil {
		return "", false, fmt.Errorf("%s: %w", CheckoutsFile, err)
	}
	dir = f.Checkouts[ParentRepository]
	if dir == "" {
		return "", false, nil
	}
	if !filepath.IsAbs(dir) {
		dir = filepath.Join(root, dir)
	}
	return dir, true, nil
}

// SourceURL is where root's pins file says the parent publishes its tags'
// source, ReleaseURL when it names none.
func SourceURL(root string) string {
	data, err := os.ReadFile(filepath.Join(root, PinsFile))
	if err != nil {
		return ReleaseURL
	}
	var pins struct {
		Sources map[string]string `yaml:"sources"`
	}
	if yaml.Unmarshal(data, &pins) != nil || pins.Sources[ParentRepository] == "" {
		return ReleaseURL
	}
	return strings.TrimSuffix(pins.Sources[ParentRepository], "/")
}

// Runner runs name with args in dir.
type Runner func(dir, name string, args ...string) error

// Pinned reads the parent release the corpus at root pins.
func Pinned(root string) (string, error) {
	data, err := os.ReadFile(filepath.Join(root, PinsFile))
	if err != nil {
		return "", err
	}
	var pins struct {
		Pins map[string]string `yaml:"pins"`
	}
	if err := yaml.Unmarshal(data, &pins); err != nil {
		return "", fmt.Errorf("%s: %w", PinsFile, err)
	}
	tag := pins.Pins[ParentRepository]
	if tag == "" {
		return "", fmt.Errorf("%s pins no release of %s", PinsFile, ParentRepository)
	}
	return tag, nil
}

// Run runs the spec tool's command (convert, fmt, or check) over scope in
// the repository at dir, at the release dir pins: it lays out the release's
// source and builds the tool once per tag, then returns the lines the tool
// printed. A non-nil error with lines means the tool ran and reported
// problems. In an agent built with the words, the agent's own copy of the
// program runs in process instead, with no source laid out and no Go
// toolchain (GH-33).
func Run(dir, command, scope string) ([]string, error) {
	if InProcess != nil {
		return runInProcess(dir, command, scope)
	}
	tag, err := Pinned(dir)
	if err != nil {
		return nil, err
	}
	rel, err := ReleaseDir(tag)
	if err != nil {
		return nil, err
	}
	src := filepath.Join(rel, "src")
	if err := EnsureSource(src, tag); err != nil {
		return nil, err
	}
	tool, err := Ensure(rel, filepath.Join(src, "agent-core"), quiet)
	if err != nil {
		return nil, err
	}
	cmd := exec.Command(tool, command, scope)
	cmd.Dir = dir
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	return report(out, stderr.Bytes(), err)
}

// InProcess runs the program in this process — its command and scope as
// args — when an agent build linked it in (GH-33); nil otherwise.
var InProcess func(args []string, stdout, stderr io.Writer) int

// runInProcess runs InProcess over scope in the repository at dir, which
// the program reads as its working directory.
func runInProcess(dir, command, scope string) ([]string, error) {
	prev, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	if err := os.Chdir(dir); err != nil {
		return nil, err
	}
	defer os.Chdir(prev)
	var out, stderr bytes.Buffer
	if code := InProcess([]string{command, scope}, &out, &stderr); code != 0 {
		err = fmt.Errorf("exit status %d", code)
	}
	return report(out.Bytes(), stderr.Bytes(), err)
}

// report is a run's printed lines, and its failure in the tool's words.
func report(out, stderr []byte, err error) ([]string, error) {
	var lines []string
	for _, l := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if l != "" {
			lines = append(lines, l)
		}
	}
	if err != nil {
		if msg := strings.TrimPrefix(strings.TrimSpace(string(stderr)), "spectool: "); msg != "" {
			return lines, fmt.Errorf("%s", msg)
		}
		return lines, fmt.Errorf("run the spec tool: %w", err)
	}
	return lines, nil
}

// quiet runs name and keeps its output for the error.
func quiet(dir, name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("%s %s: %w: %s", name, strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return nil
}

// ReleaseDir is where a release is downloaded and verified, once per tag,
// under the user cache directory.
func ReleaseDir(tag string) (string, error) {
	base, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "scrum-master", "release", tag), nil
}

// Download fetches url.
func Download(url string) ([]byte, error) {
	client := &http.Client{Timeout: 5 * time.Minute}
	resp, err := client.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET %s: %s", url, resp.Status)
	}
	return io.ReadAll(resp.Body)
}

// EnsureSource extracts tag's source archive into src and commits it there
// as a repository tagged tag, so the audit resolves derived_from
// entries offline against it.
func EnsureSource(src, tag string) error { return EnsureSourceFrom(ReleaseURL, src, tag) }

// EnsureSourceFrom is EnsureSource from the forge at base.
func EnsureSourceFrom(base, src, tag string) error {
	if gitQuiet(src, "rev-parse", "--verify", "-q", "refs/tags/"+tag) == nil {
		return nil
	}
	archive, err := Download(base + "/archive/refs/tags/" + tag + ".tar.gz")
	if err != nil {
		return fmt.Errorf("the source of %s: %w", tag, err)
	}
	if err := os.RemoveAll(src); err != nil {
		return err
	}
	if err := os.MkdirAll(src, 0o755); err != nil {
		return err
	}
	untar := exec.Command("tar", "-xzf", "-", "--strip-components=1")
	untar.Dir, untar.Stdin = src, bytes.NewReader(archive)
	if out, err := untar.CombinedOutput(); err != nil {
		return fmt.Errorf("extract the source of %s: %w: %s", tag, err, out)
	}
	if _, err := os.Stat(filepath.Join(src, "agent-core", "go.mod")); err != nil {
		return fmt.Errorf("the source of %s has no agent-core/go.mod: %w", tag, err)
	}
	for _, args := range [][]string{
		{"init", "-q", "-b", "main"},
		{"add", "-A"},
		{"-c", "user.name=scrum-master", "-c", "user.email=scrum-master@example.invalid", "commit", "-q", "-m", "release " + tag},
		{"tag", tag},
	} {
		if err := gitQuiet(src, args...); err != nil {
			return fmt.Errorf("lay out the source of %s as a repository: %w", tag, err)
		}
	}
	return nil
}

func gitQuiet(dir string, args ...string) error {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return nil
}

// Binary is where the spec tool built from this Program lives
// under a release's directory.
func Binary(dir string) string {
	sum := sha256.Sum256([]byte(Program("main", "")))
	return filepath.Join(dir, "spectool-built-"+hex.EncodeToString(sum[:6]))
}

// Ensure builds the spec tool against the release's agent-core
// source once per tag and per version of Main, so a change to the
// tool's own source is never served by a binary built before it.
func Ensure(releaseDir, agentCore string, run Runner) (string, error) {
	binary := Binary(releaseDir)
	if _, err := os.Stat(binary); err == nil {
		return binary, nil
	}
	build := filepath.Join(releaseDir, "spectool-src")
	if err := writeSource(build, agentCore); err != nil {
		return "", err
	}
	if err := run(build, "go", "build", "-mod=mod", "-o", binary, "."); err != nil {
		return "", fmt.Errorf("build the spec tool in %s: %w", releaseDir, err)
	}
	return binary, nil
}

// moduleOf reads the module path a go.mod declares. The parent's agent-core
// keeps the module path it was forked with, which differs from the fork's
// repository path, so it is read rather than named.
func moduleOf(gomod string) (string, error) {
	f, err := os.Open(gomod)
	if err != nil {
		return "", err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		if rest, ok := strings.CutPrefix(strings.TrimSpace(sc.Text()), "module "); ok {
			return strings.TrimSpace(rest), nil
		}
	}
	return "", fmt.Errorf("%s declares no module", gomod)
}

// writeSource writes the tool's module: its main and a go.mod that
// resolves the release's library module to its source, with the release's
// own checksums.
func writeSource(dir, releaseSource string) error {
	module, err := moduleOf(filepath.Join(releaseSource, "go.mod"))
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	mod := fmt.Sprintf("module scrum-master/spectool\n\ngo 1.26\n\nrequire %s v0.0.0\n\nreplace %s => %s\n",
		module, module, releaseSource)
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte(mod), 0o644); err != nil {
		return err
	}
	if sum, err := os.ReadFile(filepath.Join(releaseSource, "go.sum")); err == nil {
		if err := os.WriteFile(filepath.Join(dir, "go.sum"), sum, 0o644); err != nil {
			return err
		}
	}
	return os.WriteFile(filepath.Join(dir, "main.go"), []byte(Program("main", module)), 0o644)
}

// Program is the tool's source in package pkg, importing the release's
// library from module: it lists the tracked documents with git, picks each
// one's declaration from the release's built-in declarations, indexes the
// corpus so references link across documents, and converts, formats, or
// diagnoses each one. Built as main it is the spec tool; an agent built
// with the words links it as a package and runs the check in process
// (GH-33).
func Program(pkg, module string) string {
	src := "package " + pkg + "\n\n" + programImports + programBody
	if pkg == "main" {
		src += programMain
	}
	return strings.ReplaceAll(src, "RELEASE_MODULE", module)
}

const programImports = `import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"regexp"
	"strings"

	"RELEASE_MODULE/pkg/spec/markdown"
)

`

const programMain = `
func main() { os.Exit(Run(os.Args[1:], os.Stdout, os.Stderr)) }
`

const programBody = `// hierarchyPath names a hierarchy document (theme, initiative, epic,
// feature, story) or an objective document, scrum-master's types written as
// SRDs in the release's format; the release declares no location for them,
// so the tool reads them with the SRD declaration (design.md hierarchy).
var hierarchyPath = regexp.MustCompile("(^|/)(themes|initiatives|epics|features|stories|objectives)/([a-z]+/)?(th|in|ep|ft|st|obj)[0-9]{3}-[a-z0-9-]+\\.(yaml|md)$")

// hierarchyTypes names the scope of each hierarchy directory.
var hierarchyTypes = map[string]string{"themes": "theme", "initiatives": "initiative", "epics": "epic", "features": "feature", "stories": "story", "objectives": "objective"}

// declarationFor picks a document's declaration and the type its scope
// names: a hierarchy document's is the SRD's, under the scope of its kind.
func declarationFor(declarations markdown.Declarations, path string, source bool) (*markdown.Declaration, string, bool) {
	if m := hierarchyPath.FindStringSubmatch(path); m != nil {
		d, ok := declarations["srd"]
		return d, hierarchyTypes[m[2]], ok
	}
	var d *markdown.Declaration
	var ok bool
	if source {
		d, ok = declarations.ForSource(path)
	} else {
		d, ok = declarations.ForMarkdown(path)
	}
	if !ok {
		return nil, "", false
	}
	return d, d.Type(), true
}

// Run is the tool on one command line, its command and scope: findings go
// to stdout, a failure to stderr, and the exit code is returned.
func Run(args []string, stdout, stderr io.Writer) int {
	if len(args) != 2 {
		fmt.Fprintln(stderr, "usage: spectool convert|fmt|check <scope>")
		return 2
	}
	if err := run(args[0], args[1], stdout); err != nil {
		fmt.Fprintln(stderr, "spectool: "+err.Error())
		return 1
	}
	return 0
}

func run(command, scope string, out io.Writer) error {
	declarations, err := markdown.Default()
	if err != nil {
		return err
	}
	tracked, err := exec.Command("git", "ls-files", "*.yaml", "*.md").Output()
	if err != nil {
		return fmt.Errorf("list the tracked documents: %w", err)
	}
	paths := strings.Fields(string(tracked))
	links := markdown.LinkIndexes{}
	for _, path := range paths {
		if _, _, ok := declarationFor(declarations, path, false); ok && strings.HasSuffix(path, ".md") {
			if data, err := os.ReadFile(path); err == nil {
				links.Add(path, data)
			}
		}
	}
	matched, problems := 0, 0
	for _, path := range paths {
		switch {
		case command == "convert" && strings.HasSuffix(path, ".yaml"):
			d, docType, ok := declarationFor(declarations, path, true)
			if !ok || !inScope(path, docType, scope) {
				continue
			}
			matched++
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			md, report, err := d.ConvertYAML(data, path)
			if err != nil {
				fmt.Fprintln(out, "FAIL " + err.Error())
				problems++
				continue
			}
			if err := os.WriteFile(report.Target, md, 0o644); err != nil {
				return err
			}
			if err := os.Remove(path); err != nil {
				return err
			}
			fmt.Fprintln(out, strings.Join(report.Lines(), "\n"))
		case (command == "fmt" || command == "check") && strings.HasSuffix(path, ".md"):
			d, docType, ok := declarationFor(declarations, path, false)
			if !ok || !inScope(path, docType, scope) {
				continue
			}
			matched++
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			formatted, findings, err := d.FormatLinked(data, path, links.For(path))
			if err != nil && command == "check" {
				// A document that does not parse is a finding about it; the
				// rest of the corpus is still checked (srd045 R8.2).
				fmt.Fprintln(out, path + ": parse: " + strings.TrimPrefix(err.Error(), path+": "))
				problems++
				continue
			}
			if err != nil {
				return err
			}
			if command == "check" {
				for _, f := range d.Diagnose(data, path) {
					fmt.Fprintln(out, f.String())
					problems++
				}
				if formatted != nil && !bytes.Equal(formatted, data) {
					fmt.Fprintln(out, path + ": not-canonical: run mage fmt")
					problems++
				}
				continue
			}
			if formatted == nil {
				for _, f := range findings {
					fmt.Fprintln(out, f.String())
				}
				problems++
				continue
			}
			if !bytes.Equal(formatted, data) {
				if err := os.WriteFile(path, formatted, 0o644); err != nil {
					return err
				}
				fmt.Fprintln(out, "formatted " + path)
			}
		}
	}
	if matched == 0 {
		return fmt.Errorf("no corpus document matches %q", scope)
	}
	if problems > 0 {
		return fmt.Errorf("%d problem(s) in %d document(s)", problems, matched)
	}
	return nil
}

// inScope reads a scope as all, a document type, a directory, or a path.
func inScope(path, documentType, scope string) bool {
	scope = strings.TrimSuffix(scope, "/")
	return scope == "all" || scope == documentType || path == scope || strings.HasPrefix(path, scope+"/")
}
`
