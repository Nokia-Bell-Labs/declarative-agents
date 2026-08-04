// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Nokia-Bell-Labs/declarative-agents/magefiles/appmanifest"
	"gopkg.in/yaml.v3"
)

const (
	applicationModule      = "github.com/Nokia-Bell-Labs/declarative-agents/applications/prose-editor"
	applicationManifest    = "agents/application.yaml"
	canonicalCorpusProfile = "agents/knowledge-manager/corpus-ingest/profile.yaml"
	canonicalRelease       = "v0.20260803.0"
	demoConfigFile         = "demo.yaml"
)

var (
	requiredREADMESections = []string{
		"Purpose",
		"Status",
		"Composition",
		"Capabilities",
		"Ownership Boundaries",
		"Planned Entry Points",
		"Verification",
		"Documentation",
	}
	plannedLocalRoots = []string{
		"corpus-ingest",
		"specialist-editor",
		"structure-rag",
		"tightening-rag",
		"voice-critic",
		"voice-rag",
		"workflow-orchestrator",
	}
)

type demoConfig struct {
	CatalogRoot string `yaml:"catalog_root"`
}

type applicationStats struct {
	Application struct {
		Ownership           string   `json:"ownership"`
		ModuleStatus        string   `json:"module_status"`
		AgentsContributed   int      `json:"agents_contributed"`
		CanonicalReferences int      `json:"canonical_references"`
		CanonicalProfiles   []string `json:"canonical_profiles"`
		PlannedRoots        []string `json:"planned_roots"`
	} `json:"application"`
}

// Audit validates the audit-only manifest and documentation corpus.
func Audit() error {
	root, err := applicationRootFromWorkingDirectory()
	if err != nil {
		return err
	}
	count, err := auditApplication(root)
	if err != nil {
		return err
	}
	fmt.Printf("audit: validated Prose Editor manifest and %d YAML documents\n", count)
	return nil
}

// Stats reports planned composition without contributing an agents section.
func Stats() error {
	root, err := applicationRootFromWorkingDirectory()
	if err != nil {
		return err
	}
	manifest, err := loadApplicationManifest(root)
	if err != nil {
		return err
	}
	encoded, err := json.Marshal(newStats(manifest))
	if err != nil {
		return fmt.Errorf("encode Prose Editor stats: %w", err)
	}
	fmt.Println(string(encoded))
	return nil
}

func auditApplication(root string) (int, error) {
	manifest, err := loadApplicationManifest(root)
	if err != nil {
		return 0, err
	}
	if err := validateAuditOnlyManifest(manifest); err != nil {
		return 0, err
	}
	count, err := auditYAMLDocuments(root)
	if err != nil {
		return 0, err
	}
	if err := auditREADME(root); err != nil {
		return 0, err
	}
	if err := auditStatusClaims(root); err != nil {
		return 0, err
	}
	return count, nil
}

func loadApplicationManifest(root string) (appmanifest.Manifest, error) {
	config, err := loadDemoConfig(root)
	if err != nil {
		return appmanifest.Manifest{}, err
	}
	catalogRoot, err := resolveCatalogRoot(root, config)
	if err != nil {
		return appmanifest.Manifest{}, err
	}
	if err := requireRegularFile(filepath.Join(catalogRoot, filepath.FromSlash(canonicalCorpusProfile))); err != nil {
		return appmanifest.Manifest{}, fmt.Errorf("canonical corpus-ingest dependency: %w", err)
	}
	return appmanifest.Load(
		filepath.Join(root, filepath.FromSlash(applicationManifest)),
		appmanifest.Options{ApplicationRoot: root, CatalogRoot: catalogRoot},
	)
}

