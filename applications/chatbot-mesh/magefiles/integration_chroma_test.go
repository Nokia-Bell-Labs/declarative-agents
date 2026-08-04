// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestDemoChromaIngestTimeoutDefaultAndOverride(t *testing.T) {
	t.Parallel()
	t.Run("default", func(t *testing.T) {
		t.Parallel()
		if got := demoChromaIngestTimeout(t.TempDir()); got != chromaIngestTimeoutDefault {
			t.Fatalf("demoChromaIngestTimeout() = %s, want %s", got, chromaIngestTimeoutDefault)
		}
	})
	t.Run("override", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		writeDemoConfig(t, root, "chroma_ingest_timeout: 17m30s")
		config, err := loadDemoConfig(root)
		if err != nil {
			t.Fatalf("loadDemoConfig: %v", err)
		}
		if config.ChromaIngestTimeout != "17m30s" {
			t.Fatalf("parsed chroma_ingest_timeout = %q, want 17m30s", config.ChromaIngestTimeout)
		}
		if got := demoChromaIngestTimeout(root); got != 17*time.Minute+30*time.Second {
			t.Fatalf("demoChromaIngestTimeout() = %s, want 17m30s", got)
		}
	})
}

func TestChromaIngestTimeoutRejectsUnusableValues(t *testing.T) {
	t.Parallel()
	for _, value := range []string{"", "   ", "not-a-duration", "0s", "-5s"} {
		if got := chromaIngestTimeoutFrom(demoConfig{ChromaIngestTimeout: value}); got != chromaIngestTimeoutDefault {
			t.Errorf("chromaIngestTimeoutFrom(%q) = %s, want default %s",
				value, got, chromaIngestTimeoutDefault)
		}
	}
}

func TestRunChromaAgentTimeoutDiagnosis(t *testing.T) {
	t.Parallel()
	err := runChromaAgentWithTimeout(10*time.Millisecond, func(ctx context.Context) ([]byte, error) {
		<-ctx.Done()
		return []byte("last complete ingest event"), errors.New("signal: killed")
	})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("runChromaAgentWithTimeout() error = %v, want context deadline exceeded", err)
	}
	for _, want := range []string{
		"chroma ingest exceeded its 10ms whole-run timeout",
		"after ",
		"deadline ",
		"chroma_ingest_timeout in " + demoConfigFile,
		"last complete ingest event",
	} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("timeout error = %q, want it to name %q", err, want)
		}
	}
	if strings.Contains(err.Error(), "signal: killed") {
		t.Errorf("timeout error retained bare process diagnosis: %q", err)
	}
}

// TestRunChromaAgentWaitsForRunnerCleanup proves the bounded wrapper does not
// return when its context expires; it returns only after the command boundary
// has finished its kill-and-Wait path and therefore reaped the owned child.
func TestRunChromaAgentWaitsForRunnerCleanup(t *testing.T) {
	t.Parallel()
	timeoutObserved := make(chan struct{})
	allowReap := make(chan struct{})
	reaped := make(chan struct{})
	result := make(chan error, 1)

	go func() {
		result <- runChromaAgentWithTimeout(10*time.Millisecond, func(ctx context.Context) ([]byte, error) {
			<-ctx.Done()
			close(timeoutObserved)
			<-allowReap
			close(reaped)
			return nil, ctx.Err()
		})
	}()

	select {
	case <-timeoutObserved:
	case <-time.After(time.Second):
		t.Fatal("runner did not observe the ingest deadline")
	}
	select {
	case err := <-result:
		t.Fatalf("bounded run returned before runner cleanup: %v", err)
	default:
	}

	close(allowReap)
	select {
	case err := <-result:
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("bounded run error = %v, want context deadline exceeded", err)
		}
	case <-time.After(time.Second):
		t.Fatal("bounded run did not return after runner cleanup")
	}
	select {
	case <-reaped:
	default:
		t.Fatal("bounded run returned before runner reported the child reaped")
	}
}

func TestStartRequiredChromaContainerClassifiesLaunchOutcome(t *testing.T) {
	t.Parallel()
	launchErr := errors.New("docker run failed")
	tests := []struct {
		name    string
		id      string
		err     error
		wantErr bool
	}{
		{name: "started", id: "container-id"},
		{name: "docker failure", err: launchErr, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			id, err := startRequiredChromaContainer("/data", func(string) (string, error) {
				return tt.id, tt.err
			})
			if tt.wantErr {
				if !errors.Is(err, launchErr) {
					t.Fatalf("error = %v, want wrapped launch error", err)
				}
				return
			}
			if err != nil || id != tt.id {
				t.Fatalf("result = (%q, %v), want (%q, nil)", id, err, tt.id)
			}
		})
	}
}

