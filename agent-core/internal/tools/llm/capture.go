// Copyright (c) 2026 Nokia. All rights reserved.

package llm

import (
	"fmt"
	"strings"
)

// CaptureLevel controls how much model content is recorded in telemetry.
type CaptureLevel string

const (
	CaptureOff   CaptureLevel = "off"
	CaptureDelta CaptureLevel = "delta"
	CaptureFull  CaptureLevel = "full"
)

// ParseCaptureLevel validates and returns a content-capture level.
func ParseCaptureLevel(value string) (CaptureLevel, error) {
	level := CaptureLevel(strings.TrimSpace(value))
	switch level {
	case CaptureOff, CaptureDelta, CaptureFull:
		return level, nil
	default:
		return "", fmt.Errorf("invalid telemetry capture level %q (want off, delta, or full)", value)
	}
}

// CapturesFullContent reports whether current verbose content attributes should
// be recorded. Delta deliberately remains counts-only until delta attributes
// are implemented.
func (l CaptureLevel) CapturesFullContent() bool {
	return l == CaptureFull
}
