// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package kindrig

import (
	"fmt"
	"strings"
	"time"
)

// PinnedImageRefs names the three references one digest-pinned import uses.
// Source is the immutable cache on the host; HostImport is a tag docker save
// can name; Node is how containerd and Kubernetes resolve that tag after
// import. A host import tag this run creates is removed after a successful
// import (ENG01 table 7).
type PinnedImageRefs struct {
	Source     string
	HostImport string
	Node       string
}

// PinnedImageReferences validates source and host-import names and computes
// the node name containerd will record.
func PinnedImageReferences(source, hostImport string) (PinnedImageRefs, error) {
	source = strings.TrimSpace(source)
	hostImport = strings.TrimSpace(hostImport)
	if source == "" {
		return PinnedImageRefs{}, fmt.Errorf("pinned image source is required")
	}
	if hostImport == "" {
		return PinnedImageRefs{}, fmt.Errorf("pinned image host import tag is required")
	}
	if strings.Contains(hostImport, "@") {
		return PinnedImageRefs{}, fmt.Errorf(
			"host import tag %q carries a digest; docker save needs a tag", hostImport)
	}
	return PinnedImageRefs{
		Source:     source,
		HostImport: hostImport,
		Node:       NormalizeNodeImageReference(hostImport),
	}, nil
}

// NormalizeNodeImageReference is the name containerd and kubelet resolve for
// a docker-saved tag: Docker Hub short names become docker.io/library/...,
// user/name becomes docker.io/user/name, and an explicit registry is kept.
func NormalizeNodeImageReference(ref string) string {
	name, tag, digest := splitReference(strings.TrimSpace(ref))
	if name == "" {
		return strings.TrimSpace(ref)
	}
	normalized := normalizeNodeRepository(name)
	if tag != "" {
		normalized += ":" + tag
	}
	if digest != "" {
		normalized += "@" + digest
	}
	return normalized
}

func normalizeNodeRepository(name string) string {
	name = strings.TrimPrefix(strings.ToLower(name), dockerHubIndex+"/")
	slash := strings.IndexByte(name, '/')
	if slash < 0 {
		return DockerHubRegistry + "/" + dockerHubLibrary + "/" + name
	}
	first := name[:slash]
	if first == DockerHubRegistry || first == "localhost" || strings.ContainsAny(first, ".:") {
		if first == DockerHubRegistry && !strings.Contains(name[slash+1:], "/") {
			return DockerHubRegistry + "/" + dockerHubLibrary + "/" + name[slash+1:]
		}
		return name
	}
	return DockerHubRegistry + "/" + name
}

// installStep is one command of a pinned-image install, named for its phase
// line so a slow boot names the step that stalled (GH-2226).
type installStep struct {
	phase   string
	command []string
}

func dockerImagePresent(run CommandRunner, image string) bool {
	_, err := run("docker", "image", "inspect", "--format", "{{.Id}}", image)
	return err == nil
}

func runPhase(run CommandRunner, cluster, component, phase string, command []string) error {
	return runInstallSteps(run, cluster, component, []installStep{{phase: phase, command: command}})
}

// importPinnedImage pulls a digest-pinned source if needed, tags a host-only
// import name when that tag is absent, imports the host platform into the
// node, and removes a host tag this run created. A pre-existing host tag and
// the source digest stay. Import failure rolls back a tag this run created.
func importPinnedImage(
	run CommandRunner, cluster, component, sourceImage, hostImport string,
) error {
	refs, err := PinnedImageReferences(sourceImage, hostImport)
	if err != nil {
		return err
	}
	if !dockerImagePresent(run, refs.Source) {
		if err := runPhase(run, cluster, component, "image-pull",
			[]string{"docker", "pull", "--platform", HostPlatform(), refs.Source}); err != nil {
			return err
		}
	} else {
		LogPhase(cluster, "image-pull", "skipped", time.Now(), "image="+refs.Source)
	}

	createdHostImport := !dockerImagePresent(run, refs.HostImport)
	imported := false
	if createdHostImport {
		if err := runPhase(run, cluster, component, "image-tag",
			[]string{"docker", "tag", refs.Source, refs.HostImport}); err != nil {
			return err
		}
		defer func() {
			if !imported && refs.HostImport != refs.Source {
				_, _ = run("docker", "image", "rm", refs.HostImport)
			}
		}()
	}
	if err := runPhase(run, cluster, component, "image-load",
		nodeImportCommand(refs.HostImport, cluster)); err != nil {
		return err
	}
	imported = true
	if createdHostImport && refs.HostImport != refs.Source {
		if err := runPhase(run, cluster, component, "image-untag",
			[]string{"docker", "image", "rm", refs.HostImport}); err != nil {
			return err
		}
	}
	return nil
}

