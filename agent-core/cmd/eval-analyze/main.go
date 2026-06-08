// Copyright (c) 2026 Nokia. All rights reserved.

// Command eval-analyze reads evaluation session results and prints
// reports. It has no dependency on the agent runtime — it only reads
// result JSON files and produces tables, progression timelines, and CSV.
//
// Usage:
//
//	eval-analyze [flags] <session-dir...>
package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/pkg/eval"
)

func main() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

var rootCmd = &cobra.Command{
	Use:   "eval-analyze [session-dir...]",
	Short: "Analyze results from one or more evaluation sessions",
	Long: `Analyze results from one or more session directories. When multiple
directories are provided, results are merged for cross-run comparison.

Examples:
  eval-analyze benchmark/results/2026-06-05-22-54
  eval-analyze results/run1 results/run2
  eval-analyze --progression benchmark/results/2026-06-05-22-54`,
	Version: "v0.0.0-dev",
	Args:    cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		groups, err := eval.LoadMultiple(args)
		if err != nil {
			return err
		}
		if len(groups) == 0 {
			return fmt.Errorf("no results found in provided directories")
		}

		w := cmd.OutOrStdout()

		showProgression, _ := cmd.Flags().GetBool("progression")
		showDetailed, _ := cmd.Flags().GetBool("detailed")
		csvPath, _ := cmd.Flags().GetString("csv")

		stats := eval.ComputeModelStats(groups)
		eval.PrintModelTable(w, stats)

		if showDetailed {
			fmt.Fprintln(w)
			rows := eval.ComputeDetailed(groups)
			eval.PrintDetailedTable(w, rows)
		}

		if showProgression {
			fmt.Fprintf(w, "\n--- Tool Progression ---\n\n")
			eval.PrintProgression(w, groups)
		}

		if csvPath != "" {
			if err := eval.WriteCSV(csvPath, groups); err != nil {
				fmt.Fprintf(os.Stderr, "CSV write error: %v\n", err)
			} else {
				fmt.Fprintf(os.Stderr, "CSV written to %s\n", csvPath)
			}
		}

		return nil
	},
}

func init() {
	rootCmd.Flags().Bool("progression", false, "show per-run tool progression timelines")
	rootCmd.Flags().Bool("detailed", false, "show per-(sample, model) convergence breakdown")
	rootCmd.Flags().String("csv", "", "write detailed per-run CSV to this path")
}
