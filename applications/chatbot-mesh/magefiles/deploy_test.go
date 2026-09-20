// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Nokia-Bell-Labs/declarative-agents/magefiles/kindrig"
	"gopkg.in/yaml.v3"
)

func TestChatbotDeployCoordinatesResolveCompletely(t *testing.T) {
	t.Parallel()
	coordinates := chatbotDeployCoordinates("/tmp/staged-chart")
	for name, value := range map[string]string{
		"release":   coordinates.Release,
		"namespace": coordinates.Namespace,
		"chart":     coordinates.ChartPath,
		"values":    coordinates.ValuesPath,
		"timeout":   coordinates.Timeout,
	} {
		if strings.TrimSpace(value) == "" {
			t.Errorf("coordinate %s is empty", name)
		}
	}
	if coordinates.Release != chatbotDemoRelease {
		t.Errorf("release = %q, want %q", coordinates.Release, chatbotDemoRelease)
	}
	// The imperative path passed no --namespace and landed wherever the
	// kubeconfig pointed. Every deploy word carries one, so the choice is
	// written down rather than inherited.
	if coordinates.Namespace != chatbotDemoNamespace {
		t.Errorf("namespace = %q, want %q", coordinates.Namespace, chatbotDemoNamespace)
	}
	if !strings.HasSuffix(coordinates.ValuesPath, filepath.Join("ci", chatbotDemoValuesFile)) {
		t.Errorf("values = %q, want the staged kind overlay", coordinates.ValuesPath)
	}
}

type chatbotOverrides struct {
	Image struct {
		Repository any `yaml:"repository"`
		Tag        any `yaml:"tag"`
		PullPolicy any `yaml:"pullPolicy"`
	} `yaml:"image"`
	Ingress struct {
		Enabled   any `yaml:"enabled"`
		ClassName any `yaml:"className"`
		Host      any `yaml:"host"`
	} `yaml:"ingress"`
	Chatbot struct {
		UIArchiveConfigMap any `yaml:"uiArchiveConfigMap"`
		UIArchiveChecksum  any `yaml:"uiArchiveChecksum"`
	} `yaml:"chatbot"`
}

func decodeChatbotOverrides(t *testing.T, document string) chatbotOverrides {
	t.Helper()
	var decoded chatbotOverrides
	if err := yaml.Unmarshal([]byte(document), &decoded); err != nil {
		t.Fatalf("overrides do not decode as YAML: %v\n%s", err, document)
	}
	return decoded
}

// A commit-shaped tag and a hex checksum both decode as numbers when bare and
// stop matching what the chart expects. --set-string prevented that before the
// migration, so the document has to keep the quotes. Assert the type.
func TestChatbotDeployOverridesKeepTagAndChecksumAsStrings(t *testing.T) {
	t.Parallel()
	decoded := decodeChatbotOverrides(t, chatbotDeployOverrides(
		"declarative-agents/chatbot-mesh:20260919",
		[]externalUIAsset{{Component: "chatbot", ConfigMapName: "demo-chatbot-ui", Checksum: "1234567890"}}))

	for name, value := range map[string]any{
		"image.tag":                  decoded.Image.Tag,
		"chatbot.uiArchiveChecksum":  decoded.Chatbot.UIArchiveChecksum,
		"chatbot.uiArchiveConfigMap": decoded.Chatbot.UIArchiveConfigMap,
	} {
		if _, ok := value.(string); !ok {
			t.Errorf("%s decoded as %T (%v), want a string", name, value, value)
		}
	}
	if decoded.Image.Tag != "20260919" {
		t.Errorf("image.tag = %v, want the numeric-looking tag preserved", decoded.Image.Tag)
	}
	if decoded.Chatbot.UIArchiveChecksum != "1234567890" {
		t.Errorf("checksum = %v, want the digits preserved as a string", decoded.Chatbot.UIArchiveChecksum)
	}
}

