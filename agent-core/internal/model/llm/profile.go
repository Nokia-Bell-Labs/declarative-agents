// Copyright (c) 2026 Nokia. All rights reserved.

package llm

import (
	"embed"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/model/prompt"
)

//go:embed profiles/*.yaml
var embeddedProfiles embed.FS

// --- YAML schema types ---

// ProfileSpec is the deserialized form of a profile YAML file.
type ProfileSpec struct {
	ProfileName   string         `yaml:"name"`
	MatchPrefixes []string       `yaml:"match_prefixes"`
	MachineName   string         `yaml:"machine,omitempty"`
	Envelope      *EnvelopeSpec  `yaml:"envelope"`
	StrictFormat  bool           `yaml:"strict_format"`
	Pipeline      []PipelineStep `yaml:"extraction_pipeline"`
}

// EnvelopeSpec defines envelope tags in YAML. A nil pointer (YAML null)
// means the model uses bare JSON with no wrapping tags.
type EnvelopeSpec struct {
	Open  string `yaml:"open"`
	Close string `yaml:"close"`
}

// PipelineStep is a single extraction step. YAML allows both a bare
// string ("strip_code_fences") and a map with parameters
// ("extract_envelope: {open: ..., close: ...}").
type PipelineStep struct {
	Name   string
	Params map[string]string
}

func (p *PipelineStep) UnmarshalYAML(value *yaml.Node) error {
	switch value.Kind {
	case yaml.ScalarNode:
		p.Name = value.Value
		return nil
	case yaml.MappingNode:
		if len(value.Content) < 2 {
			return fmt.Errorf("pipeline step map must have exactly one key")
		}
		p.Name = value.Content[0].Value
		p.Params = make(map[string]string)
		inner := value.Content[1]
		if inner.Kind == yaml.MappingNode {
			for i := 0; i < len(inner.Content)-1; i += 2 {
				p.Params[inner.Content[i].Value] = inner.Content[i+1].Value
			}
		}
		return nil
	default:
		return fmt.Errorf("pipeline step must be a string or map, got %v", value.Kind)
	}
}

// --- Profile registry ---

// ProfileRegistry holds loaded profiles and resolves them by model name.
type ProfileRegistry struct {
	profiles    []ProfileSpec
	defaultSpec ProfileSpec
}

// addProfileEntry unmarshals a single YAML profile and adds it to the registry.
func addProfileEntry(reg *ProfileRegistry, filename string, data []byte) error {
	var spec ProfileSpec
	if err := yaml.Unmarshal(data, &spec); err != nil {
		return fmt.Errorf("parse profile %s: %w", filename, err)
	}
	if spec.ProfileName == "" {
		return fmt.Errorf("profile %s: missing 'name' field", filename)
	}
	if spec.ProfileName == "default" || len(spec.MatchPrefixes) == 0 {
		reg.defaultSpec = spec
	} else {
		reg.profiles = append(reg.profiles, spec)
	}
	return nil
}

// LoadProfiles reads all .yaml files from a directory and returns a registry.
func LoadProfiles(dir string) (*ProfileRegistry, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read profiles dir %s: %w", dir, err)
	}

	reg := &ProfileRegistry{}
	for _, e := range entries {
		if e.IsDir() || (!strings.HasSuffix(e.Name(), ".yaml") && !strings.HasSuffix(e.Name(), ".yml")) {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			return nil, fmt.Errorf("read profile %s: %w", e.Name(), err)
		}
		if err := addProfileEntry(reg, e.Name(), data); err != nil {
			return nil, err
		}
	}

	if reg.defaultSpec.ProfileName == "" {
		return nil, fmt.Errorf("profiles dir %s: no default profile found (need a file with name: default or empty match_prefixes)", dir)
	}

	return reg, nil
}

// LoadProfilesFromBytes creates a registry from raw YAML byte slices
// (typically from go:embed). Each slice is one profile file.
func LoadProfilesFromBytes(files map[string][]byte) (*ProfileRegistry, error) {
	reg := &ProfileRegistry{}
	for name, data := range files {
		if err := addProfileEntry(reg, name, data); err != nil {
			return nil, err
		}
	}
	if reg.defaultSpec.ProfileName == "" {
		return nil, fmt.Errorf("no default profile found in embedded profiles")
	}
	return reg, nil
}

