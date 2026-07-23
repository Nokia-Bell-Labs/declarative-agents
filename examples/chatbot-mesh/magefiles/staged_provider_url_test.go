// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"

	"gopkg.in/yaml.v3"
	"strings"
	"testing"
)

// This is the check that would have caught GH-728. The chart co-generates the
// REST client's base_url from the configured LLM URL, so rest.yaml was right in
// cluster while the staged tool declarations still named localhost -- and
// localhost is correct on a developer machine, so every local target passed.
// Only an agent running somewhere that is not the Ollama host could fail, which
// made the kind smoke the first and only place it surfaced.
//
// So this asserts over the staged declarations: an address that must differ
// between a local run and a deployment has to be an environment reference with
// a local default, never a bare loopback literal (srd013 R5.7).

// providerURLRE matches a provider_url value in a declarations file.
var providerURLRE = regexp.MustCompile(`provider_url:\s*"([^"]*)"`)

// loopbackHosts are the addresses that are only reachable when the agent shares
// a network namespace with the provider.
var loopbackHosts = []string{"localhost", "127.0.0.1", "[::1]", "0.0.0.0"}

func isLoopback(value string) bool {
	for _, host := range loopbackHosts {
		if strings.Contains(value, host) {
			return true
		}
	}
	return false
}

// stagedDeclarationFiles returns every *.yaml under the agent directories the
// chart stages, excluding test fixtures, which never run in a pod.
func stagedDeclarationFiles(t *testing.T) []string {
	t.Helper()
	root, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	profilesRoot := filepath.Dir(root)
	var files []string
	for _, program := range chartProfilePrograms() {
		if !strings.HasPrefix(program.src, "agents/") {
			continue
		}
		src := filepath.Join(profilesRoot, program.src)
		err := filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
			if err != nil || info.IsDir() || filepath.Ext(path) != ".yaml" {
				return err
			}
			if strings.Contains(path, string(filepath.Separator)+"tests"+string(filepath.Separator)) {
				return nil
			}
			files = append(files, path)
			return nil
		})
		if err != nil {
			t.Fatalf("walk %s: %v", src, err)
		}
	}
	if len(files) == 0 {
		t.Fatal("no staged declaration files found; the guard would pass vacuously")
	}
	return files
}

// TestStagedDeclarationsParameterizeTheProvider fails when a staged program
// hard-codes a loopback provider address, which a deployed pod cannot reach.
func TestStagedDeclarationsParameterizeTheProvider(t *testing.T) {
	t.Parallel()
	checked := 0
	for _, path := range stagedDeclarationFiles(t) {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		for _, match := range providerURLRE.FindAllStringSubmatch(string(data), -1) {
			checked++
			value := match[1]
			if strings.Contains(value, "${") {
				continue
			}
			if isLoopback(value) {
				rel, _ := filepath.Rel(filepath.Dir(filepath.Dir(path)), path)
				t.Errorf("%s declares provider_url %q; a staged declaration must use "+
					"${OLLAMA_URL:-<local default>} so a deployment can reach the provider (srd013 R5.7)",
					rel, value)
			}
		}
	}
	if checked == 0 {
		t.Fatal("no provider_url values inspected; the pattern changed and this guard went blind")
	}
}

// envDoc is the slice of a rendered Deployment this guard reads. renderedDoc in
// provisioning_route_render_test.go carries ingress and policy fields instead,
// so the container env is parsed locally rather than widening that struct.
type envDoc struct {
	Kind     string `yaml:"kind"`
	Metadata struct {
		Name string `yaml:"name"`
	} `yaml:"metadata"`
	Spec struct {
		Template struct {
			Spec struct {
				Containers []struct {
					Name string `yaml:"name"`
					Env  []struct {
						Name  string `yaml:"name"`
						Value string `yaml:"value"`
					} `yaml:"env"`
				} `yaml:"containers"`
			} `yaml:"spec"`
		} `yaml:"template"`
	} `yaml:"spec"`
}

// chatbotProviderEnv renders the chart with the external-Ollama values the kind
// smoke uses and returns the chatbot container's OLLAMA_URL.
func chatbotProviderEnv(t *testing.T) (string, bool) {
	t.Helper()
	if _, err := exec.LookPath("helm"); err != nil {
		t.Skip("helm not on PATH")
	}
	chartDir := findChartDir(t)
	out, err := exec.Command("helm", "template", "rel", chartDir,
		"-f", filepath.Join(chartDir, "ci", "kind-values.yaml")).CombinedOutput()
	if err != nil {
		t.Fatalf("helm template: %v\n%s", err, out)
	}
	for _, chunk := range strings.Split(string(out), "\n---") {
		var doc envDoc
		if err := yaml.Unmarshal([]byte(chunk), &doc); err != nil {
			continue
		}
		if doc.Kind != "Deployment" || !strings.HasSuffix(doc.Metadata.Name, "-chatbot") {
			continue
		}
		for _, container := range doc.Spec.Template.Spec.Containers {
			for _, env := range container.Env {
				if env.Name == "OLLAMA_URL" {
					return env.Value, true
				}
			}
		}
		return "", false
	}
	t.Fatal("no chatbot Deployment in the rendered chart")
	return "", false
}

// TestChatbotDeploymentSuppliesTheProviderReference is the other half of the
// contract: the reference in the declarations is inert unless the chart sets
// it, and the chatbot Deployment carried no env at all before GH-728.
func TestChatbotDeploymentSuppliesTheProviderReference(t *testing.T) {
	t.Parallel()
	value, ok := chatbotProviderEnv(t)

	if !ok {
		t.Fatal("chatbot Deployment sets no OLLAMA_URL; the staged declarations would fall back to localhost")
	}
	if isLoopback(value) {
		t.Errorf("chatbot Deployment sets OLLAMA_URL=%q, which a pod cannot reach", value)
	}
}