// The ingress settings are what make the demo reachable at its host, and they
// were --set flags before the migration.
func TestChatbotDeployOverridesCarryTheIngress(t *testing.T) {
	t.Parallel()
	decoded := decodeChatbotOverrides(t, chatbotDeployOverrides("image:tag", nil))
	if decoded.Ingress.Enabled != true {
		t.Errorf("ingress.enabled = %v, want true", decoded.Ingress.Enabled)
	}
	if decoded.Ingress.ClassName != chatbotDemoIngressClass {
		t.Errorf("ingress.className = %v, want %q", decoded.Ingress.ClassName, chatbotDemoIngressClass)
	}
	if decoded.Ingress.Host != chatbotDemoHost {
		t.Errorf("ingress.host = %v, want %q", decoded.Ingress.Host, chatbotDemoHost)
	}
	if decoded.Image.PullPolicy != "Never" {
		t.Errorf("image.pullPolicy = %v, want Never", decoded.Image.PullPolicy)
	}
}

// Every externalized asset has to name its own ConfigMap, or the chart mounts
// nothing for that component.
func TestChatbotDeployOverridesNameEveryAssetConfigMap(t *testing.T) {
	t.Parallel()
	document := chatbotDeployOverrides("image:tag", []externalUIAsset{
		{Component: "chatbot", ConfigMapName: "demo-chatbot-ui", Checksum: "aaa"},
		{Component: "observer", ConfigMapName: "demo-observer-ui", Checksum: "bbb"},
	})
	var decoded map[string]any
	if err := yaml.Unmarshal([]byte(document), &decoded); err != nil {
		t.Fatalf("overrides do not decode: %v\n%s", err, document)
	}
	for component, configMap := range map[string]string{
		"chatbot":  "demo-chatbot-ui",
		"observer": "demo-observer-ui",
	} {
		section, ok := decoded[component].(map[string]any)
		if !ok {
			t.Errorf("no %s section in:\n%s", component, document)
			continue
		}
		if section["uiArchiveConfigMap"] != configMap {
			t.Errorf("%s.uiArchiveConfigMap = %v, want %q", component, section["uiArchiveConfigMap"], configMap)
		}
	}
}

// The budget gate reads the overrides document from the workspace the machine
// writes it into, so both have to agree on the path. If they diverge, the
// measurement silently stops covering the values the release actually gets.
func TestChatbotBudgetReadsTheWorkspaceOverrides(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	overrides := chatbotDeployOverrides("image:tag", nil)
	if err := writeDeployOverrides(root, overrides); err != nil {
		t.Fatal(err)
	}
	path := kindrig.DeployOverridesPath(root, chatbotDemoRelease)
	written, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("overrides not at the workspace path the machine uses: %v", err)
	}
	if string(written) != overrides {
		t.Error("written overrides differ from the document handed to the machine")
	}
	args := chatbotBudgetValueArgs(root, "/tmp/staged")
	if !containsValue(args, path) {
		t.Errorf("budget args = %v, want them to read %q", args, path)
	}
	if !containsValue(args, filepath.Join("/tmp/staged", "ci", chatbotDemoValuesFile)) {
		t.Errorf("budget args = %v, want the staged overlay too", args)
	}
}

// The undeploy coordinates are bare tokens the undeploy words never read, and
// they must carry no YAML metacharacter: the renderer substitutes into YAML
// without quoting, so a colon or a bracket renders a file that will not parse
// (GH-2349).
func TestChatbotUndeployPlaceholdersAreYAMLSafe(t *testing.T) {
	t.Parallel()
	for _, value := range []string{"undeploy-names-no-chart", "undeploy-reads-no-values"} {
		if strings.ContainsAny(value, ":#{}[]&*!|>'\"%@`,") {
			t.Errorf("placeholder %q carries a YAML metacharacter", value)
		}
	}
}

func containsValue(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
