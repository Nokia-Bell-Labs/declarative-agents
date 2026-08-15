// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package core

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
)

func (d *DoltCheckpoint) ConversationReference() (string, bool) {
	d.refMu.RLock()
	defer d.refMu.RUnlock()
	return d.currentConversationRef, d.currentConversationRef != ""
}

func (d *DoltCheckpoint) ResolveConversationSnapshot(reference string) (json.RawMessage, error) {
	snapshot, err := d.resolveSnapshot(
		reference,
		"conversation",
		ErrConversationReferenceInvalid,
		ErrConversationReferenceUnavailable,
	)
	return json.RawMessage(snapshot), err
}

func (d *DoltCheckpoint) DomainReference() (string, bool) {
	d.refMu.RLock()
	defer d.refMu.RUnlock()
	return d.currentDomainRef, d.currentDomainRef != ""
}

func (d *DoltCheckpoint) ResolveDomainSnapshot(reference string) ([]byte, error) {
	return d.resolveSnapshot(
		reference,
		"domain",
		ErrDomainReferenceInvalid,
		ErrDomainReferenceUnavailable,
	)
}

func (d *DoltCheckpoint) resolveSnapshot(
	reference, column string,
	invalid, unavailable error,
) ([]byte, error) {
	parsed, err := parseCheckpointReference(reference)
	if err != nil || parsed.backend != "dolt" || parsed.runID != d.runID {
		return nil, fmt.Errorf("%w: dolt checkpoint run %q", invalid, d.runID)
	}
	if err := verifyDoltReference(d.db, parsed); err != nil {
		if errors.Is(err, ErrConversationReferenceInvalid) {
			return nil, fmt.Errorf("%w: dolt checkpoint run %q step %d",
				invalid, parsed.runID, parsed.step)
		}
		return nil, err
	}
	snapshot, err := loadMachineSnapshotAtRevision(d.db, parsed.runID, parsed.revision, column)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("%w: dolt checkpoint run %q step %d",
			unavailable, parsed.runID, parsed.step)
	}
	if err != nil {
		return nil, fmt.Errorf("%w: resolve %s snapshot run %q step %d: %v",
			ErrDolt, column, parsed.runID, parsed.step, err)
	}
	return snapshot, nil
}

func (d *DoltCheckpoint) savedSnapshotReferences(
	position Position,
	step int,
	terminal bool,
	revision string,
) (string, string, error) {
	if terminal || step < 0 ||
		(len(position.Snapshot.Conversation) == 0 && len(position.Snapshot.Domain) == 0) {
		return "", "", nil
	}
	ref, err := formatCheckpointReference("dolt", d.runID, step, revision)
	if err != nil {
		return "", "", fmt.Errorf("%w: save: checkpoint reference: %v", ErrDolt, err)
	}
	var conversationRef, domainRef string
	if len(position.Snapshot.Conversation) > 0 {
		conversationRef = ref
	}
	if len(position.Snapshot.Domain) > 0 {
		domainRef = ref
	}
	return conversationRef, domainRef, nil
}

func (d *DoltCheckpoint) validateSnapshotReferences(
	position Position,
	step int,
	terminal bool,
) error {
	if terminal || step < 0 ||
		(len(position.Snapshot.Conversation) == 0 && len(position.Snapshot.Domain) == 0) {
		return nil
	}
	if !validReferencePart(d.runID) {
		return fmt.Errorf("%w: save: checkpoint reference run", ErrDolt)
	}
	return nil
}

func (d *DoltCheckpoint) refreshSnapshotReferences(position Position, execution Execution) error {
	d.setSnapshotReferences("", "")
	if len(execution) == 0 ||
		(len(position.Snapshot.Conversation) == 0 && len(position.Snapshot.Domain) == 0) {
		return nil
	}
	step := len(execution) - 1
	revision, err := headRevision(d.db)
	if err != nil {
		return fmt.Errorf("%w: load: checkpoint reference HEAD: %v", ErrDolt, err)
	}
	ref, err := formatCheckpointReference("dolt", d.runID, step, revision)
	if err != nil {
		return fmt.Errorf("%w: load: checkpoint reference: %v", ErrDolt, err)
	}
	var conversationRef, domainRef string
	if len(position.Snapshot.Conversation) > 0 {
		conversationRef = ref
	}
	if len(position.Snapshot.Domain) > 0 {
		domainRef = ref
	}
	d.setSnapshotReferences(conversationRef, domainRef)
	return nil
}

func (d *DoltCheckpoint) setConversationReference(reference string) {
	d.refMu.Lock()
	defer d.refMu.Unlock()
	d.currentConversationRef = reference
}

func (d *DoltCheckpoint) setDomainReference(reference string) {
	d.refMu.Lock()
	defer d.refMu.Unlock()
	d.currentDomainRef = reference
}

