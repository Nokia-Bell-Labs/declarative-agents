// Copyright (c) 2026 Nokia. All rights reserved.

package core

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestFileStoreSaveLoadListDelete(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := NewFileStore(t.TempDir())

	require.NoError(t, store.Save(ctx, "checkpoint/cp-1", []byte(`{"ok":true}`)))
	require.NoError(t, store.Save(ctx, "history/run-1", []byte(`{"steps":1}`)))
	require.NoError(t, store.Save(ctx, "checkpoint/cp-2", []byte(`{"ok":false}`)))

	data, err := store.Load(ctx, "checkpoint/cp-1")
	require.NoError(t, err)
	require.JSONEq(t, `{"ok":true}`, string(data))

	keys, err := store.List(ctx, "checkpoint/")
	require.NoError(t, err)
	require.Equal(t, []string{"checkpoint/cp-1", "checkpoint/cp-2"}, keys)

	allKeys, err := store.List(ctx, "")
	require.NoError(t, err)
	require.Equal(t, []string{"checkpoint/cp-1", "checkpoint/cp-2", "history/run-1"}, allKeys)

	require.NoError(t, store.Delete(ctx, "checkpoint/cp-1"))
	keys, err = store.List(ctx, "checkpoint/")
	require.NoError(t, err)
	require.Equal(t, []string{"checkpoint/cp-2"}, keys)
}

func TestFileStoreStoresOneJSONFilePerKey(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	root := t.TempDir()
	store := NewFileStore(root)

	require.NoError(t, store.Save(ctx, "checkpoint/cp-1", []byte(`{"ok":true}`)))

	data, err := os.ReadFile(filepath.Join(root, "checkpoint", "cp-1.json"))
	require.NoError(t, err)
	require.JSONEq(t, `{"ok":true}`, string(data))
}

func TestFileStoreMissingLoadErrorIncludesKeyAndRoot(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	root := t.TempDir()
	store := NewFileStore(root)

	_, err := store.Load(ctx, "checkpoint/missing")

	require.Error(t, err)
	require.Contains(t, err.Error(), "checkpoint/missing")
	require.Contains(t, err.Error(), root)
}

func TestFileStoreRejectsUnsafeKeys(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := NewFileStore(t.TempDir())

	for _, key := range []string{"", "../secret", "checkpoint/../secret", "/absolute"} {
		err := store.Save(ctx, key, []byte(`{}`))
		require.Error(t, err, "key %q should be rejected", key)
		require.Contains(t, err.Error(), key)
		require.Contains(t, err.Error(), store.Root)
	}
}

func TestFileStoreListMissingRootIsEmpty(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := NewFileStore(filepath.Join(t.TempDir(), "missing"))

	keys, err := store.List(ctx, "checkpoint/")

	require.NoError(t, err)
	require.Empty(t, keys)
}
