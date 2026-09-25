package analyzer

import (
	"strings"
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

func TestDetectAnomalies_BoundarySignatures(t *testing.T) {
	tests := []struct {
		name     string
		deltaX   int64
		deltaY   int64
		wantKind AnomalyKind // empty means neither overflow nor saturation
	}{
		{name: "8-bit unsigned 255 (should be -1)", deltaX: 255, wantKind: AnomalyIntegerOverflow},
		{name: "16-bit unsigned 65535", deltaX: 65535, wantKind: AnomalyIntegerOverflow},
		{name: "9-bit slip 256", deltaY: -256, wantKind: AnomalyIntegerOverflow},
		{name: "8-bit report max 127", deltaX: 127, wantKind: AnomalySaturation},
		{name: "8-bit descriptor min -127", deltaX: -127, wantKind: AnomalySaturation},
		{name: "8-bit report min -128", deltaY: -128, wantKind: AnomalySaturation},
		{name: "16-bit report max 32767", deltaX: 32767, wantKind: AnomalySaturation},
		{name: "16-bit report min -32768", deltaY: -32768, wantKind: AnomalySaturation},
		{name: "ordinary delta 15", deltaX: 15, deltaY: 10},
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

			for _, kind := range []AnomalyKind{AnomalyIntegerOverflow, AnomalySaturation} {
				if got, want := hasKind(anomalies, kind), kind == tt.wantKind; got != want {
					t.Fatalf("delta (%d, %d): %s present = %v, want %v (anomalies: %+v)",
						tt.deltaX, tt.deltaY, kind, got, want, anomalies)
				}
			}
		})
	}
}

func TestSessionSummary_Diagnoses(t *testing.T) {
	t0 := time.Date(2026, 9, 24, 10, 0, 0, 0, time.UTC)
	hid := func(ms int, dx int64) Event {
		return Event{Timestamp: t0.Add(time.Duration(ms) * time.Millisecond), Source: SourceHID, DeviceName: "Imprint", DeltaX: dx}
	}

	tests := []struct {
		name   string
		events []Event
		want   []string
		reject []string
	}{
		{
			name:   "saturated reports point at DPI",
			events: []Event{hid(0, 3), hid(1, 90), hid(2, 127)},
			want:   []string{"Report Saturation (1 occurrences)", "DPI", "Large Raw Deltas (2 occurrences"},
			reject: []string{"Integer Overflow", "SPI"},
		},
		{
			name:   "direction flip is diagnosed",
			events: []Event{hid(0, 3), hid(1, -40)},
			want:   []string{"Direction Flips (1 occurrences)"},
		},
		{
			name:   "clean movement",
			events: []Event{hid(0, 3), hid(1, 4)},
			want:   []string{"Clean Movement Profile"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := New(Config{JumpThreshold: 50})
			for _, e := range tt.events {
				a.Process(e)
			}
			joined := strings.Join(a.Summary().Diagnoses, "\n")
			for _, w := range tt.want {
				if !strings.Contains(joined, w) {
					t.Errorf("diagnoses missing %q:\n%s", w, joined)
				}
			}
			for _, r := range tt.reject {
				if strings.Contains(joined, r) {
					t.Errorf("diagnoses must not contain %q:\n%s", r, joined)
				}
			}
		})
	}
}

var testDisplays = []Display{
	{ID: 2, Bounds: Rect{X: 0, Y: 0, Width: 2560, Height: 1440}, IsMain: true},
	{ID: 3, Bounds: Rect{X: 511, Y: 1440, Width: 1440, Height: 900}, IsMain: false},
}

func hasKind(anomalies []Anomaly, kind AnomalyKind) bool {
	for _, an := range anomalies {
		if an.Kind == kind {
			return true
		}
	}
	return false
}

