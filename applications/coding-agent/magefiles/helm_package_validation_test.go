// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"archive/tar"
	"compress/gzip"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

func TestHelmSchemaFixtureMatrix(t *testing.T) {
	if _, err := exec.LookPath("helm"); err != nil {
		t.Skip("helm not on PATH")
	}
	chart := preparedTestChart(t)
	valid, err := filepath.Glob(filepath.Join(chart, "schema-fixtures", "valid-*.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(valid)
	for _, fixture := range valid {
		t.Run(filepath.Base(fixture), func(t *testing.T) {
			output, err := exec.Command("helm", "lint", chart, "-f", fixture).CombinedOutput()
			if err != nil {
				t.Fatalf("valid fixture failed: %v\n%s", err, output)
			}
		})
	}
	expected := map[string]string{
		"invalid-image.yaml":     "/image/repository",
		"invalid-models.yaml":    "/ollama/models",
		"invalid-mount.yaml":     "/workspace/mountPath",
		"invalid-port.yaml":      "/roles/executor/requestPort",
		"invalid-replicas.yaml":  "/roles/planner/replicas",
		"invalid-resources.yaml": "/roles/critic/resources/requests/memory",
		"invalid-storage.yaml":   "/workspace/size",
		"invalid-url.yaml":       "/llm/externalURL",
	}
	for name, reason := range expected {
		t.Run(name, func(t *testing.T) {
			fixture := filepath.Join(chart, "schema-fixtures", name)
			output, err := exec.Command("helm", "lint", chart, "-f", fixture).CombinedOutput()
			if err == nil {
				t.Fatalf("invalid fixture rendered successfully:\n%s", output)
			}
			if !strings.Contains(string(output), reason) {
				t.Fatalf("failure does not identify %s:\n%s", reason, output)
			}
		})
	}
}

func TestHelmSemanticValidationSurvivesSchemaBypass(t *testing.T) {
	chart := preparedTestChart(t)
	cases := []struct {
		name, fixture, reason string
	}{
		{"port conflict", "invalid-port.yaml", "role ports conflict"},
		{"unsafe mount", "invalid-mount.yaml", "workspace.mountPath must be /work"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			output, err := exec.Command(
				"helm", "template", "semantic", chart,
				"--skip-schema-validation",
				"-f", filepath.Join(chart, "schema-fixtures", tc.fixture),
			).CombinedOutput()
			if err == nil || !strings.Contains(string(output), tc.reason) {
				t.Fatalf("semantic validation error = %v, want %q\n%s", err, tc.reason, output)
			}
		})
	}
}

func TestHelmWorkspaceAndTelemetryCombinationsRender(t *testing.T) {
	chart := preparedTestChart(t)
	existing := helmTemplate(t, chart, "-f",
		filepath.Join(chart, "schema-fixtures", "valid-existing-workspace.yaml"))
	if strings.Contains(existing, "kind: PersistentVolumeClaim") ||
		strings.Count(existing, "claimName: coding-agent-shared-workspace") != 3 {
		t.Fatal("existing workspace claim did not replace chart PVC for all roles")
	}
	spool := helmTemplate(t, chart, "-f",
		filepath.Join(chart, "schema-fixtures", "valid-collector-spool.yaml"))
	if !strings.Contains(spool, "app.kubernetes.io/component: collector") ||
		!strings.Contains(spool, "COLLECTOR_MODE") {
		t.Fatal("collector agent spool topology is incoherent")
	}
	disabled := helmTemplate(t, chart, "-f",
		filepath.Join(chart, "schema-fixtures", "valid-no-telemetry.yaml"))
	for _, forbidden := range []string{
		"app.kubernetes.io/component: collector",
		`- "--otel-otlp-endpoint"`,
	} {
		if strings.Contains(disabled, forbidden) {
			t.Errorf("disabled telemetry render contains %q", forbidden)
		}
	}
}

func TestPreparedPackageRejectsManifestAndFileTampering(t *testing.T) {
	cases := []struct {
		name   string
		tamper func(*testing.T, string)
		reason string
	}{
		{
			name: "stale deployment shard",
			tamper: func(t *testing.T, root string) {
				var manifest deploymentPackageManifest
				path := filepath.Join(root, "deployment-manifest.yaml")
				readYAMLFile(t, path, &manifest)
				manifest.Shards[0].Path = "old-critic"
				if err := writeYAML(path, manifest); err != nil {
					t.Fatal(err)
				}
			},
			reason: "stale or malformed",
		},
		{
			name: "unknown role manifest field",
			tamper: func(t *testing.T, root string) {
				path := filepath.Join(root, "manifests", "planner.yaml")
				file, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0)
				if err != nil {
					t.Fatal(err)
				}
				_, _ = file.WriteString("unknown_field: true\n")
				_ = file.Close()
			},
			reason: "field unknown_field not found",
		},
		{
			name: "missing role manifest",
			tamper: func(t *testing.T, root string) {
				if err := os.Remove(filepath.Join(root, "manifests", "critic.yaml")); err != nil {
					t.Fatal(err)
				}
			},
			reason: "read",
		},
		{
			name: "missing role file",
			tamper: func(t *testing.T, root string) {
				if err := os.Remove(filepath.Join(root, "executor", "agents", "executor", "machine.yaml")); err != nil {
					t.Fatal(err)
				}
			},
			reason: "role files",
		},
		{
			name: "unexpected role file",
			tamper: func(t *testing.T, root string) {
				writeTestFile(t, filepath.Join(root, "critic", "unexpected.yaml"), "unexpected: true\n")
			},
			reason: "role files",
		},
		{
			name: "tampered checksum",
			tamper: func(t *testing.T, root string) {
				mutateRoleManifest(t, root, "planner", func(manifest *rolePackageManifest) {
					manifest.Checksum = strings.Repeat("0", 64)
				})
			},
			reason: "checksum mismatch",
		},
		{
			name: "tampered partition size",
			tamper: func(t *testing.T, root string) {
				mutateRoleManifest(t, root, "critic", func(manifest *rolePackageManifest) {
					manifest.ConfigMaps[0].SizeBytes++
				})
			},
			reason: "size",
		},
		{
			name: "duplicate partition asset",
			tamper: func(t *testing.T, root string) {
				mutateRoleManifest(t, root, "executor", func(manifest *rolePackageManifest) {
					manifest.ConfigMaps = append(manifest.ConfigMaps, configMapPartition{
						Index: 1, SizeBytes: manifest.ConfigMaps[0].SizeBytes,
						Files: append([]string(nil), manifest.ConfigMaps[0].Files...),
					})
				})
			},
			reason: "does not match deployment manifest",
		},
		{
			name: "missing partitions",
			tamper: func(t *testing.T, root string) {
				mutateRoleManifest(t, root, "planner", func(manifest *rolePackageManifest) {
					manifest.ConfigMaps = nil
				})
			},
			reason: "does not match deployment manifest",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := copiedCanonicalPackage(t)
			tc.tamper(t, root)
			err := validatePreparedPackage(root)
			if err == nil || !strings.Contains(err.Error(), tc.reason) {
				t.Fatalf("validation error = %v, want %q", err, tc.reason)
			}
		})
	}
}

