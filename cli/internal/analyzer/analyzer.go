package analyzer

import (
	"cmp"
	"fmt"
	"math"
	"slices"
	"time"
)

// leapTolerancePx is how far (in points) the cursor may move beyond what the event delta
// explains before it counts as a teleport. Recorded sessions show sub-pixel rounding of at most ~2 points.
const leapTolerancePx = 20

// burstIntervalMs is the report spacing below which a large HID delta counts as a burst.
// A full-speed USB device is polled at most once per millisecond.
const burstIntervalMs = 0.4

// burstDelta is the delta magnitude a report needs before a short interval is reported as a burst.
const burstDelta = 20

// deltaStream accumulates delta statistics for one event source.
type deltaStream struct {
	events       int64
	xValues      []int64
	yValues      []int64
	bucketCounts [6]int64
}

func (d *deltaStream) add(e Event) {
	d.events++
	d.xValues = append(d.xValues, e.DeltaX)
	d.yValues = append(d.yValues, e.DeltaY)

	mag := max(abs(e.DeltaX), abs(e.DeltaY))
	switch {
	case mag <= 2:
		d.bucketCounts[0]++
	case mag <= 10:
		d.bucketCounts[1]++
	case mag <= 30:
		d.bucketCounts[2]++
	case mag <= 60:
		d.bucketCounts[3]++
	case mag <= 120:
		d.bucketCounts[4]++
	default:
		d.bucketCounts[5]++
	}
}

type activeSaturation struct {
	count     int
	startTime time.Time
	lastTime  time.Time
	sumX      int64
	sumY      int64
}

// Analyzer inspects cursor events in real time or from recorded logs.
//
// HID reports (raw sensor counts) and CG events (post-acceleration cursor positions) are
// separate streams: each event is only ever compared with the previous event of the same
// stream, and for HID of the same device.
type Analyzer struct {
	config    Config
	prevHID   map[string]Event
	prevCG    *Event
	startTime time.Time
	lastTime  time.Time

	events []Event

	totalEvents      int64
	anomalyCount     int64
	anomalyBreakdown map[AnomalyKind]int

	activeSat        map[string]*activeSaturation
	saturationRuns   []SaturationRun
	maxSaturationRun *SaturationRun
	cursorLeaps      []CursorLeap
	maxCursorLeap    *CursorLeap

	hid deltaStream
	cg  deltaStream
}

// New creates an Analyzer with given configuration.
func New(cfg Config) *Analyzer {
	if cfg.JumpThreshold <= 0 {
		cfg.JumpThreshold = 50
	}
	if cfg.LeapVelocityThreshold <= 0 {
		cfg.LeapVelocityThreshold = 50000.0
	}
	return &Analyzer{
		config:           cfg,
		prevHID:          make(map[string]Event),
		anomalyBreakdown: make(map[AnomalyKind]int),
		activeSat:        make(map[string]*activeSaturation),
	}
}

func hidDeviceKey(e Event) string {
	return fmt.Sprintf("%04x:%04x:%s", e.DeviceVID, e.DevicePID, e.DeviceName)
}

func intervalMs(prev, curr Event) float64 {
	if prev.Timestamp.IsZero() || curr.Timestamp.IsZero() {
		return 0
	}
	return float64(curr.Timestamp.Sub(prev.Timestamp).Microseconds()) / 1000.0
}

// Process analyzes a single event, returns any anomalies detected, and updates stats.
func (a *Analyzer) Process(curr Event) []Anomaly {
	if a.startTime.IsZero() {
		a.startTime = curr.Timestamp
	}
	a.lastTime = curr.Timestamp

	var anomalies []Anomaly
	switch curr.Source {
	case SourceCG:
		curr, anomalies = a.processCG(curr)
	default:
		curr, anomalies = a.processHID(curr)
	}

	a.totalEvents++
	for _, an := range anomalies {
		a.anomalyCount++
		a.anomalyBreakdown[an.Kind]++
	}
	a.events = append(a.events, curr)

	return anomalies
}

