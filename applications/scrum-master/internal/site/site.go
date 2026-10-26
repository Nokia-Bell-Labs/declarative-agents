// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

// Package site writes the generated status and timeline site into a
// program's reports repository (srd048): plain Markdown pages a forge
// renders as they are, each family of pages written by its own word into
// its own folder under site/. The words write files into a local checkout
// and never commit; the owner lands them through the inner loop.
package site

import (
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/reports"
)

// Dir is the site's folder in the reports repository, beside reports/,
// summaries/, and deck/ (srd018 R1.1).
const Dir = "site"

// Index is the site's front page: written once when missing and the
// program's to edit after (srd048 R2.3).
const Index = Dir + "/index.md"

// Shell is the app shell GitLab Pages serves (srd048 R7.3, GH-119): one
// page that renders the generated Markdown beside it in the browser, so a
// Pages URL reads as an app while the forge's repository view renders the
// same files as they are. It is a generated page, stamped and refreshed
// each run, and kept once a person edits it, so the shell a release
// improves reaches the programs (GH-127).
const Shell = Dir + "/index.html"

//go:embed templates/shell.html
var shellHTML string

// priorShells are the digests of the shells GH-119 and GH-121 wrote once,
// unstamped: a shell exactly as one of them wrote it is the scaffold's own,
// taken back so it refreshes from now on; any other unstamped shell is the
// program's (GH-127).
var priorShells = map[string]bool{
	"3784317c5b50cc7be07cb01714d46c463e96b4b81bda56499ebd0343f068a36e": true, // GH-119
	"8b043aee9b2a5cf1ef9c3220d35a22ca4bfabdc96e7cc132a8dff19b5f9e4cbb": true, // GH-121
}

// Folder is one generated folder and the word that owns it.
type Folder struct {
	Name, Title, Word string
}

// Folders are the generated folders, each replaced by its word on every run
// (srd048 R2.1).
var Folders = []Folder{
	{"status", "Status", "status"},
	{"timeline", "Timeline", "timeline"},
	{"history", "Requirement history", "history"},
	{"corpus", "Corpus", "corpus pages"},
}

// Page is what a run did to one file: created, refreshed, or unchanged when
// the site owns it; removed when the site no longer generates it; kept,
// with the reason, when a person wrote or edited it.
type Page struct {
	Path   string `json:"path"`
	Action string `json:"action"`
	Reason string `json:"reason,omitempty"`
}

// stampPattern is the stamp that closes every generated page: the
// scrum-master version that wrote it and the checksum of everything before
// it, so a run can tell its own unedited pages from a person's (srd048
// R2.2). It is a comment in Markdown and in HTML alike, and closes the file
// so an HTML page still opens with its doctype.
var stampPattern = regexp.MustCompile(`<!-- scrum-master site version=(\S+) sha256=([0-9a-f]{64}) -->\n?$`)

func digest(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}

// Stamp closes content, which ends in a newline, with the site's stamp.
func Stamp(content, version string) string {
	if !strings.HasSuffix(content, "\n") {
		content += "\n"
	}
	return content + fmt.Sprintf("<!-- scrum-master site version=%s sha256=%s -->\n", version, digest(content))
}

// stamped reports whether a file carries the site's stamp, whether its
// content still matches the checksum the stamp records, and the content
// without the stamp.
func stamped(content string) (ours, edited bool, body string) {
	m := stampPattern.FindStringSubmatchIndex(content)
	if m == nil {
		return false, false, content
	}
	body = content[:m[0]]
	return true, digest(body) != content[m[4]:m[5]], body
}

// StampsFile records, in a folder, the stamps of its generated files that
// cannot carry one, the JSON facts: each file's checksum and the version
// that wrote it, one per line (srd048 R2.2).
const StampsFile = ".stamps"

// commented reports whether a file's format carries the stamp as a closing
// comment: Markdown and HTML do; JSON, which has no comments, does not.
func commented(rel string) bool {
	switch strings.ToLower(filepath.Ext(rel)) {
	case ".md", ".html", ".htm":
		return true
	}
	return false
}

// manifest is a folder's StampsFile: each file's name mapped to its
// checksum and version.
type manifest map[string][2]string

func readManifest(folder string) (manifest, error) {
	m := manifest{}
	b, err := os.ReadFile(filepath.Join(folder, StampsFile))
	if errors.Is(err, fs.ErrNotExist) {
		return m, nil
	} else if err != nil {
		return nil, err
	}
	for _, line := range strings.Split(string(b), "\n") {
		if f := strings.Fields(line); len(f) == 3 {
			m[f[2]] = [2]string{f[0], f[1]}
		}
	}
	return m, nil
}

