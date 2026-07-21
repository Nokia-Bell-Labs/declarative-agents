// Copyright (c) 2026 Nokia. All rights reserved.

package spec

import "strings"

// --- Touchpoint/trace parsing helpers ---

// parseTouchpoint extracts the SRD ID and cited groups from a touchpoint string.
// A group is a requirement group (R1, R2.1) or an acceptance criterion (AC1);
// both resolve to a "srdID:group" node, so both are collected (GH-448).
// Formats: "srd005-cli R1 -- description" or "T1: srd005-cli R1 -- description".
func parseTouchpoint(tp string) (string, []string) {
	desc := strings.SplitN(tp, "--", 2)
	refs := trimTouchpointTag(desc[0])

	parts := strings.Fields(refs)
	if len(parts) == 0 {
		return "", nil
	}

	srdID := parts[0]
	if !strings.HasPrefix(srdID, "srd") {
		return "", nil
	}

	var groups []string
	for _, p := range parts[1:] {
		p = strings.TrimRight(p, ",")
		if p == "" {
			continue
		}
		if p[0] == 'R' || strings.HasPrefix(p, "AC") {
			groups = append(groups, p)
		}
	}
	return srdID, groups
}

func trimTouchpointTag(refs string) string {
	refs = strings.TrimSpace(refs)
	label, rest, ok := strings.Cut(refs, ":")
	if !ok || !isTouchpointLabel(label) {
		return refs
	}
	return strings.TrimSpace(rest)
}

func isTouchpointLabel(label string) bool {
	if len(label) < 2 || label[0] != 'T' {
		return false
	}
	for _, r := range label[1:] {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// parseACTrace extracts SRD ID and AC ID from a trace string.
// Format: "srd005-cli-entry-point AC1"
func parseACTrace(trace string) (string, string) {
	parts := strings.Fields(trace)
	if len(parts) < 2 {
		return "", ""
	}
	srdID := parts[0]
	acID := parts[1]
	if !strings.HasPrefix(srdID, "srd") {
		return "", ""
	}
	return srdID, acID
}

// buildSRDReleaseMap parses the SPECIFICATIONS.yaml overview text
// for explicit release assignments like "- 00.0: srd001 (...), srd002 (...)".
// Short IDs (e.g. "srd001") are matched to full corpus IDs (e.g. "srd001-requirement-loader")
// by prefix.
func buildSRDReleaseMap(corpus *Corpus) map[string]string {
	m := make(map[string]string)
	overview := corpus.SpecIndex.Overview
	if overview == "" {
		return m
	}
	for _, line := range strings.Split(overview, "\n") {
		release, shortIDs := parseAssignmentLine(line)
		if release == "" {
			continue
		}
		for _, shortID := range shortIDs {
			fullID := resolveShortID(shortID, corpus.SRDOrder)
			if fullID == "" {
				continue
			}
			if _, exists := m[fullID]; !exists {
				m[fullID] = release
			}
		}
	}
	return m
}

// resolveShortID maps a potentially abbreviated SRD ID (e.g. "srd001")
// to the full corpus ID (e.g. "srd001-requirement-loader"). If the short
// ID already matches exactly, it's returned as-is.
func resolveShortID(short string, srdOrder []string) string {
	for _, full := range srdOrder {
		if full == short {
			return full
		}
		if strings.HasPrefix(full, short+"-") || strings.HasPrefix(full, short+"_") {
			return full
		}
	}
	return ""
}

func parseAssignmentLine(line string) (string, []string) {
	trimmed := strings.TrimLeft(line, " \t")
	if !strings.HasPrefix(trimmed, "- ") {
		return "", nil
	}
	trimmed = trimmed[2:]

	colonIdx := strings.IndexByte(trimmed, ':')
	if colonIdx < 1 {
		return "", nil
	}
	release := trimmed[:colonIdx]

	for _, c := range release {
		if c != '.' && (c < '0' || c > '9') {
			return "", nil
		}
	}

	rest := trimmed[colonIdx+1:]
	var ids []string
	for _, field := range strings.FieldsFunc(rest, func(r rune) bool {
		return r == ',' || r == '(' || r == ')'
	}) {
		field = strings.TrimSpace(field)
		if strings.HasPrefix(field, "srd") && len(field) > 3 {
			ids = append(ids, field)
		}
	}
	return release, ids
}
