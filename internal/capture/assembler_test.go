package capture

import (
	"testing"
	"time"
)

func TestMonoClock_At(t *testing.T) {
	base := time.Date(2026, 9, 24, 10, 0, 0, 0, time.UTC)
	c := monoClock{wall: base, nanos: 5_000_000}

	tests := []struct {
		name  string
		nanos uint64
		want  time.Time
	}{
		{"same instant", 5_000_000, base},
		{"after start", 5_250_000, base.Add(250 * time.Microsecond)},
		{"before start", 4_000_000, base.Add(-time.Millisecond)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := c.at(tt.nanos); !got.Equal(tt.want) {
				t.Fatalf("at(%d) = %s, want %s", tt.nanos, got, tt.want)
			}
		})
	}
}

var (
	imprint = hidDevice{handle: 1, name: "Imprint (Patched)", vid: 0x4359, pid: 0x0000, version: 0x0022}
	other   = hidDevice{handle: 2, name: "Other Mouse", vid: 0x046d, pid: 0xc077}
)

func TestReportAssembler_GroupsValuesByReportTimestamp(t *testing.T) {
	var r reportAssembler

	if _, done := r.add(imprint, usageX, 5, 1000); done {
		t.Fatal("first value must not complete a report")
	}
	if _, done := r.add(imprint, usageY, -2, 1000); done {
		t.Fatal("value with the same timestamp must join the pending report")
	}

	got, done := r.add(imprint, usageX, 7, 2000)
	if !done {
		t.Fatal("value with a new timestamp must complete the previous report")
	}
	want := hidReport{dev: imprint, tsNanos: 1000, dx: 5, dy: -2}
	if got != want {
		t.Fatalf("completed report = %+v, want %+v", got, want)
	}

	rest := r.flush()
	if len(rest) != 1 || rest[0] != (hidReport{dev: imprint, tsNanos: 2000, dx: 7}) {
		t.Fatalf("flush = %+v, want the pending X-only report", rest)
	}
	if again := r.flush(); len(again) != 0 {
		t.Fatalf("second flush must be empty, got %+v", again)
	}
}

func TestReportAssembler_ScrollUsages(t *testing.T) {
	var r reportAssembler
	r.add(imprint, usageWheel, 1, 1000)
	r.add(imprint, usageACPan, -1, 1000)
	got := r.flush()
	if len(got) != 1 || got[0].wheel != 1 || got[0].pan != -1 {
		t.Fatalf("flush = %+v, want wheel 1 and pan -1", got)
	}
}

func TestReportAssembler_DevicesAreIndependent(t *testing.T) {
	var r reportAssembler
	r.add(imprint, usageX, 3, 1000)
	if _, done := r.add(other, usageX, 9, 1500); done {
		t.Fatal("a report from another device must not complete the Imprint report")
	}

	got := r.flush()
	if len(got) != 2 {
		t.Fatalf("expected 2 pending reports, got %+v", got)
	}
	if got[0].dev != imprint || got[1].dev != other {
		t.Fatalf("flush must return reports in timestamp order, got %+v", got)
	}
}

func TestReportAssembler_SkipsReportsWithoutMotion(t *testing.T) {
	var r reportAssembler
	r.add(imprint, usageX, 0, 1000)
	r.add(imprint, usageY, 0, 1000)
	if _, done := r.add(imprint, usageX, 4, 2000); done {
		t.Fatal("a report with no motion must not be emitted")
	}
	if got := r.flush(); len(got) != 1 || got[0].dx != 4 {
		t.Fatalf("flush = %+v, want only the moving report", got)
	}
}

func TestHIDReport_Event(t *testing.T) {
	c := monoClock{wall: time.Date(2026, 9, 24, 10, 0, 0, 0, time.UTC), nanos: 0}
	rep := hidReport{dev: imprint, tsNanos: 1_500_000, dx: 5, dy: -2, wheel: 1, pan: -1}

	ev := rep.event(42, c)
	if ev.ID != 42 || ev.DeviceName != imprint.name || ev.DeviceVID != imprint.vid || ev.DeviceVersion != imprint.version {
		t.Fatalf("device metadata not copied: %+v", ev)
	}
	if ev.DeltaX != 5 || ev.DeltaY != -2 || ev.Wheel != 1 || ev.Pan != -1 {
		t.Fatalf("motion not copied: %+v", ev)
	}
	if want := c.wall.Add(1500 * time.Microsecond); !ev.Timestamp.Equal(want) {
		t.Fatalf("timestamp = %s, want %s", ev.Timestamp, want)
	}
}
