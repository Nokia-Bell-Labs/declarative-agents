// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package objectstore

import (
	"fmt"
	"strings"
	"testing"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/catalog"
)

func paramsResult(pairs map[string]string) core.Result {
	fields := make([]string, 0, len(pairs))
	for name, value := range pairs {
		fields = append(fields, fmt.Sprintf("%q:%q", name, value))
	}
	return core.Result{Output: `{"parameters":{` + strings.Join(fields, ",") + `}}`}
}

func connectionFor(t *testing.T, scheme string) ConnectionConfig {
	t.Helper()
	switch scheme {
	case "mem":
		return ConnectionConfig{BucketURL: "mem://" + t.Name()}
	case "file":
		return ConnectionConfig{BucketURL: "file://" + t.TempDir()}
	default:
		t.Fatalf("no local connection for scheme %q", scheme)
		return ConnectionConfig{}
	}
}

func executeWrite(t *testing.T, opener *Opener, connection ConnectionConfig, key, content string) core.Result {
	t.Helper()
	builder := &WriteBuilder{Opener: opener, Connection: connection}
	result := builder.Build(paramsResult(map[string]string{"key": key, "content": content})).Execute()
	if result.Signal != core.ToolDone {
		t.Fatalf("write %s: %v %s", key, result.Signal, result.Output)
	}
	return result
}

func executeRead(t *testing.T, opener *Opener, connection ConnectionConfig, key string) core.Result {
	t.Helper()
	builder := &ReadBuilder{Opener: opener, Connection: connection}
	return builder.Build(paramsResult(map[string]string{"key": key})).Execute()
}

// srd059 R2.1 and AC2: both local schemes behave identically through the
// three commands.
func TestObjectRoundTrip(t *testing.T) {
	for _, scheme := range []string{"mem", "file"} {
		t.Run(scheme, func(t *testing.T) {
			opener := NewOpener()
			connection := connectionFor(t, scheme)
			executeWrite(t, opener, connection, "reports/alpha.txt", "alpha body")
			executeWrite(t, opener, connection, "reports/beta.txt", "beta body")
			executeWrite(t, opener, connection, "notes/gamma.txt", "gamma body")

			read := executeRead(t, opener, connection, "reports/alpha.txt")
			if read.Signal != core.ToolDone || read.Output != "alpha body" {
				t.Fatalf("read = %v %q", read.Signal, read.Output)
			}

			list := (&ListBuilder{Opener: opener, Connection: connection}).
				Build(paramsResult(map[string]string{"prefix": "reports/"})).Execute()
			if list.Signal != core.ToolDone {
				t.Fatalf("list: %v %s", list.Signal, list.Output)
			}
			for _, want := range []string{"reports/alpha.txt", "reports/beta.txt"} {
				if !strings.Contains(list.Output, want) {
					t.Errorf("list output %q misses %q", list.Output, want)
				}
			}
			if strings.Contains(list.Output, "gamma") {
				t.Errorf("list output %q leaked past its prefix", list.Output)
			}
		})
	}
}

// srd059 R3.1, R3.2, AC3: both undo directions, through the receipt alone.
func TestObjectWriteUndo(t *testing.T) {
	for _, scheme := range []string{"mem", "file"} {
		t.Run(scheme+" restore prior", func(t *testing.T) {
			opener := NewOpener()
			connection := connectionFor(t, scheme)
			executeWrite(t, opener, connection, "config.yaml", "original")
			write := executeWrite(t, opener, connection, "config.yaml", "overwritten")

			builder := &WriteBuilder{Opener: opener, Connection: connection}
			undo := builder.BuildReverser().Undo(core.Result{Receipt: write.Receipt})
			if undo.Signal != core.ToolDone {
				t.Fatalf("undo: %v %s", undo.Signal, undo.Output)
			}
			read := executeRead(t, opener, connection, "config.yaml")
			if read.Output != "original" {
				t.Fatalf("after undo, object = %q, want the prior bytes", read.Output)
			}
		})
		t.Run(scheme+" delete created", func(t *testing.T) {
			opener := NewOpener()
			connection := connectionFor(t, scheme)
			write := executeWrite(t, opener, connection, "fresh.txt", "created")

			builder := &WriteBuilder{Opener: opener, Connection: connection}
			undo := builder.BuildReverser().Undo(core.Result{Receipt: write.Receipt})
			if undo.Signal != core.ToolDone {
				t.Fatalf("undo: %v %s", undo.Signal, undo.Output)
			}
			read := executeRead(t, opener, connection, "fresh.txt")
			if read.Signal == core.ToolDone {
				t.Fatalf("object survived the undo of its creation: %q", read.Output)
			}
			// Undo again: the object is already gone, and idempotent undo
			// reports done rather than failing the rollback walk.
			again := builder.BuildReverser().Undo(core.Result{Receipt: write.Receipt})
			if again.Signal != core.ToolDone {
				t.Fatalf("second undo: %v %s", again.Signal, again.Output)
			}
		})
	}
}

