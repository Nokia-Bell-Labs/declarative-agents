// Copyright (c) 2026 Nokia. All rights reserved.

package core

func validRunStatus(status RunStatus) bool {
	switch status {
	case StatusSucceeded, StatusFailed, StatusBudgetExceeded, StatusCancelled, StatusSuspended:
		return true
	default:
		return false
	}
}
