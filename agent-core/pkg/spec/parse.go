// Copyright (c) 2026 Nokia. All rights reserved.

package spec

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// ParseSRD reads a single SRD YAML file and returns the parsed struct.
func ParseSRD(path string) (SRD, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return SRD{}, fmt.Errorf("read SRD %s: %w", path, err)
	}
	return parseSRDBytes(data, path)
}

// rawSRD is the intermediate representation for YAML unmarshalling.
type rawSRD struct {
	ID                 string                `yaml:"id"`
	Title              string                `yaml:"title"`
	Problem            string                `yaml:"problem"`
	Goals              yaml.Node             `yaml:"goals"`
	Requirements       yaml.Node             `yaml:"requirements"`
	NonGoals           yaml.Node             `yaml:"non_goals"`
	AcceptanceCriteria []AcceptanceCriterion `yaml:"acceptance_criteria"`
	DependsOn          []Dependency          `yaml:"depends_on"`
}

func parseSRDBytes(data []byte, source string) (SRD, error) {
	var raw rawSRD
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return SRD{}, fmt.Errorf("parse SRD %s: %w", source, err)
	}

	groups, orderedKeys, err := parseRequirements(&raw.Requirements)
	if err != nil {
		return SRD{}, fmt.Errorf("parse SRD %s requirements: %w", source, err)
	}

	goals, err := parseTaggedList(&raw.Goals, "goals")
	if err != nil {
		return SRD{}, fmt.Errorf("parse SRD %s: %w", source, err)
	}
	nonGoals, err := parseTaggedList(&raw.NonGoals, "non_goals")
	if err != nil {
		return SRD{}, fmt.Errorf("parse SRD %s: %w", source, err)
	}

	return SRD{
		ID:                 raw.ID,
		Title:              raw.Title,
		Problem:            raw.Problem,
		Goals:              goals,
		Requirements:       groups,
		NonGoals:           nonGoals,
		AcceptanceCriteria: raw.AcceptanceCriteria,
		DependsOn:          raw.DependsOn,
		OrderedGroups:      orderedKeys,
	}, nil
}

// parseTaggedList handles YAML lists of the form:
//
//   - G1: Some text.
//   - G2: More text.
func parseTaggedList(node *yaml.Node, field string) ([]string, error) {
	if node == nil || node.Kind == 0 {
		return nil, nil
	}
	n := node
	if n.Kind == yaml.DocumentNode && len(n.Content) > 0 {
		n = n.Content[0]
	}
	if n.Kind != yaml.SequenceNode {
		return nil, fmt.Errorf("%s: expected sequence", field)
	}

	var result []string
	for index, item := range n.Content {
		switch item.Kind {
		case yaml.ScalarNode:
			if item.Tag == "!!null" || item.Value == "" {
				return nil, fmt.Errorf("%s[%d]: expected nonempty scalar or one-entry mapping", field, index)
			}
			result = append(result, item.Value)
		case yaml.MappingNode:
			value, err := taggedListMapping(item)
			if err != nil {
				return nil, fmt.Errorf("%s[%d]: %w", field, index, err)
			}
			result = append(result, value)
		default:
			return nil, fmt.Errorf("%s[%d]: unsupported YAML kind %d", field, index, item.Kind)
		}
	}
	return result, nil
}

func taggedListMapping(item *yaml.Node) (string, error) {
	if len(item.Content) == 2 && item.Content[0].Kind == yaml.ScalarNode && item.Content[1].Kind == yaml.ScalarNode {
		return item.Content[0].Value + ": " + item.Content[1].Value, nil
	}
	fields := map[string]string{}
	for i := 0; i+1 < len(item.Content); i += 2 {
		key, value := item.Content[i], item.Content[i+1]
		if key.Kind != yaml.ScalarNode || value.Kind != yaml.ScalarNode || value.Tag == "!!null" {
			return "", fmt.Errorf("mapping keys and values must be scalars")
		}
		fields[key.Value] = value.Value
	}
	if len(fields) == 2 && fields["id"] != "" && fields["text"] != "" {
		return fields["id"] + ": " + fields["text"], nil
	}
	return "", fmt.Errorf("expected one scalar mapping entry or an id/text object")
}

