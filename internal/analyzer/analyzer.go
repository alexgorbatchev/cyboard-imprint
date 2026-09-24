package analyzer

import (
	"fmt"
	"time"
)

// Analyzer inspects cursor events in real time or from recorded logs.
type Analyzer struct {
	config    Config
	prevEvent *Event
	startTime time.Time
	lastTime  time.Time

	events []Event

	// Statistics tracking
	totalEvents      int64
	anomalyCount     int64
	anomalyBreakdown map[AnomalyKind]int

	xValues []int64
	yValues []int64

	bucketCounts [6]int64
}

// New creates an Analyzer with given configuration.
func New(cfg Config) *Analyzer {
	if cfg.JumpThreshold <= 0 {
		cfg.JumpThreshold = 50
	}
	return &Analyzer{
		config:           cfg,
		anomalyBreakdown: make(map[AnomalyKind]int),
	}
}

// Process analyzes a single event, returns any anomalies detected, and updates stats.
func (a *Analyzer) Process(curr Event) []Anomaly {
	var anomalies []Anomaly

	if a.startTime.IsZero() {
		a.startTime = curr.Timestamp
	}
	a.lastTime = curr.Timestamp

	// Compute interval if not set
	if a.prevEvent != nil && curr.IntervalMs == 0 && !curr.Timestamp.IsZero() && !a.prevEvent.Timestamp.IsZero() {
		curr.IntervalMs = float64(curr.Timestamp.Sub(a.prevEvent.Timestamp).Microseconds()) / 1000.0
	}

	// 1. Check for Integer Overflow / Boundary signatures
	if an := a.checkIntegerOverflow(curr); an != nil {
		anomalies = append(anomalies, *an)
	}

	// 2. Check for sudden jump exceeding threshold
	if an := a.checkJump(curr); an != nil {
		anomalies = append(anomalies, *an)
	}

	// 3. Check for sign flip discontinuity
	if a.prevEvent != nil {
		if an := a.checkSignFlip(*a.prevEvent, curr); an != nil {
			anomalies = append(anomalies, *an)
		}
	}

	// 4. Check for display crossing leaps (for CoreGraphics events)
	if a.prevEvent != nil && len(a.config.Displays) > 1 && curr.Source == SourceCG {
		if an := a.checkDisplayCross(*a.prevEvent, curr); an != nil {
			anomalies = append(anomalies, *an)
		}
	}

	// 5. Check burst rate glitch
	if curr.IntervalMs > 0 && curr.IntervalMs < 0.4 && (abs(curr.DeltaX) > 20 || abs(curr.DeltaY) > 20) {
		an := Anomaly{
			Kind:        AnomalyBurstRate,
			Severity:    "medium",
			Title:       "High-Frequency Sensor Burst",
			Description: fmt.Sprintf("Event arrived within %.2f ms with elevated delta (%d, %d)", curr.IntervalMs, curr.DeltaX, curr.DeltaY),
			Event:       curr,
		}
		anomalies = append(anomalies, an)
	}

	// Update statistics
	a.totalEvents++
	a.xValues = append(a.xValues, curr.DeltaX)
	a.yValues = append(a.yValues, curr.DeltaY)

	mag := abs(curr.DeltaX)
	if abs(curr.DeltaY) > mag {
		mag = abs(curr.DeltaY)
	}
	switch {
	case mag <= 2:
		a.bucketCounts[0]++
	case mag <= 10:
		a.bucketCounts[1]++
	case mag <= 30:
		a.bucketCounts[2]++
	case mag <= 60:
		a.bucketCounts[3]++
	case mag <= 120:
		a.bucketCounts[4]++
	default:
		a.bucketCounts[5]++
	}

	for _, an := range anomalies {
		a.anomalyCount++
		a.anomalyBreakdown[an.Kind]++
	}

	eventCopy := curr
	a.prevEvent = &eventCopy
	a.events = append(a.events, curr)

	return anomalies
}

