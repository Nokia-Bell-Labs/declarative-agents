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
	ID                 string               `yaml:"id"`
	Title              string               `yaml:"title"`
	Problem            string               `yaml:"problem"`
	Goals              yaml.Node            `yaml:"goals"`
	Requirements       yaml.Node            `yaml:"requirements"`
	NonGoals           yaml.Node            `yaml:"non_goals"`
	AcceptanceCriteria []AcceptanceCriterion `yaml:"acceptance_criteria"`
	DependsOn          []Dependency         `yaml:"depends_on"`
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

	goals := parseTaggedList(&raw.Goals)
	nonGoals := parseTaggedList(&raw.NonGoals)

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
//	- G1: Some text.
//	- G2: More text.
func parseTaggedList(node *yaml.Node) []string {
	if node == nil || node.Kind == 0 {
		return nil
	}
	n := node
	if n.Kind == yaml.DocumentNode && len(n.Content) > 0 {
		n = n.Content[0]
	}
	if n.Kind != yaml.SequenceNode {
		return nil
	}

	var result []string
	for _, item := range n.Content {
		switch item.Kind {
		case yaml.ScalarNode:
			result = append(result, item.Value)
		case yaml.MappingNode:
			for i := 0; i+1 < len(item.Content); i += 2 {
				k := item.Content[i].Value
				v := item.Content[i+1].Value
				result = append(result, k+": "+v)
			}
		}
	}
	return result
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
	return UseCase{
		ID:              raw.ID,
		Title:           raw.Title,
		Summary:         raw.Summary,
		Actor:           raw.Actor,
		Trigger:         raw.Trigger,
		Flow:            parseTaggedList(&raw.Flow),
		Touchpoints:     parseTaggedList(&raw.Touchpoints),
		SuccessCriteria: raw.SuccessCriteria,
		OutOfScope:      parseTaggedList(&raw.OutOfScope),
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
