// Copyright (c) 2026 Nokia. All rights reserved.

package docsapi

import (
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

type configDetail struct {
	Path    string      `json:"path"`
	Content interface{} `json:"content"`
	Raw     string      `json:"raw"`
}

type sourceDetail struct {
	Path     string `json:"path"`
	Content  string `json:"content"`
	Language string `json:"language"`
	MimeType string `json:"mimeType"`
	Size     int64  `json:"size"`
}

var sourceExtensions = map[string]bool{
	".go": true, ".yaml": true, ".yml": true, ".ts": true, ".tsx": true,
	".js": true, ".jsx": true, ".css": true, ".json": true, ".md": true,
	".txt": true, ".toml": true, ".mod": true, ".sum": true, ".html": true,
	".sh": true,
}

func (s *Server) handleGetConfig(w http.ResponseWriter, r *http.Request) {
	root := s.configsDir
	if root == "" {
		root = "configs"
	}
	detail, err := readConfigFile(root, r.PathValue("path"))
	if err != nil {
		writeContextError(w, err, "config file not found")
		return
	}
	writeDataFields(w, http.StatusOK, detail, map[string]interface{}{
		"path": detail.Path, "content": detail.Content, "raw": detail.Raw,
	})
}

func (s *Server) handleGetSource(w http.ResponseWriter, r *http.Request) {
	root := s.sourceDir
	if root == "" {
		root = "."
	}
	detail, err := readSourceFile(root, r.PathValue("path"))
	if err != nil {
		writeContextError(w, err, "file not found")
		return
	}
	writeDataFields(w, http.StatusOK, detail, map[string]interface{}{
		"path": detail.Path, "content": detail.Content, "language": detail.Language,
		"mimeType": detail.MimeType, "size": detail.Size,
	})
}

func readConfigFile(root, reqPath string) (configDetail, error) {
	cleaned, fullPath, err := cleanContextPath(root, reqPath)
	if err != nil {
		return configDetail{}, err
	}
	raw, err := os.ReadFile(fullPath)
	if err != nil {
		if os.IsNotExist(err) {
			return configDetail{}, ErrNotFound
		}
		return configDetail{}, err
	}
	var content interface{}
	if err := yaml.Unmarshal(raw, &content); err != nil {
		return configDetail{}, err
	}
	return configDetail{Path: cleaned, Content: content, Raw: string(raw)}, nil
}

func readSourceFile(root, reqPath string) (sourceDetail, error) {
	cleaned, fullPath, err := cleanContextPath(root, reqPath)
	if err != nil {
		return sourceDetail{}, err
	}
	info, err := os.Stat(fullPath)
	if err != nil {
		if os.IsNotExist(err) {
			return sourceDetail{}, ErrNotFound
		}
		return sourceDetail{}, err
	}
	if info.IsDir() {
		return sourceDetail{}, ErrInvalidPath
	}
	ext := strings.ToLower(filepath.Ext(fullPath))
	if !sourceExtensions[ext] {
		return sourceDetail{}, ErrInvalidPath
	}
	raw, err := os.ReadFile(fullPath)
	if err != nil {
		return sourceDetail{}, err
	}
	mimeType := mime.TypeByExtension(ext)
	if mimeType == "" {
		mimeType = "text/plain"
	}
	return sourceDetail{
		Path: cleaned, Content: string(raw), Language: extensionToLanguage(ext),
		MimeType: mimeType, Size: info.Size(),
	}, nil
}

func cleanContextPath(root, reqPath string) (string, string, error) {
	if reqPath == "" {
		return "", "", ErrPathRequired
	}
	cleaned := filepath.Clean(reqPath)
	if strings.Contains(cleaned, "..") {
		return "", "", ErrInvalidPath
	}
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return "", "", err
	}
	fullPath := filepath.Join(absRoot, filepath.FromSlash(cleaned))
	if fullPath == absRoot || strings.HasPrefix(fullPath, absRoot+string(filepath.Separator)) {
		return filepath.ToSlash(cleaned), fullPath, nil
	}
	return "", "", ErrInvalidPath
}

func writeContextError(w http.ResponseWriter, err error, notFound string) {
	switch err {
	case ErrPathRequired, ErrInvalidPath:
		writeDocError(w, err)
	case ErrNotFound:
		writeError(w, http.StatusNotFound, notFound)
	default:
		writeError(w, http.StatusInternalServerError, err.Error())
	}
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
