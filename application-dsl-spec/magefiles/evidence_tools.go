// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package main

import (
	"errors"
	"fmt"
	"os"
	"reflect"
	"regexp"
	"sort"
	"strings"
)

// Document checks for the agent-profile, application-profile, tool,
// REST, and inference-configuration statements (R-PROF-004..005,
// R-APP-002..004, R-TOOL, R-REST, R-LLM). Each check is a pure YAML-shape
// reading of the rule the engine's loader enforces; none imports the engine.

var (
	kebabName         = regexp.MustCompile(`^[a-z][a-z0-9]*(?:-[a-z0-9]+)*$`)
	appIdentifier     = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)
	compatibleRelease = regexp.MustCompile(`^(?:(?:applications/catalog|agent-profiles)/)?v0\.[A-Za-z0-9][A-Za-z0-9._-]*$`)
	fromSelector      = regexp.MustCompile(`^\$from\([^()\s]+\)\.[^\s.][^\s]*$`)
	dollarSelector    = regexp.MustCompile(`^\$\.[^\s.][^\s]*$`)
	credentialRef     = regexp.MustCompile(`^[A-Z_][A-Z0-9_]*$`)
	paramsReference   = regexp.MustCompile(`\{\{ params\.([A-Za-z_][A-Za-z0-9_]*) \}\}`)
)

func setOf(values ...string) map[string]bool {
	set := make(map[string]bool, len(values))
	for _, value := range values {
		set[value] = true
	}
	return set
}

func sortedMapKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func nonEmptyString(value any) bool {
	text, ok := value.(string)
	return ok && strings.TrimSpace(text) != ""
}

// stringSequence reports whether value is a sequence of non-empty strings.
func stringSequence(value any) bool {
	list, ok := value.([]any)
	if !ok {
		return false
	}
	for _, item := range list {
		if !nonEmptyString(item) {
			return false
		}
	}
	return true
}

func mappingAt(doc map[string]any, keyname string) (map[string]any, bool) {
	value, ok := doc[keyname].(map[string]any)
	return value, ok
}

// ---------------------------------------------------------------------------
// agent-profile
// ---------------------------------------------------------------------------

// checkProfileReferenceKeynames enforces R-PROF-004: the agent-profile selects
// at least one tool selection, and every declaration-source keyname is a
// sequence of paths.
func checkProfileReferenceKeynames(path string) error {
	doc, err := readYAMLMapping(path)
	if err != nil {
		return err
	}
	tools, ok := doc["tools"].([]any)
	if !ok || len(tools) == 0 || !stringSequence(tools) {
		return errors.New(`keyname "tools" must be a non-empty sequence of paths`)
	}
	for _, keyname := range []string{"tool_declarations", "tool_config_dirs", "rest_definitions", "rest_config_dirs"} {
		if value, present := doc[keyname]; present && !stringSequence(value) {
			return fmt.Errorf("keyname %q must be a sequence of paths", keyname)
		}
	}
	if value, present := doc["directory"]; present {
		if _, ok := value.(string); !ok {
			return errors.New(`keyname "directory" must be a string`)
		}
	}
	return nil
}

