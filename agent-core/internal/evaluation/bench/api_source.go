// Copyright (c) 2026 Nokia. All rights reserved.

package bench

import (
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

var sourceExtensions = map[string]bool{
	".go":   true,
	".yaml": true,
	".yml":  true,
	".ts":   true,
	".tsx":  true,
	".js":   true,
	".jsx":  true,
	".css":  true,
	".json": true,
	".md":   true,
	".txt":  true,
	".toml": true,
	".mod":  true,
	".sum":  true,
	".html": true,
	".sh":   true,
}

func (s *Server) handleGetSource(w http.ResponseWriter, r *http.Request) {
	if s.sourceDir == "" {
		writeError(w, http.StatusNotFound, "source directory not configured")
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

	fullPath := filepath.Join(s.sourceDir, filepath.FromSlash(cleaned))
	if !strings.HasPrefix(fullPath, s.sourceDir+string(filepath.Separator)) {
		writeError(w, http.StatusBadRequest, "invalid path")
		return
	}

	info, err := os.Stat(fullPath)
	if err != nil {
		if os.IsNotExist(err) {
			writeError(w, http.StatusNotFound, "file not found")
		} else {
			writeError(w, http.StatusInternalServerError, "failed to stat file")
		}
		return
	}
	if info.IsDir() {
		writeError(w, http.StatusBadRequest, "path is a directory")
		return
	}

	ext := strings.ToLower(filepath.Ext(fullPath))
	if !sourceExtensions[ext] {
		writeError(w, http.StatusBadRequest, "unsupported file type")
		return
	}

	raw, err := os.ReadFile(fullPath)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to read file")
		return
	}

	lang := extensionToLanguage(ext)
	ct := mime.TypeByExtension(ext)
	if ct == "" {
		ct = "text/plain"
	}

	writeData(w, sourceDetail{
		Path:     filepath.ToSlash(cleaned),
		Content:  string(raw),
		Language: lang,
		MimeType: ct,
		Size:     info.Size(),
	})
}

type sourceDetail struct {
	Path     string `json:"path"`
	Content  string `json:"content"`
	Language string `json:"language"`
	MimeType string `json:"mimeType"`
	Size     int64  `json:"size"`
}

func extensionToLanguage(ext string) string {
	switch ext {
	case ".go":
		return "go"
	case ".yaml", ".yml":
		return "yaml"
	case ".ts", ".tsx":
		return "typescript"
	case ".js", ".jsx":
		return "javascript"
	case ".css":
		return "css"
	case ".json":
		return "json"
	case ".md":
		return "markdown"
	case ".html":
		return "html"
	case ".sh":
		return "bash"
	default:
		return "text"
	}
}