func TestPreparedPackageRejectsConflictingEncodedAssets(t *testing.T) {
	root := copiedCanonicalPackage(t)
	roleRoot := filepath.Join(root, "planner")
	writeTestFile(t, filepath.Join(roleRoot, "collision", "file.yaml"), "one: true\n")
	writeTestFile(t, filepath.Join(roleRoot, "collision__file.yaml"), "two: true\n")
	mutateRoleManifest(t, root, "planner", func(manifest *rolePackageManifest) {
		manifest.Files = append(manifest.Files,
			"collision/file.yaml", "collision__file.yaml")
		sort.Strings(manifest.Files)
		assets := map[string]string{}
		for _, path := range manifest.Files {
			assets[path] = path
		}
		checksum, err := roleClosureChecksum(roleRoot, assets, manifest.Files)
		if err != nil {
			t.Fatal(err)
		}
		manifest.Checksum = checksum
		manifest.ConfigMaps[0].Files = append(
			manifest.ConfigMaps[0].Files,
			"collision/file.yaml", "collision__file.yaml")
	})
	err := validatePreparedPackage(root)
	if err == nil || !strings.Contains(err.Error(), "ConfigMap key conflict") {
		t.Fatalf("conflicting asset error = %v", err)
	}
}

func TestHelmPackageArchiveIsSelfContained(t *testing.T) {
	if _, err := exec.LookPath("helm"); err != nil {
		t.Skip("helm not on PATH")
	}
	packageRoot, _, cleanup := packageCanonicalDeployment(t)
	defer cleanup()
	archive, err := packageHelmChart(
		filepath.Join("..", "helm"), packageRoot, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := validateChartArchive(archive, packageRoot); err != nil {
		t.Fatal(err)
	}
	empty := t.TempDir()
	cmd := exec.Command("helm", "template", "independent", archive)
	cmd.Dir = empty
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("archive did not render independently: %v\n%s", err, output)
	}
}

