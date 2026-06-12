// Copyright (c) 2026 Nokia. All rights reserved.

package bench

import (
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

type configFile struct {
	Path string `json:"path"`
	Name string `json:"name"`
}

type configCategory struct {
	Category string       `json:"category"`
	Files    []configFile `json:"files"`
}

type configDetail struct {
	Path    string      `json:"path"`
	Content interface{} `json:"content"`
	Raw     string      `json:"raw"`
	Graph   *graphView  `json:"graph,omitempty"`
}

type graphView struct {
	States         []string          `json:"states"`
	TerminalStates []string          `json:"terminalStates"`
	Transitions    []graphTransition `json:"transitions"`
}

type graphTransition struct {
	From   string `json:"from"`
	Signal string `json:"signal"`
	To     string `json:"to"`
	Action string `json:"action,omitempty"`
}

type profileEntry struct {
	Name          string      `json:"name"`
	MatchPrefixes []string    `json:"matchPrefixes"`
	Envelope      interface{} `json:"envelope,omitempty"`
	StrictFormat  bool        `json:"strictFormat"`
	Pipeline      []string    `json:"pipeline,omitempty"`
}

func (s *Server) handleListConfigs(w http.ResponseWriter, r *http.Request) {
	if s.configsDir == "" {
		writeData(w, []configCategory{})
		return
	}

	categories := map[string][]configFile{}

	err := filepath.Walk(s.configsDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		ext := strings.ToLower(filepath.Ext(path))
		if ext != ".yaml" && ext != ".yml" {
			return nil
		}

		rel, _ := filepath.Rel(s.configsDir, path)
		rel = filepath.ToSlash(rel)

		parts := strings.SplitN(rel, "/", 2)
		category := parts[0]
		name := rel
		if len(parts) > 1 {
			name = parts[1]
		}

		categories[category] = append(categories[category], configFile{
			Path: rel,
			Name: name,
		})
		return nil
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to scan configs directory")
		return
	}

	var result []configCategory
	for cat, files := range categories {
		sort.Slice(files, func(i, j int) bool {
			return files[i].Path < files[j].Path
		})
		result = append(result, configCategory{Category: cat, Files: files})
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].Category < result[j].Category
	})

	writeData(w, result)
}

func (s *Server) handleGetConfig(w http.ResponseWriter, r *http.Request) {
	if s.configsDir == "" {
		writeError(w, http.StatusNotFound, "configs directory not configured")
		return
	}

	reqPath := r.PathValue("path")
	if reqPath == "" {
		writeError(w, http.StatusBadRequest, "path is required")
		return
	}

	cleaned := filepath.Clean(reqPath)
	if strings.Contains(cleaned, "..") {
		writeError(w, http.StatusBadRequest, "invalid path")
		return
	}

	fullPath := filepath.Join(s.configsDir, filepath.FromSlash(cleaned))
	if !strings.HasPrefix(fullPath, s.configsDir+string(filepath.Separator)) {
		writeError(w, http.StatusBadRequest, "invalid path")
		return
	}

	raw, err := os.ReadFile(fullPath)
	if err != nil {
		if os.IsNotExist(err) {
			writeError(w, http.StatusNotFound, "config file not found")
		} else {
			writeError(w, http.StatusInternalServerError, "failed to read config file")
		}
		return
	}

	var content map[string]interface{}
	if err := yaml.Unmarshal(raw, &content); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to parse YAML")
		return
	}

	detail := configDetail{
		Path:    filepath.ToSlash(cleaned),
		Content: content,
		Raw:     string(raw),
	}

	if isMachineConfig(content) {
		detail.Graph = extractGraph(content)
	}

	writeData(w, detail)
}

func isMachineConfig(content map[string]interface{}) bool {
	_, hasStates := content["states"]
	_, hasTransitions := content["transitions"]
	return hasStates && hasTransitions
}

func extractGraph(content map[string]interface{}) *graphView {
	g := &graphView{}

	if states, ok := content["states"].([]interface{}); ok {
		for _, s := range states {
			if str, ok := s.(string); ok {
				g.States = append(g.States, str)
			}
		}
	}

	if terminal, ok := content["terminal_states"].([]interface{}); ok {
		for _, s := range terminal {
			if str, ok := s.(string); ok {
				g.TerminalStates = append(g.TerminalStates, str)
			}
		}
	}

	if transitions, ok := content["transitions"].([]interface{}); ok {
		for _, t := range transitions {
			if tm, ok := t.(map[string]interface{}); ok {
				gt := graphTransition{
					From:   strVal(tm, "state"),
					Signal: strVal(tm, "signal"),
					To:     strVal(tm, "next"),
					Action: strVal(tm, "action"),
				}
				g.Transitions = append(g.Transitions, gt)
			}
		}
	}

	return g
}

func strVal(m map[string]interface{}, key string) string {
	if v, ok := m[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

func (s *Server) handleListProfiles(w http.ResponseWriter, r *http.Request) {
	if s.profilesDir == "" {
		writeData(w, []profileEntry{})
		return
	}

	entries, err := os.ReadDir(s.profilesDir)
	if err != nil {
		if os.IsNotExist(err) {
			writeData(w, []profileEntry{})
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to read profiles directory")
		return
	}

	var profiles []profileEntry
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		ext := strings.ToLower(filepath.Ext(e.Name()))
		if ext != ".yaml" && ext != ".yml" {
			continue
		}

		data, err := os.ReadFile(filepath.Join(s.profilesDir, e.Name()))
		if err != nil {
			continue
		}

		var raw map[string]interface{}
		if err := yaml.Unmarshal(data, &raw); err != nil {
			continue
		}

		entry := profileEntry{
			Name:         strVal(raw, "name"),
			StrictFormat: boolVal(raw, "strict_format"),
		}

		if prefixes, ok := raw["match_prefixes"].([]interface{}); ok {
			for _, p := range prefixes {
				if s, ok := p.(string); ok {
					entry.MatchPrefixes = append(entry.MatchPrefixes, s)
				}
			}
		}

		if envelope, ok := raw["envelope"]; ok {
			entry.Envelope = envelope
		}

		if pipeline, ok := raw["extraction_pipeline"].([]interface{}); ok {
			for _, step := range pipeline {
				switch v := step.(type) {
				case string:
					entry.Pipeline = append(entry.Pipeline, v)
				case map[string]interface{}:
					for k := range v {
						entry.Pipeline = append(entry.Pipeline, k)
					}
				}
			}
		}

		profiles = append(profiles, entry)
	}

	sort.Slice(profiles, func(i, j int) bool {
		return profiles[i].Name < profiles[j].Name
	})

	writeData(w, profiles)
}

func boolVal(m map[string]interface{}, key string) bool {
	if v, ok := m[key]; ok {
		if b, ok := v.(bool); ok {
			return b
		}
	}
	return false
}
