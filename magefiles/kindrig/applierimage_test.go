// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package kindrig

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestEnsureApplierImageReusesVerifiedIdentity(t *testing.T) {
	root := applierImageTestRoot(t, "FROM ${RUNTIME_IMAGE}\n")
	revision := strings.Repeat("a", 40)
	runtimeImage := "agent-core:test"
	applierImage := "applier:test"
	var identity applierImageIdentity
	builds := 0
	b := imageBuilder{
		run: func(string, []string, string, ...string) error {
			builds++
			return nil
		},
		output: func(_ string, _ []string, name string, args ...string) ([]byte, error) {
			if name == "git" {
				return []byte(revision), nil
			}
			switch args[len(args)-1] {
			case runtimeImage:
				return agentCoreRuntimeInspectPayload(revision, "sha256:runtime"), nil
			case applierImage:
				return applierInspectPayload(identity, "sha256:applier"), nil
			default:
				return nil, errors.New("not found")
			}
		},
		lockRoot: t.TempDir(),
		sleep:    time.Sleep,
	}
	var err error
	identity, err = b.applierIdentity(root, runtimeImage)
	if err != nil {
		t.Fatal(err)
	}
	result, err := b.ensureApplier(root, runtimeImage, applierImage)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Reused || result.ImageID != "sha256:applier" ||
		result.RuntimeID != "sha256:runtime" || builds != 0 {
		t.Fatalf("result=%+v builds=%d, want verified reuse", result, builds)
	}
}

func TestEnsureApplierImageRebuildsStaleIdentityAndVerifiesResult(t *testing.T) {
	root := applierImageTestRoot(t, "FROM ${RUNTIME_IMAGE}\nRUN true\n")
	revision := strings.Repeat("b", 40)
	runtimeImage := "agent-core:test"
	applierImage := "applier:test"
	var identity applierImageIdentity
	built := false
	var buildArgs []string
	b := imageBuilder{
		run: func(_ string, _ []string, name string, args ...string) error {
			if name == "docker" {
				built = true
				buildArgs = append([]string(nil), args...)
			}
			return nil
		},
		output: func(_ string, _ []string, name string, args ...string) ([]byte, error) {
			if name == "git" {
				return []byte(revision), nil
			}
			switch args[len(args)-1] {
			case runtimeImage:
				return agentCoreRuntimeInspectPayload(revision, "sha256:runtime"), nil
			case applierImage:
				if built {
					return applierInspectPayload(identity, "sha256:rebuilt"), nil
				}
				stale := identity
				stale.recipe = "sha256:stale"
				return applierInspectPayload(stale, "sha256:stale"), nil
			default:
				return nil, errors.New("not found")
			}
		},
		lockRoot: t.TempDir(),
		sleep:    time.Sleep,
	}
	var err error
	identity, err = b.applierIdentity(root, runtimeImage)
	if err != nil {
		t.Fatal(err)
	}
	result, err := b.ensureApplier(root, runtimeImage, applierImage)
	if err != nil {
		t.Fatal(err)
	}
	if result.Reused || result.ImageID != "sha256:rebuilt" || !built {
		t.Fatalf("result=%+v built=%v, want verified rebuild", result, built)
	}
	joined := strings.Join(buildArgs, " ")
	for _, want := range []string{
		"build --platform linux/" + runtime.GOARCH,
		"RUNTIME_IMAGE=" + runtimeImage,
		"TARGETARCH=" + runtime.GOARCH,
		"org.opencontainers.image.revision=" + revision,
		applierRecipeLabel + "=" + identity.recipe,
		applierPlatformLabel + "=" + identity.platform,
		applierRuntimeLabel + "=" + identity.runtimeID,
		"-t " + applierImage + " .",
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("docker build args missing %q:\n%s", want, joined)
		}
	}
}

