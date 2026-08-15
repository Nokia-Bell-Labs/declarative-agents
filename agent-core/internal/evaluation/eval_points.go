// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package evaluation

import (
	"encoding/json"
	"fmt"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
)

// evalPointInput is the serializable input for one declared for_each dispatch.
type evalPointInput struct {
	PointID     string    `json:"point_id"`
	Sample      Sample    `json:"sample"`
	Harness     Harness   `json:"harness"`
	Model       string    `json:"model"`
	ProfilePath string    `json:"profile_path"`
	GridPoint   GridPoint `json:"grid_point"`
	Rep         int       `json:"rep"`
}

// MaterializeEvalPointsBuilder creates the complete deterministic point collection.
type MaterializeEvalPointsBuilder struct {
	ES *EvalSessionState
}

func (b *MaterializeEvalPointsBuilder) Build(_ core.Result) core.Command {
	return &materializeEvalPointsCmd{es: b.ES}
}

type materializeEvalPointsCmd struct {
	es *EvalSessionState
}

func (c *materializeEvalPointsCmd) Name() string                   { return "materialize_eval_points" }
func (c *materializeEvalPointsCmd) Undo(_ core.Result) core.Result { return core.NoopUndo(c.Name()) }

func (c *materializeEvalPointsCmd) Execute() core.Result {
	if c.es == nil {
		return pointToolError(c.Name(), fmt.Errorf("evaluation session not initialized"))
	}
	points := make([]evalPointInput, 0,
		len(c.es.Suite.Profiles)*len(c.es.gridPoints)*len(c.es.Suite.Samples)*c.es.reps)
	for _, profile := range c.es.Suite.Profiles {
		for _, gridPoint := range c.es.gridPoints {
			for _, sample := range c.es.Suite.Samples {
				for rep := 0; rep < c.es.reps; rep++ {
					points = append(points, evalPointInput{
						PointID:     EvalPointID(sample.Name, profile.Name, profile.Model, gridPoint, rep),
						Sample:      sample,
						Harness:     Harness{Name: profile.Name, Binary: profile.Binary},
						Model:       profile.Model,
						ProfilePath: profile.Path,
						GridPoint:   gridPoint,
						Rep:         rep,
					})
				}
			}
		}
	}
	output, err := json.Marshal(map[string]any{"points": points})
	if err != nil {
		return pointToolError(c.Name(), fmt.Errorf("encode evaluation points: %w", err))
	}
	return pointToolDone(c.Name(), SigEvalPointsMaterialized, string(output))
}