// checkProfileLibraryRoots enforces R-PROF-005: each declared library root is
// a lowercase kebab name other than the implicit agent-core root and names a
// directory.
func checkProfileLibraryRoots(path string) error {
	doc, err := readYAMLMapping(path)
	if err != nil {
		return err
	}
	raw, present := doc["libraries"]
	if !present {
		return nil
	}
	libraries, ok := raw.(map[string]any)
	if !ok {
		return errors.New(`keyname "libraries" must be a mapping`)
	}
	for _, name := range sortedMapKeys(libraries) {
		if name == "agent-core" {
			return errors.New(`library root "agent-core" is implicit and cannot be declared`)
		}
		if !kebabName.MatchString(name) {
			return fmt.Errorf("library root %q is not a lowercase kebab name", name)
		}
		if !nonEmptyString(libraries[name]) {
			return fmt.Errorf("library root %q names no directory", name)
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
// application-profile
// ---------------------------------------------------------------------------

var (
	applicationKeynames = setOf("schema_version", "application", "ownership", "module_status",
		"capabilities", "roots", "runtime", "deployment", "ui", "package")
	applicationOwnership    = setOf("agent-owning", "composition-only")
	applicationStatuses     = setOf("planned", "audit_only", "partial", "dependency_gated", "implemented", "not_applicable")
	applicationCapabilities = setOf("runnable_module", "managed_service", "packaged", "helm_managed", "kind_demo", "ui")
	activeStatuses          = setOf("implemented", "partial", "dependency_gated")
)

// checkApplicationIdentity enforces R-APP-002: the application-profile names
// a supported schema, a kebab identity, a known ownership and module status,
// and no keyname outside its schema.
func checkApplicationIdentity(path string) error {
	doc, err := readYAMLMapping(path)
	if err != nil {
		return err
	}
	for _, keyname := range sortedMapKeys(doc) {
		if !applicationKeynames[keyname] {
			return fmt.Errorf("keyname %q is outside the application-profile schema", keyname)
		}
	}
	if version, _ := doc["schema_version"].(int); version != 1 {
		return fmt.Errorf("schema_version %v is not supported", doc["schema_version"])
	}
	if name, _ := doc["application"].(string); !appIdentifier.MatchString(name) {
		return fmt.Errorf("application %q must be a lower-kebab identifier", name)
	}
	if ownership, _ := doc["ownership"].(string); !applicationOwnership[ownership] {
		return fmt.Errorf("unknown application ownership %q", ownership)
	}
	if status, _ := doc["module_status"].(string); !applicationStatuses[status] {
		return fmt.Errorf("unknown module_status %q", status)
	}
	return nil
}

// checkApplicationCapabilities enforces R-APP-003: capability names and
// statuses come from closed sets, an active claim carries evidence, the
// runnable_module baseline is declared, and helm management rests on an
// active packaging claim.
func checkApplicationCapabilities(path string) error {
	doc, err := readYAMLMapping(path)
	if err != nil {
		return err
	}
	capabilities, ok := mappingAt(doc, "capabilities")
	if !ok || len(capabilities) == 0 {
		return errors.New("capabilities must be a non-empty mapping")
	}
	status := func(name string) string {
		entry, _ := capabilities[name].(map[string]any)
		value, _ := entry["status"].(string)
		return value
	}
	for _, name := range sortedMapKeys(capabilities) {
		if !applicationCapabilities[name] {
			return fmt.Errorf("unknown capability %q", name)
		}
		entry, ok := capabilities[name].(map[string]any)
		if !ok {
			return fmt.Errorf("capability %s must be a mapping", name)
		}
		if !applicationStatuses[status(name)] {
			return fmt.Errorf("capability %s has unknown status %q", name, status(name))
		}
		if activeStatuses[status(name)] {
			evidence, _ := entry["evidence"].([]any)
			if len(evidence) == 0 || !stringSequence(evidence) {
				return fmt.Errorf("capability %s status %s requires evidence", name, status(name))
			}
		}
	}
	if _, declared := capabilities["runnable_module"]; !declared {
		return errors.New("the runnable_module capability is not declared")
	}
	if module, _ := doc["module_status"].(string); module == "audit_only" && activeStatuses[status("runnable_module")] {
		return errors.New("an audit_only module claims runnable behavior")
	}
	if activeStatuses[status("helm_managed")] && !activeStatuses[status("packaged")] {
		return errors.New("helm_managed requires an active packaged capability")
	}
	return nil
}

func applicationProfileFilename(name string) bool {
	return name == "profile.yaml" ||
		(strings.HasPrefix(name, "profile-") && strings.HasSuffix(name, ".yaml")) ||
		strings.HasSuffix(name, "-profile.yaml")
}

// checkApplicationRoots enforces R-APP-004: each root has a unique kebab id,
// a known ownership, and a portable source naming an agent-profile below
// agents/<actor>; a catalog root pins a compatible release and a local root
// does not.
func checkApplicationRoots(path string) error {
	doc, err := readYAMLMapping(path)
	if err != nil {
		return err
	}
	roots, _ := doc["roots"].([]any)
	seen := map[string]bool{}
	for index, raw := range roots {
		root, ok := raw.(map[string]any)
		if !ok {
			return fmt.Errorf("roots[%d] must be a mapping", index)
		}
		id, _ := root["id"].(string)
		if !appIdentifier.MatchString(id) {
			return fmt.Errorf("root id %q must be a lower-kebab identifier", id)
		}
		if seen[id] {
			return fmt.Errorf("duplicate root id %q", id)
		}
		seen[id] = true
		ownership, _ := root["ownership"].(string)
		if ownership != "catalog" && ownership != "local" {
			return fmt.Errorf("root %s has unknown ownership %q", id, ownership)
		}
		source, _ := root["source"].(string)
		parts := strings.Split(source, "/")
		if strings.Contains(source, `\`) || strings.HasPrefix(source, "/") {
			return fmt.Errorf("root %s source must be a portable relative path", id)
		}
		for _, part := range parts {
			if part == ".." {
				return fmt.Errorf("root %s source contains traversal", id)
			}
		}
		if len(parts) < 3 || parts[0] != "agents" || !applicationProfileFilename(parts[len(parts)-1]) {
			return fmt.Errorf("root %s source must name an agent-profile below agents/<actor>", id)
		}
		release, _ := root["compatible_release"].(string)
		if ownership == "catalog" && !compatibleRelease.MatchString(release) {
			return fmt.Errorf("catalog root %s has invalid compatible_release %q", id, release)
		}
		if ownership == "local" && release != "" {
			return fmt.Errorf("local root %s declares compatible_release", id)
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
// tool declarations and selections
// ---------------------------------------------------------------------------

func toolEntries(doc map[string]any) ([]map[string]any, error) {
	raw, present := doc["tools"]
	if !present {
		return nil, nil
	}
	list, ok := raw.([]any)
	if !ok {
		return nil, errors.New("tools must be a sequence")
	}
	entries := make([]map[string]any, 0, len(list))
	for index, item := range list {
		entry, ok := item.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("tools[%d] must be a mapping", index)
		}
		entries = append(entries, entry)
	}
	return entries, nil
}

// checkToolDeclarationBody enforces R-TOOL-001: a tool-declaration document
// carries a tools body or a types body, never both, and a type unit names its
// unit.
func checkToolDeclarationBody(path string) error {
	doc, err := readYAMLMapping(path)
	if err != nil {
		return err
	}
	_, hasTools := doc["tools"]
	_, hasTypes := doc["types"]
	switch {
	case hasTools && hasTypes:
		return errors.New("a declaration cannot carry both tools and types")
	case !hasTools && !hasTypes:
		return errors.New("a declaration carries neither tools nor types")
	case hasTypes && !nonEmptyString(doc["unit"]):
		return errors.New("a type unit must declare unit")
	}
	if hasTools {
		if _, ok := doc["tools"].([]any); !ok {
			return errors.New("tools must be a sequence")
		}
	}
	return nil
}

// checkToolEntryKind enforces R-TOOL-002: every entry is named and is either
// a builtin naming its factory or an exec entry naming its binary.
func checkToolEntryKind(path string) error {
	doc, err := readYAMLMapping(path)
	if err != nil {
		return err
	}
	entries, err := toolEntries(doc)
	if err != nil {
		return err
	}
	for index, entry := range entries {
		name, _ := entry["name"].(string)
		if strings.TrimSpace(name) == "" {
			return fmt.Errorf("tools[%d] has no name", index)
		}
		kind, _ := entry["type"].(string)
		switch kind {
		case "builtin":
			if !nonEmptyString(entry["init"]) {
				return fmt.Errorf("builtin tool %q has no init", name)
			}
		case "exec", "":
			if !nonEmptyString(entry["binary"]) {
				return fmt.Errorf("tool %q has no binary", name)
			}
		default:
			return fmt.Errorf("tool %q has unknown type %q", name, kind)
		}
	}
	return nil
}

var (
	toolVisibility     = setOf("", "internal", "external")
	toolReversibility  = setOf("", "reversible", "compensatable", "irreversible")
	toolPreconditions  = setOf("", "git_repo")
	execUndoStrategies = setOf("", "noop", "workspace_restore", "compensating_action", "irreversible")
)

// checkToolContractVocabulary enforces R-TOOL-003: visibility, reversibility
// classification, precondition, and an exec entry's undo strategy come from
// their closed vocabularies.
func checkToolContractVocabulary(path string) error {
	doc, err := readYAMLMapping(path)
	if err != nil {
		return err
	}
	entries, err := toolEntries(doc)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		name, _ := entry["name"].(string)
		if visibility, _ := entry["visibility"].(string); !toolVisibility[visibility] {
			return fmt.Errorf("tool %q has unknown visibility %q", name, visibility)
		}
		reversibility, _ := entry["reversibility"].(map[string]any)
		if classification, _ := reversibility["classification"].(string); !toolReversibility[classification] {
			return fmt.Errorf("tool %q has unknown reversibility classification %q", name, classification)
		}
		if precondition, _ := entry["precondition"].(string); !toolPreconditions[precondition] {
			return fmt.Errorf("tool %q has unknown precondition %q", name, precondition)
		}
		undo, _ := entry["undo"].(map[string]any)
		strategy, _ := undo["strategy"].(string)
		if kind, _ := entry["type"].(string); kind != "builtin" && !execUndoStrategies[strategy] {
			return fmt.Errorf("exec tool %q names unsupported undo strategy %q", name, strategy)
		}
	}
	return nil
}

var execParameterExtensions = []string{"source", "flag", "positional", "position"}

// checkToolExecFields enforces R-TOOL-004: exec-only fields stay off builtin
// entries, and on exec entries stdin, output mode, and parameter extensions
// take their declared shapes.
func checkToolExecFields(path string) error {
	doc, err := readYAMLMapping(path)
	if err != nil {
		return err
	}
	entries, err := toolEntries(doc)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if err := checkOneToolExecFields(entry); err != nil {
			return err
		}
	}
	return nil
}

func checkOneToolExecFields(entry map[string]any) error {
	name, _ := entry["name"].(string)
	output, _ := entry["output"].(map[string]any)
	mode, hasMode := output["mode"]
	parameters, _ := entry["parameters"].(map[string]any)
	properties, _ := parameters["properties"].(map[string]any)
	if kind, _ := entry["type"].(string); kind == "builtin" {
		for _, field := range []string{"env", "stdin_source", "stdin_max_bytes"} {
			if _, present := entry[field]; present {
				return fmt.Errorf("builtin tool %q cannot declare %s", name, field)
			}
		}
		if hasMode {
			return fmt.Errorf("builtin tool %q cannot declare output.mode", name)
		}
		for _, parameter := range sortedMapKeys(properties) {
			property, _ := properties[parameter].(map[string]any)
			for _, field := range execParameterExtensions {
				if _, present := property[field]; present {
					return fmt.Errorf("builtin tool %q parameter %q cannot declare exec field %q", name, parameter, field)
				}
			}
		}
		return nil
	}
	source, hasSource := entry["stdin_source"]
	if hasSource {
		text, _ := source.(string)
		if !fromSelector.MatchString(text) {
			return fmt.Errorf("tool %q stdin_source must be a $from(label).path selector", name)
		}
	}
	if limit, present := entry["stdin_max_bytes"]; present {
		bytes, _ := limit.(int)
		if bytes <= 0 || !hasSource {
			return fmt.Errorf("tool %q stdin_max_bytes requires stdin_source and must be positive", name)
		}
	}
	if hasMode && mode != "raw" && mode != "structured" {
		return fmt.Errorf("tool %q output mode %v must be raw or structured", name, mode)
	}
	for _, parameter := range sortedMapKeys(properties) {
		property, _ := properties[parameter].(map[string]any)
		if value, present := property["default"]; present {
			if _, ok := value.(string); !ok {
				return fmt.Errorf("tool %q parameter %q default must be a string", name, parameter)
			}
		}
		if value, present := property["position"]; present {
			if _, ok := value.(int); !ok {
				return fmt.Errorf("tool %q parameter %q position must be an integer", name, parameter)
			}
		}
	}
	return nil
}

// checkToolSignatureSingleSource enforces R-TOOL-005: a signature and the
// legacy contract blocks beside it never state the same thing twice, and
// identical emits lists are the only overlap allowed.
func checkToolSignatureSingleSource(path string) error {
	doc, err := readYAMLMapping(path)
	if err != nil {
		return err
	}
	entries, err := toolEntries(doc)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		signature, ok := entry["signature"].(map[string]any)
		if !ok {
			continue
		}
		name, _ := entry["name"].(string)
		if parameters, _ := entry["parameters"].(map[string]any); nonEmptyString(signature["input"]) && len(parameters) > 0 {
			return fmt.Errorf("tool %q declares both signature.input and parameters", name)
		}
		output, _ := entry["output"].(map[string]any)
		if schema, _ := output["schema"].(map[string]any); nonEmptyString(signature["output"]) && len(schema) > 0 {
			return fmt.Errorf("tool %q declares both signature.output and output.schema", name)
		}
		signed, _ := signature["emits"].([]any)
		legacy, _ := entry["emits"].([]any)
		if len(signed) > 0 && len(legacy) > 0 && !reflect.DeepEqual(signed, legacy) {
			return fmt.Errorf("tool %q declares signature.emits and emits that differ", name)
		}
	}
	return nil
}

var (
	typeSubsetKeywords = setOf("type", "properties", "required", "items", "description", "enum")
	typeSubsetTypes    = setOf("object", "array", "string", "number", "integer", "boolean")
)

// checkToolTypeSubset enforces R-TOOL-006: every declared type is named and
// its schema stays inside the closed declaration type subset at every depth.
func checkToolTypeSubset(path string) error {
	doc, err := readYAMLMapping(path)
	if err != nil {
		return err
	}
	raw, present := doc["types"]
	if !present {
		return nil
	}
	types, ok := raw.([]any)
	if !ok {
		return errors.New("types must be a sequence")
	}
	for index, item := range types {
		declared, _ := item.(map[string]any)
		if !nonEmptyString(declared["name"]) {
			return fmt.Errorf("types[%d] has no name", index)
		}
		schema, _ := declared["schema"].(map[string]any)
		if err := checkTypeSchema(schema, fmt.Sprintf("types[%d].schema", index)); err != nil {
			return err
		}
	}
	return nil
}

func checkTypeSchema(schema map[string]any, at string) error {
	if schema == nil {
		return nil
	}
	if _, isRef := schema["$type"]; isRef {
		if len(schema) != 1 {
			return fmt.Errorf("%s: a $type reference carries a sibling keyword", at)
		}
		return nil
	}
	for _, keyword := range sortedMapKeys(schema) {
		if !typeSubsetKeywords[keyword] {
			return fmt.Errorf("%s: keyword %q is outside the declaration type subset", at, keyword)
		}
	}
	if value, present := schema["type"]; present {
		name, _ := value.(string)
		if !typeSubsetTypes[name] {
			return fmt.Errorf("%s: type %v is outside the declaration type subset", at, value)
		}
	}
	properties, _ := schema["properties"].(map[string]any)
	for _, name := range sortedMapKeys(properties) {
		nested, _ := properties[name].(map[string]any)
		if err := checkTypeSchema(nested, at+".properties."+name); err != nil {
			return err
		}
	}
	items, _ := schema["items"].(map[string]any)
	return checkTypeSchema(items, at+".items")
}

// checkToolFragmentShape enforces R-TOOL-007: a fragment names its unit,
// declares a body for its params to fill, and does not itself expand; a
// document that is not a fragment carries no $param reference.
func checkToolFragmentShape(path string) error {
	doc, err := readYAMLMapping(path)
	if err != nil {
		return err
	}
	_, hasParams := doc["params"]
	_, hasTools := doc["tools"]
	_, hasTypes := doc["types"]
	if !hasParams {
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if strings.Contains(stripYAMLComments(string(data)), "$param(") {
			return errors.New("the document references $param but declares no params")
		}
		return nil
	}
	if !hasTools && !hasTypes {
		return errors.New("a fragment declares params but no tools or types body")
	}
	if !nonEmptyString(doc["unit"]) {
		return errors.New("a fragment must declare unit")
	}
	if _, expands := doc["expand"]; expands {
		return errors.New("a fragment expands another fragment")
	}
	return nil
}

// stripYAMLComments drops whole-line comments, so a fixture header that
// mentions $param does not read as a reference.
func stripYAMLComments(text string) string {
	var kept []string
	for _, line := range strings.Split(text, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "#") {
			continue
		}
		kept = append(kept, line)
	}
	return strings.Join(kept, "\n")
}

// checkToolSelectionNames enforces R-TOOL-009: a tool selection carries only
// the tools keyname, a sequence of tool names.
func checkToolSelectionNames(path string) error {
	doc, err := readYAMLMapping(path)
	if err != nil {
		return err
	}
	for _, keyname := range sortedMapKeys(doc) {
		if keyname != "tools" {
			return fmt.Errorf("keyname %q is outside the tool selection schema", keyname)
		}
	}
	if !stringSequence(doc["tools"]) {
		return errors.New("tools must be a sequence of tool names")
	}
	return nil
}

// ---------------------------------------------------------------------------
// REST definitions
// ---------------------------------------------------------------------------

var restDefinitionKeynames = setOf("version", "clients", "servers", "openapi", "auth", "limits",
	"retry_policies", "response_mappings")

func restRoot(path string) (map[string]any, error) {
	doc, err := readYAMLMapping(path)
	if err != nil {
		return nil, err
	}
	rest, ok := mappingAt(doc, "rest")
	if !ok {
		return nil, errors.New(`keyname "rest" is absent or not a mapping`)
	}
	return rest, nil
}

// restOperation is one outbound operation and whether its resource verb
// already marks it as mutating.
type restOperation struct {
	name     string
	body     map[string]any
	resource bool
	verb     string
}

func restOperations(rest map[string]any) []restOperation {
	clients, _ := rest["clients"].(map[string]any)
	var operations []restOperation
	for _, clientName := range sortedMapKeys(clients) {
		client, _ := clients[clientName].(map[string]any)
		standalone, _ := client["operations"].(map[string]any)
		for _, name := range sortedMapKeys(standalone) {
			body, _ := standalone[name].(map[string]any)
			operations = append(operations, restOperation{name: clientName + "." + name, body: body})
		}
		resources, _ := client["resources"].(map[string]any)
		for _, resourceName := range sortedMapKeys(resources) {
			resource, _ := resources[resourceName].(map[string]any)
			verbs, _ := resource["operations"].(map[string]any)
			for _, verb := range sortedMapKeys(verbs) {
				body, _ := verbs[verb].(map[string]any)
				operations = append(operations, restOperation{
					name: clientName + "." + resourceName + "." + verb, body: body, resource: true, verb: verb,
				})
			}
		}
	}
	return operations
}

// checkRESTDefinitionRoot enforces R-REST-001: one rest mapping that names its
// format version, carries only the definition families, and does not carry
// the reserved document_resources family.
func checkRESTDefinitionRoot(path string) error {
	rest, err := restRoot(path)
	if err != nil {
		return err
	}
	if !nonEmptyString(rest["version"]) {
		return errors.New("rest.version is required")
	}
	for _, keyname := range sortedMapKeys(rest) {
		if keyname == "document_resources" {
			return errors.New("rest.document_resources is a reserved target-format keyname")
		}
		if !restDefinitionKeynames[keyname] {
			return fmt.Errorf("rest.%s is outside the REST definition schema", keyname)
		}
	}
	return nil
}

var restResourceVerbs = setOf("get", "set", "create", "delete")

// checkRESTResourceOperations enforces R-REST-003: a resource's operation map
// uses only the resource verbs and carries no reserved metadata keyname.
func checkRESTResourceOperations(path string) error {
	rest, err := restRoot(path)
	if err != nil {
		return err
	}
	clients, _ := rest["clients"].(map[string]any)
	for _, clientName := range sortedMapKeys(clients) {
		client, _ := clients[clientName].(map[string]any)
		resources, _ := client["resources"].(map[string]any)
		for _, resourceName := range sortedMapKeys(resources) {
			resource, _ := resources[resourceName].(map[string]any)
			for _, reserved := range []string{"id_field", "version_field"} {
				if _, present := resource[reserved]; present {
					return fmt.Errorf("resource %s.%s carries reserved %s", clientName, resourceName, reserved)
				}
			}
			verbs, _ := resource["operations"].(map[string]any)
			for _, verb := range sortedMapKeys(verbs) {
				if !restResourceVerbs[verb] {
					return fmt.Errorf("resource %s.%s uses unsupported operation %q", clientName, resourceName, verb)
				}
			}
		}
	}
	return nil
}

func statusCodes(mapping any) []int {
	entry, _ := mapping.(map[string]any)
	list, _ := entry["status"].([]any)
	codes := make([]int, 0, len(list))
	for _, item := range list {
		if code, ok := item.(int); ok {
			codes = append(codes, code)
		}
	}
	return codes
}

// checkRESTStatusOwners enforces R-REST-004: within one operation each HTTP
// status selects exactly one of its success and failure mappings.
func checkRESTStatusOwners(path string) error {
	rest, err := restRoot(path)
	if err != nil {
		return err
	}
	for _, operation := range restOperations(rest) {
		mappings := []any{operation.body["success"]}
		failures, _ := operation.body["failures"].([]any)
		mappings = append(mappings, failures...)
		owners := map[int]bool{}
		for _, mapping := range mappings {
			for _, code := range statusCodes(mapping) {
				if owners[code] {
					return fmt.Errorf("operation %s maps HTTP status %d more than once", operation.name, code)
				}
				owners[code] = true
			}
		}
	}
	return nil
}

func restMutates(operation restOperation) bool {
	if operation.resource && (operation.verb == "set" || operation.verb == "create" || operation.verb == "delete") {
		return true
	}
	method, _ := operation.body["method"].(string)
	switch strings.ToUpper(method) {
	case "POST", "PUT", "PATCH", "DELETE":
		return true
	}
	return false
}

// checkRESTMutatingContract enforces R-REST-005: a mutating operation
// declares its side effects and reversibility; an irreversible one requires
// confirmation and a compensatable one names its compensating operation.
func checkRESTMutatingContract(path string) error {
	rest, err := restRoot(path)
	if err != nil {
		return err
	}
	for _, operation := range restOperations(rest) {
		if !restMutates(operation) {
			continue
		}
		if effects, _ := operation.body["side_effects"].([]any); len(effects) == 0 {
			return fmt.Errorf("operation %s mutates state without side_effects", operation.name)
		}
		reversibility, _ := operation.body["reversibility"].(map[string]any)
		classification, _ := reversibility["classification"].(string)
		switch classification {
		case "":
			return fmt.Errorf("operation %s mutates state without reversibility", operation.name)
		case "irreversible":
			if confirmed, _ := reversibility["requires_confirmation"].(bool); !confirmed {
				return fmt.Errorf("operation %s is irreversible without confirmation", operation.name)
			}
		case "compensatable":
			compensation, _ := operation.body["compensation"].(map[string]any)
			if !nonEmptyString(compensation["operation"]) {
				return fmt.Errorf("operation %s is compensatable without a compensation operation", operation.name)
			}
		}
	}
	return nil
}

var (
	restAuthTypes     = setOf("none", "basic", "bearer", "header_token", "query_token")
	restRedirectModes = setOf("", "none", "same_host", "allowlist")
)

// checkRESTPolicyVocabulary enforces R-REST-006: auth profile types and
// redirect modes come from their closed vocabularies.
func checkRESTPolicyVocabulary(path string) error {
	rest, err := restRoot(path)
	if err != nil {
		return err
	}
	auth, _ := rest["auth"].(map[string]any)
	for _, name := range sortedMapKeys(auth) {
		profile, _ := auth[name].(map[string]any)
		if kind, _ := profile["type"].(string); !restAuthTypes[kind] {
			return fmt.Errorf("auth profile %q has unsupported type %q", name, kind)
		}
	}
	limits, _ := rest["limits"].(map[string]any)
	for _, name := range sortedMapKeys(limits) {
		profile, _ := limits[name].(map[string]any)
		redirect, _ := profile["redirect"].(map[string]any)
		if mode, _ := redirect["mode"].(string); !restRedirectModes[mode] {
			return fmt.Errorf("limit profile %q has unsupported redirect mode %q", name, mode)
		}
	}
	return nil
}

var restBindings = setOf("emit_signal", "emit_dynamic_signal", "lifecycle_control", "read_state",
	"invoke_handler", "stream_events", "health", "static_metadata", "machine_request", "signal_source",
	"static_assets", "redirect", "monitor_proxy", "mock", "mock_log")

func restEndpoints(rest map[string]any, visit func(server string, body map[string]any, endpoint string, entry map[string]any) error) error {
	servers, _ := rest["servers"].(map[string]any)
	for _, serverName := range sortedMapKeys(servers) {
		server, _ := servers[serverName].(map[string]any)
		endpoints, _ := server["endpoints"].(map[string]any)
		if len(endpoints) == 0 {
			if err := visit(serverName, server, "", nil); err != nil {
				return err
			}
		}
		for _, endpointName := range sortedMapKeys(endpoints) {
			entry, _ := endpoints[endpointName].(map[string]any)
			if err := visit(serverName, server, endpointName, entry); err != nil {
				return err
			}
		}
	}
	return nil
}

// publicListenerAddress mirrors the loader's reading: an empty host or a
// wildcard host binds every interface.
func publicListenerAddress(address string) bool {
	host := address
	if index := strings.LastIndex(address, ":"); index >= 0 && !strings.HasSuffix(address, "]") {
		host = address[:index]
	}
	return host == "" || host == "0.0.0.0" || host == "::" || host == "[::]"
}

// checkRESTServerEndpoints enforces R-REST-007: every endpoint names a binding
// the runtime handles, an emit_dynamic_signal endpoint carries its
// allowlist, and a server binding every interface references a limit profile
// that allows a public listener.
func checkRESTServerEndpoints(path string) error {
	rest, err := restRoot(path)
	if err != nil {
		return err
	}
	limits, _ := rest["limits"].(map[string]any)
	checked := map[string]bool{}
	return restEndpoints(rest, func(serverName string, server map[string]any, endpointName string, endpoint map[string]any) error {
		if !checked[serverName] {
			checked[serverName] = true
			address, _ := server["address"].(string)
			if publicListenerAddress(address) {
				ref, _ := server["limits_ref"].(string)
				limit, _ := limits[ref].(map[string]any)
				network, _ := limit["network"].(map[string]any)
				if allowed, _ := network["allow_public_listener"].(bool); !allowed {
					return fmt.Errorf("server %q binds public address without allow_public_listener", serverName)
				}
			}
		}
		if endpoint == nil {
			return nil
		}
		binding, _ := endpoint["binding"].(string)
		if !restBindings[binding] {
			return fmt.Errorf("endpoint %q has no handled binding (got %q)", endpointName, binding)
		}
		if binding == "emit_dynamic_signal" {
			if allowed, _ := endpoint["allowed_signals"].([]any); len(allowed) == 0 {
				return fmt.Errorf("endpoint %q emit_dynamic_signal requires allowed_signals", endpointName)
			}
		}
		return nil
	})
}

// checkRESTMachineRequest enforces R-REST-008: a machine_request endpoint names
// what it runs, bounds the run with a timeout, maps terminal outcomes to
// responses, and carries no reserved document_resources.
func checkRESTMachineRequest(path string) error {
	rest, err := restRoot(path)
	if err != nil {
		return err
	}
	return restEndpoints(rest, func(_ string, _ map[string]any, endpointName string, endpoint map[string]any) error {
		if binding, _ := endpoint["binding"].(string); binding != "machine_request" {
			return nil
		}
		request, _ := endpoint["machine_request"].(map[string]any)
		if !nonEmptyString(request["profile"]) && !nonEmptyString(request["machine"]) {
			return fmt.Errorf("endpoint %q machine_request names neither profile nor machine", endpointName)
		}
		if !nonEmptyString(request["timeout"]) {
			return fmt.Errorf("endpoint %q machine_request requires timeout", endpointName)
		}
		response, _ := request["response"].(map[string]any)
		states, _ := response["terminal_states"].(map[string]any)
		signals, _ := response["terminal_signals"].(map[string]any)
		if len(states) == 0 && len(signals) == 0 {
			return fmt.Errorf("endpoint %q machine_request requires terminal_states or terminal_signals", endpointName)
		}
		if _, reserved := request["document_resources"]; reserved {
			return fmt.Errorf("endpoint %q machine_request.document_resources is reserved", endpointName)
		}
		return nil
	})
}

// ---------------------------------------------------------------------------
// inference configuration and chat dialects
// ---------------------------------------------------------------------------

// checkLLMInvokeConfig enforces R-LLM-001: an invoke_llm entry's config names
// its model and manifest state, and names a provider or a dialect, not both.
func checkLLMInvokeConfig(path string) error {
	doc, err := readYAMLMapping(path)
	if err != nil {
		return err
	}
	entries, err := toolEntries(doc)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if init, _ := entry["init"].(string); init != "invoke_llm" {
			continue
		}
		name, _ := entry["name"].(string)
		config, _ := entry["config"].(map[string]any)
		if !nonEmptyString(config["model"]) {
			return fmt.Errorf("invoke_llm tool %q config requires model", name)
		}
		if !nonEmptyString(config["manifest_state"]) {
			return fmt.Errorf("invoke_llm tool %q config requires manifest_state", name)
		}
		if nonEmptyString(config["provider"]) && nonEmptyString(config["dialect"]) {
			return fmt.Errorf("invoke_llm tool %q config names both provider and dialect", name)
		}
	}
	return nil
}

// checkLLMDialectHeader enforces R-LLM-002: a chat dialect names a kebab unit
// and its provider, a rooted path called by POST, and an auth type.
func checkLLMDialectHeader(path string) error {
	doc, err := readYAMLMapping(path)
	if err != nil {
		return err
	}
	if unit, _ := doc["unit"].(string); !kebabName.MatchString(unit) {
		return fmt.Errorf("unit %q is not a lowercase kebab name", unit)
	}
	if !nonEmptyString(doc["provider_name"]) {
		return errors.New("provider_name is required")
	}
	if route, _ := doc["path"].(string); !strings.HasPrefix(route, "/") {
		return fmt.Errorf("path %q must start with /", route)
	}
	if method, present := doc["method"]; present && method != "POST" {
		return fmt.Errorf("method %v is not POST", method)
	}
	auth, _ := doc["auth"].(map[string]any)
	if !nonEmptyString(auth["type"]) {
		return errors.New("auth.type is required")
	}
	return nil
}

var dialectParams = setOf("messages", "model", "temperature", "seed", "num_ctx")

// checkLLMDialectBody enforces R-LLM-003: the body references only the params
// invoke_llm supplies and places the conversation exactly once, as a whole
// scalar.
func checkLLMDialectBody(path string) error {
	doc, err := readYAMLMapping(path)
	if err != nil {
		return err
	}
	body, _ := doc["body"].(map[string]any)
	if len(body) == 0 {
		return errors.New("body is required")
	}
	placed := 0
	var walk func(value any) error
	walk = func(value any) error {
		switch typed := value.(type) {
		case map[string]any:
			for _, key := range sortedMapKeys(typed) {
				if err := walk(typed[key]); err != nil {
					return err
				}
			}
		case []any:
			for _, item := range typed {
				if err := walk(item); err != nil {
					return err
				}
			}
		case string:
			matches := paramsReference.FindAllStringSubmatch(typed, -1)
			if strings.Contains(typed, "{{") && len(matches) == 0 {
				return fmt.Errorf("body value %q is not a {{ params.<name> }} reference", typed)
			}
			for _, match := range matches {
				if !dialectParams[match[1]] {
					return fmt.Errorf("body references params.%s, which invoke_llm does not supply", match[1])
				}
				if match[1] == "messages" {
					if typed != match[0] {
						return fmt.Errorf("body places params.messages inside %q", typed)
					}
					placed++
				}
			}
		}
		return nil
	}
	if err := walk(body); err != nil {
		return err
	}
	if placed != 1 {
		return fmt.Errorf("body places params.messages %d times", placed)
	}
	return nil
}

var dialectFailureSignals = setOf("ProviderUnauthorized", "ProviderThrottled", "ProviderUnavailable")

// checkLLMDialectReply enforces R-LLM-004: the reply text and token counts are
// $. selectors, and the failure map sends each error status, once, to one of
// the three provider failure signals.
func checkLLMDialectReply(path string) error {
	doc, err := readYAMLMapping(path)
	if err != nil {
		return err
	}
	response, _ := doc["response"].(map[string]any)
	text, _ := response["text"].(string)
	if !dollarSelector.MatchString(text) {
		return fmt.Errorf("response.text %q is not a $. selector", text)
	}
	for _, field := range []string{"input_tokens", "output_tokens"} {
		if value, present := response[field]; present {
			selector, _ := value.(string)
			if !dollarSelector.MatchString(selector) {
				return fmt.Errorf("response.%s %q is not a $. selector", field, selector)
			}
		}
	}
	failures, _ := doc["failures"].([]any)
	if len(failures) == 0 {
		return errors.New("failures is required")
	}
	owners := map[int]bool{}
	for index, raw := range failures {
		failure, _ := raw.(map[string]any)
		if signal, _ := failure["signal"].(string); !dialectFailureSignals[signal] {
			return fmt.Errorf("failures[%d] signal %q is not a provider failure signal", index, signal)
		}
		codes := statusCodes(failure)
		if len(codes) == 0 {
			return fmt.Errorf("failures[%d] names no status", index)
		}
		for _, code := range codes {
			if code < 400 || code > 599 {
				return fmt.Errorf("failures[%d] status %d is not an error status", index, code)
			}
			if owners[code] {
				return fmt.Errorf("failures map HTTP status %d more than once", code)
			}
			owners[code] = true
		}
	}
	return nil
}

// checkLLMDialectAuth enforces R-LLM-005: the dialect names credentials by
// environment-variable reference and carries exactly the references its auth
// type needs.
func checkLLMDialectAuth(path string) error {
	doc, err := readYAMLMapping(path)
	if err != nil {
		return err
	}
	auth, _ := doc["auth"].(map[string]any)
	kind, _ := auth["type"].(string)
	if !restAuthTypes[kind] {
		return fmt.Errorf("auth.type %q is not a REST auth type", kind)
	}
	refs := map[string]string{}
	for _, field := range []string{"token_ref", "username_ref", "password_ref"} {
		if value, present := auth[field]; present {
			ref, _ := value.(string)
			if !credentialRef.MatchString(ref) {
				return fmt.Errorf("auth.%s %q is not an environment variable name", field, ref)
			}
			refs[field] = ref
		}
	}
	switch kind {
	case "none":
		if len(refs) > 0 {
			return errors.New("auth type none names a credential reference")
		}
	case "basic":
		if refs["username_ref"] == "" || refs["password_ref"] == "" {
			return errors.New("auth type basic requires username_ref and password_ref")
		}
	default:
		if refs["token_ref"] == "" {
			return fmt.Errorf("auth type %s requires token_ref", kind)
		}
	}
	return nil
}
