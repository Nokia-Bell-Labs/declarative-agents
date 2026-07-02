// Copyright (c) 2026 Nokia. All rights reserved.

package stl

import (
	toolvalidation "github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/validation"
)

type (
	ToolTracker          = toolvalidation.ToolTracker
	ValidateBuilder      = toolvalidation.ValidateBuilder
	ValidateSpecState    = toolvalidation.SpecState
	LoadCorpusBuilder    = toolvalidation.LoadCorpusBuilder
	ValidateSpecsBuilder = toolvalidation.ValidateSpecsBuilder
	FormatReportBuilder  = toolvalidation.FormatReportBuilder
)

var (
	NewToolTracker            = toolvalidation.NewToolTracker
	ValidateToolSpec          = toolvalidation.ValidateToolSpec
	RegisterValidateFactories = toolvalidation.RegisterSpecFactories
)
