package analyzer

import (
	"fmt"
	"math"
	"time"
)

// Source indicates where the cursor event was captured from.
type Source string

const (
	SourceHID Source = "hid" // Raw IOHIDManager event directly from USB driver
	SourceCG  Source = "cg"  // CoreGraphics WindowServer cursor event
)

// AnomalyKind identifies the specific category of detected glitch.
type AnomalyKind string

const (
	AnomalyJump            AnomalyKind = "jump"
	AnomalyIntegerOverflow AnomalyKind = "integer_overflow"
	AnomalySignFlip        AnomalyKind = "sign_flip"
	AnomalyDisplayCross    AnomalyKind = "display_cross"
	AnomalyBurstRate       AnomalyKind = "burst_rate"
)

// Rect represents screen bounds in CoreGraphics coordinates.
type Rect struct {
	X      float64 `json:"x"`
	Y      float64 `json:"y"`
	Width  float64 `json:"width"`
	Height float64 `json:"height"`
}

// Contains returns true if the coordinate is within the rectangle.
func (r Rect) Contains(x, y float64) bool {
	return x >= r.X && x <= (r.X+r.Width) && y >= r.Y && y <= (r.Y+r.Height)
}

// Display represents an active or online monitor.
type Display struct {
	ID     uint32 `json:"id"`
	Bounds Rect   `json:"bounds"`
	IsMain bool   `json:"is_main"`
}

// Event is a normalized pointing device movement event.
type Event struct {
	ID          uint64    `json:"id"`
	Timestamp   time.Time `json:"timestamp"`
	Source      Source    `json:"source"`
	DeviceName  string    `json:"device_name,omitempty"`
	DeviceVID   uint32    `json:"device_vid,omitempty"`
	DevicePID   uint32    `json:"device_pid,omitempty"`
	CursorX     float64   `json:"cursor_x,omitempty"`
	CursorY     float64   `json:"cursor_y,omitempty"`
	DeltaX      int64     `json:"delta_x"`
	DeltaY      int64     `json:"delta_y"`
	SubframeX   float64   `json:"subframe_x,omitempty"`
	SubframeY   float64   `json:"subframe_y,omitempty"`
	IntervalMs  float64   `json:"interval_ms,omitempty"`
	RawReportID uint32    `json:"raw_report_id,omitempty"`
}

// Anomaly describes an irregular motion or firmware glitch.
type Anomaly struct {
	Kind        AnomalyKind       `json:"kind"`
	Severity    string            `json:"severity"` // "high", "medium", "low"
	Title       string            `json:"title"`
	Description string            `json:"description"`
	Event       Event             `json:"event"`
	Details     map[string]string `json:"details,omitempty"`
}

// Config configures analyzer parameters and thresholds.
type Config struct {
	JumpThreshold int64     // delta magnitude (e.g. 50 counts) considered a jump
	Displays      []Display // known displays for boundary cross detection
}

// HistogramBucket counts events within a delta magnitude range.
type HistogramBucket struct {
	Min   int64 `json:"min"`
	Max   int64 `json:"max"`
	Count int64 `json:"count"`
}

// DeltaStats records distribution statistics for an axis.
type DeltaStats struct {
	Min    int64   `json:"min"`
	Max    int64   `json:"max"`
	Mean   float64 `json:"mean"`
	StdDev float64 `json:"std_dev"`
}

// Summary aggregates statistics and diagnostic conclusions.
type Summary struct {
	TotalEvents      int64               `json:"total_events"`
	Duration         time.Duration       `json:"duration"`
	AvgHz            float64             `json:"avg_hz"`
	AnomalyCount     int64               `json:"anomaly_count"`
	AnomalyBreakdown map[AnomalyKind]int `json:"anomaly_breakdown"`
	XStats           DeltaStats          `json:"x_stats"`
	YStats           DeltaStats          `json:"y_stats"`
	Buckets          []HistogramBucket   `json:"buckets"`
	Diagnoses        []string            `json:"diagnoses"`
}

func (s Summary) String() string {
	return fmt.Sprintf("Events: %d, Anomalies: %d, X[min=%d, max=%d, mean=%.1f], Y[min=%d, max=%d, mean=%.1f]",
		s.TotalEvents, s.AnomalyCount, s.XStats.Min, s.XStats.Max, s.XStats.Mean, s.YStats.Min, s.YStats.Max, s.YStats.Mean)
}

func abs(v int64) int64 {
	if v < 0 {
		return -v
	}
	return v
}

func stdDev(values []int64, mean float64) float64 {
	if len(values) < 2 {
		return 0
	}
	var sumSq float64
	for _, v := range values {
		diff := float64(v) - mean
		sumSq += diff * diff
	}
	return math.Sqrt(sumSq / float64(len(values)-1))
}
