// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// These cover the executor-enabled render (GH-733): that the Deployment, its
// Service, and the profile keys it mounts agree with each other.
//
// helm/ci/kind-values.yaml disables the executor -- its image bundles helm,
// kubectl, and the chart, and the smoke tests kind-load only the runtime image --
// so every cluster-level test in the example stands up a mesh without it. The
// packaging path that carries the executor into a cluster is therefore proven
// only here, at the render.
//
// The NetworkPolicy is already covered by TestExecutorApplyStaysCreatorOnly,
// which walks its from-entries and ports; these do not repeat it.

// executorRender is the subset of an executor manifest these tests read: the
// Deployment's pod labels, args, and volumes, and the Service's selector.
type executorRender struct {
	Kind     string `yaml:"kind"`
	Metadata struct {
		Name string `yaml:"name"`
	} `yaml:"metadata"`
	Spec struct {
		// A Service's selector is a flat map; a Deployment's nests under
		// matchLabels. Parsed loosely so one struct reads both -- a typed
		// map[string]string silently dropped every Deployment chunk.
		Selector map[string]any `yaml:"selector"`
		Template struct {
			Metadata struct {
				Labels map[string]string `yaml:"labels"`
			} `yaml:"metadata"`
			Spec struct {
				Containers []struct {
					Name         string   `yaml:"name"`
					Args         []string `yaml:"args"`
					VolumeMounts []struct {
						Name      string `yaml:"name"`
						MountPath string `yaml:"mountPath"`
					} `yaml:"volumeMounts"`
				} `yaml:"containers"`
				Volumes []struct {
					Name      string `yaml:"name"`
					ConfigMap struct {
						Name  string `yaml:"name"`
						Items []struct {
							Key  string `yaml:"key"`
							Path string `yaml:"path"`
						} `yaml:"items"`
					} `yaml:"configMap"`
				} `yaml:"volumes"`
			} `yaml:"spec"`
		} `yaml:"template"`
	} `yaml:"spec"`
}

// renderExecutorChart stages the chart through the production packaging path and
// returns its executor manifests. Staging matters here: the profiles ConfigMap
// carries what the packaging step copied, which is the thing GH-485 got wrong.
func renderExecutorChart(t *testing.T, sets ...string) []executorRender {
	t.Helper()
	if _, err := exec.LookPath("helm"); err != nil {
		t.Skip("helm not on PATH")
	}
	chartDir := findChartDir(t)
	staged, cleanup, err := stageSmokeChart(chartDir, filepath.Dir(chartDir))
	if err != nil {
		t.Fatalf("stage chart: %v", err)
	}
	defer cleanup()

	args := []string{"template", "rel", staged, "--set", "executor.enabled=true"}
	for _, set := range sets {
		args = append(args, "--set", set)
	}
	out, err := exec.Command("helm", args...).CombinedOutput()
	if err != nil {
		t.Fatalf("helm template: %v\n%s", err, out)
	}
	var docs []executorRender
	for _, chunk := range strings.Split(string(out), "\n---") {
		var doc executorRender
		if err := yaml.Unmarshal([]byte(chunk), &doc); err != nil {
			continue // not a manifest, such as the NOTES preamble
		}
		if doc.Kind != "" {
			docs = append(docs, doc)
		}
	}
	if len(docs) == 0 {
		t.Fatal("the render produced no manifests")
	}
	return docs
}

// executorDoc finds the executor manifest of one kind.
func executorDoc(t *testing.T, docs []executorRender, kind string) executorRender {
	t.Helper()
	for _, doc := range docs {
		if doc.Kind == kind && strings.HasSuffix(doc.Metadata.Name, "-executor") {
			return doc
		}
	}
	t.Fatalf("no executor %s rendered", kind)
	return executorRender{}
}

