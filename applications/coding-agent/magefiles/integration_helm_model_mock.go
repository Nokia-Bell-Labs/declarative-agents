// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

const (
	codingModelMockProfile = "agents/mock/profile.yaml"
	codingModelMockFixture = "testdata/integration/helm-smoke/coding-model-mock.yaml"
	codingModelMockModel   = "qwen3.6:35b-mlx"
)

// codingModelMockManifest stages the canonical catalog mock closure and emits
// rig-owned Kubernetes resources. They are deliberately outside the production
// Helm package: the chart still depends on an external Ollama-compatible URL.
func codingModelMockManifest(
	roots integrationRoots,
	agentImage string,
) (string, func(), error) {
	dir, err := os.MkdirTemp("", "coding-model-mock-*")
	if err != nil {
		return "", nil, err
	}
	cleanup := func() { _ = os.RemoveAll(dir) }

	profileRoot := filepath.Join(dir, "profiles")
	files, err := stageExternalProfileClosure(
		roots.Profiles,
		codingModelMockProfile,
		codingModelMockProfile,
		profileRoot,
	)
	if err != nil {
		cleanup()
		return "", nil, fmt.Errorf("stage canonical mock closure: %w", err)
	}
	profileData := make(map[string]string, len(files))
	profileItems := make([]map[string]string, 0, len(files))
	for _, filename := range files {
		data, readErr := os.ReadFile(filepath.Join(profileRoot, filepath.FromSlash(filename)))
		if readErr != nil {
			cleanup()
			return "", nil, fmt.Errorf("read staged mock profile %s: %w", filename, readErr)
		}
		key := strings.ReplaceAll(filename, "/", "__")
		profileData[key] = string(data)
		profileItems = append(profileItems, map[string]string{"key": key, "path": filename})
	}

	fixture, err := os.ReadFile(filepath.Join(
		roots.Application, filepath.FromSlash(codingModelMockFixture)))
	if err != nil {
		cleanup()
		return "", nil, fmt.Errorf("read coding model mock fixture: %w", err)
	}

	labels := map[string]string{
		"app.kubernetes.io/name":       "coding-model",
		"app.kubernetes.io/component":  "mock",
		"app.kubernetes.io/managed-by": "coding-agent-integration",
	}
	documents := []map[string]interface{}{
		{
			"apiVersion": "v1",
			"kind":       "ConfigMap",
			"metadata":   map[string]interface{}{"name": "coding-model-profiles", "labels": labels},
			"data":       profileData,
		},
		{
			"apiVersion": "v1",
			"kind":       "ConfigMap",
			"metadata":   map[string]interface{}{"name": "coding-model-fixture", "labels": labels},
			"data":       map[string]string{"coding-model-mock.yaml": string(fixture)},
		},
		{
			"apiVersion": "apps/v1",
			"kind":       "Deployment",
			"metadata":   map[string]interface{}{"name": "coding-model", "labels": labels},
			"spec": map[string]interface{}{
				"replicas": 1,
				"selector": map[string]interface{}{"matchLabels": map[string]string{"app": "coding-model"}},
				"template": map[string]interface{}{
					"metadata": map[string]interface{}{"labels": map[string]string{
						"app":                         "coding-model",
						"app.kubernetes.io/component": "mock",
					}},
					"spec": map[string]interface{}{
						"automountServiceAccountToken":  false,
						"terminationGracePeriodSeconds": 10,
						"securityContext": map[string]interface{}{
							"runAsNonRoot": true,
							"runAsUser":    10001,
							"runAsGroup":   10001,
							"fsGroup":      10001,
							"seccompProfile": map[string]string{
								"type": "RuntimeDefault",
							},
						},
						"containers": []map[string]interface{}{{
							"name":            "mock",
							"image":           agentImage,
							"imagePullPolicy": "Never",
							"args": []string{
								"--profile", "/profiles/" + codingModelMockProfile,
								"--directory", "/work",
								"--otel-service-name", "coding-model-mock",
							},
							"env": []map[string]string{
								{"name": "MOCK_ADDRESS", "value": "0.0.0.0:11434"},
								{"name": "MOCK_ALLOW_PUBLIC_LISTENER", "value": "true"},
								{"name": "MOCK_FIXTURES", "value": "/fixtures/coding-model-mock.yaml"},
							},
							"ports": []map[string]interface{}{{
								"name": "http", "containerPort": 11434, "protocol": "TCP",
							}},
							"readinessProbe": map[string]interface{}{
								"httpGet":             map[string]interface{}{"path": "/_mock/health", "port": "http"},
								"initialDelaySeconds": 1,
								"periodSeconds":       2,
							},
							"livenessProbe": map[string]interface{}{
								"httpGet":             map[string]interface{}{"path": "/_mock/health", "port": "http"},
								"initialDelaySeconds": 5,
								"periodSeconds":       10,
							},
							"securityContext": map[string]interface{}{
								"allowPrivilegeEscalation": false,
								"readOnlyRootFilesystem":   true,
								"capabilities": map[string]interface{}{
									"drop": []string{"ALL"},
								},
							},
							"volumeMounts": []map[string]interface{}{
								{"name": "profiles", "mountPath": "/profiles", "readOnly": true},
								{"name": "fixture", "mountPath": "/fixtures", "readOnly": true},
								{"name": "work", "mountPath": "/work"},
								{"name": "tmp", "mountPath": "/tmp"},
							},
						}},
						"volumes": []map[string]interface{}{
							{
								"name": "profiles",
								"projected": map[string]interface{}{
									"defaultMode": 292,
									"sources": []map[string]interface{}{{
										"configMap": map[string]interface{}{
											"name": "coding-model-profiles", "items": profileItems,
										},
									}},
								},
							},
							{
								"name": "fixture",
								"configMap": map[string]interface{}{
									"name": "coding-model-fixture",
									"items": []map[string]string{{
										"key": "coding-model-mock.yaml", "path": "coding-model-mock.yaml",
									}},
								},
							},
							{"name": "work", "emptyDir": map[string]interface{}{}},
							{"name": "tmp", "emptyDir": map[string]interface{}{}},
						},
					},
				},
			},
		},
		{
			"apiVersion": "v1",
			"kind":       "Service",
			"metadata":   map[string]interface{}{"name": "coding-model", "labels": labels},
			"spec": map[string]interface{}{
				"selector": map[string]string{"app": "coding-model"},
				"ports": []map[string]interface{}{{
					"name": "http", "port": 11434, "targetPort": "http", "protocol": "TCP",
				}},
			},
		},
	}

	var manifest strings.Builder
	for index, document := range documents {
		data, marshalErr := yaml.Marshal(document)
		if marshalErr != nil {
			cleanup()
			return "", nil, fmt.Errorf("marshal coding model mock document %d: %w", index, marshalErr)
		}
		if index > 0 {
			manifest.WriteString("---\n")
		}
		manifest.Write(data)
	}
	path := filepath.Join(dir, "coding-model-mock.yaml")
	if err := os.WriteFile(path, []byte(manifest.String()), 0o644); err != nil {
		cleanup()
		return "", nil, err
	}
	return path, cleanup, nil
}

