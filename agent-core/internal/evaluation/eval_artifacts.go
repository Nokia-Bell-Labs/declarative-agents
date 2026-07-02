// Copyright (c) 2026 Nokia. All rights reserved.

package evaluation

// Evaluator artifact file names. These form a contract between the
// evaluator (which writes them) and the bench API (which reads them).
// All artifacts are written under the per-point directory.
const (
	ArtifactTrace      = "trace.ndjson"
	ArtifactMeta       = "meta.json"
	ArtifactExperiment = "experiment.yaml"
	ArtifactDocDir     = "doc"
)
