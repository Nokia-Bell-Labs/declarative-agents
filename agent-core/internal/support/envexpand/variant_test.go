// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package envexpand

import (
	"strings"
	"testing"
)

func TestSelectVariantReadsDefaultsAndPattern(t *testing.T) {
	variant, err := SelectVariant("stages/${P:-cohere}-${S:-rerank}.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if variant.Default != "stages/cohere-rerank.yaml" || variant.Pattern != "stages/*-*.yaml" {
		t.Fatalf("variant = %+v", variant)
	}
	if empty, err := SelectVariant("stages/s${X:-}.yaml"); err != nil || empty.Default != "stages/s.yaml" {
		t.Fatalf("empty default = %+v, %v", empty, err)
	}
}

func TestSelectVariantRefusesPathsWithoutADefault(t *testing.T) {
	for path, want := range map[string]string{
		"stages/s-${X}.yaml":         "needs the form ${NAME:-default}",
		"stages/s-${not valid}.yaml": "is not ${NAME:-default}",
		"stages/s-${X:-a.yaml":       "is not ${NAME:-default}",
	} {
		if _, err := SelectVariant(path); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("SelectVariant(%q) error = %v, want %q", path, err, want)
		}
	}
}

func TestExpandStringSelectsTheBoundVariant(t *testing.T) {
	t.Setenv("GH2235_ENVEXPAND", "b")
	if got := ExpandString("stages/s-${GH2235_ENVEXPAND:-a}.yaml"); got != "stages/s-b.yaml" {
		t.Fatalf("ExpandString = %q", got)
	}
	if !Templated("a/${X:-b}") || Templated("a/b.yaml") {
		t.Fatal("Templated misreads a path")
	}
}
