// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package lifecycle

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/checkpoint"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/catalog"
)

// The history and rollback lifecycle families run a fresh single-action machine
// that inspects (or rewrites) a *different* run's persisted checkpoint. Their
// target run id and rollback target iteration are not CLI flags — the design
// keeps lifecycle-specific parameters out of the universal flag set
// (TestRootCommandHasNoLifecycleOnlyFlags) — so they arrive through the
// universal --request file. Rather than switching on tool identity, each tool
// declares where those values come from with a $request.<field> config source
// (for example checkpoint: $request.checkpoint). This file resolves those
// declared sources generically against the parsed request, and — when a tool
// opts into a checkpoint source and both a Dolt DSN and a target run are
// present — opens a separate read/revert backend pinned to the target run so
// the inspecting machine never persists over the run it is reading
// (srd009-lifecycle rel02.0-uc002, srd036-dolt-state-persistence R5/R6).

// Request is the checkpoint-operation request payload read from the
// --request file for the history and rollback families.
type Request struct {
	// Suite is the universal request file path itself. Evaluator words consume
	// it only when their ToolDef declares $request.suite.
	Suite string `yaml:"-"`
	// Checkpoint names the target run branch to inspect or roll back.
	Checkpoint string `yaml:"checkpoint"`
	// ToIteration names an execution iteration; rollback resolves its last
	// persisted step as the DB rewind boundary. Nil means unset.
	ToIteration *int `yaml:"to_iteration"`
}

// LoadRequest parses the --request file as a lifecycle checkpoint request.
// An empty path yields the zero request (no target, no iteration).
func LoadRequest(path string) (Request, error) {
	var req Request
	if path == "" {
		return req, nil
	}
	req.Suite = path
	data, err := os.ReadFile(path)
	if err != nil {
		return req, fmt.Errorf("read lifecycle request %q: %w", path, err)
	}
	if err := yaml.Unmarshal(data, &req); err != nil {
		return req, fmt.Errorf("parse lifecycle request %q: %w", path, err)
	}
	return req, nil
}

// requestSourcePrefix marks a config value that resolves from the universal
// --request file at composition time, e.g. "$request.checkpoint".
const requestSourcePrefix = "$request."

// requestSourceField returns the request field named by a "$request.<field>"
// config value and whether the value is such a selector.
func requestSourceField(v interface{}) (string, bool) {
	s, ok := v.(string)
	if !ok || !strings.HasPrefix(s, requestSourcePrefix) {
		return "", false
	}
	return strings.TrimPrefix(s, requestSourcePrefix), true
}

// requestSources maps each supported request field to its resolved value and
// whether the field is present; absent fields let a tool keep its config default.
func (r Request) requestSources() map[string]func() (interface{}, bool) {
	return map[string]func() (interface{}, bool){
		"suite":      func() (interface{}, bool) { return r.Suite, r.Suite != "" },
		"checkpoint": func() (interface{}, bool) { return r.Checkpoint, r.Checkpoint != "" },
		"to_iteration": func() (interface{}, bool) {
			if r.ToIteration == nil {
				return nil, false
			}
			return *r.ToIteration, true
		},
	}
}

// DefsDeclareRequestSources reports whether any selected tool declares a
// $request.<field> config source, i.e. whether the request-driven path applies.
func DefsDeclareRequestSources(defs []catalog.ToolDef) bool {
	for i := range defs {
		for _, v := range defs[i].Config {
			if _, ok := requestSourceField(v); ok {
				return true
			}
		}
	}
	return false
}

// ResolvedSource is provenance for one $request.<field> config substitution.
type ResolvedSource struct {
	Tool, Config, Field string
}

// ResolveRequestSources replaces each declared $request.<field> config value
// before typed config decode and returns provenance for composition-root wiring.
// Unset fields are deleted so defaults apply; unknown fields fail as typos.
func ResolveRequestSources(defs []catalog.ToolDef, req Request) ([]ResolvedSource, error) {
	sources := req.requestSources()
	var resolved []ResolvedSource
	for i := range defs {
		for key, v := range defs[i].Config {
			field, ok := requestSourceField(v)
			if !ok {
				continue
			}
			resolve, known := sources[field]
			if !known {
				return nil, fmt.Errorf("tool %q config %q references unknown request source %s%s",
					defs[i].Name, key, requestSourcePrefix, field)
			}
			if value, present := resolve(); present {
				defs[i].Config[key] = value
				resolved = append(resolved, ResolvedSource{
					Tool: defs[i].Name, Config: key, Field: field,
				})
			} else {
				delete(defs[i].Config, key)
			}
		}
	}
	return resolved, nil
}

// ValidateDeclaredRequestSources loads the --request file at requestPath and
// resolves declared $request sources on defs. requestPath is the only
// runtimeConfig field the former composition-root helper read.
func ValidateDeclaredRequestSources(requestPath string, defs []catalog.ToolDef) error {
	if !DefsDeclareRequestSources(defs) {
		return nil
	}
	request, err := LoadRequest(requestPath)
	if err != nil {
		return err
	}
	_, err = ResolveRequestSources(defs, request)
	return err
}

func resolvesCheckpointTarget(resolved []ResolvedSource) bool {
	for _, source := range resolved {
		if source.Config == "checkpoint" && source.Field == "checkpoint" {
			return true
		}
	}
	return false
}

// ResolveCheckpoint wires the checkpoint-operation backend for history and
// rollback. Request selectors are resolved generically for every selected
// tool, but a separate backend opens only when resolution provenance says a
// config named checkpoint consumed $request.checkpoint. Unrelated request
// sources therefore cannot activate lifecycle backend wiring.
func ResolveCheckpoint(requestPath, doltDSN string, defs []catalog.ToolDef, loopCheckpoint core.Checkpoint) (checkpoint.Opened, error) {
	if !DefsDeclareRequestSources(defs) {
		return checkpoint.Opened{Checkpoint: loopCheckpoint}, nil
	}
	req, err := LoadRequest(requestPath)
	if err != nil {
		return checkpoint.Opened{}, err
	}
	resolved, err := ResolveRequestSources(defs, req)
	if err != nil {
		return checkpoint.Opened{}, err
	}
	if doltDSN == "" || req.Checkpoint == "" || !resolvesCheckpointTarget(resolved) {
		return checkpoint.Opened{Checkpoint: loopCheckpoint}, nil
	}
	target, err := checkpoint.OpenDolt(doltDSN, req.Checkpoint, nil)
	if err != nil {
		return checkpoint.Opened{}, fmt.Errorf("open target checkpoint %q: %w", req.Checkpoint, err)
	}
	return checkpoint.Opened{
		Checkpoint: target,
		CloseFunc:  target.Close,
		Label:      fmt.Sprintf("target checkpoint %q", req.Checkpoint),
	}, nil
}