func TestEnsureApplierImageRejectsUnverifiedRuntimeTag(t *testing.T) {
	root := applierImageTestRoot(t, "FROM ${RUNTIME_IMAGE}\n")
	revision := strings.Repeat("c", 40)
	b := imageBuilder{
		run: func(string, []string, string, ...string) error {
			t.Fatal("build ran with an unverified runtime")
			return nil
		},
		output: func(_ string, _ []string, name string, _ ...string) ([]byte, error) {
			if name == "git" {
				return []byte(revision), nil
			}
			return dockerImageInspectPayload(
				"sha256:runtime", "linux", runtime.GOARCH,
				map[string]string{"org.opencontainers.image.revision": revision}), nil
		},
		lockRoot: t.TempDir(),
		sleep:    time.Sleep,
	}
	_, err := b.ensureApplier(root, "agent-core:mutable", "applier:test")
	if err == nil || !strings.Contains(err.Error(), "does not carry the verified") {
		t.Fatalf("error=%v, want unverified runtime rejection", err)
	}
}

func TestApplierRecipeChangesWithDockerfile(t *testing.T) {
	root := applierImageTestRoot(t, "FROM ${RUNTIME_IMAGE}\n")
	revision := strings.Repeat("d", 40)
	b := imageBuilder{
		output: func(_ string, _ []string, name string, _ ...string) ([]byte, error) {
			if name == "git" {
				return []byte(revision), nil
			}
			return agentCoreRuntimeInspectPayload(revision, "sha256:runtime"), nil
		},
	}
	first, err := b.applierIdentity(root, "agent-core:test")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(root, "applier.Dockerfile"),
		[]byte("FROM ${RUNTIME_IMAGE}\nRUN apk add helm\n"), 0o644,
	); err != nil {
		t.Fatal(err)
	}
	second, err := b.applierIdentity(root, "agent-core:test")
	if err != nil {
		t.Fatal(err)
	}
	if first.recipe == second.recipe {
		t.Fatalf("recipe stayed %s after Dockerfile changed", first.recipe)
	}
}

func TestEnsureApplierImageRejectsUnlabeledRetagAfterBuild(t *testing.T) {
	root := applierImageTestRoot(t, "FROM ${RUNTIME_IMAGE}\n")
	revision := strings.Repeat("f", 40)
	runtimeImage := "agent-core:test"
	applierImage := "applier:test"
	built := false
	b := imageBuilder{
		run: func(_ string, _ []string, name string, _ ...string) error {
			if name == "docker" {
				built = true
			}
			return nil
		},
		output: func(_ string, _ []string, name string, args ...string) ([]byte, error) {
			if name == "git" {
				return []byte(revision), nil
			}
			switch args[len(args)-1] {
			case runtimeImage:
				return agentCoreRuntimeInspectPayload(revision, "sha256:runtime"), nil
			case applierImage:
				if !built {
					return nil, errors.New("not found")
				}
				return dockerImageInspectPayload(
					"sha256:unlabeled", "linux", runtime.GOARCH,
					map[string]string{
						"org.opencontainers.image.revision":         revision,
						"io.declarative-agents.agent-core.recipe":   "sha256:runtime-recipe",
						"io.declarative-agents.agent-core.platform": "linux/" + runtime.GOARCH,
					},
				), nil
			default:
				return nil, errors.New("not found")
			}
		},
		lockRoot: t.TempDir(),
		sleep:    time.Sleep,
	}
	_, err := b.ensureApplier(root, runtimeImage, applierImage)
	if err == nil || !strings.Contains(err.Error(),
		"does not carry the required revision, recipe, platform, and runtime identity") {
		t.Fatalf("error=%v, want unlabeled retag rejection", err)
	}
}