type codingModelMockLog struct {
	Count    int                      `json:"count"`
	Requests []codingModelMockRequest `json:"requests"`
}

type codingModelMockRequest struct {
	Method  string `json:"method"`
	Path    string `json:"path"`
	Body    string `json:"body"`
	Matched bool   `json:"matched"`
}

type codingModelMockChatRequest struct {
	Model    string        `json:"model"`
	Stream   bool          `json:"stream"`
	Messages []interface{} `json:"messages"`
}

func readCodingModelMockLog(environment codingSmokeEnvironment) (codingModelMockLog, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	output, err := environment.run(ctx, "kubectl", "exec",
		"-n", codingHelmNamespace,
		"deployment/coding-model", "-c", "mock",
		"--", "wget", "-qO-", "http://127.0.0.1:11434/_mock/log")
	if err != nil {
		return codingModelMockLog{}, fmt.Errorf(
			"read coding model mock request log: %w: %s", err, strings.TrimSpace(string(output)))
	}
	var log codingModelMockLog
	if err := json.Unmarshal(output, &log); err != nil {
		return codingModelMockLog{}, fmt.Errorf("decode coding model mock request log: %w", err)
	}
	return log, nil
}

func verifyCodingModelMockLog(log codingModelMockLog) error {
	if log.Count != len(log.Requests) {
		return fmt.Errorf("mock log count %d != %d requests", log.Count, len(log.Requests))
	}
	var chats []codingModelMockChatRequest
	for index, request := range log.Requests {
		if !request.Matched {
			return fmt.Errorf("mock request %d was unmatched: %s %s", index+1, request.Method, request.Path)
		}
		switch {
		case request.Method == "GET" && request.Path == "/api/tags":
			continue
		case request.Method == "POST" && request.Path == "/api/chat":
			var chat codingModelMockChatRequest
			if err := json.Unmarshal([]byte(request.Body), &chat); err != nil {
				return fmt.Errorf("decode chat request %d: %w", len(chats)+1, err)
			}
			if chat.Model != codingModelMockModel {
				return fmt.Errorf("chat request %d model = %q, want %q",
					len(chats)+1, chat.Model, codingModelMockModel)
			}
			if chat.Stream {
				return fmt.Errorf("chat request %d sets stream=true", len(chats)+1)
			}
			if len(chat.Messages) == 0 {
				return fmt.Errorf("chat request %d has no messages", len(chats)+1)
			}
			chats = append(chats, chat)
		default:
			return fmt.Errorf("unexpected matched mock request %d: %s %s",
				index+1, request.Method, request.Path)
		}
	}
	if len(chats) != 3 {
		return fmt.Errorf("mock logged %d chat requests, want exactly 3", len(chats))
	}
	bodies := make([]string, len(chats))
	for index := range chats {
		data, _ := json.Marshal(chats[index].Messages)
		bodies[index] = string(data)
	}
	if !strings.Contains(bodies[0], "software planning assistant") &&
		!strings.Contains(bodies[0], "implementation planner for a Go software project") {
		return errors.New("chat request 1 is not the planner turn")
	}
	if strings.Contains(bodies[1], `"tool":"edit"`) || strings.Contains(bodies[1], `"replacements":1`) {
		return errors.New("chat request 2 unexpectedly includes a completed edit")
	}
	if !strings.Contains(bodies[2], `"tool":"edit"`) &&
		!strings.Contains(bodies[2], `\"tool\":\"edit\"`) &&
		!strings.Contains(bodies[2], `"replacements":1`) {
		return errors.New("chat request 3 does not carry the prior edit result")
	}
	return nil
}

func verifyLiveCodingModelMockLog(environment codingSmokeEnvironment) error {
	log, err := readCodingModelMockLog(environment)
	if err != nil {
		return err
	}
	return verifyCodingModelMockLog(log)
}
