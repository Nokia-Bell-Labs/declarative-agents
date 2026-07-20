// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

// Config is the provisioner's runtime configuration. The non-secret settings come
// from the chart as env; the read and apply tokens are read from a Secret mounted
// as files, named by --read-token-file and --apply-token-file, so the secret
// values never pass through the process environment.
type Config struct {
	Addr        string
	StateFile   string // JSON mesh view, projected from the chart-rendered values ConfigMap
	ReadToken   string
	ApplyToken  string
	Release     string
	ChartDir    string // the chart mounted into the pod, for helm upgrade
	Namespace   string
	Deployment  string // the chatbot Deployment to report rollout status for
	KubeContext string
}

func loadConfig(readTokenPath, applyTokenPath string) (Config, error) {
	readToken, err := tokenFromFile(readTokenPath)
	if err != nil {
		return Config{}, err
	}
	applyToken, err := tokenFromFile(applyTokenPath)
	if err != nil {
		return Config{}, err
	}
	return Config{
		Addr:       envOr("PROVISION_ADDR", ":18090"),
		StateFile:  envOr("PROVISION_STATE_FILE", "/etc/provisioner/mesh.json"),
		ReadToken:  readToken,
		ApplyToken: applyToken,
		Release:    envOr("PROVISION_RELEASE", "chatbot-mesh"),
		ChartDir:   envOr("PROVISION_CHART_DIR", "/chart"),
		Namespace:  envOr("PROVISION_NAMESPACE", "default"),
		Deployment: envOr("PROVISION_DEPLOYMENT", "chatbot-mesh-chatbot"),
	}, nil
}

// tokenFromFile reads a bearer token from a mounted Secret file, trimming
// surrounding whitespace. An empty path yields an empty token, so the read token
// (a read-only credential) is optional while the apply token is checked in main.
func tokenFromFile(path string) (string, error) {
	if path == "" {
		return "", nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read token file %s: %w", path, err)
	}
	return strings.TrimSpace(string(data)), nil
}

func main() {
	readTokenPath := flag.String("read-token-file", "", "path to a file holding the read API token (optional)")
	applyTokenPath := flag.String("apply-token-file", "", "path to a file holding the apply API token (required)")
	flag.Parse()

	cfg, err := loadConfig(*readTokenPath, *applyTokenPath)
	if err != nil {
		log.Fatalf("provisioner: %v", err)
	}
	if cfg.ApplyToken == "" {
		log.Fatal("provisioner: an apply token is required (--apply-token-file)")
	}
	srv := &Server{
		State:      fileState(cfg.StateFile),
		Apply:      helmApply(cfg),
		Rollout:    kubectlRollout(cfg),
		ReadToken:  cfg.ReadToken,
		ApplyToken: cfg.ApplyToken,
	}
	log.Printf("provisioner listening on %s (release %s, deployment %s)", cfg.Addr, cfg.Release, cfg.Deployment)
	if err := http.ListenAndServe(cfg.Addr, srv.Routes()); err != nil {
		log.Fatalf("provisioner: %v", err)
	}
}

// fileState reads the mesh view the chart projects from its values into a mounted
// ConfigMap, so the read path needs only read-only RBAC on that ConfigMap.
func fileState(path string) func() (MeshView, error) {
	return func() (MeshView, error) {
		data, err := os.ReadFile(path)
		if err != nil {
			return MeshView{}, fmt.Errorf("read state file %s: %w", path, err)
		}
		var view MeshView
		if err := json.Unmarshal(data, &view); err != nil {
			return MeshView{}, fmt.Errorf("parse state file: %w", err)
		}
		return view, nil
	}
}

// helmApply renders the validated mesh view as helm --set args and upgrades the
// release in place, which re-renders the co-generated topology and rolls the
// chatbot (srd003 R2/R3). It never contacts a running agent.
func helmApply(cfg Config) func(MeshView) error {
	return func(view MeshView) error {
		args := []string{"upgrade", cfg.Release, cfg.ChartDir,
			"--namespace", cfg.Namespace, "--reuse-values", "--wait"}
		for _, set := range view.HelmSetArgs() {
			args = append(args, "--set", set)
		}
		cmd := exec.Command("helm", args...)
		cmd.Stdout, cmd.Stderr = os.Stderr, os.Stderr
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("helm upgrade %s: %w", cfg.Release, err)
		}
		return nil
	}
}

// kubectlRollout reports the chatbot Deployment's rollout progress from its status
// subresource, so the panel can poll after an apply.
func kubectlRollout(cfg Config) func() (RolloutStatus, error) {
	return func() (RolloutStatus, error) {
		out, err := exec.Command("kubectl", "get", "deployment", cfg.Deployment,
			"--namespace", cfg.Namespace,
			"-o", "jsonpath={.status.readyReplicas}/{.spec.replicas}/{.metadata.generation}").Output()
		if err != nil {
			return RolloutStatus{Phase: "unknown"}, fmt.Errorf("kubectl get deployment: %w", err)
		}
		ready, desired, gen := parseRolloutFields(string(out))
		phase := "progressing"
		if desired > 0 && ready >= desired {
			phase = "complete"
		}
		return RolloutStatus{Phase: phase, Ready: ready, Desired: desired, Revision: gen}, nil
	}
}

func parseRolloutFields(s string) (ready, desired, gen int) {
	parts := strings.Split(strings.TrimSpace(s), "/")
	get := func(i int) int {
		if i >= len(parts) {
			return 0
		}
		n, _ := strconv.Atoi(strings.TrimSpace(parts[i]))
		return n
	}
	return get(0), get(1), get(2)
}

func envOr(name, fallback string) string {
	if v := os.Getenv(name); v != "" {
		return v
	}
	return fallback
}