// parseTouchpointList parses the use-case touchpoints field, which accepts three
// forms: a scalar string ("srdNNN R1 -- desc"), a tagged mapping
// ({T1: "srdNNN R1 -- desc"}), or an object ({id, target, reason}). The object
// form is folded into the canonical "<target> -- <reason>" string that
// parseTouchpoint already parses into an SRD touches edge and its citation
// groups; without this, the flattened "target: srdNNN AC1" string defeats
// parseTouchpoint and every SRD is falsely orphaned (GH-448). Scalar and tagged
// forms pass through unchanged.
func parseTouchpointList(node *yaml.Node, field string) ([]string, error) {
	if node == nil || node.Kind == 0 {
		return nil, nil
	}
	n := node
	if n.Kind == yaml.DocumentNode && len(n.Content) > 0 {
		n = n.Content[0]
	}
	if n.Kind != yaml.SequenceNode {
		return nil, fmt.Errorf("%s: expected sequence", field)
	}

	var result []string
	for index, item := range n.Content {
		switch item.Kind {
		case yaml.ScalarNode:
			if item.Tag == "!!null" || item.Value == "" {
				return nil, fmt.Errorf("%s[%d]: expected nonempty scalar or mapping", field, index)
			}
			result = append(result, item.Value)
		case yaml.MappingNode:
			value, err := touchpointFromMapping(item)
			if err != nil {
				return nil, fmt.Errorf("%s[%d]: %w", field, index, err)
			}
			result = append(result, value)
		default:
			return nil, fmt.Errorf("%s[%d]: unsupported YAML kind %d", field, index, item.Kind)
		}
	}
	return result, nil
}

// touchpointFromMapping folds an object-format touchpoint ({id, target, reason})
// into a single canonical "<target> -- <reason>" string. The id is a display
// label the graph does not need, so it is dropped rather than prefixed (a
// non-Tn label would otherwise defeat parseTouchpoint). A mapping without a
// target key is the tagged form ({T1: "srdNNN R1 -- desc"}) and keeps its
// "key: value" flattening.
func touchpointFromMapping(item *yaml.Node) (string, error) {
	fields := make(map[string]string, len(item.Content)/2)
	for i := 0; i+1 < len(item.Content); i += 2 {
		keyNode, valueNode := item.Content[i], item.Content[i+1]
		if keyNode.Kind != yaml.ScalarNode || valueNode.Kind != yaml.ScalarNode || valueNode.Tag == "!!null" {
			return "", fmt.Errorf("mapping keys and values must be scalars")
		}
		k, v := keyNode.Value, valueNode.Value
		if _, duplicate := fields[k]; duplicate {
			return "", fmt.Errorf("duplicate key %q", k)
		}
		fields[k] = v
	}
	target := fields["target"]
	if target == "" {
		if len(fields) != 1 {
			return "", fmt.Errorf("tagged mapping must contain exactly one entry")
		}
		for key, value := range fields {
			if key == "" || value == "" {
				return "", fmt.Errorf("tagged mapping key and value must be nonempty")
			}
			return key + ": " + value, nil
		}
	}
	for key := range fields {
		if key != "id" && key != "target" && key != "reason" {
			return "", fmt.Errorf("object mapping contains unknown key %q", key)
		}
	}
	if reason := fields["reason"]; reason != "" {
		return target + " -- " + reason, nil
	}
	return target, nil
}

func parseRequirements(node *yaml.Node) (map[string]RequirementGroup, []string, error) {
	if node == nil || node.Kind == 0 {
		return nil, nil, nil
	}

	n := node
	if n.Kind == yaml.DocumentNode && len(n.Content) > 0 {
		n = n.Content[0]
	}
	if n.Kind != yaml.MappingNode {
		return nil, nil, fmt.Errorf("requirements: expected mapping, got kind %d", n.Kind)
	}

	groups := make(map[string]RequirementGroup)
	var orderedKeys []string

	for i := 0; i+1 < len(n.Content); i += 2 {
		keyNode := n.Content[i]
		valNode := n.Content[i+1]
		groupID := keyNode.Value
		orderedKeys = append(orderedKeys, groupID)

		group, err := parseRequirementGroup(groupID, valNode)
		if err != nil {
			return nil, nil, fmt.Errorf("group %s: %w", groupID, err)
		}
		groups[groupID] = group
	}

	return groups, orderedKeys, nil
}

type rawGroup struct {
	Title string    `yaml:"title"`
	Items yaml.Node `yaml:"items"`
}

func parseRequirementGroup(groupID string, node *yaml.Node) (RequirementGroup, error) {
	var rg rawGroup
	if err := node.Decode(&rg); err != nil {
		return RequirementGroup{}, fmt.Errorf("decode group: %w", err)
	}

	items, err := parseRequirementItems(groupID, &rg.Items)
	if err != nil {
		return RequirementGroup{}, err
	}

	return RequirementGroup{Title: rg.Title, Items: items}, nil
}