// DefaultProfileRegistry creates a registry from the embedded profile
// YAML files shipped with agent-core. This is the standard way for
// agents to obtain a fully populated ProfileRegistry without loading
// files from disk at runtime.
func DefaultProfileRegistry() (*ProfileRegistry, error) {
	return LoadProfilesFromFS(embeddedProfiles)
}

// LoadProfilesFromFS walks an fs.FS looking for .yaml files and loads
// them into a ProfileRegistry. The FS must contain the files under
// "profiles/" (matching the go:embed layout).
func LoadProfilesFromFS(fsys fs.FS) (*ProfileRegistry, error) {
	reg := &ProfileRegistry{}
	err := fs.WalkDir(fsys, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if !strings.HasSuffix(d.Name(), ".yaml") && !strings.HasSuffix(d.Name(), ".yml") {
			return nil
		}
		data, readErr := fs.ReadFile(fsys, path)
		if readErr != nil {
			return fmt.Errorf("read profile %s: %w", path, readErr)
		}
		return addProfileEntry(reg, path, data)
	})
	if err != nil {
		return nil, err
	}
	if reg.defaultSpec.ProfileName == "" {
		return nil, fmt.Errorf("no default profile found in FS")
	}
	return reg, nil
}

// ResolveProfile returns a ResponseParser for the given model name by
// matching against loaded profile prefixes. Falls back to default.
func (r *ProfileRegistry) ResolveProfile(model string) ResponseParser {
	lower := strings.ToLower(model)
	for _, spec := range r.profiles {
		for _, prefix := range spec.MatchPrefixes {
			if strings.HasPrefix(lower, strings.ToLower(prefix)) {
				return newYAMLProfile(spec)
			}
		}
	}
	return newYAMLProfile(r.defaultSpec)
}

// ResolveProfileName returns a ResponseParser for a named parser profile.
func (r *ProfileRegistry) ResolveProfileName(name string) (ResponseParser, bool) {
	if name == r.defaultSpec.ProfileName {
		return newYAMLProfile(r.defaultSpec), true
	}
	for _, spec := range r.profiles {
		if spec.ProfileName == name {
			return newYAMLProfile(spec), true
		}
	}
	return nil, false
}

// ResolveProfileSpec returns the ProfileSpec for the given model.
// Use this when you need the spec itself (e.g. for Machine()).
func (r *ProfileRegistry) ResolveProfileSpec(model string) ProfileSpec {
	lower := strings.ToLower(model)
	for _, spec := range r.profiles {
		for _, prefix := range spec.MatchPrefixes {
			if strings.HasPrefix(lower, strings.ToLower(prefix)) {
				return spec
			}
		}
	}
	return r.defaultSpec
}

// ProfileNames returns the names of all loaded profiles.
func (r *ProfileRegistry) ProfileNames() []string {
	names := make([]string, 0, len(r.profiles)+1)
	names = append(names, r.defaultSpec.ProfileName)
	for _, p := range r.profiles {
		names = append(names, p.ProfileName)
	}
	return names
}

// DefaultProfile creates a ResponseParser with default settings (used
// as fallback when no profile registry is available). These values
// mirror profiles/default.yaml; keep them in sync.
func DefaultProfile() ResponseParser {
	return newYAMLProfile(ProfileSpec{
		ProfileName:  "default",
		Envelope:     &EnvelopeSpec{Open: "[tool_call]", Close: "[/tool_call]"},
		StrictFormat: false,
		Pipeline: []PipelineStep{
			{Name: "extract_envelope", Params: map[string]string{"open": "[tool_call]", "close": "[/tool_call]"}},
		},
	})
}

// --- YAML-backed ResponseParser implementation ---

// yamlProfile implements ResponseParser using a ProfileSpec loaded from YAML.
type yamlProfile struct {
	spec     ProfileSpec
	pipeline []extractionStep
}

type extractionStep func(string) string

func newYAMLProfile(spec ProfileSpec) *yamlProfile {
	p := &yamlProfile{spec: spec}
	p.pipeline = buildPipeline(spec.Pipeline)
	return p
}

func (p *yamlProfile) Name() string    { return p.spec.ProfileName }
func (p *yamlProfile) Machine() string { return p.spec.MachineName }

