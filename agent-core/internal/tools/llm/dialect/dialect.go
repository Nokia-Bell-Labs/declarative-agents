// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

// Package dialect loads a provider library's chat-dialect.yaml, the file that
// fills invoke_llm's provider-specific steps: how the request body is built,
// where the reply text and usage sit in the response, which statuses are
// provider failures, and how the call authenticates (srd058 R2). The body,
// failure map, and auth profile use the REST format's grammar, so a dialect
// is checked by the rules a REST operation is held to.
package dialect

import (
	"fmt"
	"os"
	"regexp"
	"strings"

	modelllm "github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/model/llm"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/support/yamlstrict"
	restdef "github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/rest/definition"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/rest/validation"
)

// FileName is the chat dialect's name inside a provider library (srd058 R1.3).
const FileName = "chat-dialect.yaml"

// The shared failure vocabulary a provider operation emits (srd058 R1.4).
const (
	ProviderUnauthorized = "ProviderUnauthorized"
	ProviderThrottled    = "ProviderThrottled"
	ProviderUnavailable  = "ProviderUnavailable"
)

var failureSignals = map[string]bool{
	ProviderUnauthorized: true, ProviderThrottled: true, ProviderUnavailable: true,
}

var (
	unitName          = regexp.MustCompile(`^[a-z][a-z0-9]*(?:-[a-z0-9]+)*$`)
	credentialRefName = regexp.MustCompile(`^[A-Z_][A-Z0-9_]*$`)
)

// Chat is one decoded chat dialect.
type Chat struct {
	Unit string `yaml:"unit"`
	// ProviderName is the gen_ai.provider.name the inference span carries.
	ProviderName string                  `yaml:"provider_name"`
	Method       string                  `yaml:"method,omitempty"`
	Path         string                  `yaml:"path"`
	Auth         restdef.AuthProfile     `yaml:"auth"`
	Body         map[string]interface{}  `yaml:"body"`
	Response     Response                `yaml:"response"`
	Failures     []restdef.StatusMapping `yaml:"failures"`
	// ParserProfile names the response parser profile; empty keeps the
	// model-based resolution invoke_llm applies today.
	ParserProfile string `yaml:"parser_profile,omitempty"`
	// ParserProfiles are the library's own parser profiles, consulted before
	// the embedded registry, so a provider ships its parser as YAML
	// (srd058 R3.4).
	ParserProfiles []modelllm.ProfileSpec `yaml:"parser_profiles,omitempty"`
}

// Response names where the reply sits in a decoded response body.
type Response struct {
	Text string `yaml:"text"`
	// TextRequired makes a reply with no text a failure rather than an empty
	// answer.
	TextRequired bool   `yaml:"text_required,omitempty"`
	InputTokens  string `yaml:"input_tokens,omitempty"`
	OutputTokens string `yaml:"output_tokens,omitempty"`
}

// Load reads and validates the dialect at path.
func Load(path string) (Chat, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Chat{}, fmt.Errorf("chat dialect %s: %w", path, err)
	}
	chat, err := Parse(data)
	if err != nil {
		return Chat{}, fmt.Errorf("chat dialect %s: %w", path, err)
	}
	return chat, nil
}

// Parse strictly decodes and validates dialect bytes.
func Parse(data []byte) (Chat, error) {
	var chat Chat
	if err := yamlstrict.Unmarshal(data, &chat); err != nil {
		return Chat{}, err
	}
	if chat.Method == "" {
		chat.Method = "POST"
	}
	return chat, chat.validate()
}

func (c Chat) validate() error {
	if !unitName.MatchString(c.Unit) {
		return fmt.Errorf("unit %q is not a lowercase kebab name", c.Unit)
	}
	if strings.TrimSpace(c.ProviderName) == "" {
		return fmt.Errorf("provider_name is required")
	}
	if !strings.HasPrefix(c.Path, "/") {
		return fmt.Errorf("path %q must start with /", c.Path)
	}
	if c.Method != "POST" {
		return fmt.Errorf("method %q is not supported; a chat call is a POST", c.Method)
	}
	if err := validateAuth(c.Auth); err != nil {
		return err
	}
	if err := validateBody(c.Body); err != nil {
		return err
	}
	if err := c.Response.validate(); err != nil {
		return err
	}
	if _, err := c.Profiles(); err != nil {
		return err
	}
	return c.validateFailures()
}

