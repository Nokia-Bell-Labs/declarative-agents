// Copyright (c) 2026 Nokia. All rights reserved.

package core

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const fileStoreExt = ".json"

// FileStore persists StateStore blobs as one JSON file per key below Root.
type FileStore struct {
	Root string
}

// NewFileStore returns a StateStore backed by root.
func NewFileStore(root string) *FileStore {
	return &FileStore{Root: root}
}

func (f *FileStore) Save(_ context.Context, key string, data []byte) error {
	path, err := f.pathForKey(key)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("filestore save key %q in %q: mkdir: %w", key, f.Root, err)
	}
	if err := os.WriteFile(path, append([]byte(nil), data...), 0o644); err != nil {
		return fmt.Errorf("filestore save key %q in %q: write: %w", key, f.Root, err)
	}
	return nil
}

func (f *FileStore) Load(_ context.Context, key string) ([]byte, error) {
	path, err := f.pathForKey(key)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("filestore load key %q in %q: read: %w", key, f.Root, err)
	}
	return data, nil
}

func (f *FileStore) List(_ context.Context, prefix string) ([]string, error) {
	if err := f.validatePrefix(prefix); err != nil {
		return nil, err
	}
	var keys []string
	if _, err := os.Stat(f.Root); err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("filestore list prefix %q in %q: stat: %w", prefix, f.Root, err)
	}
	if err := filepath.WalkDir(f.Root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || filepath.Ext(path) != fileStoreExt {
			return nil
		}
		rel, err := filepath.Rel(f.Root, path)
		if err != nil {
			return err
		}
		key := strings.TrimSuffix(filepath.ToSlash(rel), fileStoreExt)
		if strings.HasPrefix(key, prefix) {
			keys = append(keys, key)
		}
		return nil
	}); err != nil {
		return nil, fmt.Errorf("filestore list prefix %q in %q: walk: %w", prefix, f.Root, err)
	}
	sort.Strings(keys)
	return keys, nil
}

func (f *FileStore) Delete(_ context.Context, key string) error {
	path, err := f.pathForKey(key)
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("filestore delete key %q in %q: remove: %w", key, f.Root, err)
	}
	return nil
}

func (f *FileStore) pathForKey(key string) (string, error) {
	if f.Root == "" {
		return "", fmt.Errorf("filestore key %q: missing root directory", key)
	}
	if err := f.validateKey(key); err != nil {
		return "", err
	}
	return filepath.Join(f.Root, filepath.FromSlash(key)+fileStoreExt), nil
}

func (f *FileStore) validateKey(key string) error {
	if key == "" || strings.TrimSpace(key) != key {
		return fmt.Errorf("filestore key %q in %q: invalid empty or padded key", key, f.Root)
	}
	if strings.HasPrefix(key, "/") || filepath.IsAbs(key) {
		return fmt.Errorf("filestore key %q in %q: absolute keys are not allowed", key, f.Root)
	}
	clean := pathClean(key)
	if clean != key || clean == "." || strings.HasPrefix(clean, "../") || clean == ".." {
		return fmt.Errorf("filestore key %q in %q: path traversal is not allowed", key, f.Root)
	}
	return nil
}

func (f *FileStore) validatePrefix(prefix string) error {
	if prefix == "" {
		return nil
	}
	if strings.HasPrefix(prefix, "/") || filepath.IsAbs(prefix) {
		return fmt.Errorf("filestore list prefix %q in %q: absolute prefixes are not allowed", prefix, f.Root)
	}
	clean := pathClean(prefix)
	if clean == ".." || strings.HasPrefix(clean, "../") {
		return fmt.Errorf("filestore list prefix %q in %q: path traversal is not allowed", prefix, f.Root)
	}
	return nil
}

func pathClean(key string) string {
	return filepath.ToSlash(filepath.Clean(filepath.FromSlash(key)))
}

var _ StateStore = (*FileStore)(nil)