func (p *yamlProfile) EnvelopeConfig() (*prompt.Envelope, bool) {
	if p.spec.Envelope == nil {
		return nil, p.spec.StrictFormat
	}
	return &prompt.Envelope{
		Open:  p.spec.Envelope.Open,
		Close: p.spec.Envelope.Close,
	}, p.spec.StrictFormat
}

func (p *yamlProfile) ExtractToolCall(raw string) string {
	s := raw
	for _, step := range p.pipeline {
		s = step(s)
	}
	return s
}

// buildPipeline converts YAML pipeline step definitions into executable
// functions.
func buildPipeline(steps []PipelineStep) []extractionStep {
	var pipeline []extractionStep
	for _, step := range steps {
		switch step.Name {
		case "strip_code_fences":
			pipeline = append(pipeline, StripCodeFences)
		case "strip_thinking_blocks":
			pipeline = append(pipeline, StripThinkingBlocks)
		case "extract_envelope":
			open := step.Params["open"]
			close_ := step.Params["close"]
			pipeline = append(pipeline, func(s string) string {
				return ExtractWithEnvelope(s, open, close_)
			})
		case "extract_native_token":
			token := step.Params["token"]
			if token == "" {
				token = step.Name
				for _, v := range step.Params {
					token = v
					break
				}
			}
			pipeline = append(pipeline, MakeNativeTokenExtractor(token))
		case "extract_braces":
			pipeline = append(pipeline, ExtractBraces)
		}
	}
	return pipeline
}

// --- extraction functions ---

// StripCodeFences removes markdown ```...``` code fences, keeping only
// the content between the first opening and its matching close.
func StripCodeFences(raw string) string {
	s := strings.TrimSpace(raw)

	const fence = "```"
	start := strings.Index(s, fence)
	if start < 0 {
		return s
	}

	afterOpen := s[start+len(fence):]
	if nl := strings.Index(afterOpen, "\n"); nl >= 0 {
		afterOpen = afterOpen[nl+1:]
	}

	if end := strings.Index(afterOpen, fence); end >= 0 {
		return strings.TrimSpace(afterOpen[:end])
	}
	return strings.TrimSpace(afterOpen)
}

// ExtractWithEnvelope tries to extract JSON from between open/close
// tags. Falls back to brace extraction if no envelope is found.
func ExtractWithEnvelope(raw, openTag, closeTag string) string {
	s := strings.TrimSpace(raw)

	if start := strings.Index(s, openTag); start >= 0 {
		inner := s[start+len(openTag):]
		if end := strings.Index(inner, closeTag); end >= 0 {
			return strings.TrimSpace(inner[:end])
		}
		return ExtractBraces(inner)
	}

	return ExtractBraces(s)
}

// StripThinkingBlocks removes <think>...</think> and
// <thinking>...</thinking> blocks that thinking-mode models prepend
// to their output.
func StripThinkingBlocks(raw string) string {
	s := raw
	for _, tag := range []string{"think", "thinking"} {
		open := "<" + tag + ">"
		close := "</" + tag + ">"
		for {
			start := strings.Index(s, open)
			if start < 0 {
				break
			}
			end := strings.Index(s[start:], close)
			if end < 0 {
				s = s[:start]
				break
			}
			s = s[:start] + s[start+end+len(close):]
		}
	}
	return strings.TrimSpace(s)
}

// ExtractBraces isolates JSON by finding the first '{' and last '}'.
func ExtractBraces(s string) string {
	s = strings.TrimSpace(s)
	if idx := strings.Index(s, "{"); idx > 0 {
		s = s[idx:]
	}
	if idx := strings.LastIndex(s, "}"); idx >= 0 && idx < len(s)-1 {
		s = s[:idx+1]
	}
	return s
}

// MakeNativeTokenExtractor builds an extraction step for model-native
// tokens like Gemma's <tool_call|>. If the token is found, extracts
// braces from the text preceding it.
func MakeNativeTokenExtractor(token string) extractionStep {
	return func(s string) string {
		if idx := strings.Index(s, token); idx >= 0 {
			return ExtractBraces(s[:idx])
		}
		return s
	}
}

// Verify yamlProfile satisfies ResponseParser at compile time.
var _ ResponseParser = (*yamlProfile)(nil)