func (a *Analyzer) checkIntegerOverflow(curr Event) *Anomaly {
	var matched string
	dx, dy := curr.DeltaX, curr.DeltaY

	switch {
	case dx == 255 || dy == 255:
		matched = "255 (0xFF: 8-bit unsigned cast of -1)"
	case dx == -128 || dy == -128:
		matched = "-128 (0x80: 8-bit min signed boundary / SPI error)"
	case dx == 127 || dy == 127:
		matched = "127 (0x7F: 8-bit max signed saturation)"
	case dx == 256 || dy == 256 || dx == -256 || dy == -256:
		matched = "256 (9-bit byte alignment / shifting slip)"
	case dx == 65535 || dy == 65535:
		matched = "65535 (0xFFFF: 16-bit unsigned cast of -1)"
	case dx == -32768 || dy == -32768:
		matched = "-32768 (0x8000: 16-bit min signed boundary)"
	case dx == 32767 || dy == 32767:
		matched = "32767 (0x7FFF: 16-bit max signed boundary)"
	}

	if matched != "" {
		return &Anomaly{
			Kind:        AnomalyIntegerOverflow,
			Severity:    "high",
			Title:       "Firmware Integer Overflow Signature",
			Description: fmt.Sprintf("Delta (%d, %d) matches known integer boundary: %s", dx, dy, matched),
			Event:       curr,
			Details: map[string]string{
				"signature": matched,
			},
		}
	}
	return nil
}

func (a *Analyzer) checkJump(curr Event) *Anomaly {
	thresh := a.config.JumpThreshold
	if abs(curr.DeltaX) >= thresh || abs(curr.DeltaY) >= thresh {
		return &Anomaly{
			Kind:        AnomalyJump,
			Severity:    "high",
			Title:       "Sudden Delta Leap",
			Description: fmt.Sprintf("Delta (%d, %d) exceeded jump threshold (%d)", curr.DeltaX, curr.DeltaY, thresh),
			Event:       curr,
		}
	}
	return nil
}

func (a *Analyzer) checkSignFlip(prev Event, curr Event) *Anomaly {
	// If previous motion was gentle positive and current is a large negative jump, or vice-versa
	if (prev.DeltaX > 0 && prev.DeltaX <= 5 && curr.DeltaX < -20) ||
		(prev.DeltaX < 0 && prev.DeltaX >= -5 && curr.DeltaX > 20) ||
		(prev.DeltaY > 0 && prev.DeltaY <= 5 && curr.DeltaY < -20) ||
		(prev.DeltaY < 0 && prev.DeltaY >= -5 && curr.DeltaY > 20) {
		return &Anomaly{
			Kind:        AnomalySignFlip,
			Severity:    "medium",
			Title:       "Discontinuous Direction Flip",
			Description: fmt.Sprintf("Motion abruptly inverted from (%d, %d) to (%d, %d)", prev.DeltaX, prev.DeltaY, curr.DeltaX, curr.DeltaY),
			Event:       curr,
		}
	}
	return nil
}

func (a *Analyzer) checkDisplayCross(prev Event, curr Event) *Anomaly {
	var prevDisp, currDisp *Display
	for i := range a.config.Displays {
		d := &a.config.Displays[i]
		if d.Bounds.Contains(prev.CursorX, prev.CursorY) {
			prevDisp = d
		}
		if d.Bounds.Contains(curr.CursorX, curr.CursorY) {
			currDisp = d
		}
	}

	if prevDisp != nil && currDisp != nil && prevDisp.ID != currDisp.ID {
		return &Anomaly{
			Kind:        AnomalyDisplayCross,
			Severity:    "high",
			Title:       "Multi-Monitor Display Boundary Leap",
			Description: fmt.Sprintf("Cursor teleported from Display %d to Display %d with delta (%d, %d)", prevDisp.ID, currDisp.ID, curr.DeltaX, curr.DeltaY),
			Event:       curr,
			Details: map[string]string{
				"from_display": fmt.Sprintf("%d", prevDisp.ID),
				"to_display":   fmt.Sprintf("%d", currDisp.ID),
			},
		}
	}
	return nil
}

