// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/magefile/mage/sh"
)

const (
	paperSlug  = "agentic-design-patterns"
	outputDir  = "generated-files"
	figuresDir = "figures"
	csl        = "templates/ieee.csl"
)

// All generates figures then builds the PDF (default target).
func All() error {
	if err := Figures(); err != nil {
		return err
	}
	return PDF()
}

// Build compiles the design-patterns paper artifacts.
func Build() error {
	return PDF()
}

// Figures renders all PlantUML diagrams to PNG.
func Figures() error {
	pumls, err := filepath.Glob(filepath.Join(figuresDir, "*.puml"))
	if err != nil {
		return err
	}
	if len(pumls) == 0 {
		fmt.Println("no .puml files found")
		return nil
	}
	sort.Strings(pumls)
	for _, puml := range pumls {
		png := strings.TrimSuffix(puml, ".puml") + ".png"
		pumlInfo, err := os.Stat(puml)
		if err != nil {
			return err
		}
		if pngInfo, err := os.Stat(png); err == nil && pngInfo.ModTime().After(pumlInfo.ModTime()) {
			continue
		}
		fmt.Printf("plantuml %s\n", puml)
		if err := sh.Run("plantuml", "-tpng", "-o", ".", puml); err != nil {
			return fmt.Errorf("plantuml %s: %w", puml, err)
		}
	}
	return nil
}

// PDF compiles markdown chapters into an IEEE two-column PDF.
func PDF() error {
	if err := Figures(); err != nil {
		return err
	}

	mds, err := discoverMarkdownChapters()
	if err != nil {
		return err
	}
	if len(mds) == 0 {
		return fmt.Errorf("no [0-9][0-9]-*.md chapter files found")
	}

	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return fmt.Errorf("mkdir %s: %w", outputDir, err)
	}

	date := time.Now().Format("2006-01-02")
	out := filepath.Join(outputDir, fmt.Sprintf("%s-v%s.pdf", paperSlug, date))

	args := []string{
		"--citeproc",
		"--csl=" + csl,
		"--bibliography=references.yaml",
		"--lua-filter=templates/ieee-twocolumn.lua",
		"--include-in-header=templates/ieee-preamble.tex",
		"--from", "markdown",
		"--pdf-engine=xelatex",
		"--syntax-highlighting=idiomatic",
	}
	args = append(args, mds...)
	args = append(args, "-o", out)

	fmt.Printf("generating %s from %d chapters\n", out, len(mds))
	if err := sh.Run("pandoc", args...); err != nil {
		return fmt.Errorf("pandoc: %w", err)
	}
	return nil
}

// Clean removes generated PNGs and PDFs.
func Clean() error {
	return cleanArtifacts(figuresDir, outputDir, filepath.Glob, os.ReadDir, os.Remove)
}

func cleanArtifacts(
	figures, output string,
	glob func(string) ([]string, error),
	readDir func(string) ([]os.DirEntry, error),
	remove func(string) error,
) error {
	var errs []error
	pngs, err := glob(filepath.Join(figures, "*.png"))
	if err != nil {
		errs = append(errs, fmt.Errorf("list generated figures: %w", err))
	}
	for _, f := range pngs {
		fmt.Printf("rm %s\n", f)
		if err := remove(f); err != nil && !os.IsNotExist(err) {
			errs = append(errs, fmt.Errorf("remove %s: %w", f, err))
		}
	}

	entries, err := readDir(output)
	if err != nil {
		if !os.IsNotExist(err) {
			errs = append(errs, fmt.Errorf("list generated output %s: %w", output, err))
		}
	} else {
		for _, e := range entries {
			path := filepath.Join(output, e.Name())
			fmt.Printf("rm %s\n", path)
			if err := remove(path); err != nil && !os.IsNotExist(err) {
				errs = append(errs, fmt.Errorf("remove %s: %w", path, err))
			}
		}
	}
	return errors.Join(errs...)
}

func discoverMarkdownChapters() ([]string, error) {
	entries, err := os.ReadDir(".")
	if err != nil {
		return nil, err
	}
	var mds []string
	for _, e := range entries {
		name := e.Name()
		if len(name) >= 3 && name[0] >= '0' && name[0] <= '9' && name[1] >= '0' && name[1] <= '9' && name[2] == '-' && strings.HasSuffix(name, ".md") {
			mds = append(mds, name)
		}
	}
	sort.Strings(mds)
	return mds, nil
}
