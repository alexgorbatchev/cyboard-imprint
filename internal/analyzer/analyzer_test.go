package analyzer

import (
	"testing"
	"time"
)

func TestDetectAnomalies_NormalMotion(t *testing.T) {
	config := Config{
		JumpThreshold: 50,
	}
	a := New(config)

	t0 := time.Now()
	e1 := Event{
		Timestamp: t0,
		DeltaX:    2,
		DeltaY:    1,
		Source:    SourceHID,
	}
	e2 := Event{
		Timestamp: t0.Add(8 * time.Millisecond),
		DeltaX:    3,
		DeltaY:    2,
		Source:    SourceHID,
	}

	anomalies := a.Process(e1)
	if len(anomalies) != 0 {
		t.Fatalf("expected 0 anomalies on first event, got %d", len(anomalies))
	}

	anomalies = a.Process(e2)
	if len(anomalies) != 0 {
		t.Fatalf("expected 0 anomalies on normal gentle motion, got %d", len(anomalies))
	}
}

func TestDetectAnomalies_JumpAnomaly(t *testing.T) {
	config := Config{
		JumpThreshold: 50,
	}
	a := New(config)

	t0 := time.Now()
	a.Process(Event{
		Timestamp: t0,
		DeltaX:    1,
		DeltaY:    1,
		Source:    SourceHID,
	})

	anomalies := a.Process(Event{
		Timestamp: t0.Add(8 * time.Millisecond),
		DeltaX:    120, // Jump!
		DeltaY:    2,
		Source:    SourceHID,
	})

	if len(anomalies) == 0 {
		t.Fatalf("expected jump anomaly, got none")
	}

	foundJump := false
	for _, an := range anomalies {
		if an.Kind == AnomalyJump {
			foundJump = true
			break
		}
	}
	if !foundJump {
		t.Fatalf("expected AnomalyJump in anomalies: %+v", anomalies)
	}
}

func TestDetectAnomalies_IntegerOverflowSignatures(t *testing.T) {
	tests := []struct {
		name         string
		deltaX       int64
		deltaY       int64
		wantOverflow bool
		wantDetails  string
	}{
		{
			name:         "8-bit unsigned 255 (should be -1)",
			deltaX:       255,
			deltaY:       0,
			wantOverflow: true,
			wantDetails:  "8-bit",
		},
		{
			name:         "8-bit max negative -128 (0x80)",
			deltaX:       0,
			deltaY:       -128,
			wantOverflow: true,
			wantDetails:  "8-bit",
		},
		{
			name:         "8-bit max positive 127 (0x7F)",
			deltaX:       127,
			deltaY:       0,
			wantOverflow: true,
			wantDetails:  "8-bit",
		},
		{
			name:         "16-bit unsigned 65535",
			deltaX:       65535,
			deltaY:       0,
			wantOverflow: true,
			wantDetails:  "16-bit",
		},
		{
			name:         "16-bit max negative -32768",
			deltaX:       0,
			deltaY:       -32768,
			wantOverflow: true,
			wantDetails:  "16-bit",
		},
		{
			name:         "ordinary delta 15",
			deltaX:       15,
			deltaY:       10,
			wantOverflow: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := New(Config{JumpThreshold: 50})
			t0 := time.Now()
			a.Process(Event{Timestamp: t0, DeltaX: 1, DeltaY: 1, Source: SourceHID})
			anomalies := a.Process(Event{
				Timestamp: t0.Add(8 * time.Millisecond),
				DeltaX:    tt.deltaX,
				DeltaY:    tt.deltaY,
				Source:    SourceHID,
			})

			found := false
			for _, an := range anomalies {
				if an.Kind == AnomalyIntegerOverflow {
					found = true
					break
				}
			}

			if found != tt.wantOverflow {
				t.Fatalf("delta (%d, %d): got overflow=%v, want %v (anomalies: %+v)",
					tt.deltaX, tt.deltaY, found, tt.wantOverflow, anomalies)
			}
		})
	}
}

func TestDetectAnomalies_DisplayCross(t *testing.T) {
	displays := []Display{
		{ID: 2, Bounds: Rect{X: 0, Y: 0, Width: 2560, Height: 1440}, IsMain: true},
		{ID: 3, Bounds: Rect{X: 511, Y: 1440, Width: 1440, Height: 900}, IsMain: false},
	}

	a := New(Config{
		JumpThreshold: 50,
		Displays:      displays,
	})

	t0 := time.Now()
	// Cursor on Display 0
	a.Process(Event{
		Timestamp: t0,
		CursorX:   1000,
		CursorY:   1300,
		DeltaX:    5,
		DeltaY:    5,
		Source:    SourceCG,
	})

	// Cursor teleports to Display 1 far away
	anomalies := a.Process(Event{
		Timestamp: t0.Add(8 * time.Millisecond),
		CursorX:   800,
		CursorY:   2000, // Now on Display 1 (1440 + 560)
		DeltaX:    -200,
		DeltaY:    700,
		Source:    SourceCG,
	})

	foundCross := false
	for _, an := range anomalies {
		if an.Kind == AnomalyDisplayCross {
			foundCross = true
			break
		}
	}
	if !foundCross {
		t.Fatalf("expected AnomalyDisplayCross, got anomalies: %+v", anomalies)
	}
}

func TestSessionSummary_StatsAndDiagnosis(t *testing.T) {
	a := New(Config{JumpThreshold: 50})
	t0 := time.Now()

	// Feed 10 normal events
	for i := 0; i < 10; i++ {
		a.Process(Event{
			Timestamp: t0.Add(time.Duration(i*8) * time.Millisecond),
			DeltaX:    int64(i%3 + 1),
			DeltaY:    int64(i%2 + 1),
			Source:    SourceHID,
		})
	}

	// Feed 2 overflow events
	a.Process(Event{
		Timestamp: t0.Add(88 * time.Millisecond),
		DeltaX:    255,
		DeltaY:    0,
		Source:    SourceHID,
	})
	a.Process(Event{
		Timestamp: t0.Add(96 * time.Millisecond),
		DeltaX:    -128,
		DeltaY:    0,
		Source:    SourceHID,
	})

	summary := a.Summary()
	if summary.TotalEvents != 12 {
		t.Fatalf("expected 12 total events, got %d", summary.TotalEvents)
	}
	if summary.AnomalyBreakdown[AnomalyIntegerOverflow] != 2 {
		t.Fatalf("expected 2 overflow anomalies, got %d", summary.AnomalyBreakdown[AnomalyIntegerOverflow])
	}
	if summary.AnomalyBreakdown[AnomalyJump] != 2 {
		t.Fatalf("expected 2 jump anomalies, got %d", summary.AnomalyBreakdown[AnomalyJump])
	}
	if len(summary.Diagnoses) == 0 {
		t.Fatalf("expected diagnosis recommendations, got none")
	}
}
