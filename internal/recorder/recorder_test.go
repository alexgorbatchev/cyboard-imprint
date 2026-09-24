package recorder

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/alexgorbatchev/mouse-issues/internal/analyzer"
	"github.com/alexgorbatchev/mouse-issues/internal/device"
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

	recording, err := Read(&buf)
	if err != nil {
		t.Fatalf("Read failed: %v", err)
	}
	if recording.Session != nil {
		t.Fatalf("expected no session header, got %+v", recording.Session)
	}
	readEvents := recording.Events

	if len(readEvents) != 2 {
		t.Fatalf("expected 2 read events, got %d", len(readEvents))
	}
	if readEvents[0].DeltaX != 2 || readEvents[1].DeltaX != 255 {
		t.Fatalf("mismatched deltas: got %d and %d", readEvents[0].DeltaX, readEvents[1].DeltaX)
	}
}

func TestRecordAndReadSession(t *testing.T) {
	var buf bytes.Buffer
	rec := NewWriter(&buf)

	t0 := time.Date(2026, 9, 24, 12, 0, 0, 0, time.UTC)
	header := SessionHeader{
		StartedAt:    t0,
		Mode:         "both",
		DeviceFilter: "Imprint",
		Threshold:    40,
		Displays: []analyzer.Display{
			{ID: 1, Bounds: analyzer.Rect{Width: 2560, Height: 1440}, IsMain: true},
		},
		Devices: []device.Info{
			{Name: "Imprint (Patched)", VendorID: 0x4359, VersionNumber: 0x0022},
		},
	}
	if err := rec.WriteSession(header); err != nil {
		t.Fatalf("WriteSession failed: %v", err)
	}
	if err := rec.WriteEvent(analyzer.Event{ID: 1, Timestamp: t0, Source: analyzer.SourceHID, DeltaX: 3}); err != nil {
		t.Fatalf("WriteEvent failed: %v", err)
	}
	if err := rec.WriteAnomaly(analyzer.Anomaly{Kind: analyzer.AnomalyJump}); err != nil {
		t.Fatalf("WriteAnomaly failed: %v", err)
	}

	recording, err := Read(&buf)
	if err != nil {
		t.Fatalf("Read failed: %v", err)
	}
	if recording.Session == nil {
		t.Fatal("expected session header")
	}
	got := *recording.Session
	if !got.StartedAt.Equal(t0) || got.Mode != "both" || got.DeviceFilter != "Imprint" || got.Threshold != 40 {
		t.Fatalf("session header mismatch: %+v", got)
	}
	if len(got.Displays) != 1 || got.Displays[0].Bounds.Width != 2560 {
		t.Fatalf("displays mismatch: %+v", got.Displays)
	}
	if len(got.Devices) != 1 || got.Devices[0].VersionNumber != 0x0022 {
		t.Fatalf("devices mismatch: %+v", got.Devices)
	}
	if len(recording.Events) != 1 || recording.Events[0].DeltaX != 3 {
		t.Fatalf("events mismatch: %+v", recording.Events)
	}
}

func TestReadRejectsMalformedLine(t *testing.T) {
	if _, err := Read(strings.NewReader("{\"type\":\"event\"}\nnot json\n")); err == nil {
		t.Fatal("expected an error for a malformed line")
	}
}
