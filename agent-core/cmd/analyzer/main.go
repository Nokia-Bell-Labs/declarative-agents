package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/pkg/stl"
)

func main() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

var rootCmd = &cobra.Command{
	Use:     "analyzer",
	Short:   "Analyze and visualize evaluation results",
	Version: "v0.0.0-dev",
}

var reportCmd = &cobra.Command{
	Use:   "report [session-dir...]",
	Short: "Print evaluation reports to stdout",
	Args:  cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		groups, err := stl.LoadMultiple(args)
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

		stats := stl.ComputeModelStats(groups)
		stl.PrintModelTable(w, stats)

		if showDetailed {
			fmt.Fprintln(w)
			rows := stl.ComputeDetailed(groups)
			stl.PrintDetailedTable(w, rows)
		}

		if showProgression {
			fmt.Fprintf(w, "\n--- Tool Progression ---\n\n")
			stl.PrintProgression(w, groups)
		}

		if csvPath != "" {
			if err := stl.WriteCSV(csvPath, groups); err != nil {
				fmt.Fprintf(os.Stderr, "CSV write error: %v\n", err)
			} else {
				fmt.Fprintf(os.Stderr, "CSV written to %s\n", csvPath)
			}
		}

		return nil
	},
}

func init() {
	reportCmd.Flags().Bool("progression", false, "show per-run tool progression timelines")
	reportCmd.Flags().Bool("detailed", false, "show per-(sample, model) convergence breakdown")
	reportCmd.Flags().String("csv", "", "write detailed per-run CSV to this path")

	rootCmd.AddCommand(reportCmd)
	rootCmd.AddCommand(serveCmd)
}