func TestEnsureApplierImageReusesAfterWaitOnSharedTag(t *testing.T) {
	root := applierImageTestRoot(t, "FROM ${RUNTIME_IMAGE}\n")
	revision := strings.Repeat("a", 40)
	runtimeImage := "agent-core:test"
	applierImage := "applier:shared"
	var identity applierImageIdentity
	var mu sync.Mutex
	built, builds := false, 0
	buildStarted := make(chan struct{})
	releaseBuild := make(chan struct{})
	b := imageBuilder{
		run: func(_ string, _ []string, name string, _ ...string) error {
			if name != "docker" {
				return nil
			}
			mu.Lock()
			builds++
			if builds == 1 {
				close(buildStarted)
			}
			mu.Unlock()
			<-releaseBuild
			mu.Lock()
			built = true
			mu.Unlock()
			return nil
		},
		output: func(_ string, _ []string, name string, args ...string) ([]byte, error) {
			if name == "git" {
				return []byte(revision), nil
			}
			switch args[len(args)-1] {
			case runtimeImage:
				return agentCoreRuntimeInspectPayload(revision, "sha256:runtime"), nil
			case applierImage:
				mu.Lock()
				ready := built
				mu.Unlock()
				if !ready {
					return nil, errors.New("not found")
				}
				return applierInspectPayload(identity, "sha256:shared"), nil
			default:
				return nil, errors.New("not found")
			}
		},
		lockRoot: t.TempDir(),
		sleep:    func(time.Duration) { time.Sleep(time.Millisecond) },
	}
	var err error
	identity, err = b.applierIdentity(root, runtimeImage)
	if err != nil {
		t.Fatal(err)
	}
	const callers = 5
	results := make(chan ApplierImageResult, callers)
	errs := make(chan error, callers)
	for range callers {
		go func() {
			result, ensureErr := b.ensureApplier(root, runtimeImage, applierImage)
			results <- result
			errs <- ensureErr
		}()
	}
	<-buildStarted
	close(releaseBuild)
	for range callers {
		if ensureErr := <-errs; ensureErr != nil {
			t.Fatal(ensureErr)
		}
		if result := <-results; result.ImageID != "sha256:shared" {
			t.Fatalf("result=%+v, want shared image", result)
		}
	}
	if builds != 1 {
		t.Fatalf("docker builds=%d, want 1", builds)
	}
}

