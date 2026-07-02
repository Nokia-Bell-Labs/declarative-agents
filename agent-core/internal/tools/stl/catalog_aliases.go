// Copyright (c) 2026 Nokia. All rights reserved.

package stl

import "github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/catalog"

const (
	ContractSeverityInfo    = catalog.ContractSeverityInfo
	ContractSeverityWarning = catalog.ContractSeverityWarning
	ContractSeverityError   = catalog.ContractSeverityError
)

const (
	ToolContractStrict   = catalog.ToolContractStrict
	ToolContractMigrated = catalog.ToolContractMigrated
	ToolContractLegacy   = catalog.ToolContractLegacy
)

const (
	ContractAuditComplete = catalog.ContractAuditComplete
	ContractAuditPartial  = catalog.ContractAuditPartial
	ContractAuditMissing  = catalog.ContractAuditMissing
)

type (
	ToolDef                   = catalog.ToolDef
	ToolRequirements          = catalog.ToolRequirements
	ToolOutputContract        = catalog.ToolOutputContract
	ToolSideEffect            = catalog.ToolSideEffect
	ToolSideEffects           = catalog.ToolSideEffects
	ToolReversibility         = catalog.ToolReversibility
	ToolUndoContract          = catalog.ToolUndoContract
	ToolErrorContract         = catalog.ToolErrorContract
	ToolRelationships         = catalog.ToolRelationships
	ToolOverlap               = catalog.ToolOverlap
	ParamMapping              = catalog.ParamMapping
	ToolDefsFile              = catalog.ToolDefsFile
	ToolSelectionFile         = catalog.ToolSelectionFile
	AgentProfile              = catalog.AgentProfile
	ChildAgentConfig          = catalog.ChildAgentConfig
	CheckpointHistoryConfig   = catalog.CheckpointHistoryConfig
	CheckpointRollbackConfig  = catalog.CheckpointRollbackConfig
	LLMToolConfig             = catalog.LLMToolConfig
	LoadSuiteConfig           = catalog.LoadSuiteConfig
	RunPointConfig            = catalog.RunPointConfig
	ServeUIToolConfig         = catalog.ServeUIToolConfig
	ContractValidationOptions = catalog.ContractValidationOptions
	ContractFinding           = catalog.ContractFinding
	ContractAuditEntry        = catalog.ContractAuditEntry
)

var (
	LoadToolSelection                 = catalog.LoadToolSelection
	LoadToolSelections                = catalog.LoadToolSelections
	LoadToolDeclarations              = catalog.LoadToolDeclarations
	LoadToolDeclarationsFromDirs      = catalog.LoadToolDeclarationsFromDirs
	SelectTools                       = catalog.SelectTools
	LoadToolDefs                      = catalog.LoadToolDefs
	ParseToolDefs                     = catalog.ParseToolDefs
	MergeToolDefs                     = catalog.MergeToolDefs
	LoadProfile                       = catalog.LoadProfile
	DecodeToolConfig                  = catalog.DecodeToolConfig
	ValidateChildAgentConfig          = catalog.ValidateChildAgentConfig
	ValidateRunPointConfig            = catalog.ValidateRunPointConfig
	ValidateToolEmits                 = catalog.ValidateToolEmits
	ValidateToolContracts             = catalog.ValidateToolContracts
	AuditToolContracts                = catalog.AuditToolContracts
	ValidateResultSchemaCompatibility = catalog.ValidateResultSchemaCompatibility
)
