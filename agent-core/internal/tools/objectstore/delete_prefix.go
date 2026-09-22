// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package objectstore

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
	"gocloud.dev/blob"
)

type deletePrefixOutcome struct {
	Status      string    `json:"status"`
	Application string    `json:"application"`
	BucketURL   string    `json:"bucket_url"`
	Prefix      string    `json:"prefix"`
	Matched     int       `json:"matched"`
	Deleted     int       `json:"deleted"`
	Remaining   int       `json:"remaining"`
	KeysSHA256  string    `json:"keys_sha256"`
	At          time.Time `json:"at"`
}

type deletePrefixCmd struct {
	opener *Opener
	config DeletePrefixConfig
}

func (d *deletePrefixCmd) Name() string { return InitObjectDeletePrefix }
func (d *deletePrefixCmd) Undo(core.Result) core.Result {
	return core.NoopUndo(d.Name())
}

func (d *deletePrefixCmd) Execute() core.Result {
	ctx := context.Background()
	bucket, done, err := d.opener.Open(ctx, d.config.Connection)
	if err != nil {
		return commandError(d.Name(), err)
	}
	defer done()
	keys, err := keysUnderPrefix(ctx, bucket, d.config.Prefix)
	if err != nil {
		return commandError(d.Name(), err)
	}
	outcome := deletePrefixOutcome{
		Status: "purged", Application: d.config.Application,
		BucketURL: d.config.Connection.BucketURL, Prefix: d.config.Prefix,
		Matched: len(keys), KeysSHA256: hashKeys(keys), At: time.Now().UTC(),
	}
	if len(keys) > d.config.MaxObjects {
		outcome.Status = "refused"
		outcome.Remaining = len(keys)
		return d.finish(outcome)
	}
	for _, key := range keys {
		if err := bucket.Delete(ctx, key); err == nil {
			outcome.Deleted++
		}
	}
	remaining, err := keysUnderPrefix(ctx, bucket, d.config.Prefix)
	if err != nil {
		return commandError(d.Name(), fmt.Errorf("verify prefix %q: %w", d.config.Prefix, err))
	}
	outcome.Remaining = len(remaining)
	if outcome.Remaining != 0 || outcome.Deleted != outcome.Matched {
		outcome.Status = "partial"
	}
	return d.finish(outcome)
}

func (d *deletePrefixCmd) finish(outcome deletePrefixOutcome) core.Result {
	encoded, err := json.Marshal(outcome)
	if err != nil {
		return commandError(d.Name(), fmt.Errorf("encode purge outcome: %w", err))
	}
	if err := appendPurgeAudit(d.config.AuditPath, encoded); err != nil {
		return commandError(d.Name(), err)
	}
	return core.Result{
		Output: string(encoded), Signal: core.ToolDone, CommandName: d.Name(),
	}
}

func keysUnderPrefix(ctx context.Context, bucket *blob.Bucket, prefix string) ([]string, error) {
	iterator := bucket.List(&blob.ListOptions{Prefix: prefix})
	var keys []string
	for {
		object, err := iterator.Next(ctx)
		if err == io.EOF {
			sort.Strings(keys)
			return keys, nil
		}
		if err != nil {
			return nil, fmt.Errorf("list prefix %q: %w", prefix, err)
		}
		if !object.IsDir {
			keys = append(keys, object.Key)
		}
	}
}

func hashKeys(keys []string) string {
	sum := sha256.Sum256([]byte(strings.Join(keys, "\n")))
	return hex.EncodeToString(sum[:])
}

func appendPurgeAudit(path string, encoded []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create purge audit directory: %w", err)
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("open purge audit %s: %w", path, err)
	}
	if _, err := file.Write(append(encoded, '\n')); err != nil {
		_ = file.Close()
		return fmt.Errorf("write purge audit %s: %w", path, err)
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return fmt.Errorf("sync purge audit %s: %w", path, err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close purge audit %s: %w", path, err)
	}
	return nil
}

type DeletePrefixBuilder struct {
	Opener *Opener
	Config DeletePrefixConfig
}

func (b *DeletePrefixBuilder) Build(core.Result) core.Command {
	return &deletePrefixCmd{opener: b.Opener, config: b.Config}
}
