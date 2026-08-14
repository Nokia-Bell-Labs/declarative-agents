// Copyright (c) 2026 Nokia. All rights reserved.

package validation

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/pkg/spec"
)

const (
	specStateCodecVersion = 1
	specReceiptVersion    = 1
	maxSpecReferenceBytes = 512
	maxSpecReceiptBytes   = 1024
)

// DomainReferenceProvider reports the latest authoritative AgentSnapshot.Domain
// reference before a validation command mutates SpecState.
type DomainReferenceProvider interface {
	DomainReference() (string, bool)
}

// DomainSnapshotResolver resolves a receipt reference to the validation-state
// codec embedded in the authoritative AgentSnapshot.Domain value.
type DomainSnapshotResolver interface {
	ResolveValidationSnapshot(reference string) ([]byte, error)
}

type specUndoSupport struct {
	provider    DomainReferenceProvider
	resolver    DomainSnapshotResolver
	snapshot    specSnapshot
	receipt     string
	hasSnapshot bool
}

func validationCommandName(selected, fallback string) string {
	if selected != "" {
		return selected
	}
	return fallback
}

func newSpecUndoSupport(
	provider DomainReferenceProvider,
	resolver DomainSnapshotResolver,
) specUndoSupport {
	return specUndoSupport{provider: provider, resolver: resolver}
}

func (u *specUndoSupport) capture(vs *SpecState) error {
	snapshot, receipt, err := captureSpecUndo(vs, u.provider)
	if err != nil {
		return err
	}
	u.snapshot = snapshot
	u.receipt = receipt
	u.hasSnapshot = true
	return nil
}

func (u *specUndoSupport) restore(commandName string, vs *SpecState, prior core.Result) core.Result {
	return undoSpecState(commandName, vs, prior, u.snapshot, u.hasSnapshot, u.resolver)
}

type specSnapshot struct {
	directory       string
	targetDirectory string
	suitePaths      []string
	corpus          *spec.Corpus
	graph           *spec.Graph
	charters        []spec.Charter
	findings        []spec.Finding
	testInventory   *spec.GoTestInventory
	hasErrors       bool
	corpusOptional  bool
}

func snapshotSpec(vs *SpecState) specSnapshot {
	return specSnapshot{
		directory:       vs.Directory,
		targetDirectory: vs.TargetDirectory,
		suitePaths:      append([]string(nil), vs.SuitePaths...),
		corpus:          vs.Corpus,
		graph:           vs.Graph,
		charters:        append([]spec.Charter(nil), vs.Charters...),
		findings:        append([]spec.Finding(nil), vs.Findings...),
		testInventory:   vs.TestInventory,
		hasErrors:       vs.HasErrors,
		corpusOptional:  vs.CorpusOptional,
	}
}

func (s specSnapshot) restore(vs *SpecState) {
	vs.Directory = s.directory
	vs.TargetDirectory = s.targetDirectory
	vs.SuitePaths = append([]string(nil), s.suitePaths...)
	vs.Corpus = s.corpus
	vs.Graph = s.graph
	vs.Charters = append([]spec.Charter(nil), s.charters...)
	vs.Findings = append([]spec.Finding(nil), s.findings...)
	vs.TestInventory = s.testInventory
	vs.HasErrors = s.hasErrors
	vs.CorpusOptional = s.corpusOptional
}

type specStateEnvelope struct {
	Version int             `json:"version"`
	State   *specStateValue `json:"state"`
}

type specStateValue struct {
	Directory       string                `json:"directory"`
	TargetDirectory string                `json:"target_directory"`
	SuitePaths      []string              `json:"suite_paths"`
	Corpus          *spec.Corpus          `json:"corpus"`
	GraphLoaded     bool                  `json:"graph_loaded"`
	Charters        []spec.Charter        `json:"charters"`
	Findings        []spec.Finding        `json:"findings"`
	TestInventory   *spec.GoTestInventory `json:"test_inventory"`
	HasErrors       bool                  `json:"has_errors"`
	CorpusOptional  bool                  `json:"corpus_optional"`
}

