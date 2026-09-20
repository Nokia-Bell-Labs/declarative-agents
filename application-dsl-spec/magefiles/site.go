// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	goldmarkhtml "github.com/yuin/goldmark/renderer/html"
)

// pagesBaseURL is the published root of the rendered specification. The
// /latest/ tree is the canonical form of every page; each release also
// publishes an immutable /v<version>/ copy.
const pagesBaseURL = "https://nokia-bell-labs.github.io/declarative-agents"

const siteDir = "generated-files/site"

// chapterHeadingPattern pulls the page title from a chapter's first heading.
var chapterHeadingPattern = regexp.MustCompile(`(?m)^# (.+)$`)

// terminologyTermPattern collects the defined terms from the chapter 02
// class/instance table and compound-term prose, for DefinedTerm metadata.
var terminologyTermPattern = regexp.MustCompile(
	`\b(?:application|agent|machine)-(?:profile|instance)\b|\bstage-fragment\b|\bmachine-template\b`)

// Site renders the chapters and builds the static Pages tree: one HTML page
// per chapter under /latest/ and /v<version>/, an index, sitemap.xml, and
// JSON-LD metadata (TechArticle per chapter, DefinedTerm entries on the
// terminology chapter).
func Site() error {
	if err := Render(); err != nil {
		return err
	}
	language, err := loadLanguage(languagePath)
	if err != nil {
		return err
	}
	return buildSite(renderOutputDir, siteDir, language)
}

func buildSite(renderedDir, outputDir string, language languageFile) error {
	chapters, err := discoverChapters(renderedDir)
	if err != nil {
		return err
	}
	if len(chapters) == 0 {
		return fmt.Errorf("no rendered chapters in %s", renderedDir)
	}

	converter := goldmark.New(
		goldmark.WithExtensions(extension.GFM),
		// The rendered markdown embeds this specification's own anchor tags,
		// which must pass through to hold the stable statement anchors.
		goldmark.WithRendererOptions(goldmarkhtml.WithUnsafe()),
	)

	versions := []string{"latest", "v" + language.Language.Version}
	var sitemap []string
	for _, version := range versions {
		for _, chapter := range chapters {
			source, err := os.ReadFile(filepath.Join(renderedDir, chapter))
			if err != nil {
				return err
			}
			var body bytes.Buffer
			if err := converter.Convert(source, &body); err != nil {
				return fmt.Errorf("convert %s: %w", chapter, err)
			}
			page := strings.TrimSuffix(chapter, ".md") + ".html"
			document := sitePage(language, chapters, chapter, string(source), body.String(), version)
			target := filepath.Join(outputDir, version, page)
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			if err := os.WriteFile(target, []byte(document), 0o644); err != nil {
				return err
			}
			sitemap = append(sitemap, pagesBaseURL+"/"+version+"/"+page)
		}
	}

	index := siteIndex(language, chapters, versions)
	if err := os.WriteFile(filepath.Join(outputDir, "index.html"), []byte(index), 0o644); err != nil {
		return err
	}
	sitemap = append(sitemap, pagesBaseURL+"/")
	sort.Strings(sitemap)
	if err := os.WriteFile(filepath.Join(outputDir, "sitemap.xml"), []byte(siteSitemap(sitemap)), 0o644); err != nil {
		return err
	}
	fmt.Printf("site: %d pages across %d versions under %s\n", len(chapters), len(versions), outputDir)
	return nil
}

func chapterTitle(source, fallback string) string {
	if match := chapterHeadingPattern.FindStringSubmatch(source); match != nil {
		return strings.TrimSpace(match[1])
	}
	return fallback
}