func (d *DoltCheckpoint) setSnapshotReferences(conversation, domain string) {
	d.refMu.Lock()
	defer d.refMu.Unlock()
	d.currentConversationRef = conversation
	d.currentDomainRef = domain
}

func commitDoltTransaction(tx Transaction, message string) (string, error) {
	var revision string
	err := tx.QueryRow(
		`CALL DOLT_COMMIT('-A', '--allow-empty', '-m', ?)`,
		message,
	).Scan(&revision)
	return revision, err
}

func headRevision(db Database) (string, error) {
	var revision string
	err := db.QueryRow(`SELECT HASHOF('HEAD')`).Scan(&revision)
	return revision, err
}

func verifyDoltReference(db Database, reference checkpointReference) error {
	asOf, err := renderDoltASOfRevision(reference.revision)
	if err != nil {
		return invalidDoltReference(reference, nil)
	}
	message, err := loadCommitMessageAtRevision(db, reference.revision)
	if err != nil {
		return invalidDoltReference(reference, err)
	}
	signal, err := loadTransitionSignalAtRevision(db, reference)
	if err != nil {
		return invalidDoltReference(reference, err)
	}
	if message != commitMessage(reference.step, Signal(signal)) {
		return invalidDoltReference(reference, nil)
	}
	var count int
	err = db.QueryRow(
		fmt.Sprintf(
			`SELECT COUNT(*) FROM execution_steps AS OF %s WHERE run_id = ? AND step_index = ?`,
			asOf,
		),
		reference.runID, reference.step,
	).Scan(&count)
	if err != nil {
		return fmt.Errorf("%w: resolve checkpoint identity: %v", ErrDolt, err)
	}
	if count != 1 {
		return fmt.Errorf("%w: dolt checkpoint run %q step %d",
			ErrConversationReferenceInvalid, reference.runID, reference.step)
	}
	return nil
}

func invalidDoltReference(reference checkpointReference, cause error) error {
	if cause != nil && !errors.Is(cause, sql.ErrNoRows) {
		return fmt.Errorf("%w: resolve checkpoint identity: %v", ErrDolt, cause)
	}
	return fmt.Errorf("%w: dolt checkpoint run %q step %d",
		ErrConversationReferenceInvalid, reference.runID, reference.step)
}

func loadCommitMessageAtRevision(db Database, revision string) (string, error) {
	var message string
	err := db.QueryRow(
		`SELECT message FROM dolt_log WHERE commit_hash = ? LIMIT 1`,
		revision,
	).Scan(&message)
	return message, err
}

func loadTransitionSignalAtRevision(db Database, reference checkpointReference) (string, error) {
	asOf, err := renderDoltASOfRevision(reference.revision)
	if err != nil {
		return "", err
	}
	var signal string
	err = db.QueryRow(
		fmt.Sprintf(
			"SELECT `signal` FROM transitions AS OF %s WHERE run_id = ? AND step_index = ?",
			asOf,
		),
		reference.runID, reference.step,
	).Scan(&signal)
	return signal, err
}

func (d *DoltCheckpoint) setRevertedSnapshotReferences(runID string, step int, revision string) {
	d.setSnapshotReferences("", "")
	if runID != d.runID {
		return
	}
	ref, err := formatCheckpointReference("dolt", runID, step, revision)
	if err != nil {
		return
	}
	if _, err := d.ResolveConversationSnapshot(ref); err == nil {
		d.setConversationReference(ref)
	}
	if _, err := d.ResolveDomainSnapshot(ref); err == nil {
		d.setDomainReference(ref)
	}
}

func loadMachineSnapshotAtRevision(
	db Database,
	runID, revision, column string,
) ([]byte, error) {
	if column != "conversation" && column != "domain" {
		return nil, ErrConversationReferenceInvalid
	}
	asOf, err := renderDoltASOfRevision(revision)
	if err != nil {
		return nil, err
	}
	var snapshot sql.NullString
	err = db.QueryRow(
		fmt.Sprintf(`SELECT %s FROM machines AS OF %s WHERE run_id = ?`, column, asOf),
		runID,
	).Scan(&snapshot)
	if err != nil {
		return nil, err
	}
	if !snapshot.Valid || snapshot.String == "" {
		return nil, sql.ErrNoRows
	}
	return []byte(snapshot.String), nil
}

// renderDoltASOfRevision returns a SQL literal only after enforcing Dolt's
// HASHOF/DOLT_COMMIT hash grammar. Dolt 2.x hashes are 32 lowercase base32
// characters (digits plus a-v); no caller-controlled quoting reaches SQL.
func renderDoltASOfRevision(revision string) (string, error) {
	if !validReferenceRevision("dolt", revision) {
		return "", ErrConversationReferenceInvalid
	}
	return "'" + revision + "'", nil
}