func (a *Analyzer) processHID(curr Event) (Event, []Anomaly) {
	var anomalies []Anomaly
	key := hidDeviceKey(curr)
	prev, hasPrev := a.prevHID[key]
	if hasPrev && curr.IntervalMs == 0 {
		curr.IntervalMs = intervalMs(prev, curr)
	}

	anBoundary := a.checkBoundary(curr)
	if anBoundary != nil {
		anomalies = append(anomalies, *anBoundary)
	}
	if an := a.checkJump(curr); an != nil {
		anomalies = append(anomalies, *an)
	}
	if hasPrev {
		if an := a.checkSignFlip(prev, curr); an != nil {
			anomalies = append(anomalies, *an)
		}
	}
	if an := checkBurstRate(curr); an != nil {
		anomalies = append(anomalies, *an)
	}

	// Track saturation runs
	if anBoundary != nil && anBoundary.Kind == AnomalySaturation {
		act := a.activeSat[key]
		if act == nil {
			act = &activeSaturation{
				count:     1,
				startTime: curr.Timestamp,
				lastTime:  curr.Timestamp,
				sumX:      curr.DeltaX,
				sumY:      curr.DeltaY,
			}
			a.activeSat[key] = act
		} else {
			act.count++
			act.lastTime = curr.Timestamp
			act.sumX += curr.DeltaX
			act.sumY += curr.DeltaY
		}
	} else {
		if act := a.activeSat[key]; act != nil {
			if act.count >= 2 {
				run := SaturationRun{
					Count:     act.count,
					Duration:  act.lastTime.Sub(act.startTime),
					SumDeltaX: act.sumX,
					SumDeltaY: act.sumY,
					StartTime: act.startTime,
					EndTime:   act.lastTime,
				}
				a.saturationRuns = append(a.saturationRuns, run)
				if a.maxSaturationRun == nil || run.Count > a.maxSaturationRun.Count {
					a.maxSaturationRun = &run
				}
			}
			delete(a.activeSat, key)
		}
	}

	a.hid.add(curr)
	a.prevHID[key] = curr
	return curr, anomalies
}

func (a *Analyzer) processCG(curr Event) (Event, []Anomaly) {
	var anomalies []Anomaly
	if a.prevCG != nil {
		if curr.IntervalMs == 0 {
			curr.IntervalMs = intervalMs(*a.prevCG, curr)
		}
		if an := a.checkCursorLeap(*a.prevCG, curr); an != nil {
			anomalies = append(anomalies, *an)
		}
	}

	a.cg.add(curr)
	prev := curr
	a.prevCG = &prev
	return curr, anomalies
}

func checkBurstRate(curr Event) *Anomaly {
	if curr.IntervalMs <= 0 || curr.IntervalMs >= burstIntervalMs {
		return nil
	}
	if abs(curr.DeltaX) <= burstDelta && abs(curr.DeltaY) <= burstDelta {
		return nil
	}
	return &Anomaly{
		Kind:        AnomalyBurstRate,
		Severity:    "medium",
		Title:       "High-Frequency Sensor Burst",
		Description: fmt.Sprintf("HID report arrived %.3f ms after the previous one with elevated delta (%d, %d)", curr.IntervalMs, curr.DeltaX, curr.DeltaY),
		Event:       curr,
	}
}