func validateAuditOnlyManifest(manifest appmanifest.Manifest) error {
	if manifest.Application != "prose-editor" ||
		manifest.Ownership != "composition-only" ||
		manifest.ModuleStatus != "audit_only" {
		return fmt.Errorf("application manifest identity/status is not audit-only Prose Editor")
	}
	wantCapabilities := map[string]string{
		"runnable_module": "audit_only",
		"managed_service": "planned",
		"packaged":        "planned",
		"helm_managed":    "planned",
		"kind_demo":       "planned",
		"ui":              "not_applicable",
	}
	if len(manifest.Capabilities) != len(wantCapabilities) {
		return fmt.Errorf("application manifest capabilities = %d, want %d",
			len(manifest.Capabilities), len(wantCapabilities))
	}
	for name, want := range wantCapabilities {
		if got := manifest.Capabilities[name].Status; got != want {
			return fmt.Errorf("capability %s status = %q, want %q", name, got, want)
		}
	}
	if manifest.Runtime.MountPath != "" || manifest.Runtime.ImageContainsProfiles ||
		len(manifest.Deployment.Entries) != 0 || len(manifest.UI.Assets) != 0 {
		return errors.New("audit-only manifest must not declare a runtime, deployment, or UI surface")
	}

	var local, catalog []string
	for _, root := range manifest.Roots {
		if !root.Planned {
			return fmt.Errorf("root %s must remain planned while the module is audit-only", root.ID)
		}
		switch root.Ownership {
		case "local":
			local = append(local, root.ID)
		case "catalog":
			if root.ID != "catalog-corpus-ingest" ||
				root.Source != canonicalCorpusProfile ||
				root.RuntimePath != canonicalCorpusProfile ||
				root.CompatibleRelease != canonicalRelease {
				return fmt.Errorf("catalog root %s is not the canonical corpus-ingest reference", root.ID)
			}
			catalog = append(catalog, root.ID)
		}
	}
	sort.Strings(local)
	if strings.Join(local, "\x00") != strings.Join(plannedLocalRoots, "\x00") {
		return fmt.Errorf("planned local roots = %v, want %v", local, plannedLocalRoots)
	}
	if len(catalog) != 1 {
		return fmt.Errorf("canonical roots = %v, want [catalog-corpus-ingest]", catalog)
	}
	return nil
}

func auditYAMLDocuments(root string) (int, error) {
	var files []string
	err := filepath.WalkDir(filepath.Join(root, "docs"), func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if !entry.IsDir() && (strings.HasSuffix(entry.Name(), ".yaml") || strings.HasSuffix(entry.Name(), ".yml")) {
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		return 0, fmt.Errorf("walk Prose Editor documents: %w", err)
	}
	files = append(files,
		filepath.Join(root, filepath.FromSlash(applicationManifest)),
		filepath.Join(root, demoConfigFile),
		filepath.Join(root, ".golangci.yml"),
	)
	sort.Strings(files)
	for _, path := range files {
		if err := parseSingleYAML(path); err != nil {
			return 0, err
		}
	}
	return len(files), nil
}

func parseSingleYAML(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read %s: %w", filepath.ToSlash(path), err)
	}
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	var document yaml.Node
	if err := decoder.Decode(&document); err == io.EOF {
		return nil
	} else if err != nil {
		return fmt.Errorf("parse %s: %w", filepath.ToSlash(path), err)
	}
	if len(document.Content) == 0 {
		return fmt.Errorf("parse %s: empty YAML document", filepath.ToSlash(path))
	}
	var trailing yaml.Node
	if err := decoder.Decode(&trailing); err == nil {
		return fmt.Errorf("parse %s: expected exactly one YAML document", filepath.ToSlash(path))
	} else if err != io.EOF {
		return fmt.Errorf("parse %s: %w", filepath.ToSlash(path), err)
	}
	return nil
}

func auditREADME(root string) error {
	data, err := os.ReadFile(filepath.Join(root, "README.md"))
	if err != nil {
		return fmt.Errorf("read README.md: %w", err)
	}
	content := string(data)
	for _, section := range requiredREADMESections {
		if !strings.Contains(content, "\n## "+section+"\n") {
			return fmt.Errorf("README.md missing required section %q", section)
		}
	}
	return nil
}

