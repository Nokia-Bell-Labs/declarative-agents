// Copyright (c) 2026 Nokia. All rights reserved.

package stl

import (
	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/internal/tools/filesystem"
)

type (
	ReadBuilder      = filesystem.ReadBuilder
	WriteBuilder     = filesystem.WriteBuilder
	EditBuilder      = filesystem.EditBuilder
	FindBuilder      = filesystem.FindBuilder
	ListFilesBuilder = filesystem.ListFilesBuilder
)

var (
	ValidatePath      = filesystem.ValidatePath
	RelPath           = filesystem.RelPath
	IsBinary          = filesystem.IsBinary
	ReadToolSpec      = filesystem.ReadToolSpec
	WriteToolSpec     = filesystem.WriteToolSpec
	EditToolSpec      = filesystem.EditToolSpec
	FindToolSpec      = filesystem.FindToolSpec
	ListFilesToolSpec = filesystem.ListFilesToolSpec
)

type workspaceUndoPayload struct {
	WorkspaceRestore struct {
		Paths []string `json:"paths"`
	} `json:"workspace_restore"`
}
