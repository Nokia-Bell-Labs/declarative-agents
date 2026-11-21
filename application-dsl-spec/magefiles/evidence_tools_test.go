// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package main

import (
	"path/filepath"
	"testing"
)

// toolsFixtureCases runs every fixture the profile, application, tool, REST,
// and inference-configuration statements own through the check that owns it.
var toolsFixtureCases = []struct {
	check   func(string) error
	path    string
	invalid bool
}{
	{checkProfileReferenceKeynames, "profile-references/valid.yaml", false},
	{checkProfileReferenceKeynames, "profile-references/empty-tools.yaml", true},
	{checkProfileReferenceKeynames, "profile-references/scalar-reference.yaml", true},
	{checkProfileLibraryRoots, "profile-libraries/valid.yaml", false},
	{checkProfileLibraryRoots, "profile-libraries/invalid.yaml", true},

	{checkApplicationIdentity, "application-identity/valid.yaml", false},
	{checkApplicationIdentity, "application-identity/unknown-ownership.yaml", true},
	{checkApplicationIdentity, "application-identity/unknown-keyname.yaml", true},
	{checkApplicationCapabilities, "application-capabilities/valid.yaml", false},
	{checkApplicationCapabilities, "application-capabilities/evidence-missing.yaml", true},
	{checkApplicationCapabilities, "application-capabilities/no-runnable-module.yaml", true},
	{checkApplicationRoots, "application-roots/valid.yaml", false},
	{checkApplicationRoots, "application-roots/local-compatible-release.yaml", true},
	{checkApplicationRoots, "application-roots/source-outside-agents.yaml", true},

	{checkToolDeclarationBody, "tool-declaration-body/valid.yaml", false},
	{checkToolDeclarationBody, "tool-declaration-body/types-unit.yaml", false},
	{checkToolDeclarationBody, "tool-declaration-body/both-bodies.yaml", true},
	{checkToolDeclarationBody, "tool-declaration-body/no-body.yaml", true},
	{checkToolEntryKind, "tool-entry-kind/valid.yaml", false},
	{checkToolEntryKind, "tool-entry-kind/builtin-without-init.yaml", true},
	{checkToolEntryKind, "tool-entry-kind/unknown-type.yaml", true},
	{checkToolContractVocabulary, "tool-vocabulary/valid.yaml", false},
	{checkToolContractVocabulary, "tool-vocabulary/unknown-visibility.yaml", true},
	{checkToolContractVocabulary, "tool-vocabulary/exec-undo-strategy.yaml", true},
	{checkToolExecFields, "tool-exec-fields/valid.yaml", false},
	{checkToolExecFields, "tool-exec-fields/builtin-env.yaml", true},
	{checkToolExecFields, "tool-exec-fields/invalid-stdin.yaml", true},
	{checkToolSignatureSingleSource, "tool-signature/valid.yaml", false},
	{checkToolSignatureSingleSource, "tool-signature/input-twice.yaml", true},
	{checkToolSignatureSingleSource, "tool-signature/emits-differ.yaml", true},
	{checkToolTypeSubset, "tool-types/valid.yaml", false},
	{checkToolTypeSubset, "tool-types/keyword-outside-subset.yaml", true},
	{checkToolTypeSubset, "tool-types/reference-with-sibling.yaml", true},
	{checkToolFragmentShape, "tool-fragment/valid.yaml", false},
	{checkToolFragmentShape, "tool-fragment/param-without-params.yaml", true},
	{checkToolFragmentShape, "tool-fragment/nested-expansion.yaml", true},
	{checkToolSelectionNames, "tool-selection/valid.yaml", false},
	{checkToolSelectionNames, "tool-selection/declaration-entry.yaml", true},

	{checkRESTDefinitionRoot, "rest-root/valid.yaml", false},
	{checkRESTDefinitionRoot, "rest-root/missing-version.yaml", true},
	{checkRESTDefinitionRoot, "rest-root/document-resources.yaml", true},
	{checkRESTResourceOperations, "rest-resource/valid.yaml", false},
	{checkRESTResourceOperations, "rest-resource/rpc-verb.yaml", true},
	{checkRESTResourceOperations, "rest-resource/reserved-field.yaml", true},
	{checkRESTStatusOwners, "rest-status/valid.yaml", false},
	{checkRESTStatusOwners, "rest-status/overlapping.yaml", true},
	{checkRESTMutatingContract, "rest-mutating/valid.yaml", false},
	{checkRESTMutatingContract, "rest-mutating/missing-reversibility.yaml", true},
	{checkRESTMutatingContract, "rest-mutating/irreversible-unconfirmed.yaml", true},
	{checkRESTPolicyVocabulary, "rest-policy/valid.yaml", false},
	{checkRESTPolicyVocabulary, "rest-policy/unknown-auth.yaml", true},
	{checkRESTPolicyVocabulary, "rest-policy/unknown-redirect.yaml", true},
	{checkRESTServerEndpoints, "rest-endpoint/valid.yaml", false},
	{checkRESTServerEndpoints, "rest-endpoint/unknown-binding.yaml", true},
	{checkRESTServerEndpoints, "rest-endpoint/dynamic-without-allowlist.yaml", true},
	{checkRESTServerEndpoints, "rest-endpoint/public-listener.yaml", true},
	{checkRESTMachineRequest, "rest-machine-request/valid.yaml", false},
	{checkRESTMachineRequest, "rest-machine-request/missing-timeout.yaml", true},
	{checkRESTMachineRequest, "rest-machine-request/no-response-map.yaml", true},

	{checkLLMInvokeConfig, "llm-config/valid.yaml", false},
	{checkLLMInvokeConfig, "llm-config/provider-and-dialect.yaml", true},
	{checkLLMInvokeConfig, "llm-config/missing-model.yaml", true},
	{checkLLMDialectHeader, "llm-dialect-header/valid.yaml", false},
	{checkLLMDialectHeader, "llm-dialect-header/get-method.yaml", true},
	{checkLLMDialectHeader, "llm-dialect-header/relative-path.yaml", true},
	{checkLLMDialectBody, "llm-dialect-body/valid.yaml", false},
	{checkLLMDialectBody, "llm-dialect-body/messages-inside-string.yaml", true},
	{checkLLMDialectBody, "llm-dialect-body/unsupplied-param.yaml", true},
	{checkLLMDialectReply, "llm-dialect-reply/valid.yaml", false},
	{checkLLMDialectReply, "llm-dialect-reply/success-status.yaml", true},
	{checkLLMDialectReply, "llm-dialect-reply/foreign-signal.yaml", true},
	{checkLLMDialectAuth, "llm-dialect-auth/valid.yaml", false},
	{checkLLMDialectAuth, "llm-dialect-auth/credential-literal.yaml", true},
	{checkLLMDialectAuth, "llm-dialect-auth/none-with-credential.yaml", true},
}

func TestToolsFixtureChecksMatchDeclaredVerdicts(t *testing.T) {
	for _, tc := range toolsFixtureCases {
		t.Run(tc.path, func(t *testing.T) {
			err := tc.check(filepath.Join("..", "1.0", "fixtures", "document", filepath.FromSlash(tc.path)))
			if err != nil {
				t.Logf("%s: %v", tc.path, err)
			}
			if tc.invalid && err == nil {
				t.Fatalf("%s declared invalid but passed its check", tc.path)
			}
			if !tc.invalid && err != nil {
				t.Fatalf("%s declared valid but failed its check: %v", tc.path, err)
			}
		})
	}
}

func TestToolsFixtureChecksRejectMissingRootMapping(t *testing.T) {
	path := writeEvidenceFile(t, t.TempDir(), "sequence.yaml", "- a\n")
	for _, check := range []func(string) error{
		checkProfileReferenceKeynames, checkApplicationIdentity, checkToolDeclarationBody,
		checkRESTDefinitionRoot, checkLLMDialectHeader,
	} {
		if err := check(path); err == nil {
			t.Fatal("a check passed a document whose root is not a mapping")
		}
	}
}
