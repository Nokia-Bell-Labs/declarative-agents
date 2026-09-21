// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/Nokia-Bell-Labs/declarative-agents/magefiles/chartconf"
	"github.com/Nokia-Bell-Labs/declarative-agents/magefiles/pinsurvey"
)

// Bump resolves every pinned upstream dependency against its registry and
// reports two properties per pin: whether the recorded digest is still what
// the tag resolves to, and whether a newer release exists
// (srd006-pin-currency, ENG01 "Pin currency").
//
// It reports and changes nothing: no file is edited, no branch is created,
// and it exits zero whatever it finds. A version bump is a decision, and
// this target exists to put the decision in front of a person rather than
// to take it. It reaches the network, so it is not part of mage audit.
func Bump() error {
	root, err := os.Getwd()
	if err != nil {
		return err
	}
	pins, err := collectPins(root)
	if err != nil {
		return err
	}
	results := pinsurvey.Survey(pinsurvey.NewClient(), pins)
	printPinReport(os.Stdout, results)
	return nil
}

// collectPins joins the two halves of the inventory: chart images read out
// of the rendered charts, and the pins no render exposes from the checked-in
// list (srd006 R1.1, R1.2).
func collectPins(root string) ([]pinsurvey.Pin, error) {
	declared, err := pinsurvey.DeclaredPins()
	if err != nil {
		return nil, err
	}
	chartPins, err := renderedChartPins(root)
	if err != nil {
		// A chart that will not render is a conformance problem, which the
		// audit reports; here it must not silence the declared half.
		fmt.Fprintf(os.Stderr, "bump: chart pins unavailable (%v); reporting declared pins only\n", err)
	}
	pins := append(declared, chartPins...)
	pins = pinsurvey.SkipRepositoryImages(pins, chartconf.RepositoryImagePrefixes)
	return pinsurvey.Deduplicate(pins), nil
}

func renderedChartPins(root string) ([]pinsurvey.Pin, error) {
	var pins []pinsurvey.Pin
	for _, application := range conformanceApplications {
		source := filepath.Join(root, "applications", application, "helm")
		overlays, err := valuesOverlays(source)
		if err != nil {
			return nil, err
		}
		chart, cleanup, err := stageChartForRender(root, application)
		if err != nil {
			return nil, err
		}
		for _, overlay := range append([]string{defaultsOverlay}, overlays...) {
			rendered, err := renderChart(chart, overlay)
			if err != nil {
				cleanup()
				return nil, err
			}
			documents, err := chartconf.Parse(rendered)
			if err != nil {
				cleanup()
				return nil, err
			}
			chartPins, err := pinsurvey.ChartPins(application, overlay, chartconf.ImageReferences(documents))
			if err != nil {
				cleanup()
				return nil, err
			}
			pins = append(pins, chartPins...)
		}
		cleanup()
	}
	return pins, nil
}

// printPinReport groups results by verdict, moved first, so the report opens
// with what needs a decision (srd006 R2.1, R6.1).
func printPinReport(out *os.File, results []pinsurvey.Result) {
	counts := pinsurvey.Counts(results)
	headings := []struct {
		verdict pinsurvey.Verdict
		heading string
	}{
		{pinsurvey.Moved, "moved: the tag no longer resolves to the recorded digest"},
		{pinsurvey.Unknown, "unknown: could not be checked"},
		{pinsurvey.Behind, "behind: a newer release exists"},
		{pinsurvey.Current, "current"},
	}
	for _, group := range headings {
		var matching []pinsurvey.Result
		for _, result := range results {
			if result.Verdict == group.verdict {
				matching = append(matching, result)
			}
		}
		if len(matching) == 0 {
			continue
		}
		fmt.Fprintf(out, "\n%s (%d)\n", group.heading, len(matching))
		sort.SliceStable(matching, func(i, j int) bool {
			return matching[i].Pin.Location < matching[j].Pin.Location
		})
		for _, result := range matching {
			fmt.Fprintf(out, "  %s\n", result)
		}
	}
	fmt.Fprintf(out, "\n%d pins: %d moved, %d unknown, %d behind, %d current\n",
		len(results), counts[pinsurvey.Moved], counts[pinsurvey.Unknown],
		counts[pinsurvey.Behind], counts[pinsurvey.Current])
}