// EncodeSpecState deterministically encodes the complete logical validation
// state stored in AgentSnapshot.Domain. Graph structure is represented by its
// presence plus the authoritative parsed corpus from which it is derived.
func EncodeSpecState(vs *SpecState) (json.RawMessage, error) {
	if vs == nil {
		return nil, fmt.Errorf("encode validation state: nil SpecState")
	}
	snap := snapshotSpec(vs)
	return json.Marshal(specStateEnvelope{
		Version: specStateCodecVersion,
		State: &specStateValue{
			Directory:       snap.directory,
			TargetDirectory: snap.targetDirectory,
			SuitePaths:      snap.suitePaths,
			Corpus:          snap.corpus,
			GraphLoaded:     snap.graph != nil,
			Charters:        snap.charters,
			Findings:        snap.findings,
			TestInventory:   snap.testInventory,
			HasErrors:       snap.hasErrors,
			CorpusOptional:  snap.corpusOptional,
		},
	})
}

// RestoreSpecState restores a detached, validated codec value. A persisted
// graph is rebuilt only from the persisted corpus and never from workspace
// files, so changed files cannot alter rollback state.
func RestoreSpecState(vs *SpecState, data []byte) error {
	if vs == nil {
		return fmt.Errorf("restore validation state: nil SpecState")
	}
	snap, err := decodeSpecState(data)
	if err != nil {
		return err
	}
	snap.restore(vs)
	return nil
}

func decodeSpecState(data []byte) (specSnapshot, error) {
	var envelope specStateEnvelope
	if err := decodeStrictJSON(data, &envelope); err != nil {
		return specSnapshot{}, fmt.Errorf("decode validation state: %w", err)
	}
	if envelope.Version != specStateCodecVersion {
		return specSnapshot{}, fmt.Errorf(
			"decode validation state: unsupported version %d", envelope.Version,
		)
	}
	if envelope.State == nil {
		return specSnapshot{}, fmt.Errorf("decode validation state: missing state")
	}
	value := envelope.State
	var graph *spec.Graph
	if value.GraphLoaded {
		if value.Corpus == nil {
			return specSnapshot{}, fmt.Errorf("decode validation state: graph has no corpus")
		}
		var err error
		graph, err = spec.BuildGraph(value.Corpus)
		if err != nil {
			return specSnapshot{}, fmt.Errorf("decode validation state: rebuild graph: %w", err)
		}
	}
	return specSnapshot{
		directory:       value.Directory,
		targetDirectory: value.TargetDirectory,
		suitePaths:      append([]string(nil), value.SuitePaths...),
		corpus:          value.Corpus,
		graph:           graph,
		charters:        append([]spec.Charter(nil), value.Charters...),
		findings:        append([]spec.Finding(nil), value.Findings...),
		testInventory:   value.TestInventory,
		hasErrors:       value.HasErrors,
		corpusOptional:  value.CorpusOptional,
	}, nil
}

// specReceipt is a bounded reference to the authoritative prior state. The
// digest binds the reference to the exact logical SpecState observed before the
// command, so a valid reference to a different checkpoint is rejected.
type specReceipt struct {
	Version         int    `json:"version"`
	DomainReference string `json:"domain_reference,omitempty"`
	StateSHA256     string `json:"state_sha256"`
}

func captureSpecUndo(
	vs *SpecState,
	provider DomainReferenceProvider,
) (specSnapshot, string, error) {
	snap := snapshotSpec(vs)
	state, err := EncodeSpecState(vs)
	if err != nil {
		return specSnapshot{}, "", err
	}
	var reference string
	if provider != nil {
		reference, _ = provider.DomainReference()
	}
	if len(reference) > maxSpecReferenceBytes {
		return specSnapshot{}, "", fmt.Errorf(
			"validation domain reference exceeds %d bytes", maxSpecReferenceBytes,
		)
	}
	digest := sha256.Sum256(state)
	receipt, err := json.Marshal(specReceipt{
		Version:         specReceiptVersion,
		DomainReference: reference,
		StateSHA256:     hex.EncodeToString(digest[:]),
	})
	if err != nil {
		return specSnapshot{}, "", fmt.Errorf("encode validation receipt: %w", err)
	}
	if len(receipt) > maxSpecReceiptBytes {
		return specSnapshot{}, "", fmt.Errorf(
			"validation receipt exceeds %d bytes", maxSpecReceiptBytes,
		)
	}
	return snap, string(receipt), nil
}

