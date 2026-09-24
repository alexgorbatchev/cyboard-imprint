package recorder

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"

	"github.com/alexgorbatchev/mouse-issues/internal/analyzer"
)

// RecordLine wraps an event or anomaly line in the recorded stream.
type RecordLine struct {
	Type    string           `json:"type"` // "event" or "anomaly"
	Event   *analyzer.Event   `json:"event,omitempty"`
	Anomaly *analyzer.Anomaly `json:"anomaly,omitempty"`
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

// ReadEvents parses all events from an io.Reader containing recorded NDJSON lines.
func ReadEvents(r io.Reader) ([]analyzer.Event, error) {
	var events []analyzer.Event
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
				events = append(events, ev)
				continue
			}
			return nil, fmt.Errorf("decoding record on line %d: %w", lineNum, err)
		}

		if line.Event != nil {
			events = append(events, *line.Event)
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scanning record stream: %w", err)
	}

	return events, nil
}
