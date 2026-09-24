package capture

import (
	"context"

	"github.com/alexgorbatchev/mouse-issues/internal/analyzer"
)

// Mode determines which capture layer to listen on.
type Mode string

const (
	ModeBoth Mode = "both" // CoreGraphics cursor coordinates + HID raw deltas
	ModeHID  Mode = "hid"  // Raw unaccelerated USB HID deltas from hardware
	ModeCG   Mode = "cg"   // WindowServer / CoreGraphics post-acceleration events
)

// Options configures the event listener.
type Options struct {
	Mode         Mode
	DeviceFilter string // Filter by device name, VID:PID, or substring (empty = all)
}

// Handler is invoked on each received pointing event.
type Handler func(event analyzer.Event)

// Session manages the lifecycle of an active capture stream.
type Session interface {
	Start(ctx context.Context, handler Handler) error
}
