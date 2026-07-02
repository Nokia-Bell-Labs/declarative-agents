// Copyright (c) 2026 Nokia. All rights reserved.

package stl

import (
	toolvalidation "gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/internal/tools/validation"
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