func (m manifest) write(folder string) error {
	names := make([]string, 0, len(m))
	for n := range m {
		names = append(names, n)
	}
	slices.Sort(names)
	var b strings.Builder
	b.WriteString("# The site's stamps for the generated files here that cannot carry one: sha256, version, file.\n")
	for _, n := range names {
		fmt.Fprintf(&b, "%s %s %s\n", m[n][0], m[n][1], n)
	}
	return os.WriteFile(filepath.Join(folder, StampsFile), []byte(b.String()), 0o644)
}

// stampOf reads whether the file at path, holding content, is the site's
// and unedited, by its closing stamp or its folder's StampsFile, and its
// content without the stamp.
func stampOf(path, content string) (ours, edited bool, body string, err error) {
	if commented(path) {
		ours, edited, body = stamped(content)
		return ours, edited, body, nil
	}
	m, err := readManifest(filepath.Dir(path))
	if err != nil {
		return false, false, content, err
	}
	e, ok := m[filepath.Base(path)]
	return ok, ok && e[0] != digest(content), content, nil
}

// Write writes one generated page at rel under the reports checkout dir,
// stamped. A page the site wrote and no one edited is replaced; a page with
// no stamp, or whose stamp no longer matches, is a person's and is kept.
func Write(dir, rel string, content []byte, version string, dryRun bool) (Page, error) {
	p := Page{Path: filepath.ToSlash(rel)}
	text := string(content)
	if !strings.HasSuffix(text, "\n") {
		text += "\n"
	}
	path := filepath.Join(dir, filepath.FromSlash(rel))
	cur, err := os.ReadFile(path)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		p.Action = "created"
	case err != nil:
		return p, err
	default:
		ours, edited, body, err := stampOf(path, string(cur))
		if err != nil {
			return p, err
		}
		switch {
		case !ours:
			p.Action, p.Reason = "kept", "not generated by the site; the site leaves it"
			return p, nil
		case edited:
			p.Action, p.Reason = "kept", "edited by hand; the site keeps it and no longer refreshes it"
			return p, nil
		case body == text:
			p.Action = "unchanged"
			return p, nil
		}
		p.Action = "refreshed"
	}
	if dryRun {
		return p, nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return p, err
	}
	if commented(rel) {
		return p, os.WriteFile(path, []byte(Stamp(text, version)), 0o644)
	}
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		return p, err
	}
	m, err := readManifest(filepath.Dir(path))
	if err != nil {
		return p, err
	}
	m[filepath.Base(path)] = [2]string{digest(text), version}
	return p, m.write(filepath.Dir(path))
}

// Publish writes a word's pages, keyed by their paths under the reports
// checkout, into folder, and removes the pages it generated there before
// that it no longer generates; a person's page in the folder is kept and
// reported (srd048 R2.1, R2.2). A folder's subfolders are other words'
// (the gantt views under the timeline's), so only the files directly in it
// are its own.
func Publish(dir, folder string, pages map[string][]byte, version string, dryRun bool) ([]Page, error) {
	var out []Page
	paths := make([]string, 0, len(pages))
	for rel := range pages {
		paths = append(paths, filepath.ToSlash(rel))
	}
	slices.Sort(paths)
	for _, rel := range paths {
		p, err := Write(dir, rel, pages[rel], version, dryRun)
		if err != nil {
			return out, err
		}
		out = append(out, p)
	}
	root := filepath.Join(dir, Dir, folder)
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		if err != nil {
			return err
		}
		if d.IsDir() {
			if path != root {
				return filepath.SkipDir
			}
			return nil
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		if slices.Contains(paths, filepath.ToSlash(rel)) || d.Name() == StampsFile {
			return nil
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		p := Page{Path: filepath.ToSlash(rel)}
		ours, edited, _, err := stampOf(path, string(b))
		if err != nil {
			return err
		}
		switch {
		case !ours:
			p.Action, p.Reason = "kept", "not generated by the site; the site leaves it"
		case edited:
			p.Action, p.Reason = "kept", "edited by hand; the site keeps it though it no longer generates it"
		default:
			p.Action, p.Reason = "removed", "no longer generated"
			if !dryRun {
				if err := os.Remove(path); err != nil {
					return err
				}
				if !commented(path) {
					m, err := readManifest(filepath.Dir(path))
					if err != nil {
						return err
					}
					delete(m, filepath.Base(path))
					if err := m.write(filepath.Dir(path)); err != nil {
						return err
					}
				}
			}
		}
		out = append(out, p)
		return nil
	})
	return out, err
}

