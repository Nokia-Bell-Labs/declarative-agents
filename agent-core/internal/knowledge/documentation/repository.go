// Copyright (c) 2026 Nokia. All rights reserved.

// Package docsapi serves the Knowledge Manager documentation corpus.
package docsapi

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

var (
	// ErrNotConfigured reports that no documentation root was configured.
	ErrNotConfigured = errors.New("docs directory not configured")
	// ErrPathRequired reports that a document request did not name a path.
	ErrPathRequired = errors.New("path is required")
	// ErrInvalidPath reports a path traversal or root escape attempt.
	ErrInvalidPath = errors.New("invalid path")
	// ErrNotFound reports that a requested document does not exist.
	ErrNotFound = errors.New("document not found")
	// ErrReadDocument reports a non-not-found read failure.
	ErrReadDocument = errors.New("failed to read document")
)

// Entry is one document in the browsable documentation index.
type Entry struct {
	Path     string `json:"path"`
	Name     string `json:"name"`
	Category string `json:"category"`
}

// Detail is one fetched document with parsed YAML when parsing succeeds.
type Detail struct {
	Path    string      `json:"path"`
	Content interface{} `json:"content"`
	Raw     string      `json:"raw"`
}

// Repository reads documentation files from one configured root.
type Repository struct {
	root string
}

// NewRepository creates a documentation repository rooted at docsDir.
func NewRepository(docsDir string) Repository {
	return Repository{root: docsDir}
}

// List returns all YAML documents sorted by category and path.
func (r Repository) List() ([]Entry, error) {
	if r.root == "" {
		return []Entry{}, nil
	}
	var docs []Entry
	err := filepath.WalkDir(r.root, func(path string, entry os.DirEntry, err error) error {
		if err != nil || entry.IsDir() || !isYAML(path) {
			return nil
		}
		rel, err := filepath.Rel(r.root, path)
		if err != nil {
			return nil
		}
		docs = append(docs, entryFromPath(filepath.ToSlash(rel)))
		return nil
	})
	if err != nil {
		if os.IsNotExist(err) {
			return []Entry{}, nil
		}
		return nil, fmt.Errorf("scan docs directory: %w", err)
	}
	sortEntries(docs)
	return docs, nil
}

// Get returns one document by repository-relative path.
func (r Repository) Get(reqPath string) (Detail, error) {
	if r.root == "" {
		return Detail{}, ErrNotConfigured
	}
	cleaned, err := cleanDocPath(reqPath)
	if err != nil {
		return Detail{}, err
	}
	fullPath, err := r.fullPath(cleaned)
	if err != nil {
		return Detail{}, err
	}
	raw, err := os.ReadFile(fullPath)
	if err != nil {
		if os.IsNotExist(err) {
			return Detail{}, ErrNotFound
		}
		return Detail{}, fmt.Errorf("%w: %v", ErrReadDocument, err)
	}
	return detailFromRaw(cleaned, raw), nil
}

func entryFromPath(rel string) Entry {
	name := filepath.Base(rel)
	return Entry{
		Path:     rel,
		Name:     strings.TrimSuffix(name, filepath.Ext(name)),
		Category: Categorize(rel),
	}
}

// Categorize maps a repository-relative document path to the browser category.
func Categorize(rel string) string {
	parts := strings.Split(rel, "/")
	if len(parts) == 1 {
		return "overview"
	}
	switch {
	case strings.HasPrefix(rel, "specs/software-requirements/"):
		return "srd"
	case strings.HasPrefix(rel, "specs/semantic-models/"):
		return "semantic-model"
	case strings.HasPrefix(rel, "specs/config-formats/"):
		return "config-format"
	case strings.HasPrefix(rel, "specs/use-cases/"):
		return "use-case"
	case strings.HasPrefix(rel, "specs/test-suites/"):
		return "test-suite"
	default:
		return parts[0]
	}
}

func cleanDocPath(reqPath string) (string, error) {
	if reqPath == "" {
		return "", ErrPathRequired
	}
	cleaned := filepath.Clean(reqPath)
	if strings.Contains(cleaned, "..") {
		return "", ErrInvalidPath
	}
	return filepath.ToSlash(cleaned), nil
}

func (r Repository) fullPath(cleaned string) (string, error) {
	fullPath := filepath.Join(r.root, filepath.FromSlash(cleaned))
	if fullPath == r.root || strings.HasPrefix(fullPath, r.root+string(filepath.Separator)) {
		return fullPath, nil
	}
	return "", ErrInvalidPath
}

func detailFromRaw(path string, raw []byte) Detail {
	var content interface{}
	if err := yaml.Unmarshal(raw, &content); err != nil {
		content = nil
	}
	return Detail{Path: path, Content: content, Raw: string(raw)}
}

func sortEntries(docs []Entry) {
	sort.Slice(docs, func(i, j int) bool {
		if docs[i].Category != docs[j].Category {
			return docs[i].Category < docs[j].Category
		}
		return docs[i].Path < docs[j].Path
	})
}

func isYAML(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	return ext == ".yaml" || ext == ".yml"
}
