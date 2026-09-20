// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func builtSite(t *testing.T) string {
	t.Helper()
	rendered := t.TempDir()
	writeChapter(t, rendered, "01-introduction.md",
		"# 1. Introduction\n\n<a id=\"R-INTRO-001\"></a>\n\n> **R-INTRO-001** (document, MUST) — body.\n")
	writeChapter(t, rendered, "02-terminology.md",
		"# 2. Terminology\n\n| agent-profile | agent-instance |\n|---|---|\n\nA machine-template expands.\n")
	output := t.TempDir()
	language := renderLanguage()
	language.Language.Title = "Libretto"
	language.Language.Version = "0.1.0"
	if err := buildSite(rendered, output, language); err != nil {
		t.Fatal(err)
	}
	return output
}

func TestSiteBuildsVersionedPagesWithStableAnchors(t *testing.T) {
	site := builtSite(t)
	for _, page := range []string{
		"latest/01-introduction.html",
		"v0.1.0/01-introduction.html",
	} {
		data, err := os.ReadFile(filepath.Join(site, filepath.FromSlash(page)))
		if err != nil {
			t.Fatalf("%s: %v", page, err)
		}
		content := string(data)
		for _, want := range []string{
			`<a id="R-INTRO-001"></a>`,
			`<link rel="canonical" href="` + pagesBaseURL + `/latest/01-introduction.html">`,
			`"@type":"TechArticle"`,
		} {
			if !strings.Contains(content, want) {
				t.Errorf("%s missing %q", page, want)
			}
		}
	}
}

func TestSiteEmitsDefinedTermsOnTerminologyPage(t *testing.T) {
	site := builtSite(t)
	data, err := os.ReadFile(filepath.Join(site, "latest", "02-terminology.html"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`"@type":"DefinedTerm"`,
		`"name":"agent-profile"`,
		`"name":"machine-template"`,
	} {
		if !strings.Contains(string(data), want) {
			t.Errorf("terminology page missing %q", want)
		}
	}
}

func TestSiteSitemapCoversAllPagesAndIndex(t *testing.T) {
	site := builtSite(t)
	data, err := os.ReadFile(filepath.Join(site, "sitemap.xml"))
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)
	for _, want := range []string{
		pagesBaseURL + "/latest/01-introduction.html",
		pagesBaseURL + "/latest/02-terminology.html",
		pagesBaseURL + "/v0.1.0/02-terminology.html",
		pagesBaseURL + "/</loc>",
	} {
		if !strings.Contains(content, want) {
			t.Errorf("sitemap missing %q:\n%s", want, content)
		}
	}
	if _, err := os.Stat(filepath.Join(site, "index.html")); err != nil {
		t.Errorf("index.html missing: %v", err)
	}
}

// TestRepositorySiteBuilds proves the tracked chapters and language build a
// complete site: render into a scratch directory, then generate the tree.
func TestRepositorySiteBuilds(t *testing.T) {
	language, err := loadLanguage(filepath.Join("..", languagePath))
	if err != nil {
		t.Fatal(err)
	}
	rendered := t.TempDir()
	if err := renderChapters("..", rendered, language); err != nil {
		t.Fatal(err)
	}
	site := t.TempDir()
	if err := buildSite(rendered, site, language); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(site, "latest", "08-population-invariants.html"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `<a id="R-POP-001"></a>`) {
		t.Error("population chapter lost its statement anchor")
	}
}
