// Copyright (c) 2026 Nokia. All rights reserved.

package rest

import (
	"fmt"
	"regexp"
	"strings"
)

var (
	bodyParamPattern     = regexp.MustCompile(`params\.([A-Za-z_][A-Za-z0-9_]*)`)
	pathParamPattern     = regexp.MustCompile(`\{([A-Za-z_][A-Za-z0-9_]*)(?:\.\.\.)?\}`)
	pathParamFullPattern = regexp.MustCompile(`^\{([A-Za-z_][A-Za-z0-9_]*)(\.\.\.)?\}$`)
)

type pathParam struct {
	name     string
	catchAll bool
}

func validateDeclaredInputs(name string, operation Operation) error {
	params, err := pathParams("operation "+name, operation.Path)
	if err != nil {
		return err
	}
	for _, param := range params {
		if _, ok := operation.Params.Path[param.name]; !ok {
			return fmt.Errorf("operation %q path param %q is not declared", name, param.name)
		}
	}
	for _, field := range bodyTemplateFields(operation.Body) {
		if !operation.Params.declares(field) {
			return fmt.Errorf("operation %q body references undeclared param %q", name, field)
		}
	}
	return nil
}

func pathParams(owner, path string) ([]pathParam, error) {
	seen := map[string]bool{}
	parts := pathSegments(path)
	params := []pathParam{}
	for i, segment := range parts {
		param, ok, err := pathParamSegment(owner, segment)
		if err != nil {
			return nil, err
		}
		if !ok {
			continue
		}
		if err := validatePathParam(owner, param, seen, i, len(parts)); err != nil {
			return nil, err
		}
		seen[param.name] = true
		params = append(params, param)
	}
	return params, nil
}

func validatePathParam(owner string, param pathParam, seen map[string]bool, index, total int) error {
	if seen[param.name] {
		return fmt.Errorf("%s path param %q is ambiguous", owner, param.name)
	}
	if param.catchAll && index != total-1 {
		return fmt.Errorf("%s catch-all path param %q must be final", owner, param.name)
	}
	return nil
}

func pathParamSegment(owner, segment string) (pathParam, bool, error) {
	if !strings.ContainsAny(segment, "{}") {
		return pathParam{}, false, nil
	}
	match := pathParamFullPattern.FindStringSubmatch(segment)
	if match == nil {
		return pathParam{}, false, fmt.Errorf("%s has malformed path param segment %q", owner, segment)
	}
	return pathParam{name: match[1], catchAll: match[2] == "..."}, true, nil
}

func bodyTemplateFields(value interface{}) []string {
	var fields []string
	collectBodyTemplateFields(value, &fields)
	return fields
}

func collectBodyTemplateFields(value interface{}, fields *[]string) {
	switch typed := value.(type) {
	case string:
		for _, match := range bodyParamPattern.FindAllStringSubmatch(typed, -1) {
			*fields = append(*fields, match[1])
		}
	case []interface{}:
		for _, item := range typed {
			collectBodyTemplateFields(item, fields)
		}
	case map[string]interface{}:
		for _, item := range typed {
			collectBodyTemplateFields(item, fields)
		}
	}
}
