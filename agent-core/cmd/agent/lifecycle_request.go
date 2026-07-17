// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/catalog"
)

// The history and rollback lifecycle families run a fresh single-action machine
// that inspects (or rewrites) a *different* run's persisted checkpoint. Their
// target run id and rollback target iteration are not CLI flags — the design
// keeps lifecycle-specific parameters out of the universal flag set
// (TestRootCommandHasNoLifecycleOnlyFlags) — so they arrive through the
// universal --request file. This file parses that request and routes it two
// ways: it injects the checkpoint id and to_iteration into the checkpoint tool
// configs, and it opens a separate read/revert backend pinned to the target run
// so the inspecting machine never persists over the run it is reading
// (srd009-lifecycle rel02.0-uc002, srd036-dolt-state-persistence R5/R6).

// lifecycleRequest is the checkpoint-operation request payload read from the
// --request file for the history and rollback families.
type lifecycleRequest struct {
	// Checkpoint names the target run branch to inspect or roll back.
	Checkpoint string `yaml:"checkpoint"`
	// ToIteration is the rollback target step; nil means unset.
	ToIteration *int `yaml:"to_iteration"`
}

// loadLifecycleRequest parses the --request file as a lifecycle checkpoint
// request. An empty path yields the zero request (no target, no iteration).
func loadLifecycleRequest(path string) (lifecycleRequest, error) {
	var req lifecycleRequest
	if path == "" {
		return req, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return req, fmt.Errorf("read lifecycle request %q: %w", path, err)
	}
	if err := yaml.Unmarshal(data, &req); err != nil {
		return req, fmt.Errorf("parse lifecycle request %q: %w", path, err)
	}
	return req, nil
}

// defsSelectCheckpointOps reports whether the profile selects a checkpoint
// history or rollback tool, i.e. whether the request-driven target path applies.
func defsSelectCheckpointOps(defs []catalog.ToolDef) bool {
	for _, def := range defs {
		switch def.Init {
		case "checkpoint_history", "checkpoint_rollback":
			return true
		}
	}
	return false
}

// applyLifecycleRequest injects the request's checkpoint id and to_iteration
// into the matching checkpoint tool configs, so the tools select the target run
// and rollback receives its required target iteration without a CLI flag.
func applyLifecycleRequest(defs []catalog.ToolDef, req lifecycleRequest) {
	for i := range defs {
		switch defs[i].Init {
		case "checkpoint_history", "checkpoint_rollback":
			if defs[i].Config == nil {
				defs[i].Config = map[string]interface{}{}
			}
			if req.Checkpoint != "" {
				defs[i].Config["checkpoint"] = req.Checkpoint
			}
			if defs[i].Init == "checkpoint_rollback" && req.ToIteration != nil {
				defs[i].Config["to_iteration"] = *req.ToIteration
			}
		}
	}
}

// resolveLifecycleCheckpoint wires the checkpoint-operation backend for history
// and rollback. When the profile selects a checkpoint op it parses the request,
// injects it into the tool configs, and — when a Dolt DSN and a target run are
// both present — opens a separate backend pinned to that run with no terminal
// merge, so read/rollback touch the target run branch while the inspecting
// machine keeps persisting to its own run through loopCheckpoint. Absent a
// target it returns loopCheckpoint unchanged, preserving prior behavior.
func resolveLifecycleCheckpoint(cfg runtimeConfig, defs []catalog.ToolDef, loopCheckpoint core.Checkpoint) (core.Checkpoint, error) {
	if !defsSelectCheckpointOps(defs) {
		return loopCheckpoint, nil
	}
	req, err := loadLifecycleRequest(cfg.Request)
	if err != nil {
		return nil, err
	}
	applyLifecycleRequest(defs, req)
	if cfg.DoltDSN == "" || req.Checkpoint == "" {
		return loopCheckpoint, nil
	}
	target, err := core.OpenDoltCheckpoint(cfg.DoltDSN, req.Checkpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("open target checkpoint %q: %w", req.Checkpoint, err)
	}
	return target, nil
}
