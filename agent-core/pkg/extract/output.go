// Copyright (c) 2026 Petar Djukic. All rights reserved.
// SPDX-License-Identifier: MIT

package extract

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"

	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/pkg/graph"
	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/pkg/spec"
)

// OutputTask is the YAML-serializable task format. It is designed to be
// compatible with the apply subcommand's inputTask struct.
type OutputTask struct {
	ID      string       `yaml:"id"`
	SRDID   string       `yaml:"srd_id"`
	Release string       `yaml:"release,omitempty"`
	Weight  int          `yaml:"weight"`
	Status  string       `yaml:"status"`
	Deps    []string     `yaml:"deps,omitempty"`
	Items   []OutputItem `yaml:"items"`
	SRD     SRDContext   `yaml:"srd"`
}

// OutputItem is one requirement item within a task.
type OutputItem struct {
	ID   string `yaml:"id"`
	Text string `yaml:"text"`
}

// SRDContext holds the SRD metadata needed by the apply subcommand
// for prompt assembly. Matches plan.SRDContext.
type SRDContext struct {
	Problem            string   `yaml:"problem"`
	Goals              []string `yaml:"goals"`
	AcceptanceCriteria []string `yaml:"acceptance_criteria"`
}

// TasksFile is the top-level YAML structure for the tasks.yaml file.
type TasksFile struct {
	Header Header       `yaml:"header"`
	Tasks  []OutputTask `yaml:"tasks"`
}

// Header contains metadata about the generation run.
type Header struct {
	GeneratedAt     string `yaml:"generated_at"`
	SourceDir       string `yaml:"source_dir"`
	WeightThreshold int    `yaml:"weight_threshold"`
	TaskCount       int    `yaml:"task_count"`
	PlannerVersion  string `yaml:"planner_version"`
}

// BuildOutputTasks converts extracted Tasks into OutputTasks by enriching
// them with SRD context from the corpus and computing inter-task dependencies.
func BuildOutputTasks(tasks []*Task, g *graph.Graph, corpus *spec.Corpus) []OutputTask {
	nodeToTask := make(map[string]string)
	for _, t := range tasks {
		for _, nid := range t.NodeIDs {
			nodeToTask[nid] = t.ID
		}
	}

	var output []OutputTask
	for _, t := range tasks {
		srd := corpus.SRDs[t.SRDID]

		items := make([]OutputItem, len(t.NodeIDs))
		for i, nid := range t.NodeIDs {
			n, _ := g.Node(nid)
			items[i] = OutputItem{ID: stripSRDPrefix(nid, t.SRDID), Text: n.Text}
		}

		deps := computeTaskDeps(t, g, nodeToTask)

		acStrings := make([]string, len(srd.AcceptanceCriteria))
		for i, ac := range srd.AcceptanceCriteria {
			acStrings[i] = ac.ID + ": " + ac.Criterion
		}

		output = append(output, OutputTask{
			ID:      t.ID,
			SRDID:   t.SRDID,
			Release: t.Release,
			Weight:  t.Weight,
			Status:  "pending",
			Deps:    deps,
			Items:   items,
			SRD: SRDContext{
				Problem:            srd.Problem,
				Goals:              srd.Goals,
				AcceptanceCriteria: acStrings,
			},
		})
	}
	return output
}

// computeTaskDeps finds which other tasks this task depends on by
// checking if any predecessor nodes belong to different tasks.
func computeTaskDeps(t *Task, g *graph.Graph, nodeToTask map[string]string) []string {
	taskSet := make(map[string]bool)
	myNodes := make(map[string]bool)
	for _, nid := range t.NodeIDs {
		myNodes[nid] = true
	}

	for _, nid := range t.NodeIDs {
		preds, _ := g.Predecessors(nid)
		for _, p := range preds {
			if myNodes[p.ID] {
				continue
			}
			if depTask, ok := nodeToTask[p.ID]; ok && depTask != t.ID {
				taskSet[depTask] = true
			}
		}
	}

	var deps []string
	for d := range taskSet {
		deps = append(deps, d)
	}
	return deps
}

func stripSRDPrefix(nodeID, srdID string) string {
	prefix := srdID + "-"
	if len(nodeID) > len(prefix) && nodeID[:len(prefix)] == prefix {
		return nodeID[len(prefix):]
	}
	return nodeID
}

// WriteTasks serializes a task list to a YAML file.
func WriteTasks(tasks []OutputTask, path, sourceDir string, weightThreshold int, version string) error {
	if version == "" {
		version = "dev"
	}
	tf := TasksFile{
		Header: Header{
			GeneratedAt:     time.Now().UTC().Format(time.RFC3339),
			SourceDir:       sourceDir,
			WeightThreshold: weightThreshold,
			TaskCount:       len(tasks),
			PlannerVersion:  version,
		},
		Tasks: tasks,
	}

	data, err := yaml.Marshal(&tf)
	if err != nil {
		return fmt.Errorf("marshal tasks: %w", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write tasks to %s: %w", path, err)
	}
	return nil
}

// ReadTasks deserializes a tasks.yaml file.
func ReadTasks(path string) (*TasksFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read tasks from %s: %w", path, err)
	}
	var tf TasksFile
	if err := yaml.Unmarshal(data, &tf); err != nil {
		return nil, fmt.Errorf("unmarshal tasks: %w", err)
	}
	if len(tf.Tasks) == 0 {
		return nil, fmt.Errorf("tasks file %s contains no tasks", path)
	}
	return &tf, nil
}
