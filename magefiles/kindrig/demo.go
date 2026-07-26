// Copyright (c) 2026 Nokia. All rights reserved.

package kindrig

import (
	"fmt"
	"strings"
	"time"
)

const IngressNGINXManifest = "https://raw.githubusercontent.com/kubernetes/ingress-nginx/controller-v1.13.3/deploy/static/provider/kind/deploy.yaml"

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

// InstallIngress installs the pinned kind ingress controller and observes its
// readiness. The caller supplies a runner bound to the demo cluster.
func InstallIngress(run CommandRunner) error {
	commands := [][]string{
		{"apply", "-f", IngressNGINXManifest},
		{"wait", "--namespace", "ingress-nginx",
			"--for=condition=Ready", "pod",
			"--selector=app.kubernetes.io/component=controller",
			"--timeout=180s"},
	}
	for _, args := range commands {
		output, err := run("kubectl", args...)
		if err != nil {
			return fmt.Errorf("kubectl %s: %w: %s",
				strings.Join(args, " "), err, strings.TrimSpace(string(output)))
		}
	}
	return nil
}
