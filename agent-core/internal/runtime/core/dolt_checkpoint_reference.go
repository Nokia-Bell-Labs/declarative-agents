// Copyright (c) 2026 Nokia. All rights reserved.

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
	parsed, err := parseCheckpointReference(reference)
	if err != nil || parsed.backend != "dolt" || parsed.runID != d.runID {
		return nil, fmt.Errorf("%w: dolt checkpoint run %q", ErrConversationReferenceInvalid, d.runID)
	}
	if err := verifyDoltReference(d.db, parsed); err != nil {
		return nil, err
	}
	conversation, err := loadConversationAtRevision(d.db, parsed.runID, parsed.revision)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("%w: dolt checkpoint run %q step %d",
			ErrConversationReferenceUnavailable, parsed.runID, parsed.step)
	}
	if err != nil {
		return nil, fmt.Errorf("%w: resolve conversation run %q step %d: %v",
			ErrDolt, parsed.runID, parsed.step, err)
	}
	return conversation, nil
}

func (d *DoltCheckpoint) savedConversationReference(
	position Position,
	step int,
	terminal bool,
	revision string,
) (string, error) {
	if terminal || step < 0 || len(position.Snapshot.Conversation) == 0 {
		return "", nil
	}
	ref, err := formatCheckpointReference("dolt", d.runID, step, revision)
	if err != nil {
		return "", fmt.Errorf("%w: save: conversation reference: %v", ErrDolt, err)
	}
	return ref, nil
}

func (d *DoltCheckpoint) validateConversationReference(
	position Position,
	step int,
	terminal bool,
) error {
	if terminal || step < 0 || len(position.Snapshot.Conversation) == 0 {
		return nil
	}
	if !validReferencePart(d.runID) {
		return fmt.Errorf("%w: save: conversation reference run", ErrDolt)
	}
	return nil
}

func (d *DoltCheckpoint) refreshConversationReference(position Position, execution Execution) error {
	d.setConversationReference("")
	if len(execution) == 0 || len(position.Snapshot.Conversation) == 0 {
		return nil
	}
	step := len(execution) - 1
	revision, err := headRevision(d.db)
	if err != nil {
		return fmt.Errorf("%w: load: conversation reference HEAD: %v", ErrDolt, err)
	}
	ref, err := formatCheckpointReference("dolt", d.runID, step, revision)
	if err != nil {
		return fmt.Errorf("%w: load: conversation reference: %v", ErrDolt, err)
	}
	d.setConversationReference(ref)
	return nil
}

func (d *DoltCheckpoint) setConversationReference(reference string) {
	d.refMu.Lock()
	defer d.refMu.Unlock()
	d.currentConversationRef = reference
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
		return fmt.Errorf("%w: resolve conversation identity: %v", ErrDolt, err)
	}
	if count != 1 {
		return fmt.Errorf("%w: dolt checkpoint run %q step %d",
			ErrConversationReferenceInvalid, reference.runID, reference.step)
	}
	return nil
}

func invalidDoltReference(reference checkpointReference, cause error) error {
	if cause != nil && !errors.Is(cause, sql.ErrNoRows) {
		return fmt.Errorf("%w: resolve conversation identity: %v", ErrDolt, cause)
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

func (d *DoltCheckpoint) setRevertedConversationReference(runID string, step int, revision string) {
	d.setConversationReference("")
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
}

func loadConversationAtRevision(db Database, runID, revision string) (json.RawMessage, error) {
	asOf, err := renderDoltASOfRevision(revision)
	if err != nil {
		return nil, err
	}
	var conversation sql.NullString
	err = db.QueryRow(
		fmt.Sprintf(`SELECT conversation FROM machines AS OF %s WHERE run_id = ?`, asOf),
		runID,
	).Scan(&conversation)
	if err != nil {
		return nil, err
	}
	if !conversation.Valid || conversation.String == "" {
		return nil, sql.ErrNoRows
	}
	return json.RawMessage(conversation.String), nil
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