func decodeSpecReceipt(receipt string) (specReceipt, bool, error) {
	if receipt == "" {
		return specReceipt{}, false, nil
	}
	if len(receipt) > maxSpecReceiptBytes {
		return specReceipt{}, false, fmt.Errorf("receipt exceeds %d bytes", maxSpecReceiptBytes)
	}
	var r specReceipt
	if err := decodeStrictJSON([]byte(receipt), &r); err != nil {
		return specReceipt{}, false, err
	}
	if r.Version != specReceiptVersion {
		return specReceipt{}, false, fmt.Errorf("unsupported receipt version %d", r.Version)
	}
	if r.DomainReference == "" {
		return specReceipt{}, false, fmt.Errorf("missing domain reference")
	}
	if len(r.DomainReference) > maxSpecReferenceBytes {
		return specReceipt{}, false, fmt.Errorf("domain reference exceeds %d bytes", maxSpecReferenceBytes)
	}
	digest, err := hex.DecodeString(r.StateSHA256)
	if err != nil || len(digest) != sha256.Size || hex.EncodeToString(digest) != r.StateSHA256 {
		return specReceipt{}, false, fmt.Errorf("invalid state_sha256")
	}
	return r, true, nil
}

func decodeStrictJSON(data []byte, target interface{}) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		if err == nil {
			return fmt.Errorf("multiple JSON values")
		}
		return err
	}
	return nil
}

// undoSpecState reverses a spec-validation step. The live path preserves exact
// pointer identity from the command-local snapshot. A fresh command resolves
// and verifies the bounded reference before publishing detached restored state.
func undoSpecState(
	commandName string,
	vs *SpecState,
	prior core.Result,
	snap specSnapshot,
	ok bool,
	resolver DomainSnapshotResolver,
) core.Result {
	if ok {
		snap.restore(vs)
		return specUndoSuccess(commandName)
	}
	return undoSpecStateFromReceipt(commandName, vs, prior.Receipt, resolver)
}

func undoSpecStateFromReceipt(
	commandName string,
	vs *SpecState,
	receipt string,
	resolver DomainSnapshotResolver,
) core.Result {
	r, present, err := decodeSpecReceipt(receipt)
	if err != nil {
		return specUndoError(commandName, "decode receipt", err)
	}
	if !present {
		return specUndoError(commandName, "", fmt.Errorf("no validation state snapshot recorded"))
	}
	state, err := resolveSpecReceipt(r, resolver)
	if err != nil {
		return specUndoError(commandName, "", err)
	}
	if err := RestoreSpecState(vs, state); err != nil {
		return specUndoError(commandName, "restore domain snapshot", err)
	}
	return specUndoSuccess(commandName)
}

func resolveSpecReceipt(r specReceipt, resolver DomainSnapshotResolver) ([]byte, error) {
	if resolver == nil {
		return nil, fmt.Errorf("validation domain snapshot resolver unavailable")
	}
	state, err := resolver.ResolveValidationSnapshot(r.DomainReference)
	if err != nil {
		return nil, fmt.Errorf("resolve domain reference: %w", err)
	}
	digest := sha256.Sum256(state)
	if hex.EncodeToString(digest[:]) != r.StateSHA256 {
		return nil, fmt.Errorf("domain reference does not match prior validation state")
	}
	return state, nil
}

func specUndoError(commandName, operation string, cause error) core.Result {
	var err error
	if operation == "" {
		err = fmt.Errorf("undo %s: %w", commandName, cause)
	} else {
		err = fmt.Errorf("undo %s: %s: %w", commandName, operation, cause)
	}
	return core.Result{
		Signal: core.CommandError, CommandName: commandName,
		Output: err.Error(), Err: err,
	}
}

func specUndoSuccess(commandName string) core.Result {
	return core.Result{Signal: core.ToolDone, CommandName: commandName, Output: "undo: restored validation state"}
}
