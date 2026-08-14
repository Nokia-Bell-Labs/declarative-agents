// Copyright (c) 2026 Nokia. All rights reserved.

package llm

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseCaptureLevel(t *testing.T) {
	t.Parallel()
	for _, want := range []CaptureLevel{CaptureOff, CaptureDelta, CaptureFull} {
		want := want
		t.Run(string(want), func(t *testing.T) {
			t.Parallel()
			got, err := ParseCaptureLevel(string(want))
			require.NoError(t, err)
			require.Equal(t, want, got)
		})
	}
}

func TestParseCaptureLevelRejectsInvalidValue(t *testing.T) {
	t.Parallel()
	_, err := ParseCaptureLevel("verbose")
	require.ErrorContains(t, err, "want off, delta, or full")
}

func TestOnlyFullCaptureRecordsCurrentVerboseContent(t *testing.T) {
	t.Parallel()
	require.False(t, CaptureOff.CapturesFullContent())
	require.False(t, CaptureDelta.CapturesFullContent())
	require.True(t, CaptureFull.CapturesFullContent())
}
