// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package main

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const (
	copyrightHolderLine = "Copyright (c) 2026 Nokia"
	spdxBSD3Clause      = "SPDX-License-Identifier: BSD-3-Clause"
)

var copyrightHeaderByExt = map[string][2]string{
	".go": {
		"// " + copyrightHolderLine,
		"// " + spdxBSD3Clause,
	},
	".yaml": {
		"# " + copyrightHolderLine,
		"# " + spdxBSD3Clause,
	},
	".yml": {
		"# " + copyrightHolderLine,
		"# " + spdxBSD3Clause,
	},
	".md": {
		"<!-- " + copyrightHolderLine + " -->",
		"<!-- " + spdxBSD3Clause + " -->",
	},
}

func copyrightHeaderLines(ext string) ([2]string, bool) {
	lines, ok := copyrightHeaderByExt[ext]
	return lines, ok
}

func copyrightHeaderPrefix(ext string) (string, bool) {
	lines, ok := copyrightHeaderLines(ext)
	if !ok {
		return "", false
	}
	return lines[0] + "\n" + lines[1] + "\n", true
}

// markdownFrontmatterEnd returns the byte offset immediately after the closing
// YAML frontmatter marker when content starts with ---. Files without
// frontmatter return 0. A missing closer returns an error.
func markdownFrontmatterEnd(content []byte) (int, error) {
	normalized := bytes.ReplaceAll(content, []byte("\r\n"), []byte("\n"))
	if !bytes.HasPrefix(normalized, []byte("---\n")) {
		if bytes.Equal(bytes.TrimRight(normalized, "\n"), []byte("---")) {
			return 0, fmt.Errorf("markdown frontmatter is missing a closing ---")
		}
		return 0, nil
	}
	rest := normalized[4:]
	idx := bytes.Index(rest, []byte("\n---\n"))
	if idx >= 0 {
		return 4 + idx + len("\n---\n"), nil
	}
	if bytes.HasSuffix(rest, []byte("\n---")) {
		return len(normalized), nil
	}
	if bytes.Equal(rest, []byte("---")) {
		return len(normalized), nil
	}
	return 0, fmt.Errorf("markdown frontmatter is missing a closing ---")
}

func copyrightHeaderOffset(content []byte, ext string) (int, error) {
	if ext != ".md" {
		return 0, nil
	}
	return markdownFrontmatterEnd(content)
}

func checkCopyrightHeader(content []byte, ext string) error {
	prefix, ok := copyrightHeaderPrefix(ext)
	if !ok {
		return fmt.Errorf("unsupported extension %q", ext)
	}
	normalized := bytes.ReplaceAll(content, []byte("\r\n"), []byte("\n"))
	offset, err := copyrightHeaderOffset(normalized, ext)
	if err != nil {
		return err
	}
	if offset > len(normalized) {
		return fmt.Errorf("header offset %d exceeds file length %d", offset, len(normalized))
	}
	if !bytes.HasPrefix(normalized[offset:], []byte(prefix)) {
		return fmt.Errorf("missing canonical %s copyright header", ext)
	}
	return nil
}

func trackedCopyrightPaths(root string) ([]string, error) {
	cmd := exec.Command("git", "ls-files", "-z")
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git ls-files: %w", err)
	}
	var paths []string
	for _, name := range bytes.Split(out, []byte{0}) {
		if len(name) == 0 {
			continue
		}
		rel := string(name)
		ext := strings.ToLower(filepath.Ext(rel))
		if _, ok := copyrightHeaderByExt[ext]; !ok {
			continue
		}
		paths = append(paths, rel)
	}
	return paths, nil
}

func checkTrackedCopyrightHeaders(root string) error {
	paths, err := trackedCopyrightPaths(root)
	if err != nil {
		return err
	}
	var failed []string
	for _, rel := range paths {
		data, err := os.ReadFile(filepath.Join(root, rel))
		if err != nil {
			return fmt.Errorf("read %s: %w", rel, err)
		}
		if err := checkCopyrightHeader(data, strings.ToLower(filepath.Ext(rel))); err != nil {
			failed = append(failed, fmt.Sprintf("%s: %v", rel, err))
		}
	}
	if len(failed) > 0 {
		return fmt.Errorf("%d files missing canonical copyright headers:\n  %s",
			len(failed), strings.Join(failed, "\n  "))
	}
	return nil
}
