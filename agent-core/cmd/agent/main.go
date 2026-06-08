// Copyright (c) 2026 Nokia. All rights reserved.

// Command agent is the unified agentic-loop binary. It loads a state machine
// and tools from YAML configuration, then runs core.Loop. Different modes
// (generate, pipeline, eval) are selected entirely by config files.
//
// Usage:
//
//	agent --machine <machine.yaml> --tools <tools.yaml> [flags]
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/pkg/core"
	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/pkg/stl"
	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/pkg/telemetry"
)

var (
	flagMachine    string
	flagTools      string
	flagOTelLog    string
	flagOTelParent string
	flagModel      string
	flagOllamaURL  string
	flagNumCtx     int
	flagLLMTimeout int
	flagMaxTime    int
	flagMaxTokens  int
	flagDirectory  string
)

func main() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

var rootCmd = &cobra.Command{
	Use:   "agent",
	Short: "Unified agentic-loop binary",
	Long: `agent loads a state machine and tools from YAML config and runs core.Loop.

Different modes (generate, pipeline, eval) are selected by which
--machine and --tools files you pass.`,
	SilenceUsage: true,
	RunE:         run,
}

func init() {
	f := rootCmd.Flags()
	f.StringVar(&flagMachine, "machine", "", "path to state machine YAML (required)")
	f.StringVar(&flagTools, "tools", "", "path to tools YAML (required)")
	f.StringVar(&flagOTelLog, "otel-log-file", "", "path to OTel trace output file")
	f.StringVar(&flagOTelParent, "otel-parent-span", "", "W3C traceparent for parent span")
	f.StringVar(&flagModel, "model", "", "LLM model name")
	f.StringVar(&flagOllamaURL, "ollama-url", "http://localhost:11434", "Ollama server URL")
	f.IntVar(&flagNumCtx, "num-ctx", 0, "context window size (0 = model default)")
	f.IntVar(&flagLLMTimeout, "llm-timeout", 0, "per-LLM-call timeout in seconds (0 = no limit)")
	f.IntVar(&flagMaxTime, "max-time", 0, "max wall-clock time in seconds (0 = no limit)")
	f.IntVar(&flagMaxTokens, "max-tokens", 0, "max token budget (0 = no limit)")
	f.StringVar(&flagDirectory, "directory", "", "workspace directory")

	rootCmd.AddCommand(versionCmd)
	rootCmd.AddCommand(generateMachineCmd)
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print agent version",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("agent v0.0.0-dev")
	},
}

var generateMachineCmd = &cobra.Command{
	Use:   "generate-machine <generate-spec.yaml>",
	Short: "Generate a linear state machine from a generate spec",
	Long: `Reads a GenerateSpec YAML (points × steps) and produces a flat
MachineSpec YAML on stdout. Use this to unroll evaluator experiment
loops into linear state machines with no cycles.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		data, err := os.ReadFile(args[0])
		if err != nil {
			return fmt.Errorf("read spec: %w", err)
		}

		var gen core.GenerateSpec
		if err := yaml.Unmarshal(data, &gen); err != nil {
			return fmt.Errorf("parse spec: %w", err)
		}

		if len(gen.Points) == 0 {
			return fmt.Errorf("generate spec has no points")
		}

		spec := core.GenerateLinearMachine(gen)
		out, err := core.MarshalMachineSpec(spec)
		if err != nil {
			return fmt.Errorf("marshal machine: %w", err)
		}

		fmt.Print(string(out))
		return nil
	},
}

func run(cmd *cobra.Command, args []string) error {
	if flagMachine == "" {
		return fmt.Errorf("--machine is required")
	}
	if flagTools == "" {
		return fmt.Errorf("--tools is required")
	}

	vars := map[string]string{
		"model":      flagModel,
		"directory":  flagDirectory,
		"ollama_url": flagOllamaURL,
	}

	// Load tool definitions
	defs, err := stl.LoadToolDefs(flagTools)
	if err != nil {
		return fmt.Errorf("load tools: %w", err)
	}

	// Set up OTel if configured
	var tracer telemetry.TraceAdapter
	if flagOTelLog != "" {
		parentCtx, _ := telemetry.ParseParentSpan(flagOTelParent)
		cfg := telemetry.ExporterConfig{FilePath: flagOTelLog}
		t, shutdown, err := telemetry.NewRoot("agent", "agent.run", cfg, parentCtx)
		if err != nil {
			return fmt.Errorf("otel init: %w", err)
		}
		defer shutdown()
		tracer = telemetry.TraceAdapter{T: t}
	}

	// Build registries
	builtins := stl.NewBuiltinRegistry()
	registerBuiltinFactories(builtins)

	reg := core.NewRegistry()
	if err := stl.RegisterUnifiedTools(reg, builtins, flagDirectory, defs, vars); err != nil {
		return fmt.Errorf("register tools: %w", err)
	}

	// Run the loop
	params := core.LoopParams{
		MachineFile:  flagMachine,
		AgentName:    "agent",
		ModelName:    flagModel,
		ProviderName: "ollama",
		Trace:        tracer,
	}

	result, err := core.Loop(params, context.Background())
	if err != nil {
		return fmt.Errorf("loop: %w", err)
	}

	fmt.Fprintf(os.Stderr, "terminal state: %s\n", result.Status)
	return nil
}

// registerBuiltinFactories wires all known builtin tool factories.
// As tool implementations are moved to pkg/stl, their factories
// are registered here.
func registerBuiltinFactories(br *stl.BuiltinRegistry) {
	// File tools
	br.Register("file_read", func(def stl.ToolDef, vars map[string]string) (core.Builder, error) {
		return &stl.ReadBuilder{Root: vars["directory"]}, nil
	})
	br.Register("file_write", func(def stl.ToolDef, vars map[string]string) (core.Builder, error) {
		return &stl.WriteBuilder{Root: vars["directory"]}, nil
	})
	br.Register("file_edit", func(def stl.ToolDef, vars map[string]string) (core.Builder, error) {
		return &stl.EditBuilder{Root: vars["directory"]}, nil
	})
	br.Register("file_find", func(def stl.ToolDef, vars map[string]string) (core.Builder, error) {
		return &stl.FindBuilder{Root: vars["directory"]}, nil
	})
	br.Register("file_list", func(def stl.ToolDef, vars map[string]string) (core.Builder, error) {
		return &stl.ListFilesBuilder{Root: vars["directory"]}, nil
	})
}
