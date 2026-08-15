// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package main

import (
	"errors"
	"sync"
	"testing"
)

func TestRunBoundedHonorsLimit(t *testing.T) {
	items := []int{0, 1, 2, 3}
	release := make(chan struct{})
	started := make(chan struct{}, len(items))
	done := make(chan error, 1)
	var mu sync.Mutex
	running, maximum := 0, 0

	go func() {
		done <- runBounded(items, 2, func(int) error {
			mu.Lock()
			running++
			if running > maximum {
				maximum = running
			}
			mu.Unlock()
			started <- struct{}{}
			<-release
			mu.Lock()
			running--
			mu.Unlock()
			return nil
		})
	}()

	<-started
	<-started
	mu.Lock()
	if maximum != 2 {
		t.Fatalf("maximum concurrency = %d, want 2", maximum)
	}
	mu.Unlock()
	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func TestRunBoundedReturnsEarliestInputFailure(t *testing.T) {
	first := errors.New("first")
	second := errors.New("second")
	err := runBounded([]int{0, 1}, 2, func(index int) error {
		if index == 0 {
			return first
		}
		return second
	})
	if !errors.Is(err, first) {
		t.Fatalf("error = %v, want earliest input failure %v", err, first)
	}
}

func TestRunBoundedStopsStartingQueuedWorkAfterFailure(t *testing.T) {
	want := errors.New("failed")
	var calls []int
	err := runBounded([]int{0, 1, 2}, 1, func(index int) error {
		calls = append(calls, index)
		if index == 1 {
			return want
		}
		return nil
	})
	if !errors.Is(err, want) {
		t.Fatalf("error = %v, want %v", err, want)
	}
	if len(calls) != 2 {
		t.Fatalf("calls = %v, want queued work to stop after failure", calls)
	}
}
