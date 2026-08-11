// Copyright (c) 2026 Nokia. All rights reserved.

package main

// boundedResult retains input order when several concurrent tasks fail.
type boundedResult struct {
	index int
	err   error
}

// runBounded overlaps independent work up to limit. After observing a failure
// it starts no additional work, waits for already-running tasks, and returns
// the failure belonging to the earliest input item.
func runBounded[T any](items []T, limit int, run func(T) error) error {
	if limit < 1 {
		limit = 1
	}
	if limit > len(items) {
		limit = len(items)
	}
	if len(items) == 0 {
		return nil
	}

	results := make(chan boundedResult, limit)
	next, active := 0, 0
	launch := func(index int) {
		active++
		go func() {
			results <- boundedResult{index: index, err: run(items[index])}
		}()
	}
	for next < len(items) && active < limit {
		launch(next)
		next++
	}

	firstFailure := boundedResult{index: len(items)}
	for active > 0 {
		result := <-results
		active--
		if result.err != nil && result.index < firstFailure.index {
			firstFailure = result
		}
		if firstFailure.err == nil && next < len(items) {
			launch(next)
			next++
		}
	}
	return firstFailure.err
}