// Summary builds a complete report of the analyzed session.
func (a *Analyzer) Summary() Summary {
	duration := a.lastTime.Sub(a.startTime)
	if duration < 0 {
		duration = 0
	}

	var avgHz float64
	if duration.Seconds() > 0 {
		avgHz = float64(a.totalEvents) / duration.Seconds()
	}

	xStats := calculateStats(a.xValues)
	yStats := calculateStats(a.yValues)

	buckets := []HistogramBucket{
		{Min: 0, Max: 2, Count: a.bucketCounts[0]},
		{Min: 3, Max: 10, Count: a.bucketCounts[1]},
		{Min: 11, Max: 30, Count: a.bucketCounts[2]},
		{Min: 31, Max: 60, Count: a.bucketCounts[3]},
		{Min: 61, Max: 120, Count: a.bucketCounts[4]},
		{Min: 121, Max: 100000, Count: a.bucketCounts[5]},
	}

	diagnoses := a.generateDiagnoses()

	return Summary{
		TotalEvents:      a.totalEvents,
		Duration:         duration,
		AvgHz:            avgHz,
		AnomalyCount:     a.anomalyCount,
		AnomalyBreakdown: a.anomalyBreakdown,
		XStats:           xStats,
		YStats:           yStats,
		Buckets:          buckets,
		Diagnoses:        diagnoses,
	}
}

func calculateStats(vals []int64) DeltaStats {
	if len(vals) == 0 {
		return DeltaStats{}
	}
	minVal := vals[0]
	maxVal := vals[0]
	var sum int64
	for _, v := range vals {
		if v < minVal {
			minVal = v
		}
		if v > maxVal {
			maxVal = v
		}
		sum += v
	}
	mean := float64(sum) / float64(len(vals))
	return DeltaStats{
		Min:    minVal,
		Max:    maxVal,
		Mean:   mean,
		StdDev: stdDev(vals, mean),
	}
}

func (a *Analyzer) generateDiagnoses() []string {
	var diagnoses []string

	overflowCount := a.anomalyBreakdown[AnomalyIntegerOverflow]
	jumpCount := a.anomalyBreakdown[AnomalyJump]
	crossCount := a.anomalyBreakdown[AnomalyDisplayCross]
	burstCount := a.anomalyBreakdown[AnomalyBurstRate]

	if overflowCount > 0 {
		diagnoses = append(diagnoses, fmt.Sprintf(
			"Firmware Integer Overflow Detected (%d occurrences): Delta boundary values (e.g. 255, -128, 65535) indicate a sign-extension or signed/unsigned type cast defect in firmware. In QMK/Vial pointing device drivers, ensure motion variables are signed int8_t or int16_t, and that report descriptor min/max bounds match the coordinate type.",
			overflowCount,
		))
	}

	if jumpCount > 0 && overflowCount == 0 {
		diagnoses = append(diagnoses, fmt.Sprintf(
			"Sudden Motion Delta Spikes Detected (%d occurrences): Raw deltas jump abruptly during gentle motion without integer boundary patterns. Suspect SPI communication noise or timing delay (tSRAD / CS delay) between the microcontroller and the optical sensor (e.g. PMW3360), or physical debris/hair in the sensor well.",
			jumpCount,
		))
	}

	if crossCount > 0 {
		diagnoses = append(diagnoses, fmt.Sprintf(
			"Multi-Monitor Boundary Teleportation (%d occurrences): Cursor leaper across physical display boundaries within a single motion frame.",
			crossCount,
		))
	}

	if burstCount > 0 {
		diagnoses = append(diagnoses, fmt.Sprintf(
			"High-Frequency Sensor Burst Glitches (%d occurrences): Sensor reports fired under 0.4ms intervals, suggesting burst-mode buffer dumps or unthrottled loop execution.",
			burstCount,
		))
	}

	if len(diagnoses) == 0 {
		diagnoses = append(diagnoses, "Clean Movement Profile: No integer overflow signatures, sign flips, or abnormal leaps were detected during this capture window.")
	}

	return diagnoses
}
