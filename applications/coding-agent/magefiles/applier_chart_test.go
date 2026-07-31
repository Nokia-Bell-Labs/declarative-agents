// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// These cover the applier-enabled render (srd006): that the Deployment, its
// Service, and the profile keys it mounts agree with each other.
//
// The chart disables the applier by default -- its image bundles helm, kubectl,
// and the chart, and the smoke tests kind-load only the runtime image -- so every
// cluster-level test in the application stands up a mesh without it. The
// packaging path that carries the applier into a cluster is therefore proven only
// here, at the render.

// applierRender is the subset of an applier manifest these tests read: the
// Deployment's pod labels, args, and volumes, and the Service's selector. The
// applier's profiles volume is a projected ConfigMap source, so the volume shape
// nests under projected.sources[].configMap.
type applierRender struct {
	Kind     string `yaml:"kind"`
	Metadata struct {
		Name string `yaml:"name"`
	} `yaml:"metadata"`
	Spec struct {
		// A Service's selector is a flat map; a Deployment's nests under
		// matchLabels. Parsed loosely so one struct reads both.
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
					Projected struct {
						Sources []struct {
							ConfigMap struct {
								Name  string `yaml:"name"`
								Items []struct {
									Key  string `yaml:"key"`
									Path string `yaml:"path"`
								} `yaml:"items"`
							} `yaml:"configMap"`
						} `yaml:"sources"`
					} `yaml:"projected"`
				} `yaml:"volumes"`
			} `yaml:"spec"`
		} `yaml:"template"`
	} `yaml:"spec"`
}

// renderApplierChart stages the chart through the production packaging path and
// returns its applier manifests. Staging matters here: the applier ConfigMap
// carries what the packaging step copied.
func renderApplierChart(t *testing.T, sets ...string) []applierRender {
	t.Helper()
	chart := preparedApplierChart(t)
	args := []string{"--set", "applier.enabled=true"}
	for _, set := range sets {
		args = append(args, "--set", set)
	}
	out := helmTemplate(t, chart, args...)
	var docs []applierRender
	for _, chunk := range strings.Split(out, "\n---") {
		var doc applierRender
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

// applierDoc finds the applier manifest of one kind.
func applierDoc(t *testing.T, docs []applierRender, kind string) applierRender {
	t.Helper()
	for _, doc := range docs {
		if doc.Kind == kind && strings.HasSuffix(doc.Metadata.Name, "-applier") {
			return doc
		}
	}
	t.Fatalf("no applier %s rendered", kind)
	return applierRender{}
}

// TestApplierServiceTargetsItsDeployment proves the Service selects the pods the
// Deployment creates. Each is individually valid whatever the labels say; only
// read together do they show whether the apply surface has anything behind it.
func TestApplierServiceTargetsItsDeployment(t *testing.T) {
	docs := renderApplierChart(t)
	deployment := applierDoc(t, docs, "Deployment")
	service := applierDoc(t, docs, "Service")

	podLabels := deployment.Spec.Template.Metadata.Labels
	if len(podLabels) == 0 {
		t.Fatal("the applier Deployment sets no pod labels")
	}
	selector := stringSelector(service.Spec.Selector)
	if len(selector) == 0 {
		t.Fatal("the applier Service selects nothing; it would route to no pod")
	}
	for key, want := range selector {
		if got, ok := podLabels[key]; !ok || got != want {
			t.Errorf("the Service selects %s=%q but the Deployment's pods carry %q; the apply surface would route nowhere",
				key, want, got)
		}
	}
	if component := selector["app.kubernetes.io/component"]; component != "applier" {
		t.Errorf("the Service selects component %q, want applier", component)
	}
}

// TestApplierMountsEveryProfileItStarts proves the profile the Deployment's args
// name is actually projected into its mount: the applier Deployment must not
// mount a profile the staging list did not copy, or an enabled applier starts
// with nothing to run.
func TestApplierMountsEveryProfileItStarts(t *testing.T) {
	docs := renderApplierChart(t)
	deployment := applierDoc(t, docs, "Deployment")
	if len(deployment.Spec.Template.Spec.Containers) == 0 {
		t.Fatal("the applier Deployment declares no container")
	}
	container := deployment.Spec.Template.Spec.Containers[0]

	profilePath := argAfter(container.Args, "--profile")
	if profilePath == "" {
		t.Fatal("the applier container names no --profile; it would not know what to run")
	}
	mountPath := profileMountPath(container.VolumeMounts)
	if mountPath == "" {
		t.Fatal("the applier container mounts no profiles volume")
	}
	if !strings.HasPrefix(profilePath, mountPath) {
		t.Fatalf("the applier runs %s, which is not under its profiles mount %s", profilePath, mountPath)
	}

	// The path the agent opens, relative to the mount, must be a path the
	// profiles volume projects.
	wanted := strings.TrimPrefix(strings.TrimPrefix(profilePath, mountPath), "/")
	projected := projectedProfilePaths(deployment)
	if len(projected) == 0 {
		t.Fatal("the profiles volume projects no items; every profile would be absent")
	}
	if !projected[wanted] {
		t.Errorf("the applier starts %s but the profiles volume projects no such path; "+
			"an enabled applier would start with no profile", wanted)
	}
}

// TestApplierRendersItsWholeSurface proves an enabled applier brings its whole
// object set, so a partial render cannot pass the narrower tests above.
func TestApplierRendersItsWholeSurface(t *testing.T) {
	docs := renderApplierChart(t)
	for _, kind := range []string{"Deployment", "Service", "ServiceAccount", "NetworkPolicy"} {
		applierDoc(t, docs, kind) // fails the test if absent
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
func projectedProfilePaths(deployment applierRender) map[string]bool {
	paths := map[string]bool{}
	for _, volume := range deployment.Spec.Template.Spec.Volumes {
		if volume.Name != "profiles" {
			continue
		}
		for _, source := range volume.Projected.Sources {
			for _, item := range source.ConfigMap.Items {
				paths[item.Path] = true
			}
		}
	}
	return paths
}
