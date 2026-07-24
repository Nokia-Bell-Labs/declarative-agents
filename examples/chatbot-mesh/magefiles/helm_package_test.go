// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestHelmPackageContainsRequiredProfileEntrypoints(t *testing.T) {
	if _, err := exec.LookPath("helm"); err != nil {
		t.Skip("helm not on PATH")
	}
	chart := findChartDir(t)
	destination := t.TempDir()
	if err := packageHelmChart(chart, filepath.Dir(chart), destination); err != nil {
		t.Fatal(err)
	}
	archives, err := filepath.Glob(filepath.Join(destination, "chatbot-mesh-*.tgz"))
	if err != nil {
		t.Fatal(err)
	}
	if len(archives) != 1 {
		t.Fatalf("packaged archives = %v, want exactly one", archives)
	}
	out, err := exec.Command("helm", "template", "t", archives[0]).CombinedOutput()
	if err != nil {
		t.Fatalf("render packaged chart: %v\n%s", err, out)
	}
	render := string(out)
	for _, key := range []string{
		"agents__chatbot__profile.yaml",
		"agents__rag-server__profile.yaml",
		"agents__coordinator__profile.yaml",
		"agents__creator__profile.yaml",
		"agents__executor__profile.yaml",
		"ux__app__dist__index.html",
	} {
		if !strings.Contains(render, key+": |-") {
			t.Errorf("packaged profiles ConfigMap missing %s", key)
		}
	}
}
