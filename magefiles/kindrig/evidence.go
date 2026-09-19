// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package kindrig

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// EvidenceManifestFile indexes every evidence directory so a reader, person or
// rig-doctor, learns what was captured and what failed without listing the
// directory (srd023 R1.1).
const EvidenceManifestFile = "manifest.yaml"

// Evidence file kinds recorded in the manifest (srd023 R1.2).
const (
	EvidenceEvents        = "events"
	EvidenceClusterEvents = "cluster-events"
	EvidenceDescribe      = "describe"
	EvidencePods          = "pods"
	EvidenceRollout       = "rollout"
	EvidenceLogs          = "logs"
	EvidenceTraces        = "traces"
	EvidenceKindLogs      = "kind-logs"
)

const (
	clusterEventsFile = "cluster-events.txt"
	traceTailFile     = "traces/collector.ndjson"
	// traceTailBytes bounds the spool tail copied into evidence; the host
	// spool outlives every run and grows without limit.
	traceTailBytes = 8 << 20
)

// FailureEvidence describes the persistent diagnostics to collect when a
// scenario fails or an operator asks for a diagnosis. Directory is the final
// artifact directory, and Namespaces limits kubectl collection to
// scenario-owned namespaces. Revision and TraceSpool are optional: the
// revision is recorded in the manifest, and a trace spool path copies its
// newest spans into the evidence.
type FailureEvidence struct {
	Directory  string
	Namespaces []string
	Run        CommandRunner
	Revision   string
	TraceSpool string
}

// EvidenceFile is one manifest entry; paths are relative to the evidence
// directory.
type EvidenceFile struct {
	Path      string `yaml:"path"`
	Kind      string `yaml:"kind"`
	Namespace string `yaml:"namespace,omitempty"`
	Pod       string `yaml:"pod,omitempty"`
}

// EvidenceManifest is the contents of manifest.yaml.
type EvidenceManifest struct {
	Cluster       string         `yaml:"cluster"`
	Namespaces    []string       `yaml:"namespaces"`
	CapturedAt    string         `yaml:"captured_at"`
	Revision      string         `yaml:"revision,omitempty"`
	Files         []EvidenceFile `yaml:"files"`
	CaptureErrors []string       `yaml:"capture_errors"`
}

// Capture persists kind logs and namespace diagnostics for a cluster that stays
// running. A scenario on a shared cluster captures its own namespace this way,
// and so does mage diagnose; an owned cluster's scenario uses ReleaseAfter.
func (e FailureEvidence) Capture(kindRun Runner, cluster string) error {
	return e.capture(kindRun, cluster)
}

// evidenceRecorder accumulates manifest entries and capture errors. A failed
// step is recorded and capture continues (srd023 R1.4).
type evidenceRecorder struct {
	dir    string
	files  []EvidenceFile
	errors []error
}

func (r *evidenceRecorder) fail(err error) {
	r.errors = append(r.errors, err)
}

func (e FailureEvidence) capture(kindRun Runner, cluster string) error {
	if e.Directory == "" {
		return fmt.Errorf("evidence directory is required")
	}
	if err := os.MkdirAll(e.Directory, 0o755); err != nil {
		return fmt.Errorf("create evidence directory: %w", err)
	}
	rec := &evidenceRecorder{dir: e.Directory}
	if err := ExportLogs(kindRun, cluster, filepath.Join(e.Directory, "kind")); err != nil {
		rec.fail(err)
	} else {
		rec.files = append(rec.files, EvidenceFile{Path: "kind", Kind: EvidenceKindLogs})
	}
	if e.Run == nil {
		if len(e.Namespaces) > 0 {
			rec.fail(fmt.Errorf("kubectl diagnostic runner is required"))
		}
	} else {
		e.captureCommand(rec, EvidenceFile{Path: clusterEventsFile, Kind: EvidenceClusterEvents},
			"kubectl", "get", "events", "-A", "--sort-by=.lastTimestamp")
		for _, namespace := range e.Namespaces {
			e.captureNamespace(rec, namespace)
		}
	}
	if e.TraceSpool != "" {
		if err := copyTraceTail(e.TraceSpool, filepath.Join(e.Directory, traceTailFile)); err != nil {
			rec.fail(err)
		} else {
			rec.files = append(rec.files, EvidenceFile{Path: traceTailFile, Kind: EvidenceTraces})
		}
	}
	if err := e.writeManifest(rec, cluster); err != nil {
		rec.fail(err)
	}
	return errors.Join(rec.errors...)
}

