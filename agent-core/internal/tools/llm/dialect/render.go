// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package dialect

import (
	"fmt"
	"regexp"
	"strings"
)

// The params invoke_llm supplies to a body template (srd058 R2.2).
const (
	ParamMessages    = "messages"
	ParamModel       = "model"
	ParamTemperature = "temperature"
	ParamSeed        = "seed"
	ParamNumCtx      = "num_ctx"
)

var knownParams = map[string]bool{
	ParamMessages: true, ParamModel: true, ParamTemperature: true,
	ParamSeed: true, ParamNumCtx: true,
}

// templateReference is the REST request-body token, {{ params.<name> }}.
var templateReference = regexp.MustCompile(`\{\{ params\.([A-Za-z_][A-Za-z0-9_]*) \}\}`)

// validateBody checks the body template: every reference names a param
// invoke_llm supplies, and the conversation is placed whole, by a scalar that
// is exactly {{ params.messages }}, never spliced into text.
func validateBody(body map[string]interface{}) error {
	if len(body) == 0 {
		return fmt.Errorf("body is required")
	}
	placed := 0
	err := walkStrings(body, func(value string) error {
		for _, match := range templateReference.FindAllStringSubmatch(value, -1) {
			name := match[1]
			if !knownParams[name] {
				return fmt.Errorf("body references params.%s, which invoke_llm does not supply", name)
			}
			if name == ParamMessages && value != match[0] {
				return fmt.Errorf("body places params.messages inside %q; the conversation must be a whole scalar", value)
			}
			if name == ParamMessages {
				placed++
			}
		}
		if strings.Contains(value, "{{") && !templateReference.MatchString(value) {
			return fmt.Errorf("body value %q is not a {{ params.<name> }} reference", value)
		}
		return nil
	})
	if err != nil {
		return err
	}
	if placed != 1 {
		return fmt.Errorf("body places params.messages %d times; place the conversation exactly once", placed)
	}
	return nil
}

func walkStrings(value interface{}, visit func(string) error) error {
	switch typed := value.(type) {
	case string:
		return visit(typed)
	case map[string]interface{}:
		for _, item := range typed {
			if err := walkStrings(item, visit); err != nil {
				return err
			}
		}
	case []interface{}:
		for _, item := range typed {
			if err := walkStrings(item, visit); err != nil {
				return err
			}
		}
	}
	return nil
}

// RenderBody fills the body template. A scalar that is exactly one reference
// takes the param's value whole, so the message array lands as an array. A
// mapping entry whose whole value references a param absent from params is
// omitted, so an unset generation option stays out of the request rather than
// arriving as null.
func (c Chat) RenderBody(params map[string]interface{}) (map[string]interface{}, error) {
	rendered, _, err := renderValue(c.Body, params)
	if err != nil {
		return nil, err
	}
	return rendered.(map[string]interface{}), nil
}

func renderValue(value interface{}, params map[string]interface{}) (interface{}, bool, error) {
	switch typed := value.(type) {
	case string:
		return renderString(typed, params)
	case map[string]interface{}:
		out := make(map[string]interface{}, len(typed))
		for key, item := range typed {
			rendered, present, err := renderValue(item, params)
			if err != nil {
				return nil, false, err
			}
			if present {
				out[key] = rendered
			}
		}
		return out, true, nil
	case []interface{}:
		out := make([]interface{}, 0, len(typed))
		for _, item := range typed {
			rendered, present, err := renderValue(item, params)
			if err != nil {
				return nil, false, err
			}
			if present {
				out = append(out, rendered)
			}
		}
		return out, true, nil
	default:
		return value, true, nil
	}
}

func renderString(value string, params map[string]interface{}) (interface{}, bool, error) {
	matches := templateReference.FindAllStringSubmatch(value, -1)
	if len(matches) == 1 && matches[0][0] == value {
		param, present := params[matches[0][1]]
		return param, present, nil
	}
	for _, match := range matches {
		param, present := params[match[1]]
		if !present {
			return nil, false, fmt.Errorf("body value %q references params.%s, which this call does not supply", value, match[1])
		}
		value = strings.ReplaceAll(value, match[0], fmt.Sprint(param))
	}
	return value, true, nil
}
