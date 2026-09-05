// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package pipeline

import (
	"fmt"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/catalog"
	toolregistry "github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/registry"
)

const InitPipeline = "pipeline"

// StageInits is the closed stage vocabulary (srd049 R2.1): the pure data
// transforms. Boundary, lifecycle, and branching inits are excluded by
// construction -- a word that leaves the process, changes run state, or
// chooses a path is a state the machine must own. Extending this set amends
// srd049 R2.
var StageInits = map[string]bool{
	"compose":          true,
	"render_each":      true,
	"project":          true,
	"normalize_vector": true,
	"flat_map":         true,
	"reorder_by_index": true,
	"parse_structured": true,
	"select_subset":    true,
}

// Config is the pipeline word's declared configuration (srd049 R1.2).
type Config struct {
	Signal string        `json:"signal"`
	Stages []StageConfig `json:"stages"`
}

// StageConfig names one stage's init and carries the config that init's
// standalone word takes, verbatim (srd049 R1.2, R1.3).
type StageConfig struct {
	Name   string                 `json:"name,omitempty"`
	Init   string                 `json:"init"`
	Config map[string]interface{} `json:"config,omitempty"`
}

// RegisterFactories registers the pipeline factory. The factory resolves
// stage factories through the same registry it is registered into, which by
// word-registration time holds every selected family (srd049 R2.3).
func RegisterFactories(br *toolregistry.BuiltinRegistry) {
	br.Register(InitPipeline, factory(br))
}

func factory(br *toolregistry.BuiltinRegistry) toolregistry.BuiltinFactory {
	return func(def catalog.ToolDef, vars map[string]string) (core.Builder, error) {
		var cfg Config
		if err := catalog.DecodeToolConfig(def, &cfg); err != nil {
			return nil, err
		}
		if cfg.Signal == "" {
			return nil, fmt.Errorf("pipeline %q: a signal is required", def.Name)
		}
		if len(cfg.Stages) == 0 {
			return nil, fmt.Errorf("pipeline %q: at least one stage is required", def.Name)
		}
		stages, err := buildStages(br, def, cfg, vars)
		if err != nil {
			return nil, err
		}
		return Builder{ToolName: def.Name, Signal: core.Signal(cfg.Signal), Stages: stages}, nil
	}
}

// buildStages builds every stage eagerly (srd049 R1.5): a config the
// standalone word would refuse is refused here, at registration, with the
// pipeline and the stage named.
func buildStages(
	br *toolregistry.BuiltinRegistry,
	def catalog.ToolDef,
	cfg Config,
	vars map[string]string,
) ([]stage, error) {
	stages := make([]stage, 0, len(cfg.Stages))
	for i, sc := range cfg.Stages {
		name := sc.Name
		if name == "" {
			name = fmt.Sprintf("%d-%s", i+1, sc.Init)
		}
		if !StageInits[sc.Init] {
			return nil, fmt.Errorf(
				"pipeline %q stage %s: init %q is not a pipeline stage; the set is closed (srd049 R2.1)",
				def.Name, name, sc.Init,
			)
		}
		stageFactory, ok := br.Resolve(sc.Init)
		if !ok {
			return nil, fmt.Errorf(
				"pipeline %q stage %s: init %q has no registered factory", def.Name, name, sc.Init,
			)
		}
		builder, err := stageFactory(stageDef(def, name, sc), vars)
		if err != nil {
			return nil, fmt.Errorf("pipeline %q stage %s: %w", def.Name, name, err)
		}
		stages = append(stages, stage{name: name, builder: builder})
	}
	return stages, nil
}

// stageDef projects one stage into the ToolDef its factory expects: the
// stage's config under a name that identifies the pipeline and the stage.
func stageDef(def catalog.ToolDef, name string, sc StageConfig) catalog.ToolDef {
	return catalog.ToolDef{
		Name:   fmt.Sprintf("%s.%s", def.Name, name),
		Type:   "builtin",
		Init:   sc.Init,
		Config: sc.Config,
	}
}
