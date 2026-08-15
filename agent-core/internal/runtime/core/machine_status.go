// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package core

func validRunStatus(status RunStatus) bool {
	switch status {
	case StatusSucceeded, StatusFailed, StatusBudgetExceeded, StatusCancelled, StatusSuspended:
		return true
	default:
		return false
	}
}
