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

type docEntry struct {
	Path     string `json:"path"`
	Name     string `json:"name"`
	Category string `json:"category"`
}

type docDetail struct {
	Path    string      `json:"path"`
	Content interface{} `json:"content"`
	Raw     string      `json:"raw"`
}

func (s *Server) handleListDocs(w http.ResponseWriter, r *http.Request) {
	if s.docsDir == "" {
		writeData(w, []docEntry{})
		return
	}

	var docs []docEntry
	err := filepath.Walk(s.docsDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		ext := strings.ToLower(filepath.Ext(path))
		if ext != ".yaml" && ext != ".yml" {
			return nil
		}

		rel, _ := filepath.Rel(s.docsDir, path)
		rel = filepath.ToSlash(rel)

		category := categorizeDoc(rel)
		name := filepath.Base(rel)
		name = strings.TrimSuffix(name, filepath.Ext(name))

		docs = append(docs, docEntry{
			Path:     rel,
			Name:     name,
			Category: category,
		})
		return nil
	})
	if err != nil {
		if os.IsNotExist(err) {
			writeData(w, []docEntry{})
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to scan docs directory")
		return
	}

	sort.Slice(docs, func(i, j int) bool {
		if docs[i].Category != docs[j].Category {
			return docs[i].Category < docs[j].Category
		}
		return docs[i].Path < docs[j].Path
	})

	writeData(w, docs)
}

func categorizeDoc(rel string) string {
	parts := strings.Split(rel, "/")
	if len(parts) == 1 {
		return "overview"
	}
	if strings.HasPrefix(rel, "specs/software-requirements/") {
		return "srd"
	}
	if strings.HasPrefix(rel, "specs/semantic-"+"mod"+"els/") {
		return "semantic-model"
	}
	if strings.HasPrefix(rel, "specs/config-formats/") {
		return "config-format"
	}
	if strings.HasPrefix(rel, "specs/use-cases/") {
		return "use-case"
	}
	if strings.HasPrefix(rel, "specs/test-suites/") {
		return "test-suite"
	}
	return parts[0]
}

func (s *Server) handleGetDoc(w http.ResponseWriter, r *http.Request) {
	if s.docsDir == "" {
		writeError(w, http.StatusNotFound, "docs directory not configured")
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

	fullPath := filepath.Join(s.docsDir, filepath.FromSlash(cleaned))
	if !strings.HasPrefix(fullPath, s.docsDir+string(filepath.Separator)) &&
		fullPath != s.docsDir {
		writeError(w, http.StatusBadRequest, "invalid path")
		return
	}

	raw, err := os.ReadFile(fullPath)
	if err != nil {
		if os.IsNotExist(err) {
			writeError(w, http.StatusNotFound, "document not found")
		} else {
			writeError(w, http.StatusInternalServerError, "failed to read document")
		}
		return
	}

	var content interface{}
	if err := yaml.Unmarshal(raw, &content); err != nil {
		content = nil
	}

	writeData(w, docDetail{
		Path:    filepath.ToSlash(cleaned),
		Content: content,
		Raw:     string(raw),
	})
}
