// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

type codingModelMockResource struct {
	Kind     string `yaml:"kind"`
	Metadata struct {
		Name string `yaml:"name"`
	} `yaml:"metadata"`
	Data map[string]string `yaml:"data"`
	Spec struct {
		Ports []struct {
			Port int `yaml:"port"`
		} `yaml:"ports"`
		Template struct {
			Spec struct {
				Containers []struct {
					Name  string   `yaml:"name"`
					Image string   `yaml:"image"`
					Args  []string `yaml:"args"`
					Env   []struct {
						Name  string `yaml:"name"`
						Value string `yaml:"value"`
					} `yaml:"env"`
					ReadinessProbe struct {
						HTTPGet struct {
							Path string `yaml:"path"`
						} `yaml:"httpGet"`
					} `yaml:"readinessProbe"`
				} `yaml:"containers"`
			} `yaml:"spec"`
		} `yaml:"template"`
	} `yaml:"spec"`
}

func TestCodingModelMockManifestUsesCanonicalImageAndExactClosure(t *testing.T) {
	roots, _ := canonicalDeploymentInputs(t)
	image := "ghcr.io/nokia-bell-labs/declarative-agents/agent-core:0123456789ab"
	manifest, cleanup, err := codingModelMockManifest(roots, image)
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	file, err := os.Open(manifest)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = file.Close() }()

	resources := map[string]codingModelMockResource{}
	decoder := yaml.NewDecoder(file)
	for {
		var resource codingModelMockResource
		if err := decoder.Decode(&resource); err != nil {
			break
		}
		if resource.Kind != "" {
			resources[resource.Kind+"/"+resource.Metadata.Name] = resource
		}
	}
	if len(resources) != 4 {
		t.Fatalf("mock manifest resources = %v, want two ConfigMaps, Deployment, Service", resources)
	}

	profiles := resources["ConfigMap/coding-model-profiles"]
	wantProfileKeys := []string{
		"agents__mock__declarations.yaml",
		"agents__mock__machine.yaml",
		"agents__mock__profile.yaml",
		"agents__mock__rest.yaml",
		"agents__mock__tools.yaml",
		"agents__units__rest-service-machine-template.yaml",
	}
	if len(profiles.Data) != len(wantProfileKeys) {
		t.Fatalf("profile ConfigMap keys = %v, want exact closure %v", profiles.Data, wantProfileKeys)
	}
	for _, key := range wantProfileKeys {
		if profiles.Data[key] == "" {
			t.Errorf("profile ConfigMap missing non-empty %s", key)
		}
	}
	fixture := resources["ConfigMap/coding-model-fixture"]
	if data := fixture.Data["coding-model-mock.yaml"]; !strings.Contains(data, "path: /api/tags") ||
		strings.Count(data, "path: /api/chat") != 1 {
		t.Fatalf("fixture ConfigMap does not carry the ordered Ollama fixture:\n%s", data)
	}

	deployment := resources["Deployment/coding-model"]
	if len(deployment.Spec.Template.Spec.Containers) != 1 {
		t.Fatalf("mock containers = %#v, want one", deployment.Spec.Template.Spec.Containers)
	}
	container := deployment.Spec.Template.Spec.Containers[0]
	if container.Image != image {
		t.Fatalf("mock image = %q, want canonical agent image %q", container.Image, image)
	}
	if !containsString(container.Args, "--profile") ||
		!containsString(container.Args, "/profiles/"+codingModelMockProfile) {
		t.Fatalf("mock args = %v, want canonical mounted profile", container.Args)
	}
	env := map[string]string{}
	for _, variable := range container.Env {
		env[variable.Name] = variable.Value
	}
	for name, want := range map[string]string{
		"MOCK_ADDRESS":               "0.0.0.0:11434",
		"MOCK_ALLOW_PUBLIC_LISTENER": "true",
		"MOCK_FIXTURES":              "/fixtures/coding-model-mock.yaml",
	} {
		if env[name] != want {
			t.Errorf("%s = %q, want %q", name, env[name], want)
		}
	}
	if container.ReadinessProbe.HTTPGet.Path != "/_mock/health" {
		t.Errorf("readiness path = %q, want native mock health", container.ReadinessProbe.HTTPGet.Path)
	}
	service := resources["Service/coding-model"]
	if len(service.Spec.Ports) != 1 || service.Spec.Ports[0].Port != 11434 {
		t.Fatalf("mock Service ports = %#v, want 11434", service.Spec.Ports)
	}
}