func sitePage(language languageFile, chapters []string, chapter, source, body, version string) string {
	page := strings.TrimSuffix(chapter, ".md") + ".html"
	title := chapterTitle(source, chapter)
	canonical := pagesBaseURL + "/latest/" + page

	metadata := map[string]any{
		"@context":       "https://schema.org",
		"@type":          "TechArticle",
		"headline":       title,
		"url":            canonical,
		"version":        language.Language.Version,
		"isPartOf":       map[string]any{"@type": "CreativeWork", "name": language.Language.Title},
		"license":        "https://spdx.org/licenses/BSD-3-Clause.html",
		"publisher":      map[string]any{"@type": "Organization", "name": "Nokia Bell Labs"},
		"inLanguage":     "en",
		"encodingFormat": "text/html",
	}
	blocks := []map[string]any{metadata}
	if strings.HasPrefix(chapter, "02-") {
		for _, term := range definedTerms(source) {
			blocks = append(blocks, map[string]any{
				"@context": "https://schema.org",
				"@type":    "DefinedTerm",
				"name":     term,
				"inDefinedTermSet": map[string]any{
					"@type": "DefinedTermSet",
					"name":  language.Language.Title,
					"url":   canonical,
				},
			})
		}
	}

	var jsonld strings.Builder
	for _, block := range blocks {
		encoded, err := json.Marshal(block)
		if err != nil {
			continue
		}
		fmt.Fprintf(&jsonld, "<script type=\"application/ld+json\">%s</script>\n", encoded)
	}

	var nav strings.Builder
	for _, entry := range chapters {
		target := strings.TrimSuffix(entry, ".md") + ".html"
		label := strings.TrimSuffix(entry, ".md")
		marker := ""
		if entry == chapter {
			marker = ` aria-current="page"`
		}
		fmt.Fprintf(&nav, "<a href=%q%s>%s</a>\n", target, marker, html.EscapeString(label))
	}

	return fmt.Sprintf(`<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>%s — %s</title>
<link rel="canonical" href=%q>
%s<style>
body { font-family: Georgia, serif; max-width: 46rem; margin: 0 auto; padding: 1rem; line-height: 1.55; color: #1a1a1a; }
nav { font-family: system-ui, sans-serif; font-size: 0.8rem; display: flex; flex-wrap: wrap; gap: 0.6rem; border-bottom: 1px solid #ccc; padding-bottom: 0.6rem; margin-bottom: 1rem; }
nav a { text-decoration: none; color: #444; }
nav a[aria-current] { font-weight: bold; color: #000; }
table { border-collapse: collapse; }
th, td { border: 1px solid #bbb; padding: 0.25rem 0.5rem; text-align: left; }
blockquote { border-left: 3px solid #888; margin-left: 0; padding-left: 1rem; }
code { font-size: 0.9em; }
footer { font-family: system-ui, sans-serif; font-size: 0.75rem; color: #666; margin-top: 2rem; border-top: 1px solid #ccc; padding-top: 0.6rem; }
</style>
</head>
<body>
<nav>%s</nav>
<main>
%s</main>
<footer>%s · version %s · <a href=%q>canonical</a></footer>
</body>
</html>
`,
		html.EscapeString(title), html.EscapeString(language.Language.Title), canonical,
		jsonld.String(), nav.String(), body,
		html.EscapeString(language.Language.Title), language.Language.Version+" ("+version+")", canonical)
}

func definedTerms(source string) []string {
	seen := map[string]bool{}
	var terms []string
	for _, term := range terminologyTermPattern.FindAllString(source, -1) {
		if !seen[term] {
			seen[term] = true
			terms = append(terms, term)
		}
	}
	sort.Strings(terms)
	return terms
}

func siteIndex(language languageFile, chapters []string, versions []string) string {
	var toc strings.Builder
	for _, chapter := range chapters {
		page := strings.TrimSuffix(chapter, ".md") + ".html"
		fmt.Fprintf(&toc, "<li><a href=%q>%s</a></li>\n",
			"latest/"+page, html.EscapeString(strings.TrimSuffix(chapter, ".md")))
	}
	var links strings.Builder
	for _, version := range versions {
		fmt.Fprintf(&links, "<a href=%q>%s</a> ", version+"/"+strings.TrimSuffix(chapters[0], ".md")+".html", version)
	}
	return fmt.Sprintf(`<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>%s</title>
<link rel="canonical" href=%q>
</head>
<body>
<h1>%s</h1>
<p>Versions: %s</p>
<ul>
%s</ul>
</body>
</html>
`, html.EscapeString(language.Language.Title), pagesBaseURL+"/",
		html.EscapeString(language.Language.Title), links.String(), toc.String())
}

func siteSitemap(urls []string) string {
	var entries strings.Builder
	for _, url := range urls {
		fmt.Fprintf(&entries, "  <url><loc>%s</loc></url>\n", html.EscapeString(url))
	}
	return "<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n" +
		"<urlset xmlns=\"http://www.sitemaps.org/schemas/sitemap/0.9\">\n" +
		entries.String() + "</urlset>\n"
}