// Profiles returns the parser profile registry this dialect resolves against:
// the library's profiles ahead of the embedded ones. A named parser_profile
// must be in it.
func (c Chat) Profiles() (*modelllm.ProfileRegistry, error) {
	embedded, err := modelllm.DefaultProfileRegistry()
	if err != nil {
		return nil, fmt.Errorf("load embedded parser profiles: %w", err)
	}
	registry, err := embedded.WithLibrary(c.Unit, c.ParserProfiles)
	if err != nil {
		return nil, err
	}
	if c.ParserProfile != "" {
		if _, ok := registry.ResolveProfileName(c.ParserProfile); !ok {
			return nil, fmt.Errorf("parser_profile %q is neither a library nor an embedded profile", c.ParserProfile)
		}
	}
	return registry, nil
}

// validateAuth accepts the REST auth grammar and additionally requires every
// credential to be a reference by environment-variable name, never a value
// (srd058 R1.5).
func validateAuth(auth restdef.AuthProfile) error {
	if auth.Type == "" {
		return fmt.Errorf("auth.type is required; declare none when the provider takes no credential")
	}
	if err := validation.ValidateAuthProfile("dialect", auth); err != nil {
		return err
	}
	refs := map[string]string{
		"token_ref": auth.TokenRef, "username_ref": auth.UsernameRef, "password_ref": auth.PasswordRef,
	}
	for field, ref := range refs {
		if ref != "" && !credentialRefName.MatchString(ref) {
			return fmt.Errorf("auth.%s %q is not an environment variable name; a dialect names credentials, never carries them", field, ref)
		}
	}
	switch auth.Type {
	case "none":
		if auth.TokenRef != "" || auth.UsernameRef != "" || auth.PasswordRef != "" {
			return fmt.Errorf("auth type none names a credential reference")
		}
	case "basic":
		if auth.UsernameRef == "" || auth.PasswordRef == "" {
			return fmt.Errorf("auth type basic requires username_ref and password_ref")
		}
	default:
		if auth.TokenRef == "" {
			return fmt.Errorf("auth type %s requires token_ref", auth.Type)
		}
	}
	return nil
}

func (r Response) validate() error {
	if r.Text == "" {
		return fmt.Errorf("response.text is required")
	}
	for field, selector := range map[string]string{
		"text": r.Text, "input_tokens": r.InputTokens, "output_tokens": r.OutputTokens,
	} {
		if selector == "" {
			continue
		}
		if _, err := parseSelector(selector); err != nil {
			return fmt.Errorf("response.%s: %w", field, err)
		}
	}
	return nil
}

// validateFailures holds the failure map to the REST status rules and to the
// shared vocabulary: each failure is a 4xx or 5xx status mapped once to one of
// the three provider failure signals.
func (c Chat) validateFailures() error {
	if len(c.Failures) == 0 {
		return fmt.Errorf("failures is required; map provider error statuses to the shared failure signals")
	}
	for index, failure := range c.Failures {
		if !failureSignals[failure.Signal] {
			return fmt.Errorf("failures[%d] signal %q is not one of %s, %s, %s",
				index, failure.Signal, ProviderUnauthorized, ProviderThrottled, ProviderUnavailable)
		}
		if len(failure.Status) == 0 {
			return fmt.Errorf("failures[%d] names no status", index)
		}
		for _, status := range failure.Status {
			if status < 400 || status > 599 {
				return fmt.Errorf("failures[%d] status %d is not an error status", index, status)
			}
		}
	}
	operation := restdef.Operation{
		Success:  restdef.StatusMapping{Status: []int{200}, Signal: "LLMResponded"},
		Failures: c.Failures,
	}
	return validation.ValidateStatusMappings("dialect", operation)
}

// FailureSignal returns the failure signal status maps to, or "" when the
// dialect does not map it.
func (c Chat) FailureSignal(status int) string {
	for _, failure := range c.Failures {
		for _, mapped := range failure.Status {
			if mapped == status {
				return failure.Signal
			}
		}
	}
	return ""
}