func TestDetectAnomalies_CursorLeaps(t *testing.T) {
	t0 := time.Date(2026, 9, 24, 10, 0, 0, 0, time.UTC)
	start := Event{Timestamp: t0, Source: SourceCG, CursorX: 1000, CursorY: 1300, DeltaX: 5, DeltaY: 5}

	tests := []struct {
		name     string
		next     Event
		wantKind AnomalyKind // empty means no anomaly expected
	}{
		{
			name:     "crossing explained by delta",
			next:     Event{Source: SourceCG, CursorX: 1000, CursorY: 1500, DeltaX: 0, DeltaY: 200},
			wantKind: "",
		},
		{
			name:     "crossing not explained by delta",
			next:     Event{Source: SourceCG, CursorX: 800, CursorY: 2000, DeltaX: 3, DeltaY: 2},
			wantKind: AnomalyDisplayCross,
		},
		{
			name:     "leap within one display",
			next:     Event{Source: SourceCG, CursorX: 2000, CursorY: 300, DeltaX: 4, DeltaY: -1},
			wantKind: AnomalyCursorLeap,
		},
		{
			name:     "sub-pixel rounding",
			next:     Event{Source: SourceCG, CursorX: 1006.4, CursorY: 1305.7, DeltaX: 5, DeltaY: 5},
			wantKind: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := New(Config{JumpThreshold: 50, Displays: testDisplays})
			a.Process(start)
			tt.next.Timestamp = t0.Add(8 * time.Millisecond)
			anomalies := a.Process(tt.next)

			if tt.wantKind == "" {
				if len(anomalies) != 0 {
					t.Fatalf("expected no anomalies, got %+v", anomalies)
				}
				return
			}
			if !hasKind(anomalies, tt.wantKind) {
				t.Fatalf("expected %s, got %+v", tt.wantKind, anomalies)
			}
		})
	}
}

func TestDetectAnomalies_SourcesAreIndependent(t *testing.T) {
	t0 := time.Date(2026, 9, 24, 10, 0, 0, 0, time.UTC)
	a := New(Config{JumpThreshold: 50, Displays: testDisplays})

	// CG cursor on the secondary display, then an interleaved HID report that has no cursor position.
	a.Process(Event{Timestamp: t0, Source: SourceCG, CursorX: 900, CursorY: 2000, DeltaX: -3, DeltaY: 0})
	hid := a.Process(Event{Timestamp: t0.Add(10 * time.Microsecond), Source: SourceHID, DeviceName: "Imprint", DeltaX: -30, DeltaY: 0})
	if len(hid) != 0 {
		t.Fatalf("HID report must not be compared with a CG event, got %+v", hid)
	}

	cg := a.Process(Event{Timestamp: t0.Add(20 * time.Microsecond), Source: SourceCG, CursorX: 897, CursorY: 2000, DeltaX: -3, DeltaY: 0})
	if len(cg) != 0 {
		t.Fatalf("CG event must be compared with the previous CG event, not the HID report, got %+v", cg)
	}
}

func TestDetectAnomalies_BurstRatePerDevice(t *testing.T) {
	t0 := time.Date(2026, 9, 24, 10, 0, 0, 0, time.UTC)
	a := New(Config{JumpThreshold: 50})

	a.Process(Event{Timestamp: t0, Source: SourceHID, DeviceName: "Imprint", DeltaX: 2})
	other := a.Process(Event{Timestamp: t0.Add(100 * time.Microsecond), Source: SourceHID, DeviceName: "Other Mouse", DeltaX: 30})
	if hasKind(other, AnomalyBurstRate) {
		t.Fatalf("reports from different devices must not form a burst, got %+v", other)
	}

	burst := a.Process(Event{Timestamp: t0.Add(200 * time.Microsecond), Source: SourceHID, DeviceName: "Imprint", DeltaX: 30})
	if !hasKind(burst, AnomalyBurstRate) {
		t.Fatalf("expected burst for two Imprint reports 0.2 ms apart, got %+v", burst)
	}
}