func auditStatusClaims(root string) error {
	var architecture struct {
		Overview struct {
			Status string `yaml:"status"`
		} `yaml:"overview"`
		ImplementationStatus struct {
			Overall            string `yaml:"overall"`
			RegistryStatus     string `yaml:"registry_status"`
			ExecutableEvidence string `yaml:"executable_evidence"`
		} `yaml:"implementation_status"`
	}
	if err := readYAML(filepath.Join(root, "docs", "ARCHITECTURE.yaml"), &architecture); err != nil {
		return err
	}
	if architecture.Overview.Status != "audit_only" ||
		architecture.ImplementationStatus.Overall != "audit_only" ||
		architecture.ImplementationStatus.RegistryStatus != "audit_only" ||
		architecture.ImplementationStatus.ExecutableEvidence != "governance_only" {
		return errors.New("architecture status must claim audit-only governance evidence and no runtime")
	}

	var roadmap struct {
		Releases []struct {
			Version          string `yaml:"version"`
			Status           string `yaml:"status"`
			CapabilityStatus struct {
				RunnableModule string `yaml:"runnable_module"`
			} `yaml:"capability_status"`
			EvidenceStatus struct {
				RootRegistration   string `yaml:"root_registration"`
				ExecutableEvidence string `yaml:"executable_evidence"`
			} `yaml:"evidence_status"`
		} `yaml:"releases"`
	}
	if err := readYAML(filepath.Join(root, "docs", "road-map.yaml"), &roadmap); err != nil {
		return err
	}
	foundRelease := false
	for _, release := range roadmap.Releases {
		if release.Version != "00.1" {
			continue
		}
		foundRelease = true
		if release.Status != "planned" ||
			release.CapabilityStatus.RunnableModule != "audit_only" ||
			release.EvidenceStatus.RootRegistration != "audit_only" ||
			release.EvidenceStatus.ExecutableEvidence != "governance_only" {
			return errors.New("road-map release 00.1 must separate audit-only governance from planned runtime")
		}
	}
	if !foundRelease {
		return errors.New("road-map does not contain release 00.1")
	}

	var suite struct {
		Status        string `yaml:"status"`
		RuntimeStatus string `yaml:"runtime_status"`
		TestCases     []struct {
			CurrentEvidence string `yaml:"current_evidence"`
		} `yaml:"test_cases"`
	}
	if err := readYAML(filepath.Join(
		root, "docs", "specs", "test-suites", "test-rel00.1-prose-editor.yaml"), &suite); err != nil {
		return err
	}
	if suite.Status != "planned" || suite.RuntimeStatus != "unimplemented" {
		return errors.New("release 00.1 tracer suite must remain planned and unimplemented")
	}
	for index, testCase := range suite.TestCases {
		if testCase.CurrentEvidence != "none" {
			return fmt.Errorf("release 00.1 tracer test case %d claims runtime evidence", index+1)
		}
	}
	return nil
}

func newStats(manifest appmanifest.Manifest) applicationStats {
	var result applicationStats
	result.Application.Ownership = manifest.Ownership
	result.Application.ModuleStatus = manifest.ModuleStatus
	for _, root := range manifest.Roots {
		if root.Planned {
			result.Application.PlannedRoots = append(result.Application.PlannedRoots, root.ID)
		}
		if root.Ownership == "catalog" {
			result.Application.CanonicalReferences++
			result.Application.CanonicalProfiles = append(
				result.Application.CanonicalProfiles,
				"applications/catalog/"+root.Source,
			)
		}
	}
	sort.Strings(result.Application.CanonicalProfiles)
	sort.Strings(result.Application.PlannedRoots)
	return result
}

func applicationRootFromWorkingDirectory() (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("resolve Prose Editor working directory: %w", err)
	}
	current, err := filepath.Abs(filepath.Clean(cwd))
	if err != nil {
		return "", fmt.Errorf("resolve Prose Editor root from %q: %w", cwd, err)
	}
	for {
		data, readErr := os.ReadFile(filepath.Join(current, "go.mod"))
		if readErr == nil && strings.Contains(string(data), "module "+applicationModule) {
			return current, nil
		}
		parent := filepath.Dir(current)
		if parent == current {
			break
		}
		current = parent
	}
	return "", fmt.Errorf(
		"could not find the Prose Editor root from %s; run from applications/prose-editor or a directory beneath it",
		cwd,
	)
}

func loadDemoConfig(root string) (demoConfig, error) {
	var config demoConfig
	data, err := os.ReadFile(filepath.Join(root, demoConfigFile))
	if err != nil {
		return config, fmt.Errorf("read %s: %w", demoConfigFile, err)
	}
	if err := yaml.Unmarshal(data, &config); err != nil {
		return config, fmt.Errorf("parse %s: %w", demoConfigFile, err)
	}
	return config, nil
}

func resolveCatalogRoot(root string, config demoConfig) (string, error) {
	value := strings.TrimSpace(config.CatalogRoot)
	if value == "" {
		value = filepath.Join(root, "..", "catalog")
	} else if !filepath.IsAbs(value) {
		value = filepath.Join(root, value)
	}
	catalogRoot, err := filepath.Abs(filepath.Clean(value))
	if err != nil {
		return "", fmt.Errorf("resolve catalog_root: %w", err)
	}
	info, err := os.Stat(catalogRoot)
	if err != nil || !info.IsDir() {
		return "", fmt.Errorf("catalog_root is not a directory: %s", catalogRoot)
	}
	return catalogRoot, nil
}

func requireRegularFile(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("%s is not a regular file", path)
	}
	return nil
}

func readYAML(path string, out any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read %s: %w", filepath.ToSlash(path), err)
	}
	if err := yaml.Unmarshal(data, out); err != nil {
		return fmt.Errorf("parse %s: %w", filepath.ToSlash(path), err)
	}
	return nil
}
