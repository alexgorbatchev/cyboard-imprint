package capture

import (
	"slices"
	"time"

	"github.com/alexgorbatchev/mouse-issues/internal/analyzer"
)

// monoClock maps monotonic event timestamps (nanoseconds since boot, as converted from
// Mach absolute time) onto wall-clock time, anchored at one instant sampled on both clocks.
type monoClock struct {
	wall  time.Time
	nanos uint64
}

func (c monoClock) at(nanos uint64) time.Time {
	return c.wall.Add(time.Duration(int64(nanos) - int64(c.nanos)))
}

// hidUsage packs a HID usage page and usage ID as page<<16 | usage.
type hidUsage uint32

const (
	usageX     hidUsage = 0x0001_0030 // Generic Desktop X
	usageY     hidUsage = 0x0001_0031 // Generic Desktop Y
	usageWheel hidUsage = 0x0001_0038 // Generic Desktop Wheel
	usageACPan hidUsage = 0x000C_0238 // Consumer AC Pan (horizontal scroll)
)

// hidDevice identifies the device a HID value came from.
type hidDevice struct {
	handle  uintptr
	name    string
	vid     uint32
	pid     uint32
	version uint32
}

// hidReport is the motion carried by one HID input report.
type hidReport struct {
	dev     hidDevice
	tsNanos uint64
	dx      int64
	dy      int64
	wheel   int64
	pan     int64
}

func (r hidReport) hasMotion() bool {
	return r.dx != 0 || r.dy != 0 || r.wheel != 0 || r.pan != 0
}

func (r hidReport) event(id uint64, clock monoClock) analyzer.Event {
	return analyzer.Event{
		ID:            id,
		Timestamp:     clock.at(r.tsNanos),
		Source:        analyzer.SourceHID,
		DeviceName:    r.dev.name,
		DeviceVID:     r.dev.vid,
		DevicePID:     r.dev.pid,
		DeviceVersion: r.dev.version,
		DeltaX:        r.dx,
		DeltaY:        r.dy,
		Wheel:         r.wheel,
		Pan:           r.pan,
	}
}

// reportAssembler rebuilds whole HID reports from IOHIDManager value callbacks.
//
// IOHIDManager delivers each element of an input report as a separate value, and every
// value from one report carries that report's timestamp. Values are grouped per device
// until a value with a different timestamp arrives, so X and Y are never paired across
// reports and no counts are dropped.
type reportAssembler struct {
	pending     map[uintptr]*hidReport
	lastEmitted map[string]uint64
}

// add records one element value. When the value starts a new report, the previous report
// of the same device is returned as completed (if it carried any motion).
func (a *reportAssembler) add(dev hidDevice, usage hidUsage, val int64, tsNanos uint64) (hidReport, bool) {
	if a.pending == nil {
		a.pending = make(map[uintptr]*hidReport)
	}

	var completed hidReport
	var done bool
	cur := a.pending[dev.handle]
	if cur != nil && cur.tsNanos != tsNanos {
		if a.lastEmitted != nil && a.lastEmitted[cur.dev.name] == cur.tsNanos {
			// Skip duplicate reports dispatched by multiple HID collections on the same device.
			cur = nil
		} else {
			completed, done = *cur, cur.hasMotion()
			if done {
				if a.lastEmitted == nil {
					a.lastEmitted = make(map[string]uint64)
				}
				a.lastEmitted[cur.dev.name] = cur.tsNanos
			}
			cur = nil
		}
	}
	if cur == nil {
		cur = &hidReport{dev: dev, tsNanos: tsNanos}
		a.pending[dev.handle] = cur
	}

	switch usage {
	case usageX:
		cur.dx += val
	case usageY:
		cur.dy += val
	case usageWheel:
		cur.wheel += val
	case usageACPan:
		cur.pan += val
	}
	return completed, done
}

// flush returns all pending reports that carry motion, oldest first, and clears them.
// All values of one report are delivered within a single run loop dispatch, so pending
// reports are complete whenever the run loop is not dispatching.
func (a *reportAssembler) flush() []hidReport {
	var out []hidReport
	for handle, r := range a.pending {
		if r.hasMotion() {
			if a.lastEmitted == nil || a.lastEmitted[r.dev.name] != r.tsNanos {
				out = append(out, *r)
				if a.lastEmitted == nil {
					a.lastEmitted = make(map[string]uint64)
				}
				a.lastEmitted[r.dev.name] = r.tsNanos
			}
		}
		delete(a.pending, handle)
	}
	slices.SortFunc(out, func(x, y hidReport) int {
		switch {
		case x.tsNanos < y.tsNanos:
			return -1
		case x.tsNanos > y.tsNanos:
			return 1
		}
		return 0
	})
	return out
}