func TestChromaRequiredModelsFromConfig(t *testing.T) {
	root := t.TempDir()
	writeChromaConfigFile(t, filepath.Join(root, corpusRestAsset),
		"rest:\n  clients:\n    ollama:\n      operations:\n        embed:\n          body:\n            model: embed-model\n")
	decl := "tools:\n  - name: read_resource\n  - name: invoke_llm\n    config:\n      model: chat-model\n"
	writeChromaConfigFile(t, filepath.Join(root, "agents", "knowledge-manager", "corpus-ingest", "profile.yaml"), "name: corpus-ingest\n")
	writeChromaConfigFile(t, filepath.Join(root, "agents", "knowledge-manager", "corpus-ingest", "declarations.yaml"), decl)

	got, err := chromaRequiredModels(root)
	if err != nil {
		t.Fatalf("chromaRequiredModels: %v", err)
	}
	want := []string{"chat-model", "embed-model"}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("required models = %v, want %v (distinct, sorted)", got, want)
	}
}

func TestChromaRequiredModelsUseDeploymentEnvironment(t *testing.T) {
	t.Setenv("CORPUS_EMBEDDING_MODEL", "all-minilm")
	t.Setenv("CORPUS_CHAT_MODEL", "qwen2.5:0.5b")
	root := filepath.Dir(findChartDir(t))
	got, err := chromaRequiredModels(root)
	if err != nil {
		t.Fatalf("chromaRequiredModels: %v", err)
	}
	want := []string{"all-minilm", "qwen2.5:0.5b"}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("required models = %v, want environment-selected %v", got, want)
	}
}

func TestChromaRequiredModelsMissingInvokeLLM(t *testing.T) {
	root := t.TempDir()
	writeChromaConfigFile(t, filepath.Join(root, corpusRestAsset),
		"rest:\n  clients:\n    ollama:\n      operations:\n        embed:\n          body:\n            model: embed-model\n")
	writeChromaConfigFile(t, filepath.Join(root, "agents", "knowledge-manager", "corpus-ingest", "profile.yaml"), "name: corpus-ingest\n")
	writeChromaConfigFile(t, filepath.Join(root, "agents", "knowledge-manager", "corpus-ingest", "declarations.yaml"), "tools:\n  - name: read_resource\n")
	if _, err := chromaRequiredModels(root); err == nil {
		t.Fatal("expected an error when the ingest profile has no invoke_llm model")
	}
}

func writeChromaConfigFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func TestChromaModelInstalledTagTolerance(t *testing.T) {
	names := []string{"qwen3-embedding:8b:latest", "ornith:9b"}
	for _, model := range []string{"qwen3-embedding:8b", "qwen3-embedding:8b:latest", "ornith:9b"} {
		if !chromaModelInstalled(names, model) {
			t.Errorf("model %q should be reported installed against %v", model, names)
		}
	}
	if chromaModelInstalled(names, "nomic-embed-text") {
		t.Errorf("absent model should not be reported installed")
	}
}

func TestAssertChromaIngestTrace(t *testing.T) {
	trace := writeChromaTrace(t, []string{
		spanLine("2026-07-18T02:00:00.100000000Z", "execute_tool chroma_ready", "chroma_ready"),
		spanLine("2026-07-18T02:00:00.200000000Z", "execute_tool ollama_ready", "ollama_ready"),
		spanLine("2026-07-18T02:00:00.900000000Z", "execute_tool chroma_count", "chroma_count"),
	})
	if err := assertChromaIngestTrace(trace); err != nil {
		t.Fatalf("expected ingest trace to pass, got %v", err)
	}
}

func TestAssertChromaIngestTraceMissingWord(t *testing.T) {
	trace := writeChromaTrace(t, []string{
		spanLine("2026-07-18T02:00:00.100000000Z", "execute_tool chroma_ready", "chroma_ready"),
		spanLine("2026-07-18T02:00:00.200000000Z", "execute_tool ollama_ready", "ollama_ready"),
	})
	if err := assertChromaIngestTrace(trace); err == nil {
		t.Fatal("expected ingest trace without chroma_count to fail")
	}
}

// spanLine renders one stdouttrace-style ndjson span carrying a command.name.
func spanLine(start, name, commandName string) string {
	attrs := `{"Key":"command.name","Value":{"Type":"STRING","Value":"` + commandName + `"}}`
	return `{"Name":"` + name + `","StartTime":"` + start + `","Attributes":[` + attrs + `]}`
}

func writeChromaTrace(t *testing.T, lines []string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "trace.ndjson")
	content := ""
	for _, line := range lines {
		content += line + "\n"
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write trace: %v", err)
	}
	return path
}
