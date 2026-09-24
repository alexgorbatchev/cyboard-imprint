package recorder

import (
	"bytes"
	"testing"
	"time"

	"github.com/alexgorbatchev/mouse-issues/internal/analyzer"
)

func TestRecordAndReadEvents(t *testing.T) {
	var buf bytes.Buffer
	rec := NewWriter(&buf)

	t0 := time.Date(2026, 9, 23, 12, 0, 0, 0, time.UTC)
	events := []analyzer.Event{
		{
			ID:        1,
			Timestamp: t0,
			Source:    analyzer.SourceHID,
			DeltaX:    2,
			DeltaY:    1,
		},
		{
			ID:        2,
			Timestamp: t0.Add(8 * time.Millisecond),
			Source:    analyzer.SourceHID,
			DeltaX:    255,
			DeltaY:    -128,
		},
	}

	for _, e := range events {
		if err := rec.WriteEvent(e); err != nil {
			t.Fatalf("WriteEvent failed: %v", err)
		}
	}

	readEvents, err := ReadEvents(&buf)
	if err != nil {
		t.Fatalf("ReadEvents failed: %v", err)
	}

	if len(readEvents) != 2 {
		t.Fatalf("expected 2 read events, got %d", len(readEvents))
	}
	if readEvents[0].DeltaX != 2 || readEvents[1].DeltaX != 255 {
		t.Fatalf("mismatched deltas: got %d and %d", readEvents[0].DeltaX, readEvents[1].DeltaX)
	}
}