// checkBoundary separates two different boundary signatures. Overflow values cannot come
// from a correctly typed signed report. Saturation values are the limits QMK clamps motion to
// (MOUSE_REPORT_XY_MIN/MAX: int8 by default, int16 with MOUSE_EXTENDED_REPORT) plus -127, the
// logical minimum QMK declares in its 8-bit mouse descriptor, which macOS reports for clamped
// negative motion. They mean the sensor produced more counts in one report interval than the
// report can carry.
func (a *Analyzer) checkBoundary(curr Event) *Anomaly {
	dx, dy := curr.DeltaX, curr.DeltaY
	either := func(v int64) bool { return dx == v || dy == v }

	var overflow, saturation string
	switch {
	case either(255):
		overflow = "255 (0xFF: 8-bit unsigned cast of -1)"
	case either(65535):
		overflow = "65535 (0xFFFF: 16-bit unsigned cast of -1)"
	case either(256) || either(-256):
		overflow = "256 (9-bit byte alignment / shifting slip)"
	case either(127):
		saturation = "127 (8-bit report maximum)"
	case either(-127) || either(-128):
		saturation = "-127/-128 (8-bit report minimum)"
	case either(32767):
		saturation = "32767 (16-bit report maximum)"
	case either(-32768):
		saturation = "-32768 (16-bit report minimum)"
	}

	switch {
	case overflow != "":
		return &Anomaly{
			Kind:        AnomalyIntegerOverflow,
			Severity:    "high",
			Title:       "Firmware Integer Overflow Signature",
			Description: fmt.Sprintf("Delta (%d, %d) matches known integer boundary: %s", dx, dy, overflow),
			Event:       curr,
			Details:     map[string]string{"signature": overflow},
		}
	case saturation != "":
		return &Anomaly{
			Kind:        AnomalySaturation,
			Severity:    "high",
			Title:       "HID Report Saturation",
			Description: fmt.Sprintf("Delta (%d, %d) hit the report limit: %s", dx, dy, saturation),
			Event:       curr,
			Details:     map[string]string{"signature": saturation},
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

func (a *Analyzer) displayAt(x, y float64) *Display {
	for i := range a.config.Displays {
		if a.config.Displays[i].Bounds.Contains(x, y) {
			return &a.config.Displays[i]
		}
	}
	return nil
}

// checkCursorLeap flags a cursor that moved further than its own event delta explains,
// or that leaped at extreme velocity across frames.
func (a *Analyzer) checkCursorLeap(prev Event, curr Event) *Anomaly {
	moved := math.Hypot(curr.CursorX-prev.CursorX, curr.CursorY-prev.CursorY)
	explained := math.Hypot(float64(curr.DeltaX), float64(curr.DeltaY))
	unexplained := moved - explained
	dt := curr.Timestamp.Sub(prev.Timestamp)
	var velocity float64
	if dt > 0 {
		velocity = moved / dt.Seconds()
	}

	isUnexplainedLeap := unexplained > leapTolerancePx
	isVelocityLeap := moved >= 80 && velocity >= a.config.LeapVelocityThreshold

	if !isUnexplainedLeap && !isVelocityLeap {
		return nil
	}

	details := map[string]string{
		"moved_px":       fmt.Sprintf("%.1f", moved),
		"unexplained_px": fmt.Sprintf("%.1f", unexplained),
		"velocity_px_s":  fmt.Sprintf("%.0f", velocity),
	}
	prevDisp := a.displayAt(prev.CursorX, prev.CursorY)
	currDisp := a.displayAt(curr.CursorX, curr.CursorY)
	crossesDisplay := prevDisp != nil && currDisp != nil && prevDisp.ID != currDisp.ID

	leap := CursorLeap{
		DistancePx:       moved,
		Duration:         dt,
		VelocityPxPerSec: velocity,
		FromX:            prev.CursorX,
		FromY:            prev.CursorY,
		ToX:              curr.CursorX,
		ToY:              curr.CursorY,
		DeltaX:           curr.DeltaX,
		DeltaY:           curr.DeltaY,
		CrossesDisplay:   crossesDisplay,
		Timestamp:        curr.Timestamp,
	}
	if prevDisp != nil {
		leap.FromDisplay = prevDisp.ID
	}
	if currDisp != nil {
		leap.ToDisplay = currDisp.ID
	}
	a.cursorLeaps = append(a.cursorLeaps, leap)
	if a.maxCursorLeap == nil || moved > a.maxCursorLeap.DistancePx {
		a.maxCursorLeap = &leap
	}

	if crossesDisplay {
		details["from_display"] = fmt.Sprintf("%d", prevDisp.ID)
		details["to_display"] = fmt.Sprintf("%d", currDisp.ID)
		return &Anomaly{
			Kind:        AnomalyDisplayCross,
			Severity:    "high",
			Title:       "Multi-Monitor Display Boundary Leap",
			Description: fmt.Sprintf("Cursor jumped from Display %d to Display %d by %.0f px in %v (%.0f px/s) with delta (%d, %d)", prevDisp.ID, currDisp.ID, moved, dt.Round(time.Millisecond), velocity, curr.DeltaX, curr.DeltaY),
			Event:       curr,
			Details:     details,
		}
	}
	return &Anomaly{
		Kind:        AnomalyCursorLeap,
		Severity:    "high",
		Title:       "Cursor Leap",
		Description: fmt.Sprintf("Cursor moved %.0f px in %v (%.0f px/s) with delta (%d, %d)", moved, dt.Round(time.Millisecond), velocity, curr.DeltaX, curr.DeltaY),
		Event:       curr,
		Details:     details,
	}
}

// Summary builds a complete report of the analyzed session.
func (a *Analyzer) Summary() Summary {
	duration := a.lastTime.Sub(a.startTime)
	if duration < 0 {
		duration = 0
	}

	// Flush any active saturation run that reached the end of the recording
	for key, act := range a.activeSat {
		if act.count >= 2 {
			run := SaturationRun{
				Count:     act.count,
				Duration:  act.lastTime.Sub(act.startTime),
				SumDeltaX: act.sumX,
				SumDeltaY: act.sumY,
				StartTime: act.startTime,
				EndTime:   act.lastTime,
			}
			a.saturationRuns = append(a.saturationRuns, run)
			if a.maxSaturationRun == nil || run.Count > a.maxSaturationRun.Count {
				a.maxSaturationRun = &run
			}
		}
		delete(a.activeSat, key)
	}
	slices.SortFunc(a.saturationRuns, func(x, y SaturationRun) int {
		return cmp.Compare(y.Count, x.Count)
	})

	// Raw HID reports carry the sensor signal; CG deltas are only used when no HID stream was captured.
	stream := &a.hid
	if stream.events == 0 {
		stream = &a.cg
	}

	var avgHz float64
	if duration.Seconds() > 0 {
		avgHz = float64(stream.events) / duration.Seconds()
	}

	xStats := calculateStats(stream.xValues)
	yStats := calculateStats(stream.yValues)

	buckets := []HistogramBucket{
		{Min: 0, Max: 2, Count: stream.bucketCounts[0]},
		{Min: 3, Max: 10, Count: stream.bucketCounts[1]},
		{Min: 11, Max: 30, Count: stream.bucketCounts[2]},
		{Min: 31, Max: 60, Count: stream.bucketCounts[3]},
		{Min: 61, Max: 120, Count: stream.bucketCounts[4]},
		{Min: 121, Max: 100000, Count: stream.bucketCounts[5]},
	}

	diagnoses := a.generateDiagnoses()

	return Summary{
		TotalEvents:      a.totalEvents,
		HIDEvents:        a.hid.events,
		CGEvents:         a.cg.events,
		Duration:         duration,
		AvgHz:            avgHz,
		AnomalyCount:     a.anomalyCount,
		AnomalyBreakdown: a.anomalyBreakdown,
		XStats:           xStats,
		YStats:           yStats,
		Buckets:          buckets,
		Diagnoses:        diagnoses,
		SaturationRuns:   a.saturationRuns,
		MaxSaturationRun: a.maxSaturationRun,
		CursorLeaps:      a.cursorLeaps,
		MaxCursorLeap:    a.maxCursorLeap,
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
	count := func(kind AnomalyKind) int { return a.anomalyBreakdown[kind] }

	if n := count(AnomalySaturation); n > 0 {
		if a.maxSaturationRun != nil && a.maxSaturationRun.Count >= 2 {
			diagnoses = append(diagnoses, fmt.Sprintf(
				"Report Saturation (%d occurrences, longest run: %d consecutive reports over %s with delta sum X=%d, Y=%d): HID deltas hit the report limit, so the firmware clamped motion that did not fit in one report. The sensor is producing more counts per report interval than gentle trackball movement should; check the active DPI step first.",
				n, a.maxSaturationRun.Count, a.maxSaturationRun.Duration.Round(time.Millisecond), a.maxSaturationRun.SumDeltaX, a.maxSaturationRun.SumDeltaY,
			))
		} else {
			diagnoses = append(diagnoses, fmt.Sprintf(
				"Report Saturation (%d occurrences): HID deltas hit the report limit, so the firmware clamped motion that did not fit in one report. The sensor is producing more counts per report interval than gentle trackball movement should; check the active DPI step first.",
				n,
			))
		}
	}

	if n := count(AnomalyIntegerOverflow); n > 0 {
		diagnoses = append(diagnoses, fmt.Sprintf(
			"Firmware Integer Overflow Detected (%d occurrences): Delta values such as 255, 65535, or 256 cannot come from a correctly typed signed report. Check the report descriptor logical min/max and signed/unsigned casts of motion values in the firmware.",
			n,
		))
	}

	if n := count(AnomalyJump); n > 0 {
		diagnoses = append(diagnoses, fmt.Sprintf(
			"Large Raw Deltas (%d occurrences of |delta| >= %d counts): Sustained large deltas during gentle movement point at a DPI that is too high. Isolated spikes between small deltas (see direction flips) point at sensor misreads instead.",
			n, a.config.JumpThreshold,
		))
	}

	if n := count(AnomalySignFlip); n > 0 {
		diagnoses = append(diagnoses, fmt.Sprintf(
			"Direction Flips (%d occurrences): A large delta opposite to the preceding gentle motion. This is characteristic of sensor misreads (dirty lens, lift-off, SPI read errors) rather than DPI.",
			n,
		))
	}

	if leaps, crossings := count(AnomalyCursorLeap), count(AnomalyDisplayCross); leaps+crossings > 0 {
		if a.maxCursorLeap != nil {
			diagnoses = append(diagnoses, fmt.Sprintf(
				"Cursor Teleports (%d occurrences, %d across displays, peak leap: %.0f px at %.0f px/s): The cursor moved further than the event delta explains or experienced extreme velocity leaps across frames. The jump happened after the input device, in WindowServer or software that warps the cursor.",
				leaps+crossings, crossings, a.maxCursorLeap.DistancePx, a.maxCursorLeap.VelocityPxPerSec,
			))
		} else {
			diagnoses = append(diagnoses, fmt.Sprintf(
				"Cursor Teleports (%d occurrences, %d across displays): The cursor moved further than the event delta explains. The jump happened after the input device, in WindowServer or software that warps the cursor.",
				leaps+crossings, crossings,
			))
		}
	}

	if n := count(AnomalyBurstRate); n > 0 {
		diagnoses = append(diagnoses, fmt.Sprintf(
			"High-Frequency Report Bursts (%d occurrences): HID reports with large deltas arrived less than %.1f ms apart, faster than USB polling should deliver them. Suspect buffered reports being released in a burst.",
			n, burstIntervalMs,
		))
	}

	if len(diagnoses) == 0 {
		diagnoses = append(diagnoses, "Clean Movement Profile: No boundary signatures, large deltas, direction flips, bursts, or cursor teleports were detected during this capture window.")
	}

	return diagnoses
}
