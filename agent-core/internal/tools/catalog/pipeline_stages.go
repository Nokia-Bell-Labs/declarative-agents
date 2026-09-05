// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package catalog

// PipelineStageInits returns the stage inits a pipeline declaration names,
// so profile selection can co-select their factory families (srd049 R2.3).
// It reads the raw config leniently: selection runs before decode
// validation, and a malformed pipeline fails registration with its own
// error later.
func PipelineStageInits(def ToolDef) []string {
	if def.Init != "pipeline" || def.Config == nil {
		return nil
	}
	raw, ok := def.Config["stages"].([]interface{})
	if !ok {
		return nil
	}
	inits := []string{}
	for _, entry := range raw {
		mapping, ok := entry.(map[string]interface{})
		if !ok {
			continue
		}
		if init, ok := mapping["init"].(string); ok && init != "" {
			inits = append(inits, init)
		}
	}
	return inits
}