func (e FailureEvidence) captureNamespace(rec *evidenceRecorder, namespace string) {
	base := "namespace-" + evidenceName(namespace)
	file := func(kind string) EvidenceFile {
		return EvidenceFile{Path: base + "-" + kind + ".txt", Kind: kind, Namespace: namespace}
	}
	e.captureCommand(rec, file(EvidenceDescribe), "kubectl", "describe", "all", "-n", namespace)
	e.captureCommand(rec, file(EvidenceEvents),
		"kubectl", "get", "events", "-n", namespace, "--sort-by=.lastTimestamp")
	e.captureCommand(rec, file(EvidenceRollout),
		"kubectl", "get", "deployments,statefulsets,daemonsets", "-n", namespace, "-o", "wide")
	pods, err := e.Run("kubectl", "get", "pods", "-n", namespace, "-o", "name")
	podsFile := file(EvidencePods)
	if writeErr := writeDiagnostic(filepath.Join(e.Directory, podsFile.Path), pods, err); writeErr != nil {
		rec.fail(writeErr)
	} else {
		rec.files = append(rec.files, podsFile)
	}
	if err != nil {
		rec.fail(fmt.Errorf("list pods in %s: %w", namespace, err))
		return
	}
	for _, pod := range strings.Fields(string(pods)) {
		e.captureCommand(rec, EvidenceFile{
			Path: base + "-" + evidenceName(pod) + "-logs.txt", Kind: EvidenceLogs,
			Namespace: namespace, Pod: strings.TrimPrefix(pod, "pod/"),
		}, "kubectl", "logs", "-n", namespace, pod, "--all-containers=true",
			"--prefix=true", "--tail=-1")
	}
}

// captureCommand writes one command's output to file.Path. The file is listed
// whenever it was written, even when the command failed, because the failure
// text itself is evidence.
func (e FailureEvidence) captureCommand(
	rec *evidenceRecorder, file EvidenceFile, name string, args ...string,
) {
	output, commandErr := e.Run(name, args...)
	if err := writeDiagnostic(filepath.Join(e.Directory, file.Path), output, commandErr); err != nil {
		rec.fail(err)
		return
	}
	rec.files = append(rec.files, file)
	if commandErr != nil {
		rec.fail(fmt.Errorf("%s %s: %w", name, strings.Join(args, " "), commandErr))
	}
}

func (e FailureEvidence) writeManifest(rec *evidenceRecorder, cluster string) error {
	manifest := EvidenceManifest{
		Cluster:       cluster,
		Namespaces:    append([]string{}, e.Namespaces...),
		CapturedAt:    time.Now().UTC().Format(time.RFC3339),
		Revision:      e.Revision,
		Files:         append([]EvidenceFile{}, rec.files...),
		CaptureErrors: []string{},
	}
	for _, err := range rec.errors {
		manifest.CaptureErrors = append(manifest.CaptureErrors, err.Error())
	}
	data, err := yaml.Marshal(manifest)
	if err != nil {
		return fmt.Errorf("encode evidence manifest: %w", err)
	}
	path := filepath.Join(e.Directory, EvidenceManifestFile)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write evidence manifest %s: %w", path, err)
	}
	return nil
}

// copyTraceTail copies at most traceTailBytes of the newest whole NDJSON lines
// from spool into dest.
func copyTraceTail(spool, dest string) error {
	source, err := os.Open(spool)
	if err != nil {
		return fmt.Errorf("trace spool %s: %w", spool, err)
	}
	defer source.Close()
	info, err := source.Stat()
	if err != nil {
		return fmt.Errorf("trace spool %s: %w", spool, err)
	}
	offset := info.Size() - traceTailBytes
	if offset < 0 {
		offset = 0
	}
	if _, err := source.Seek(offset, io.SeekStart); err != nil {
		return fmt.Errorf("trace spool %s: %w", spool, err)
	}
	data, err := io.ReadAll(source)
	if err != nil {
		return fmt.Errorf("trace spool %s: %w", spool, err)
	}
	if offset > 0 {
		// The seek landed mid-line; drop the partial first line.
		if i := bytes.IndexByte(data, '\n'); i >= 0 {
			data = data[i+1:]
		} else {
			data = nil
		}
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return fmt.Errorf("create trace evidence directory: %w", err)
	}
	if err := os.WriteFile(dest, data, 0o644); err != nil {
		return fmt.Errorf("write trace evidence %s: %w", dest, err)
	}
	return nil
}

func writeDiagnostic(path string, output []byte, commandErr error) error {
	data := append([]byte(nil), output...)
	if commandErr != nil {
		data = append(data, []byte(fmt.Sprintf("\n[command failed: %v]\n", commandErr))...)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write diagnostic %s: %w", path, err)
	}
	return nil
}

var nonEvidenceName = regexp.MustCompile(`[^A-Za-z0-9._-]+`)

func evidenceName(value string) string {
	return strings.Trim(nonEvidenceName.ReplaceAllString(value, "-"), "-")
}
