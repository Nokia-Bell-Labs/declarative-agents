// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package core

import "sync"

// RunOwnership provides non-queueing, process-local single-dispatch ownership.
// A failed TryAcquire returns immediately; callers must not wait or retry
// implicitly.
type RunOwnership struct {
	mu     sync.Mutex
	next   uint64
	owners map[string]uint64
}

// TryAcquire takes ownership for runID and returns an idempotent release
// function. The token prevents a stale release from clearing a newer owner.
func (o *RunOwnership) TryAcquire(runID string) (release func(), acquired bool) {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.owners == nil {
		o.owners = make(map[string]uint64)
	}
	if _, held := o.owners[runID]; held {
		return func() {}, false
	}
	o.next++
	token := o.next
	o.owners[runID] = token
	var once sync.Once
	return func() {
		once.Do(func() {
			o.mu.Lock()
			defer o.mu.Unlock()
			if o.owners[runID] == token {
				delete(o.owners, runID)
			}
		})
	}, true
}

// Held reports whether this process currently owns dispatch for runID.
func (o *RunOwnership) Held(runID string) bool {
	o.mu.Lock()
	defer o.mu.Unlock()
	_, held := o.owners[runID]
	return held
}