// pinnedImageSteps is the pull/tag/import/untag sequence importPinnedImage
// runs. Tests and installers that compose extra kubectl steps call
// importPinnedImage first, then append cluster commands.
func pinnedImageSteps(run CommandRunner, cluster, sourceImage, hostImport string) []installStep {
	refs, err := PinnedImageReferences(sourceImage, hostImport)
	if err != nil {
		return nil
	}
	var steps []installStep
	if !dockerImagePresent(run, refs.Source) {
		steps = append(steps, installStep{"image-pull",
			[]string{"docker", "pull", "--platform", HostPlatform(), refs.Source}})
	} else {
		LogPhase(cluster, "image-pull", "skipped", time.Now(), "image="+refs.Source)
	}
	if !dockerImagePresent(run, refs.HostImport) {
		steps = append(steps,
			installStep{"image-tag", []string{"docker", "tag", refs.Source, refs.HostImport}},
			installStep{"image-load", nodeImportCommand(refs.HostImport, cluster)},
			installStep{"image-untag", []string{"docker", "image", "rm", refs.HostImport}},
		)
		return steps
	}
	return append(steps, installStep{"image-load", nodeImportCommand(refs.HostImport, cluster)})
}

// nodeImportCommand streams a host image into the kind node's containerd for
// the host platform only. kind load imports with --all-platforms, which fails
// when the host holds a multi-platform index with only its own platform's
// layers, as Docker Desktop's containerd store does for a digest-pinned pull
// (GH-2222).
func nodeImportCommand(image, cluster string) []string {
	return []string{"sh", "-c",
		`docker save "$1" | docker exec -i "$2" ctr --namespace=k8s.io images import --platform="$3" --snapshotter=overlayfs -`,
		"node-import", image, cluster + "-control-plane", HostPlatform()}
}

// KubeContext is the kubeconfig context kind writes for a cluster.
func KubeContext(cluster string) string { return "kind-" + cluster }

// inCluster binds a kubectl command to one cluster's context. The installers
// name the cluster they load images into, but kubectl otherwise follows
// whatever context happens to be current, so a stale context sends the
// manifest to one cluster while the image lands in another: the pod fails
// ErrImageNeverPull and the rollout times out naming neither (GH-2428).
// Commands that are not kubectl are returned unchanged.
func inCluster(cluster string, command []string) []string {
	if len(command) == 0 || command[0] != "kubectl" || strings.TrimSpace(cluster) == "" {
		return command
	}
	bound := make([]string, 0, len(command)+2)
	bound = append(bound, command[0], "--context", KubeContext(cluster))
	return append(bound, command[1:]...)
}

// runInstallSteps runs steps in order, logging one phase line per step, and
// stops at the first failure with the command and its output. Every kubectl
// step is bound to the named cluster's context.
func runInstallSteps(run CommandRunner, cluster, component string, steps []installStep) error {
	for _, step := range steps {
		started := time.Now()
		bound := inCluster(cluster, step.command)
		name, args := bound[0], bound[1:]
		output, err := run(name, args...)
		detail := "component=" + component
		if err != nil {
			LogPhase(cluster, step.phase, "failed", started, detail)
			return fmt.Errorf("%s %s: %w: %s", name, strings.Join(args, " "),
				err, strings.TrimSpace(string(output)))
		}
		LogPhase(cluster, step.phase, "passed", started, detail)
	}
	return nil
}
