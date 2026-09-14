// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

// Package typesys holds named declaration types: the closed schema subset,
// the registry that resolves references between types, and the errors both
// report (srd051).
package typesys

// TypeDecl is one named type declared by a type unit.
type TypeDecl struct {
	Name        string         `yaml:"name" json:"name"`
	Description string         `yaml:"description,omitempty" json:"description,omitempty"`
	Schema      map[string]any `yaml:"schema" json:"schema"`
}

// TypeUnitFile is a declaration file whose top level carries types. Unit and
// Imports follow the srd050 grammar shared with tool and REST units
// (srd051 R1.1, R1.2).
type TypeUnitFile struct {
	Unit    string     `yaml:"unit" json:"unit"`
	Imports []string   `yaml:"imports,omitempty" json:"imports,omitempty"`
	Types   []TypeDecl `yaml:"types" json:"types"`
}

// TypeRefKey is the schema field that addresses another type as unit.Name
// (srd051 R3.1).
const TypeRefKey = "$type"

// Ref returns the qualified address of a type declared by this unit.
func (f TypeUnitFile) Ref(name string) string { return f.Unit + "." + name }