func TestReceiptSurvivesRedecode(t *testing.T) {
	encoded := encodeObjectReceipt(objectReceipt{Key: "a/b", Existed: true, PriorBase64: "aGk="})
	receipt, err := decodeObjectReceipt(encoded)
	if err != nil || receipt.Key != "a/b" || !receipt.Existed {
		t.Fatalf("receipt = %+v, %v", receipt, err)
	}
	if _, err := decodeObjectReceipt(`{"existed":true}`); err == nil {
		t.Fatal("receipt without a key decoded")
	}
	if _, err := decodeObjectReceipt("not json"); err == nil {
		t.Fatal("malformed receipt decoded")
	}
}

func toolDef(config string) catalog.ToolDef {
	var def catalog.ToolDef
	def.Name = "object_read"
	var parsed map[string]interface{}
	if err := catalogUnmarshal(config, &parsed); err != nil {
		panic(err)
	}
	def.Config = parsed
	return def
}

// srd059 R2.1, AC2: selection and refusal happen at configuration time with
// the fault named.
func TestBucketSelection(t *testing.T) {
	valid := map[string]string{
		"mem":  "mem://agents",
		"file": "file:///tmp/agents",
		"gs":   "gs://agents",
		"s3":   "s3://agents",
	}
	for scheme, bucketURL := range valid {
		t.Run(scheme, func(t *testing.T) {
			schemeName, _, err := probeScheme(ConnectionConfig{BucketURL: bucketURL})
			if err != nil || schemeName != scheme {
				t.Fatalf("probe %q = %q, %v", bucketURL, schemeName, err)
			}
		})
	}
	if _, _, err := probeScheme(ConnectionConfig{BucketURL: "ftp://agents"}); err == nil ||
		!strings.Contains(err.Error(), `unknown scheme "ftp"`) {
		t.Fatalf("ftp probe: %v", err)
	}

	cases := map[string]struct {
		config string
		names  string
	}{
		"missing connection": {
			config: `{"connections": {"a": {"bucket_url": "mem://x"}}}`,
			names:  "connection is required",
		},
		"undeclared connection": {
			config: `{"connection": "b", "connections": {"a": {"bucket_url": "mem://x"}}}`,
			names:  `connection "b" is not declared`,
		},
		"missing bucket_url": {
			config: `{"connection": "a", "connections": {"a": {"endpoint": "http://x"}}}`,
			names:  "declares no bucket_url",
		},
	}
	for name, testCase := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := DecodeConfig(toolDef(testCase.config))
			if err == nil || !strings.Contains(err.Error(), testCase.names) {
				t.Fatalf("decode: %v, want %q named", err, testCase.names)
			}
		})
	}
}

// A missing parameter is a ToolFailed at build time, not a panic or a write.
func TestMissingParametersFailByName(t *testing.T) {
	opener := NewOpener()
	connection := ConnectionConfig{BucketURL: "mem://params"}
	write := (&WriteBuilder{Opener: opener, Connection: connection}).
		Build(paramsResult(map[string]string{"content": "x"})).Execute()
	if write.Signal != core.ToolFailed || !strings.Contains(write.Output, `"key"`) {
		t.Fatalf("write without key: %v %q", write.Signal, write.Output)
	}
	read := (&ReadBuilder{Opener: opener, Connection: connection}).
		Build(paramsResult(map[string]string{})).Execute()
	if read.Signal != core.ToolFailed {
		t.Fatalf("read without key: %v", read.Signal)
	}
}

// The opener shares mem buckets by URL, and distinct URLs stay distinct.
func TestMemBucketsShareByURL(t *testing.T) {
	opener := NewOpener()
	one := ConnectionConfig{BucketURL: "mem://one"}
	two := ConnectionConfig{BucketURL: "mem://two"}
	executeWrite(t, opener, one, "k", "in one")
	read := executeRead(t, opener, two, "k")
	if read.Signal == core.ToolDone {
		t.Fatalf("bucket two sees bucket one's object: %q", read.Output)
	}
	same := executeRead(t, opener, one, "k")
	if same.Output != "in one" {
		t.Fatalf("bucket one lost its object: %q", same.Output)
	}
}