func TestCodingModelMockDoesNotEnterProductionPackage(t *testing.T) {
	profiles, _, cleanup := packageCanonicalDeployment(t)
	defer cleanup()
	err := filepath.WalkDir(profiles, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if strings.Contains(filepath.ToSlash(path), "agents/mock") ||
			strings.Contains(entry.Name(), "coding-model-mock") {
			t.Errorf("production package contains smoke-only mock asset %s", path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestCodingModelMockFixtureValidatesThroughCanonicalBindingAndDialect(t *testing.T) {
	roots, _ := canonicalDeploymentInputs(t)
	fixture, err := filepath.Abs(filepath.Join("..", filepath.FromSlash(codingModelMockFixture)))
	if err != nil {
		t.Fatal(err)
	}
	profile, err := filepath.Abs(filepath.Join(roots.Profiles, filepath.FromSlash(codingModelMockProfile)))
	if err != nil {
		t.Fatal(err)
	}
	core, err := filepath.Abs(roots.Core)
	if err != nil {
		t.Fatal(err)
	}
	command := exec.Command(
		"go", "run", "./cmd/agent",
		"--profile", profile,
		"--core-root", core,
		"--validate-config",
	)
	command.Dir = core
	command.Env = append(os.Environ(),
		"MOCK_ADDRESS=127.0.0.1:0",
		"MOCK_FIXTURES="+fixture,
	)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("canonical mock rejected coding fixture: %v\n%s", err, output)
	}

	dialect, err := os.ReadFile(filepath.Join(core, "tools", "providers", "ollama", "chat-dialect.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	for _, selector := range []string{"$.message.content", "$.eval_count", "$.prompt_eval_count"} {
		if !strings.Contains(string(dialect), selector) {
			t.Errorf("Ollama dialect missing response selector %q", selector)
		}
	}
}

func TestVerifyCodingModelMockLog(t *testing.T) {
	valid := codingModelMockLog{
		Count: 4,
		Requests: []codingModelMockRequest{
			{Method: "GET", Path: "/api/tags", Matched: true},
			mockChatLogRequest(t, "You are a software planning assistant.", false),
			mockChatLogRequest(t, "Execute the supplied plan.", false),
			mockChatLogRequest(t, `[tool_call]{"tool":"edit","parameters":{"path":"greet.go"}}[/tool_call]`, false),
		},
	}
	if err := verifyCodingModelMockLog(valid); err != nil {
		t.Fatalf("valid protocol rejected: %v", err)
	}

	tests := []struct {
		name   string
		mutate func(*codingModelMockLog)
		want   string
	}{
		{
			name: "chat retry",
			mutate: func(log *codingModelMockLog) {
				log.Requests = append(log.Requests, log.Requests[len(log.Requests)-1])
				log.Count++
			},
			want: "exactly 3",
		},
		{
			name: "unmatched",
			mutate: func(log *codingModelMockLog) {
				log.Requests[1].Matched = false
			},
			want: "unmatched",
		},
		{
			name: "streaming",
			mutate: func(log *codingModelMockLog) {
				log.Requests[1] = mockChatLogRequest(t, "You are a software planning assistant.", true)
			},
			want: "stream=true",
		},
		{
			name: "wrong progression",
			mutate: func(log *codingModelMockLog) {
				log.Requests[3] = mockChatLogRequest(t, "No prior edit here.", false)
			},
			want: "prior edit",
		},
		{
			name: "unexpected route",
			mutate: func(log *codingModelMockLog) {
				log.Requests[0].Path = "/healthz"
			},
			want: "unexpected",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			log := codingModelMockLog{
				Count:    valid.Count,
				Requests: append([]codingModelMockRequest(nil), valid.Requests...),
			}
			tt.mutate(&log)
			if err := verifyCodingModelMockLog(log); err == nil ||
				!strings.Contains(err.Error(), tt.want) {
				t.Fatalf("validation error = %v, want %q", err, tt.want)
			}
		})
	}
}

func mockChatLogRequest(t *testing.T, content string, stream bool) codingModelMockRequest {
	t.Helper()
	body, err := json.Marshal(map[string]interface{}{
		"model":  codingModelMockModel,
		"stream": stream,
		"messages": []map[string]string{{
			"role": "user", "content": content,
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	return codingModelMockRequest{
		Method: "POST", Path: "/api/chat", Body: string(body), Matched: true,
	}
}
