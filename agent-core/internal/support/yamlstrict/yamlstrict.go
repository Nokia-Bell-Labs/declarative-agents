// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

// Package yamlstrict provides one strict YAML decoding policy for declarations.
package yamlstrict

import (
	"bytes"
	"fmt"
	"io"
	"reflect"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// Unmarshal decodes exactly one YAML document and rejects unknown fields.
func Unmarshal(data []byte, value any) error {
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)
	if err := decoder.Decode(value); err != nil {
		return err
	}
	var extra yaml.Node
	switch err := decoder.Decode(&extra); err {
	case io.EOF:
		return nil
	case nil:
		return fmt.Errorf("multiple YAML documents")
	default:
		return err
	}
}

// CheckFields rejects mapping keys outside allowed, including keys supplied by
// YAML merge aliases. Custom UnmarshalYAML methods call this because
// KnownFields does not descend through those methods.
func CheckFields(node *yaml.Node, allowed ...string) error {
	if node == nil || node.Kind != yaml.MappingNode {
		return nil
	}
	known := make(map[string]bool, len(allowed))
	for _, field := range allowed {
		known[field] = true
	}
	for index := 0; index+1 < len(node.Content); index += 2 {
		key, value := node.Content[index], node.Content[index+1]
		if isMergeKey(key) {
			if err := checkMergeFields(value, allowed); err != nil {
				return err
			}
			continue
		}
		if !known[key.Value] {
			return fmt.Errorf("unknown field %q", key.Value)
		}
	}
	return nil
}

func checkMergeFields(node *yaml.Node, allowed []string) error {
	if node.Kind == yaml.AliasNode {
		return CheckFields(node.Alias, allowed...)
	}
	if node.Kind == yaml.MappingNode {
		return CheckFields(node, allowed...)
	}
	if node.Kind == yaml.SequenceNode {
		for _, item := range node.Content {
			if err := checkMergeFields(item, allowed); err != nil {
				return err
			}
		}
	}
	return nil
}

// CheckKnownFields recursively checks regular struct fields. Types with custom
// UnmarshalYAML methods keep responsibility for their own polymorphic shape.
func CheckKnownFields(node *yaml.Node, schema any) error {
	typ := reflect.TypeOf(schema)
	return checkKnownType(node, typ)
}

func checkKnownType(node *yaml.Node, typ reflect.Type) error {
	for typ != nil && typ.Kind() == reflect.Pointer {
		typ = typ.Elem()
	}
	if node == nil || typ == nil || customUnmarshaller(typ) {
		return nil
	}
	if node.Kind == yaml.AliasNode {
		return checkKnownType(node.Alias, typ)
	}
	if node.Kind == yaml.SequenceNode && typ.Kind() == reflect.Struct {
		for _, item := range node.Content {
			if err := checkKnownType(item, typ); err != nil {
				return err
			}
		}
		return nil
	}
	switch typ.Kind() {
	case reflect.Struct:
		return checkStructNode(node, typ)
	case reflect.Slice, reflect.Array:
		if node.Kind == yaml.SequenceNode {
			for _, item := range node.Content {
				if err := checkKnownType(item, typ.Elem()); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func checkStructNode(node *yaml.Node, typ reflect.Type) error {
	if node.Kind != yaml.MappingNode {
		return nil
	}
	fields := structFields(typ)
	for index := 0; index+1 < len(node.Content); index += 2 {
		key, value := node.Content[index], node.Content[index+1]
		if isMergeKey(key) {
			if err := checkKnownType(value, typ); err != nil {
				return err
			}
			continue
		}
		fieldType, ok := fields[key.Value]
		if !ok {
			return fmt.Errorf("unknown field %q", key.Value)
		}
		if err := checkKnownType(value, fieldType); err != nil {
			return fmt.Errorf("%s: %w", key.Value, err)
		}
	}
	return nil
}

func structFields(typ reflect.Type) map[string]reflect.Type {
	fields := make(map[string]reflect.Type)
	for index := 0; index < typ.NumField(); index++ {
		field := typ.Field(index)
		if !field.IsExported() {
			continue
		}
		tag := strings.Split(field.Tag.Get("yaml"), ",")[0]
		if tag == "-" {
			continue
		}
		if field.Anonymous && tag == "" {
			for name, nested := range structFields(field.Type) {
				fields[name] = nested
			}
			continue
		}
		if tag == "" {
			tag = strings.ToLower(field.Name)
		}
		fields[tag] = field.Type
	}
	return fields
}

func customUnmarshaller(typ reflect.Type) bool {
	unmarshaler := reflect.TypeOf((*yaml.Unmarshaler)(nil)).Elem()
	return typ.Implements(unmarshaler) || reflect.PointerTo(typ).Implements(unmarshaler)
}

// FieldPresent reports whether a field is authored directly or through a YAML
// merge alias.
func FieldPresent(node *yaml.Node, field string) bool {
	if node == nil {
		return false
	}
	if node.Kind == yaml.AliasNode {
		return FieldPresent(node.Alias, field)
	}
	if node.Kind == yaml.SequenceNode {
		for _, item := range node.Content {
			if FieldPresent(item, field) {
				return true
			}
		}
		return false
	}
	if node.Kind != yaml.MappingNode {
		return false
	}
	for index := 0; index+1 < len(node.Content); index += 2 {
		if node.Content[index].Value == field {
			return true
		}
		if isMergeKey(node.Content[index]) && FieldPresent(node.Content[index+1], field) {
			return true
		}
	}
	return false
}

func isMergeKey(node *yaml.Node) bool {
	return node != nil && node.Value == "<<" && node.Tag == "!!merge"
}

// TagsOf returns the YAML field names accepted by a struct.
func TagsOf(value any) []string {
	typ := reflect.TypeOf(value)
	for typ != nil && typ.Kind() == reflect.Pointer {
		typ = typ.Elem()
	}
	if typ == nil || typ.Kind() != reflect.Struct {
		return nil
	}
	seen := make(map[string]bool)
	collectTags(typ, seen)
	tags := make([]string, 0, len(seen))
	for tag := range seen {
		tags = append(tags, tag)
	}
	sort.Strings(tags)
	return tags
}

func collectTags(typ reflect.Type, tags map[string]bool) {
	for index := 0; index < typ.NumField(); index++ {
		field := typ.Field(index)
		if !field.IsExported() {
			continue
		}
		tag := strings.Split(field.Tag.Get("yaml"), ",")[0]
		if field.Anonymous && tag == "" {
			collectTags(field.Type, tags)
			continue
		}
		if tag == "" {
			tag = strings.ToLower(field.Name)
		}
		if tag != "-" {
			tags[tag] = true
		}
	}
}
