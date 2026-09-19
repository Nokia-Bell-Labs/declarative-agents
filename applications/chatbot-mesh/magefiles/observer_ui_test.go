// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package main

import (
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// observerMonitorServer is the part of the observer monitor server these tests
// read: the fleet UI binding and the monitor proxy beside it.
type observerMonitorServer struct {
	Address   string `yaml:"address"`
	Endpoints map[string]struct {
		Method       string `yaml:"method"`
		Path         string `yaml:"path"`
		Binding      string `yaml:"binding"`
		StaticAssets *struct {
			Root   string `yaml:"root"`
			Bundle string `yaml:"bundle"`
			Index  string `yaml:"index"`
			SPA    bool   `yaml:"spa"`
			Config struct {
				Title           string `yaml:"title"`
				MonitoredAgents []struct {
					Name  string `yaml:"name"`
					Label string `yaml:"label"`
				} `yaml:"monitored_agents"`
				TraceBackend string `yaml:"trace_backend"`
			} `yaml:"config"`
		} `yaml:"static_assets"`
		MonitorProxy *struct {
			Upstreams map[string]string `yaml:"upstreams"`
		} `yaml:"monitor_proxy"`
	} `yaml:"endpoints"`
}

func parseObserverMonitorServer(t *testing.T, raw []byte) observerMonitorServer {
	t.Helper()
	var declaration struct {
		Rest struct {
			Servers map[string]observerMonitorServer `yaml:"servers"`
		} `yaml:"rest"`
	}
	if err := yaml.Unmarshal(expandDeclarationDefaults(raw), &declaration); err != nil {
		t.Fatalf("parse observer monitor-rest declaration: %v", err)
	}
	server, ok := declaration.Rest.Servers["observer_monitor"]
	if !ok {
		t.Fatal("observer monitor-rest declares no observer_monitor server")
	}
	return server
}

// monitoredAgentNames returns the names the fleet UI config lists.
func (server observerMonitorServer) monitoredAgentNames(t *testing.T) []string {
	t.Helper()
	ui := server.Endpoints["fleet_ui"].StaticAssets
	if ui == nil {
		t.Fatal("observer fleet_ui endpoint has no static_assets binding")
	}
	var names []string
	for _, agent := range ui.Config.MonitoredAgents {
		names = append(names, agent.Name)
	}
	return names
}

// assertObserverServesEmbeddedBundle checks srd004 R9.1/R9.2 and R2.1/R2.3 on one
// rendering of the observer monitor server: the UI is the compiled observer
// bundle configured by declaration, and every agent the UI names -- monitored
// agents and the trace backend -- has a monitor-proxy upstream.
func assertObserverServesEmbeddedBundle(t *testing.T, server observerMonitorServer) {
	t.Helper()
	ui := server.Endpoints["fleet_ui"]
	if ui.Method != "GET" || ui.Path != "/ui/{path...}" || ui.Binding != "static_assets" || ui.StaticAssets == nil {
		t.Fatalf("observer fleet_ui = %+v, want GET /ui/{path...} bound to static_assets", ui)
	}
	assets := ui.StaticAssets
	if assets.Bundle != "observer" || assets.Root != "" || !assets.SPA {
		t.Errorf("observer fleet_ui static_assets = bundle %q root %q spa %v; want the observer bundle, no root, SPA",
			assets.Bundle, assets.Root, assets.SPA)
	}
	if assets.Config.Title != "Chatbot Mesh observer" {
		t.Errorf("observer UI title = %q", assets.Config.Title)
	}
	proxy := server.Endpoints["monitor_proxy"]
	if proxy.Method != "GET" || proxy.Path != "/monitor-proxy/{agent}/{path...}" ||
		proxy.Binding != "monitor_proxy" || proxy.MonitorProxy == nil {
		t.Fatalf("observer monitor_proxy = %+v, want GET /monitor-proxy/{agent}/{path...}", proxy)
	}
	named := server.monitoredAgentNames(t)
	if assets.Config.TraceBackend != "" {
		named = append(named, assets.Config.TraceBackend)
	}
	for _, agent := range named {
		if proxy.MonitorProxy.Upstreams[agent] == "" {
			t.Errorf("the observer UI names %s, which has no monitor-proxy upstream", agent)
		}
	}
	for name, endpoint := range server.Endpoints {
		if strings.HasPrefix(endpoint.Path, "/trace-proxy") {
			t.Errorf("observer endpoint %s declares retired route %s (srd004 R2.3)", name, endpoint.Path)
		}
	}
	for _, contract := range []string{"fleet", "current_state", "root_redirect"} {
		if _, ok := server.Endpoints[contract]; !ok {
			t.Errorf("observer monitor server lost endpoint %s", contract)
		}
	}
}

// TestObserverUIRouteAndPackageContract pins the packaged declaration: the
// observer serves the embedded bundle, proxies the chatbot's monitored agents
// and the collector trace backend, and carries declaration files only (srd004
// R9.4).
func TestObserverUIRouteAndPackageContract(t *testing.T) {
	root := filepath.Join("..", "agents", "observer")
	raw, err := os.ReadFile(filepath.Join(root, "monitor-rest.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	server := parseObserverMonitorServer(t, raw)
	assertObserverServesEmbeddedBundle(t, server)

	names := server.monitoredAgentNames(t)
	if want := []string{"chatbot", "rag0", "rag1"}; !reflect.DeepEqual(names, want) {
		t.Errorf("observer monitored agents = %v, want %v (the chatbot's ui.yaml list)", names, want)
	}
	if backend := server.Endpoints["fleet_ui"].StaticAssets.Config.TraceBackend; backend != "collector" {
		t.Errorf("observer trace backend = %q, want collector", backend)
	}
	upstreams := server.Endpoints["monitor_proxy"].MonitorProxy.Upstreams
	want := map[string]string{
		"chatbot":   "http://127.0.0.1:18082",
		"rag0":      "http://127.0.0.1:18087",
		"rag1":      "http://127.0.0.1:18097",
		"collector": "http://127.0.0.1:18193",
	}
	if !reflect.DeepEqual(upstreams, want) {
		t.Errorf("observer local monitor-proxy upstreams = %v, want %v", upstreams, want)
	}

	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	var files []string
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".yaml") {
			t.Errorf("agents/observer carries %s; an embedded-bundle observer holds declaration files only", entry.Name())
		}
		files = append(files, entry.Name())
	}
	sort.Strings(files)
	if want := []string{"monitor-rest.yaml", "profile.yaml", "rest.yaml"}; !reflect.DeepEqual(files, want) {
		t.Errorf("agents/observer = %v, want %v", files, want)
	}
}

// TestObserverMonitorRestCoGeneratedForCluster pins the cluster render: the
// chart overrides the observer's monitor-rest.yaml key so the proxy reaches each
// deployed agent at its Service and the UI names exactly those agents, tracking
// ragUnits and the collector toggle as the chatbot's co-generated files do.
func TestObserverMonitorRestCoGeneratedForCluster(t *testing.T) {
	const key = "applications__chatbot-mesh__observer__monitor-rest.yaml"
	cases := []struct {
		name          string
		sets          []string
		wantAgents    []string
		wantUpstreams map[string]string
	}{
		{
			name:       "default",
			wantAgents: []string{"chatbot", "rag0", "rag1"},
			wantUpstreams: map[string]string{
				"chatbot":   "http://rel-chatbot-mesh-chatbot:18082",
				"rag0":      "http://rel-chatbot-mesh-rag0:18087",
				"rag1":      "http://rel-chatbot-mesh-rag1:18087",
				"collector": "http://rel-chatbot-mesh-collector:18193",
			},
		},
		{
			name:       "collector off",
			sets:       []string{"collector.enabled=false"},
			wantAgents: []string{"chatbot", "rag0", "rag1"},
			wantUpstreams: map[string]string{
				"chatbot": "http://rel-chatbot-mesh-chatbot:18082",
				"rag0":    "http://rel-chatbot-mesh-rag0:18087",
				"rag1":    "http://rel-chatbot-mesh-rag1:18087",
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sets := append([]string{"observer.enabled=true"}, tc.sets...)
			rendered := renderedProfileFile(t, key, sets...)
			server := parseObserverMonitorServer(t, []byte(rendered))
			assertObserverServesEmbeddedBundle(t, server)
			if server.Address != "0.0.0.0:18202" {
				t.Errorf("cluster observer monitor address = %q, want 0.0.0.0:18202", server.Address)
			}
			if names := server.monitoredAgentNames(t); !reflect.DeepEqual(names, tc.wantAgents) {
				t.Errorf("cluster monitored agents = %v, want %v", names, tc.wantAgents)
			}
			upstreams := server.Endpoints["monitor_proxy"].MonitorProxy.Upstreams
			if !reflect.DeepEqual(upstreams, tc.wantUpstreams) {
				t.Errorf("cluster monitor-proxy upstreams = %v, want %v", upstreams, tc.wantUpstreams)
			}
			if strings.Contains(rendered, "${") {
				t.Error("cluster observer monitor-rest carries an environment expansion")
			}
		})
	}
}

// renderedProfileFile renders the chart and returns one profiles ConfigMap key's
// content, de-indented from its block scalar.
func renderedProfileFile(t *testing.T, key string, sets ...string) string {
	t.Helper()
	out := helmTemplateOutput(t, sets...)
	for _, doc := range strings.Split(out, "\n---") {
		var configMap struct {
			Kind     string `yaml:"kind"`
			Metadata struct {
				Name string `yaml:"name"`
			} `yaml:"metadata"`
			Data map[string]string `yaml:"data"`
		}
		if err := yaml.Unmarshal([]byte(doc), &configMap); err != nil {
			continue
		}
		if configMap.Kind == "ConfigMap" && strings.HasSuffix(configMap.Metadata.Name, "-profiles") {
			content, ok := configMap.Data[key]
			if !ok {
				t.Fatalf("profiles ConfigMap has no %s key", key)
			}
			return content
		}
	}
	t.Fatal("rendered chart has no profiles ConfigMap")
	return ""
}