func TestHelmPackageArchiveContainsCollectorQueryMachine(t *testing.T) {
	if _, err := exec.LookPath("helm"); err != nil {
		t.Skip("helm not on PATH")
	}
	packageRoot, _, cleanup := packageCanonicalDeployment(t)
	defer cleanup()
	archive, err := packageHelmChart(
		filepath.Join("..", "helm"), packageRoot, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	names := chartArchiveFileNames(t, archive)
	want := "coding-agent/profiles/collector/agents/collector/query-machine.yaml"
	for _, name := range names {
		if name == want {
			return
		}
	}
	t.Fatalf("archive misses the collector request machine %s:\n%s", want, strings.Join(names, "\n"))
}

func TestHelmPackageTwiceExcludesPriorDistAndGeneratedProfiles(t *testing.T) {
	if _, err := exec.LookPath("helm"); err != nil {
		t.Skip("helm not on PATH")
	}
	chart := filepath.Join(t.TempDir(), "helm")
	if err := copyTree(filepath.Join("..", "helm"), chart); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, filepath.Join(chart, "dist", "prior-release.tgz"), "old archive")
	writeTestFile(t, filepath.Join(chart, "profiles", "stale.yaml"), "stale: true\n")
	packageRoot, _, cleanup := packageCanonicalDeployment(t)
	defer cleanup()
	destination := filepath.Join(chart, "dist")
	first, err := packageHelmChart(chart, packageRoot, destination)
	if err != nil {
		t.Fatal(err)
	}
	firstFiles := chartArchiveFileNames(t, first)
	second, err := packageHelmChart(chart, packageRoot, destination)
	if err != nil {
		t.Fatal(err)
	}
	secondFiles := chartArchiveFileNames(t, second)
	if strings.Join(firstFiles, "\n") != strings.Join(secondFiles, "\n") {
		t.Fatalf("repeat package inventory changed:\nfirst=%v\nsecond=%v", firstFiles, secondFiles)
	}
	for _, name := range secondFiles {
		// The chart-root dist/ is release output and must never recurse into the
		// archive; a served UI's ui/dist (srd020 R7) is legitimate chart content,
		// so the exclusion is anchored to the chart root rather than any /dist/.
		if strings.HasPrefix(name, "coding-agent/dist/") || strings.HasSuffix(name, ".tgz") ||
			strings.HasSuffix(name, "/profiles/stale.yaml") {
			t.Errorf("repeat archive embedded excluded source content %s", name)
		}
	}
	if err := validateChartArchive(second, packageRoot); err != nil {
		t.Fatal(err)
	}
}

func TestChartSourceInventoryRejectsUnclassifiedEntry(t *testing.T) {
	chart := filepath.Join(t.TempDir(), "helm")
	if err := copyTree(filepath.Join("..", "helm"), chart); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, filepath.Join(chart, "mystery.txt"), "not classified")
	err := stageChartSource(chart, filepath.Join(t.TempDir(), "staged"))
	if err == nil || !strings.Contains(err.Error(), "unclassified top-level entry") {
		t.Fatalf("unclassified source error = %v", err)
	}
}

func TestChartArchiveValidationRejectsOmissions(t *testing.T) {
	archive := filepath.Join(t.TempDir(), "coding-agent-0.1.0.tgz")
	writeTestArchive(t, archive, map[string]string{
		"coding-agent/Chart.yaml":  "apiVersion: v2\nname: coding-agent\nversion: 0.1.0\n",
		"coding-agent/values.yaml": "{}\n",
	})
	packageRoot, _, cleanup := packageCanonicalDeployment(t)
	defer cleanup()
	err := validateChartArchive(archive, packageRoot)
	if err == nil || !strings.Contains(err.Error(), "missing required files") {
		t.Fatalf("omitted archive error = %v", err)
	}
}

func chartArchiveFileNames(t *testing.T, filename string) []string {
	t.Helper()
	file, err := os.Open(filename)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = file.Close() }()
	gz, err := gzip.NewReader(file)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = gz.Close() }()
	reader := tar.NewReader(gz)
	var names []string
	for {
		header, err := reader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		if !header.FileInfo().IsDir() {
			names = append(names, filepath.ToSlash(header.Name))
		}
	}
	sort.Strings(names)
	return names
}

func copiedCanonicalPackage(t *testing.T) string {
	t.Helper()
	source, _, cleanup := packageCanonicalDeployment(t)
	t.Cleanup(cleanup)
	destination := filepath.Join(t.TempDir(), "profiles")
	if err := copyTree(source, destination); err != nil {
		t.Fatal(err)
	}
	return destination
}

func mutateRoleManifest(
	t *testing.T,
	root, role string,
	mutate func(*rolePackageManifest),
) {
	t.Helper()
	path := filepath.Join(root, "manifests", role+".yaml")
	var manifest rolePackageManifest
	readYAMLFile(t, path, &manifest)
	mutate(&manifest)
	if err := writeYAML(path, manifest); err != nil {
		t.Fatal(err)
	}
}

func writeTestArchive(t *testing.T, filename string, files map[string]string) {
	t.Helper()
	output, err := os.Create(filename)
	if err != nil {
		t.Fatal(err)
	}
	gz := gzip.NewWriter(output)
	tarWriter := tar.NewWriter(gz)
	var names []string
	for name := range files {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		content := []byte(files[name])
		if err := tarWriter.WriteHeader(&tar.Header{
			Name: name, Mode: 0o644, Size: int64(len(content)),
		}); err != nil {
			t.Fatal(err)
		}
		if _, err := tarWriter.Write(content); err != nil {
			t.Fatal(err)
		}
	}
	if err := tarWriter.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	if err := output.Close(); err != nil {
		t.Fatal(err)
	}
}