func TestEnsureApplierImageSerializesDistinctRecipesOnSharedTag(t *testing.T) {
	root := applierImageTestRoot(t, "FROM ${RUNTIME_IMAGE}\n")
	revision := strings.Repeat("e", 40)
	runtimeCommit := "declarative-agents/agent-core:commit"
	runtimeLocal := "declarative-agents/agent-core:local"
	applierImage := "declarative-agents/applier:shared"
	identityFor := func(runtimeImage string) applierImageIdentity {
		t.Helper()
		probe := imageBuilder{
			output: func(_ string, _ []string, name string, _ ...string) ([]byte, error) {
				if name == "git" {
					return []byte(revision), nil
				}
				return agentCoreRuntimeInspectPayload(revision, "sha256:runtime"), nil
			},
		}
		identity, err := probe.applierIdentity(root, runtimeImage)
		if err != nil {
			t.Fatal(err)
		}
		return identity
	}
	commitIdentity := identityFor(runtimeCommit)
	localIdentity := identityFor(runtimeLocal)
	if commitIdentity.recipe == localIdentity.recipe {
		t.Fatal("FROM tag must change the recipe so a recipe-keyed lock would not serialize")
	}
	identities := map[string]applierImageIdentity{
		runtimeCommit: commitIdentity,
		runtimeLocal:  localIdentity,
	}
	imageIDs := map[string]string{
		runtimeCommit: "sha256:applier-commit",
		runtimeLocal:  "sha256:applier-local",
	}

	var mu sync.Mutex
	inflight, overlap, builds := 0, 0, 0
	var lastRuntime string
	firstBuildStarted := make(chan struct{})
	releaseFirstBuild := make(chan struct{})
	b := imageBuilder{
		run: func(_ string, _ []string, name string, args ...string) error {
			if name != "docker" {
				return nil
			}
			runtimeImage := runtimeImageFromBuildArgs(args)
			mu.Lock()
			inflight++
			builds++
			if inflight > 1 {
				overlap++
			}
			n := builds
			lastRuntime = runtimeImage
			mu.Unlock()
			if n == 1 {
				close(firstBuildStarted)
				<-releaseFirstBuild
			}
			mu.Lock()
			inflight--
			mu.Unlock()
			return nil
		},
		output: func(_ string, _ []string, name string, args ...string) ([]byte, error) {
			if name == "git" {
				return []byte(revision), nil
			}
			image := args[len(args)-1]
			switch image {
			case runtimeCommit, runtimeLocal:
				return agentCoreRuntimeInspectPayload(revision, "sha256:runtime"), nil
			case applierImage:
				mu.Lock()
				runtimeImage := lastRuntime
				mu.Unlock()
				if runtimeImage == "" {
					return nil, errors.New("not found")
				}
				return applierInspectPayload(identities[runtimeImage], imageIDs[runtimeImage]), nil
			default:
				return nil, errors.New("not found")
			}
		},
		lockRoot: t.TempDir(),
		sleep:    func(time.Duration) { time.Sleep(time.Millisecond) },
	}

	errs := make(chan error, 2)
	go func() {
		_, err := b.ensureApplier(root, runtimeCommit, applierImage)
		errs <- err
	}()
	go func() {
		_, err := b.ensureApplier(root, runtimeLocal, applierImage)
		errs <- err
	}()
	<-firstBuildStarted
	time.Sleep(30 * time.Millisecond)
	mu.Lock()
	gotInflight, gotOverlap := inflight, overlap
	mu.Unlock()
	close(releaseFirstBuild)
	for range 2 {
		if err := <-errs; err != nil {
			t.Fatal(err)
		}
	}
	if gotInflight != 1 || gotOverlap != 0 {
		t.Fatalf("during first build inflight=%d overlap=%d, want serialized tag lock",
			gotInflight, gotOverlap)
	}
	if builds != 2 {
		t.Fatalf("docker builds=%d, want 2 distinct recipes", builds)
	}
}

func runtimeImageFromBuildArgs(args []string) string {
	for i, arg := range args {
		if arg == "--build-arg" && i+1 < len(args) && strings.HasPrefix(args[i+1], "RUNTIME_IMAGE=") {
			return strings.TrimPrefix(args[i+1], "RUNTIME_IMAGE=")
		}
	}
	return ""
}

func applierImageTestRoot(t *testing.T, dockerfile string) string {
	t.Helper()
	root := t.TempDir()
	if err := os.WriteFile(
		filepath.Join(root, "applier.Dockerfile"), []byte(dockerfile), 0o644,
	); err != nil {
		t.Fatal(err)
	}
	return root
}

func agentCoreRuntimeInspectPayload(revision, id string) []byte {
	return dockerImageInspectPayload(id, "linux", runtime.GOARCH, map[string]string{
		"org.opencontainers.image.revision":         revision,
		"io.declarative-agents.agent-core.recipe":   "sha256:runtime-recipe",
		"io.declarative-agents.agent-core.platform": "linux/" + runtime.GOARCH,
	})
}

func applierInspectPayload(identity applierImageIdentity, id string) []byte {
	return dockerImageInspectPayload(id, "linux", runtime.GOARCH, map[string]string{
		"org.opencontainers.image.revision": identity.revision,
		applierRecipeLabel:                  identity.recipe,
		applierPlatformLabel:                identity.platform,
		applierRuntimeLabel:                 identity.runtimeID,
	})
}

func dockerImageInspectPayload(
	id, osName, architecture string,
	labels map[string]string,
) []byte {
	payload := []map[string]any{{
		"Id": id, "Os": osName, "Architecture": architecture,
		"Config": map[string]any{"Labels": labels},
	}}
	data, _ := json.Marshal(payload)
	return data
}