// TestExecutorServiceTargetsItsDeployment proves the Service selects the pods the
// Deployment creates. Each is individually valid whatever the labels say; only
// read together do they show whether the apply surface has anything behind it.
func TestExecutorServiceTargetsItsDeployment(t *testing.T) {
	docs := renderExecutorChart(t)
	deployment := executorDoc(t, docs, "Deployment")
	service := executorDoc(t, docs, "Service")

	podLabels := deployment.Spec.Template.Metadata.Labels
	if len(podLabels) == 0 {
		t.Fatal("the executor Deployment sets no pod labels")
	}
	selector := stringSelector(service.Spec.Selector)
	if len(selector) == 0 {
		t.Fatal("the executor Service selects nothing; it would route to no pod")
	}
	for key, want := range selector {
		if got, ok := podLabels[key]; !ok || got != want {
			t.Errorf("the Service selects %s=%q but the Deployment's pods carry %q; the apply surface would route nowhere",
				key, want, got)
		}
	}
	if component := selector["app.kubernetes.io/component"]; component != "executor" {
		t.Errorf("the Service selects component %q, want executor", component)
	}
}

// TestExecutorMountsEveryProfileItStarts proves the profile the Deployment's args
// name is actually projected into its mount. This is GH-485 asserted over the
// render: the executor Deployment mounted a profile the staging list did not
// copy, and an enabled executor started with nothing to run.
func TestExecutorMountsEveryProfileItStarts(t *testing.T) {
	docs := renderExecutorChart(t)
	deployment := executorDoc(t, docs, "Deployment")
	if len(deployment.Spec.Template.Spec.Containers) == 0 {
		t.Fatal("the executor Deployment declares no container")
	}
	container := deployment.Spec.Template.Spec.Containers[0]

	profilePath := argAfter(container.Args, "--profile")
	if profilePath == "" {
		t.Fatal("the executor container names no --profile; it would not know what to run")
	}
	mountPath := profileMountPath(container.VolumeMounts)
	if mountPath == "" {
		t.Fatal("the executor container mounts no profiles volume")
	}
	if !strings.HasPrefix(profilePath, mountPath) {
		t.Fatalf("the executor runs %s, which is not under its profiles mount %s", profilePath, mountPath)
	}

	// The path the agent opens, relative to the mount, must be a path the
	// profiles volume projects.
	wanted := strings.TrimPrefix(strings.TrimPrefix(profilePath, mountPath), "/")
	projected := projectedProfilePaths(deployment)
	if len(projected) == 0 {
		t.Fatal("the profiles volume projects no items; every profile would be absent")
	}
	if !projected[wanted] {
		t.Errorf("the executor starts %s but the profiles volume projects no such path; "+
			"an enabled executor would start with no profile (GH-485)", wanted)
	}
}

// TestExecutorRendersItsWholeSurface proves an enabled executor brings its whole
// object set, so a partial render cannot pass the narrower tests above.
func TestExecutorRendersItsWholeSurface(t *testing.T) {
	docs := renderExecutorChart(t)
	for _, kind := range []string{"Deployment", "Service", "ServiceAccount", "NetworkPolicy"} {
		executorDoc(t, docs, kind) // fails the test if absent
	}
}

// stringSelector keeps the flat string pairs of a selector, which is the shape a
// Service uses to pick pods.
func stringSelector(selector map[string]any) map[string]string {
	flat := map[string]string{}
	for key, value := range selector {
		if text, ok := value.(string); ok {
			flat[key] = text
		}
	}
	return flat
}

// argAfter returns the value following flag in an argument list.
func argAfter(args []string, flag string) string {
	for i, arg := range args {
		if arg == flag && i+1 < len(args) {
			return args[i+1]
		}
	}
	return ""
}

// profileMountPath returns where the container mounts the profiles volume.
func profileMountPath(mounts []struct {
	Name      string `yaml:"name"`
	MountPath string `yaml:"mountPath"`
}) string {
	for _, mount := range mounts {
		if mount.Name == "profiles" {
			return mount.MountPath
		}
	}
	return ""
}

// projectedProfilePaths returns the set of paths the profiles volume projects,
// which is what actually appears under the mount.
func projectedProfilePaths(deployment executorRender) map[string]bool {
	paths := map[string]bool{}
	for _, volume := range deployment.Spec.Template.Spec.Volumes {
		if volume.Name != "profiles" {
			continue
		}
		for _, item := range volume.ConfigMap.Items {
			paths[item.Path] = true
		}
	}
	return paths
}