func parseRequirementItems(groupID string, node *yaml.Node) ([]RequirementItem, error) {
	if node == nil || node.Kind == 0 {
		return nil, nil
	}
	if node.Kind != yaml.SequenceNode {
		return nil, fmt.Errorf("items: expected sequence, got kind %d", node.Kind)
	}

	var items []RequirementItem
	for _, itemNode := range node.Content {
		if itemNode.Kind != yaml.MappingNode {
			return nil, fmt.Errorf("items: expected mapping entry, got kind %d", itemNode.Kind)
		}

		item, err := parseOneItem(itemNode)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, nil
}

func parseOneItem(node *yaml.Node) (RequirementItem, error) {
	var item RequirementItem
	item.Weight = 1

	for i := 0; i+1 < len(node.Content); i += 2 {
		key := node.Content[i].Value
		val := node.Content[i+1]

		switch key {
		case "weight":
			var w int
			if err := val.Decode(&w); err != nil {
				return RequirementItem{}, fmt.Errorf("decode weight: %w", err)
			}
			item.Weight = w
		default:
			item.ID = key
			item.Text = val.Value
		}
	}

	if item.ID == "" {
		return RequirementItem{}, fmt.Errorf("requirement item has no ID key")
	}
	return item, nil
}

// ParseRoadmap reads a road-map.yaml file.
func ParseRoadmap(path string) (Roadmap, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Roadmap{}, fmt.Errorf("read roadmap %s: %w", path, err)
	}
	var rm Roadmap
	if err := yaml.Unmarshal(data, &rm); err != nil {
		return Roadmap{}, fmt.Errorf("parse roadmap %s: %w", path, err)
	}
	return rm, nil
}

// ParseSpecIndex reads a SPECIFICATIONS.yaml file.
func ParseSpecIndex(path string) (SpecIndex, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return SpecIndex{}, fmt.Errorf("read spec index %s: %w", path, err)
	}
	var si SpecIndex
	if err := yaml.Unmarshal(data, &si); err != nil {
		return SpecIndex{}, fmt.Errorf("parse spec index %s: %w", path, err)
	}
	return si, nil
}

// rawUseCase is the intermediate representation for YAML unmarshalling.
type rawUseCase struct {
	ID              string             `yaml:"id"`
	Title           string             `yaml:"title"`
	Summary         string             `yaml:"summary"`
	Actor           string             `yaml:"actor"`
	Trigger         string             `yaml:"trigger"`
	Flow            yaml.Node          `yaml:"flow"`
	Touchpoints     yaml.Node          `yaml:"touchpoints"`
	SuccessCriteria []SuccessCriterion `yaml:"success_criteria"`
	OutOfScope      yaml.Node          `yaml:"out_of_scope"`
	TestSuite       string             `yaml:"test_suite"`
}

// ParseUseCase reads a use case YAML file.
func ParseUseCase(path string) (UseCase, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return UseCase{}, fmt.Errorf("read use case %s: %w", path, err)
	}
	var raw rawUseCase
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return UseCase{}, fmt.Errorf("parse use case %s: %w", path, err)
	}
	flow, err := parseTaggedList(&raw.Flow, "flow")
	if err != nil {
		return UseCase{}, fmt.Errorf("parse use case %s: %w", path, err)
	}
	touchpoints, err := parseTouchpointList(&raw.Touchpoints, "touchpoints")
	if err != nil {
		return UseCase{}, fmt.Errorf("parse use case %s: %w", path, err)
	}
	outOfScope, err := parseTaggedList(&raw.OutOfScope, "out_of_scope")
	if err != nil {
		return UseCase{}, fmt.Errorf("parse use case %s: %w", path, err)
	}
	return UseCase{
		ID:              raw.ID,
		Title:           raw.Title,
		Summary:         raw.Summary,
		Actor:           raw.Actor,
		Trigger:         raw.Trigger,
		Flow:            flow,
		Touchpoints:     touchpoints,
		SuccessCriteria: raw.SuccessCriteria,
		OutOfScope:      outOfScope,
		TestSuite:       raw.TestSuite,
	}, nil
}

// ParseTestSuite reads a test suite YAML file.
func ParseTestSuite(path string) (TestSuite, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return TestSuite{}, fmt.Errorf("read test suite %s: %w", path, err)
	}
	var ts TestSuite
	if err := yaml.Unmarshal(data, &ts); err != nil {
		return TestSuite{}, fmt.Errorf("parse test suite %s: %w", path, err)
	}
	return ts, nil
}
