// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package chartconf

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// nodeImagePin is the form ENG01 requires of a kind node image: a version tag
// and the digest that version resolved to, so two machines create the same
// cluster (srd005 R6.1).
var nodeImagePin = regexp.MustCompile(`^(.+):(v\d+\.\d+\.\d+)@sha256:[0-9a-f]{64}$`)

// kindConfig is the subset of a kind cluster configuration the rule reads.
type kindConfig struct {
	APIVersion string `yaml:"apiVersion"`
	Kind       string `yaml:"kind"`
	Nodes      []struct {
		Role  string `yaml:"role"`
		Image string `yaml:"image"`
	} `yaml:"nodes"`
}

// IsKindConfig reports whether a document is a kind cluster configuration.
// It is what separates a cluster configuration from a values overlay when
// both sit in a chart's ci/ directory.
func IsKindConfig(content string) bool {
	var config kindConfig
	if err := yaml.Unmarshal([]byte(content), &config); err != nil {
		return false
	}
	return config.Kind == "Cluster" && strings.HasPrefix(config.APIVersion, "kind.x-k8s.io/")
}

// CheckKindConfig applies R6.1 to one checked-in kind configuration. A node
// entry with no image inherits the kind binary's built-in default, which is
// the unpinned case the rule exists to catch.
func CheckKindConfig(path, content string) ([]Finding, error) {
	var config kindConfig
	if err := yaml.Unmarshal([]byte(content), &config); err != nil {
		return nil, fmt.Errorf("parse kind configuration %s: %w", path, err)
	}
	var findings []Finding
	for index, node := range config.Nodes {
		resource := fmt.Sprintf("node/%d", index)
		if node.Role != "" {
			resource = "node/" + node.Role
		}
		switch {
		case node.Image == "":
			findings = append(findings, Finding{path, "", "R6.1", resource, "",
				"node declares no image, so the kind binary picks one"})
		case !nodeImagePin.MatchString(node.Image):
			findings = append(findings, Finding{path, "", "R6.1", resource, node.Image,
				"node image is not pinned as kindest/node:vX.Y.Z@sha256:..."})
		}
	}
	sort.Slice(findings, func(i, j int) bool { return findings[i].Key() < findings[j].Key() })
	return findings, nil
}
