// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package main

import (
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// renderKeynameTable produces a markdown keyname table from a live schema
// source in the repository. kind "yaml" reads a config-format specification's
// schema list; kind "go" reads the yaml tags of the named struct. Either way
// the table regenerates on every render, so it cannot lag the schema it
// documents.
func renderKeynameTable(repositoryRoot, kind, source, typeName string) (string, error) {
	root, err := filepath.Abs(repositoryRoot)
	if err != nil {
		return "", fmt.Errorf("resolve repository root: %w", err)
	}
	path := filepath.Join(root, filepath.Clean(filepath.FromSlash(source)))
	rel, err := filepath.Rel(root, path)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("schema source %q escapes the repository", source)
	}
	var rows [][3]string
	switch kind {
	case "yaml":
		rows, err = yamlSchemaRows(path)
	case "go":
		if typeName == "" {
			return "", errors.New("a go source requires #TypeName")
		}
		rows, err = goStructRows(path, typeName)
	default:
		return "", fmt.Errorf("unknown keyname-table kind %q", kind)
	}
	if err != nil {
		return "", err
	}
	if len(rows) == 0 {
		return "", fmt.Errorf("schema source %q yields no keynames", source)
	}

	var table strings.Builder
	table.WriteString("| Keyname | Type | Required |\n|---|---|---|\n")
	for _, row := range rows {
		fmt.Fprintf(&table, "| `%s` | %s | %s |\n", row[0], row[1], row[2])
	}
	fmt.Fprintf(&table, "\nGenerated at render time from [`%s`](../%s).", source, source)
	return table.String(), nil
}

// yamlSchemaRows reads a config-format specification and returns one row per
// top-level schema entry.
func yamlSchemaRows(path string) ([][3]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var format struct {
		Schema []struct {
			Field    string `yaml:"field"`
			Type     string `yaml:"type"`
			Required any    `yaml:"required"`
		} `yaml:"schema"`
	}
	if err := yaml.Unmarshal(data, &format); err != nil {
		return nil, fmt.Errorf("parse schema source: %w", err)
	}
	var rows [][3]string
	for _, entry := range format.Schema {
		if entry.Field == "" {
			continue
		}
		// required is usually a boolean, but some schema entries carry a
		// conditional requirement as prose, which the table shows verbatim.
		required := "no"
		switch value := entry.Required.(type) {
		case bool:
			if value {
				required = "yes"
			}
		case string:
			required = value
		}
		rows = append(rows, [3]string{entry.Field, entry.Type, required})
	}
	return rows, nil
}

// goStructRows parses a Go source file and returns one row per yaml-tagged
// field of the named struct. A field without omitempty reads as required,
// which matches how the loaders treat their manifests.
func goStructRows(path, typeName string) ([][3]string, error) {
	file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
	if err != nil {
		return nil, err
	}
	for _, declaration := range file.Decls {
		gen, ok := declaration.(*ast.GenDecl)
		if !ok {
			continue
		}
		for _, spec := range gen.Specs {
			typeSpec, ok := spec.(*ast.TypeSpec)
			if !ok || typeSpec.Name.Name != typeName {
				continue
			}
			structType, ok := typeSpec.Type.(*ast.StructType)
			if !ok {
				return nil, fmt.Errorf("type %q is not a struct", typeName)
			}
			return structFieldRows(structType), nil
		}
	}
	return nil, fmt.Errorf("struct %q is not declared in %s", typeName, path)
}

func structFieldRows(structType *ast.StructType) [][3]string {
	var rows [][3]string
	for _, field := range structType.Fields.List {
		if field.Tag == nil {
			continue
		}
		tag := strings.Trim(field.Tag.Value, "`")
		value, ok := yamlTagValue(tag)
		if !ok {
			continue
		}
		parts := strings.Split(value, ",")
		keyname := parts[0]
		if keyname == "" || keyname == "-" {
			continue
		}
		required := "yes"
		for _, option := range parts[1:] {
			if option == "omitempty" {
				required = "no"
			}
		}
		rows = append(rows, [3]string{keyname, types.ExprString(field.Type), required})
	}
	return rows
}

// yamlTagValue extracts the yaml struct-tag value without reflect.StructTag,
// which needs a concrete struct value rather than parsed source.
func yamlTagValue(tag string) (string, bool) {
	for _, part := range strings.Fields(tag) {
		if rest, found := strings.CutPrefix(part, `yaml:"`); found {
			return strings.TrimSuffix(rest, `"`), true
		}
	}
	return "", false
}
