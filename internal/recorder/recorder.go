package recorder

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/alexgorbatchev/mouse-issues/internal/analyzer"
	"github.com/alexgorbatchev/mouse-issues/internal/device"
)

// SessionHeader describes the capture environment. It is written as the first line so a
// recording can be analyzed against the display layout and firmware it was captured with.
type SessionHeader struct {
	StartedAt    time.Time          `json:"started_at"`
	Mode         string             `json:"mode"`
	DeviceFilter string             `json:"device_filter,omitempty"`
	Threshold    int64              `json:"threshold"`
	Displays     []analyzer.Display `json:"displays"`
	Devices      []device.Info      `json:"devices"`
}

// RecordLine wraps a session, event, or anomaly line in the recorded stream.
type RecordLine struct {
	Type    string            `json:"type"` // "session", "event", or "anomaly"
	Session *SessionHeader    `json:"session,omitempty"`
	Event   *analyzer.Event   `json:"event,omitempty"`
	Anomaly *analyzer.Anomaly `json:"anomaly,omitempty"`
}

// Recording is the parsed content of a recorded NDJSON stream.
type Recording struct {
	Session *SessionHeader // nil when the stream has no session line
	Events  []analyzer.Event
}

// Writer streams JSON-delimited records to an io.Writer.
type Writer struct {
	enc *json.Encoder
}

// NewWriter creates a recorder Writer.
func NewWriter(w io.Writer) *Writer {
	return &Writer{
		enc: json.NewEncoder(w),
	}
}

// WriteSession writes the session header record.
func (w *Writer) WriteSession(h SessionHeader) error {
	if err := w.enc.Encode(RecordLine{Type: "session", Session: &h}); err != nil {
		return fmt.Errorf("encoding session header: %w", err)
	}
	return nil
}

// WriteEvent writes an Event record.
func (w *Writer) WriteEvent(e analyzer.Event) error {
	line := RecordLine{
		Type:  "event",
		Event: &e,
	}
	if err := w.enc.Encode(line); err != nil {
		return fmt.Errorf("encoding event: %w", err)
	}
	return nil
}

// WriteAnomaly writes an Anomaly record.
func (w *Writer) WriteAnomaly(a analyzer.Anomaly) error {
	line := RecordLine{
		Type:    "anomaly",
		Anomaly: &a,
	}
	if err := w.enc.Encode(line); err != nil {
		return fmt.Errorf("encoding anomaly: %w", err)
	}
	return nil
}

// Read parses a recorded NDJSON stream. Anomaly lines are skipped because analysis
// recomputes them from the events.
func Read(r io.Reader) (Recording, error) {
	var rec Recording
	scanner := bufio.NewScanner(r)

	lineNum := 0
	for scanner.Scan() {
		lineNum++
		text := scanner.Bytes()
		if len(text) == 0 {
			continue
		}

		var line RecordLine
		if err := json.Unmarshal(text, &line); err != nil {
			// Also try parsing directly as Event
			var ev analyzer.Event
			if err2 := json.Unmarshal(text, &ev); err2 == nil && !ev.Timestamp.IsZero() {
				rec.Events = append(rec.Events, ev)
				continue
			}
			return Recording{}, fmt.Errorf("decoding record on line %d: %w", lineNum, err)
		}

		switch {
		case line.Session != nil:
			rec.Session = line.Session
		case line.Event != nil:
			rec.Events = append(rec.Events, *line.Event)
		}
	}

	if err := scanner.Err(); err != nil {
		return Recording{}, fmt.Errorf("scanning record stream: %w", err)
	}

	return rec, nil
}
