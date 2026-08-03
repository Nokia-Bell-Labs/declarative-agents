// Copyright (c) 2026 Nokia. All rights reserved.

package kindrig

import (
	_ "embed"
	"fmt"
	"os"
	"runtime"
	"strings"
	"time"
)

const (
	traefikImageRepository   = "docker.io/library/traefik"
	traefikRuntimeRepository = "kindrig/traefik"
	traefikImageVersion      = "v3.7.10"
	traefikImagePlaceholder  = "KINDRIG_TRAEFIK_IMAGE"
)

var traefikImageDigests = map[string]string{
	"amd64": "sha256:0ae6898c4a6ad568e8eabad0839b1660eb54a72aa43089ead05c6f624f866b23",
	"arm64": "sha256:8f34ac785161ddfe26f4e291982967e92962c126120526a519c4cff521179436",
}

//go:embed traefik-kind.yaml
var traefikKindManifest string

// DemoUp creates or reuses a named demo cluster and invokes deploy. A failed
// deployment deletes only a cluster created by this invocation; a reused
// developer cluster is always left in place.
func DemoUp(
	run Runner,
	name, configPath string,
	wait time.Duration,
	deploy func(Cluster) error,
) error {
	cluster, err := EnsureCluster(run, name, configPath, wait)
	if err != nil {
		return err
	}
	if err := deploy(cluster); err != nil {
		cluster.Release(run)
		return fmt.Errorf("deploy demo to %s: %w", name, err)
	}
	if cluster.Created {
		fmt.Printf("demo: cluster %s created and retained; run mage demo:down to delete it\n", name)
	} else {
		fmt.Printf("demo: reused cluster %s and left it in place\n", name)
	}
	return nil
}

// DemoDown deletes the one named demo cluster and no other cluster.
func DemoDown(run Runner, name string) error {
	if !Exists(run, name) {
		fmt.Printf("demo: cluster %s does not exist\n", name)
		return nil
	}
	if _, err := run("delete", "cluster", "--name", name); err != nil {
		return fmt.Errorf("delete demo cluster %s: %w", name, err)
	}
	fmt.Printf("demo: deleted cluster %s\n", name)
	return nil
}

// InstallIngress loads and installs the pinned minimal Traefik controller, then
// observes its readiness. The caller supplies a runner bound to cluster.
//
// The image is host-pulled and kind-loaded before the manifest is applied. A
// kind node behind a TLS-intercepting proxy may not trust the host's CA, while
// the host container engine does; loading also makes repeated demo starts
// independent of registry availability. Traefik watches standard Ingress
// resources without a validating webhook, so applying an application chart can
// no longer race an admission server that is not listening yet (GH-1321).
func InstallIngress(run CommandRunner, cluster string) error {
	if strings.TrimSpace(cluster) == "" {
		return fmt.Errorf("install ingress: kind cluster name is required")
	}
	sourceImage, err := traefikImage(runtime.GOARCH)
	if err != nil {
		return err
	}
	runtimeImage := traefikRuntimeRepository + ":" + traefikImageVersion
	manifest := strings.ReplaceAll(traefikKindManifest, traefikImagePlaceholder, runtimeImage)
	path, cleanup, err := writeIngressManifest(manifest)
	if err != nil {
		return err
	}
	defer cleanup()

	commands := [][]string{
		{"docker", "pull", "--platform", "linux/" + runtime.GOARCH, sourceImage},
		{"docker", "tag", sourceImage, runtimeImage},
		{"kind", "load", "docker-image", runtimeImage, "--name", cluster},
		{"kubectl", "apply", "-f", path},
		{"kubectl", "rollout", "status", "deployment/traefik",
			"--namespace", "traefik", "--timeout=180s"},
	}
	for _, command := range commands {
		name, args := command[0], command[1:]
		output, err := run(name, args...)
		if err != nil {
			return fmt.Errorf("%s %s: %w: %s", name, strings.Join(args, " "),
				err, strings.TrimSpace(string(output)))
		}
	}
	return nil
}

func traefikImage(arch string) (string, error) {
	digest, ok := traefikImageDigests[arch]
	if !ok {
		return "", fmt.Errorf("install ingress: Traefik %s has no pinned digest for linux/%s",
			traefikImageVersion, arch)
	}
	return traefikImageRepository + ":" + traefikImageVersion + "@" + digest, nil
}

func writeIngressManifest(manifest string) (string, func(), error) {
	file, err := os.CreateTemp("", "kindrig-traefik-*.yaml")
	if err != nil {
		return "", nil, fmt.Errorf("create Traefik manifest: %w", err)
	}
	path := file.Name()
	cleanup := func() { _ = os.Remove(path) }
	if _, err := file.WriteString(manifest); err != nil {
		_ = file.Close()
		cleanup()
		return "", nil, fmt.Errorf("write Traefik manifest: %w", err)
	}
	if err := file.Close(); err != nil {
		cleanup()
		return "", nil, fmt.Errorf("close Traefik manifest: %w", err)
	}
	return path, cleanup, nil
}