// Result is what a site run did.
type Result struct {
	Dir   string `json:"dir"`
	Pages []Page `json:"pages"`
	// Notes say what a word could not read, such as a tracker no one can
	// reach.
	Notes  []string `json:"notes,omitempty"`
	DryRun bool     `json:"dry_run"`
}

// Scaffold lays down the site in the reports checkout dir (srd048 R1.2):
// the index when missing, the app shell as a generated page, and each
// generated folder's own index page, which says which word fills the
// folder until that word replaces it.
func Scaffold(dir, version string, dryRun bool) (Result, error) {
	r := Result{Dir: dir, Pages: []Page{}, DryRun: dryRun}
	if info, err := os.Stat(dir); err != nil || !info.IsDir() {
		return r, fmt.Errorf("%s is no reports repository checkout; name it with --dir or SCRUM_MASTER_REPORTS", dir)
	}
	index := Page{Path: Index, Action: "created"}
	cur, err := os.ReadFile(filepath.Join(dir, Index))
	switch {
	case err == nil && string(cur) == reports.SiteIndexStub:
		// The placeholder reports init lays down is no index of the
		// program's: the first run replaces it (GH-107).
		index.Action, index.Reason = "refreshed", "replaces the placeholder reports init laid down"
		if !dryRun {
			if err := os.WriteFile(filepath.Join(dir, Index), []byte(indexPage()), 0o644); err != nil {
				return r, err
			}
		}
	case err == nil:
		index.Action, index.Reason = "kept", "the index is the program's once written"
	case dryRun:
	default:
		if err := os.MkdirAll(filepath.Join(dir, Dir), 0o755); err != nil {
			return r, err
		}
		if err := os.WriteFile(filepath.Join(dir, Index), []byte(indexPage()), 0o644); err != nil {
			return r, err
		}
	}
	r.Pages = append(r.Pages, index)
	taken := false
	if cur, err := os.ReadFile(filepath.Join(dir, Shell)); err == nil && priorShells[digest(string(cur))] {
		taken = true
		if !dryRun {
			if err := os.Remove(filepath.Join(dir, Shell)); err != nil {
				return r, err
			}
		}
	}
	shell := Page{Path: Shell, Action: "refreshed", Reason: "takes back the shell an earlier release wrote once, so it refreshes from now on"}
	if !taken || !dryRun {
		if shell, err = Write(dir, Shell, []byte(shellHTML), version, dryRun); err != nil {
			return r, err
		}
		if taken {
			shell.Action, shell.Reason = "refreshed", "takes back the shell an earlier release wrote once, so it refreshes from now on"
		}
	}
	r.Pages = append(r.Pages, shell)
	// The shell opens on the overview: until its word runs, a page says
	// what fills it (GH-128).
	if _, err := os.Stat(filepath.Join(dir, Overview)); errors.Is(err, fs.ErrNotExist) {
		p, err := Write(dir, Overview, []byte(overviewPlaceholder), version, dryRun)
		if err != nil {
			return r, err
		}
		r.Pages = append(r.Pages, p)
	}
	for _, f := range Folders {
		rel := Dir + "/" + f.Name + "/index.md"
		if _, err := os.Stat(filepath.Join(dir, filepath.FromSlash(rel))); err == nil {
			// The folder's word has written it, or a person has; either
			// way the scaffold has nothing to add.
			continue
		}
		p, err := Write(dir, rel, []byte(placeholder(f)), version, dryRun)
		if err != nil {
			return r, err
		}
		r.Pages = append(r.Pages, p)
	}
	return r, nil
}

// indexPage is the site's front page as first written: a link to each
// generated folder.
func indexPage() string {
	var b strings.Builder
	b.WriteString("# Program site\n\nThe program's status, timeline, and requirement history, generated from the corpus and the tracker by the site skill. This page is yours to edit; the site writes it only when it is missing.\n\n")
	for _, f := range Folders {
		fmt.Fprintf(&b, "- [%s](%s/index.md)\n", f.Title, f.Name)
	}
	return b.String()
}

// placeholder is a generated folder's index before its word has run.
func placeholder(f Folder) string {
	return fmt.Sprintf("# %s\n\nNo pages yet: the site's %s word writes them.\n", f.Title, f.Word)
}

// Human is a run's report for a person: one line per page.
func Human(r Result) string {
	var b strings.Builder
	for _, p := range r.Pages {
		line := p.Action + " " + p.Path
		if p.Reason != "" {
			line += ": " + p.Reason
		}
		b.WriteString(line + "\n")
	}
	for _, n := range r.Notes {
		b.WriteString("note: " + n + "\n")
	}
	if r.DryRun {
		b.WriteString("dry run: nothing was written\n")
	}
	return b.String()
}