func TestSessionSummary_StatsUseHIDReports(t *testing.T) {
	t0 := time.Date(2026, 9, 24, 10, 0, 0, 0, time.UTC)
	a := New(Config{JumpThreshold: 50})

	for i := range 4 {
		ts := t0.Add(time.Duration(i) * time.Millisecond)
		a.Process(Event{Timestamp: ts, Source: SourceHID, DeviceName: "Imprint", DeltaX: 3, DeltaY: -2})
		a.Process(Event{Timestamp: ts.Add(50 * time.Microsecond), Source: SourceCG, CursorX: float64(100 + 40*i), CursorY: 100, DeltaX: 40, DeltaY: 0})
	}

	summary := a.Summary()
	if summary.HIDEvents != 4 || summary.CGEvents != 4 {
		t.Fatalf("expected 4 HID and 4 CG events, got %d and %d", summary.HIDEvents, summary.CGEvents)
	}
	if summary.XStats.Max != 3 || summary.YStats.Min != -2 {
		t.Fatalf("expected stats from HID reports only (x max 3, y min -2), got %+v / %+v", summary.XStats, summary.YStats)
	}
	if summary.Buckets[1].Count != 4 {
		t.Fatalf("expected 4 HID reports in the 3-10 bucket, got %+v", summary.Buckets)
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

	// Feed one overflow and one saturated event
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
	if summary.AnomalyBreakdown[AnomalyIntegerOverflow] != 1 || summary.AnomalyBreakdown[AnomalySaturation] != 1 {
		t.Fatalf("expected 1 overflow (255) and 1 saturation (-128) anomaly, got %+v", summary.AnomalyBreakdown)
	}
	if summary.AnomalyBreakdown[AnomalyJump] != 2 {
		t.Fatalf("expected 2 jump anomalies, got %d", summary.AnomalyBreakdown[AnomalyJump])
	}
	if len(summary.Diagnoses) == 0 {
		t.Fatalf("expected diagnosis recommendations, got none")
	}
}

func TestSessionSummary_SaturationRuns(t *testing.T) {
	t0 := time.Date(2026, 9, 24, 10, 0, 0, 0, time.UTC)
	a := New(Config{JumpThreshold: 50})

	// 5 consecutive saturated reports
	for i := 0; i < 5; i++ {
		a.Process(Event{
			Timestamp:  t0.Add(time.Duration(i) * time.Millisecond),
			Source:     SourceHID,
			DeviceName: "Imprint",
			DeltaX:     -127,
			DeltaY:     127,
		})
	}
	// 1 normal report breaks the run
	a.Process(Event{
		Timestamp:  t0.Add(5 * time.Millisecond),
		Source:     SourceHID,
		DeviceName: "Imprint",
		DeltaX:     10,
		DeltaY:     5,
	})

	summary := a.Summary()
	if len(summary.SaturationRuns) != 1 {
		t.Fatalf("expected 1 saturation run, got %d", len(summary.SaturationRuns))
	}
	run := summary.SaturationRuns[0]
	if run.Count != 5 {
		t.Errorf("run count = %d, want 5", run.Count)
	}
	if run.SumDeltaX != -635 || run.SumDeltaY != 635 {
		t.Errorf("run sums = (%d, %d), want (-635, 635)", run.SumDeltaX, run.SumDeltaY)
	}
	if summary.MaxSaturationRun == nil || summary.MaxSaturationRun.Count != 5 {
		t.Errorf("max saturation run = %+v, want count 5", summary.MaxSaturationRun)
	}
	if !strings.Contains(summary.Diagnoses[0], "longest run: 5 consecutive reports") {
		t.Errorf("expected diagnosis to mention longest run, got: %s", summary.Diagnoses[0])
	}
}

func TestDetectAnomalies_VelocityLeaps(t *testing.T) {
	t0 := time.Date(2026, 9, 24, 10, 0, 0, 0, time.UTC)
	a := New(Config{JumpThreshold: 50, LeapVelocityThreshold: 50000.0, Displays: testDisplays})

	// Gentle starting position
	a.Process(Event{Timestamp: t0, Source: SourceCG, CursorX: 1000, CursorY: 1000, DeltaX: 2, DeltaY: 2})

	// Extreme velocity leap: 200px movement in 1ms (200,000 px/s), even though CG delta matches moved px
	anomalies := a.Process(Event{
		Timestamp: t0.Add(1 * time.Millisecond),
		Source:    SourceCG,
		CursorX:   800,
		CursorY:   1000,
		DeltaX:    -200,
		DeltaY:    0,
	})

	if !hasKind(anomalies, AnomalyCursorLeap) {
		t.Fatalf("expected AnomalyCursorLeap on 200px/1ms leap, got %+v", anomalies)
	}

	summary := a.Summary()
	if len(summary.CursorLeaps) != 1 {
		t.Fatalf("expected 1 recorded cursor leap, got %d", len(summary.CursorLeaps))
	}
	if summary.CursorLeaps[0].DistancePx != 200 {
		t.Errorf("leap distance = %f, want 200", summary.CursorLeaps[0].DistancePx)
	}
	if summary.MaxCursorLeap == nil || summary.MaxCursorLeap.DistancePx != 200 {
		t.Errorf("max cursor leap = %+v, want 200", summary.MaxCursorLeap)
	}
}

