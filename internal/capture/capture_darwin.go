//go:build darwin

package capture

/*
#cgo LDFLAGS: -framework IOKit -framework CoreFoundation -framework ApplicationServices
#include <CoreFoundation/CoreFoundation.h>
#include <IOKit/hid/IOHIDManager.h>
#include <ApplicationServices/ApplicationServices.h>
#include <mach/mach_time.h>
#include <stdlib.h>
#include <string.h>

extern void goCGEventCallback(double x, double y, int64_t dx, int64_t dy, uint64_t tsNano);
extern void goHIDValueCallback(uintptr_t devHandle, char *devName, uint32_t vid, uint32_t pid, uint32_t version, uint32_t page, uint32_t usage, int64_t val, uint64_t tsNano);

// IOHIDValueGetTimeStamp returns Mach absolute time ticks, which are only nanoseconds
// when the timebase is 1/1 (Intel).
static uint64_t machToNanos(uint64_t ticks) {
    static mach_timebase_info_data_t timebase;
    if (timebase.denom == 0) {
        mach_timebase_info(&timebase);
    }
    return ticks * timebase.numer / timebase.denom;
}

static uint64_t nowNanos(void) {
    return machToNanos(mach_absolute_time());
}

// CGEventGetTimestamp is documented as nanoseconds since startup (and returns that for
// posted events on macOS 26), but has been reported to return Mach ticks on Apple Silicon.
// The callback runs moments after the event, so the reading closer to the current uptime
// is the right one; the other is off by the timebase ratio (~41x on Apple Silicon).
static uint64_t cgTimestampToNanos(uint64_t ts) {
    uint64_t now = nowNanos();
    uint64_t fromTicks = machToNanos(ts);
    uint64_t distNanos = now > ts ? now - ts : ts - now;
    uint64_t distTicks = now > fromTicks ? now - fromTicks : fromTicks - now;
    return distTicks < distNanos ? fromTicks : ts;
}

static uint32_t deviceNumberProperty(IOHIDDeviceRef dev, CFStringRef key) {
    uint32_t out = 0;
    CFNumberRef num = (CFNumberRef)IOHIDDeviceGetProperty(dev, key);
    if (num) CFNumberGetValue(num, kCFNumberSInt32Type, &out);
    return out;
}

static CGEventRef cEventTapCallback(CGEventTapProxy proxy, CGEventType type, CGEventRef event, void *refcon) {
    if (type == kCGEventMouseMoved || type == kCGEventLeftMouseDragged ||
        type == kCGEventRightMouseDragged || type == kCGEventOtherMouseDragged) {
        CGPoint pt = CGEventGetLocation(event);
        int64_t dx = CGEventGetIntegerValueField(event, kCGMouseEventDeltaX);
        int64_t dy = CGEventGetIntegerValueField(event, kCGMouseEventDeltaY);
        goCGEventCallback(pt.x, pt.y, dx, dy, cgTimestampToNanos(CGEventGetTimestamp(event)));
    }
    return event;
}

static void cHIDValueCallback(void *context, IOReturn result, void *sender, IOHIDValueRef value) {
    IOHIDElementRef elem = IOHIDValueGetElement(value);
    uint32_t page = IOHIDElementGetUsagePage(elem);
    uint32_t usage = IOHIDElementGetUsage(elem);

    // X, Y, Wheel (Generic Desktop) and AC Pan (Consumer) carry pointer and scroll motion.
    if ((page == 0x01 && (usage == 0x30 || usage == 0x31 || usage == 0x38)) || (page == 0x0C && usage == 0x238)) {
        IOHIDDeviceRef dev = (IOHIDDeviceRef)sender;
        CFStringRef prod = (CFStringRef)IOHIDDeviceGetProperty(dev, CFSTR(kIOHIDProductKey));
        char name[128] = "Unknown Device";
        if (prod) {
            CFStringGetCString(prod, name, sizeof(name), kCFStringEncodingUTF8);
        }

        uint32_t vid = deviceNumberProperty(dev, CFSTR(kIOHIDVendorIDKey));
        uint32_t pid = deviceNumberProperty(dev, CFSTR(kIOHIDProductIDKey));
        uint32_t version = deviceNumberProperty(dev, CFSTR(kIOHIDVersionNumberKey));

        CFIndex intVal = IOHIDValueGetIntegerValue(value);
        uint64_t ts = machToNanos(IOHIDValueGetTimeStamp(value));

        goHIDValueCallback((uintptr_t)dev, name, vid, pid, version, page, usage, (int64_t)intVal, ts);
    }
}

static CFMachPortRef createEventTap(CFRunLoopSourceRef *sourceOut) {
    CGEventMask mask = CGEventMaskBit(kCGEventMouseMoved) |
                       CGEventMaskBit(kCGEventLeftMouseDragged) |
                       CGEventMaskBit(kCGEventRightMouseDragged) |
                       CGEventMaskBit(kCGEventOtherMouseDragged);

    CFMachPortRef tap = CGEventTapCreate(
        kCGHIDEventTap,
        kCGHeadInsertEventTap,
        kCGEventTapOptionListenOnly,
        mask,
        cEventTapCallback,
        NULL
    );
    if (!tap) return NULL;

    *sourceOut = CFMachPortCreateRunLoopSource(kCFAllocatorDefault, tap, 0);
    return tap;
}

static IOHIDManagerRef createAndOpenHIDManager() {
    IOHIDManagerRef manager = IOHIDManagerCreate(kCFAllocatorDefault, kIOHIDOptionsTypeNone);
    if (!manager) return NULL;

    CFMutableArrayRef matchArray = CFArrayCreateMutable(kCFAllocatorDefault, 2, &kCFTypeArrayCallBacks);
    int usages[2] = {2, 1}; // Mouse, Pointer
    for (int i = 0; i < 2; i++) {
        CFMutableDictionaryRef dict = CFDictionaryCreateMutable(kCFAllocatorDefault, 0,
            &kCFTypeDictionaryKeyCallBacks, &kCFTypeDictionaryValueCallBacks);
        int page = 1;
        int usage = usages[i];
        CFNumberRef pageNum = CFNumberCreate(kCFAllocatorDefault, kCFNumberIntType, &page);
        CFNumberRef usageNum = CFNumberCreate(kCFAllocatorDefault, kCFNumberIntType, &usage);
        CFDictionarySetValue(dict, CFSTR(kIOHIDDeviceUsagePageKey), pageNum);
        CFDictionarySetValue(dict, CFSTR(kIOHIDDeviceUsageKey), usageNum);
        CFRelease(pageNum);
        CFRelease(usageNum);
        CFArrayAppendValue(matchArray, dict);
        CFRelease(dict);
    }
    IOHIDManagerSetDeviceMatchingMultiple(manager, matchArray);
    CFRelease(matchArray);

    IOHIDManagerRegisterInputValueCallback(manager, cHIDValueCallback, NULL);
    if (IOHIDManagerOpen(manager, kIOHIDOptionsTypeNone) != kIOReturnSuccess) {
        CFRelease(manager);
        return NULL;
    }
    return manager;
}
*/
import "C"

import (
	"context"
	"fmt"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/alexgorbatchev/mouse-issues/internal/analyzer"
)

var (
	activeSession *darwinSession
	sessionMutex  sync.Mutex
)

func currentSession() *darwinSession {
	sessionMutex.Lock()
	defer sessionMutex.Unlock()
	return activeSession
}

//export goCGEventCallback
func goCGEventCallback(x, y float64, dx, dy int64, tsNano uint64) {
	s := currentSession()
	if s == nil || s.opts.Mode == ModeHID {
		return
	}

	s.nextID++
	s.handler(analyzer.Event{
		ID:        s.nextID,
		Timestamp: s.clock.at(tsNano),
		Source:    analyzer.SourceCG,
		CursorX:   x,
		CursorY:   y,
		DeltaX:    dx,
		DeltaY:    dy,
	})
}

//export goHIDValueCallback
func goHIDValueCallback(devHandle uintptr, devName *C.char, vid, pid, version, page, usage uint32, val int64, tsNano uint64) {
	s := currentSession()
	if s == nil || s.opts.Mode == ModeCG {
		return
	}

	dev := hidDevice{handle: devHandle, name: C.GoString(devName), vid: vid, pid: pid, version: version}
	if !s.matchesDevice(dev) {
		return
	}

	if completed, done := s.reports.add(dev, hidUsage(page<<16|usage), val, tsNano); done {
		s.emitHID(completed)
	}
}

// darwinSession callbacks all run on the thread that owns the run loop, inside
// CFRunLoopRunInMode called from Start, so its fields need no locking.
type darwinSession struct {
	opts    Options
	handler Handler
	nextID  uint64
	clock   monoClock
	reports reportAssembler
}

func (s *darwinSession) matchesDevice(dev hidDevice) bool {
	if s.opts.DeviceFilter == "" {
		return true
	}
	filter := strings.ToLower(s.opts.DeviceFilter)
	return strings.Contains(strings.ToLower(dev.name), filter) ||
		strings.Contains(fmt.Sprintf("%04x:%04x", dev.vid, dev.pid), filter)
}

func (s *darwinSession) emitHID(r hidReport) {
	s.nextID++
	s.handler(r.event(s.nextID, s.clock))
}

func (s *darwinSession) flushHID() {
	for _, r := range s.reports.flush() {
		s.emitHID(r)
	}
}

// NewSession creates a Darwin native capture session.
func NewSession(opts Options) Session {
	if opts.Mode == "" {
		opts.Mode = ModeBoth
	}
	return &darwinSession{opts: opts}
}

// Start runs the event loop on macOS until ctx is cancelled.
func (s *darwinSession) Start(ctx context.Context, handler Handler) error {
	// The run loop, its sources, and every callback belong to one OS thread; without this
	// the goroutine could resume on another thread and run a run loop with no sources.
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	s.clock = monoClock{wall: time.Now(), nanos: uint64(C.nowNanos())}

	sessionMutex.Lock()
	if activeSession != nil {
		sessionMutex.Unlock()
		return fmt.Errorf("another capture session is already active")
	}
	s.handler = handler
	activeSession = s
	sessionMutex.Unlock()

	defer func() {
		sessionMutex.Lock()
		activeSession = nil
		sessionMutex.Unlock()
	}()

	var runLoop = C.CFRunLoopGetCurrent()
	var cgTap C.CFMachPortRef
	var cgSource C.CFRunLoopSourceRef
	var hidManager C.IOHIDManagerRef

	if s.opts.Mode == ModeCG || s.opts.Mode == ModeBoth {
		cgTap = C.createEventTap(&cgSource)
		if cgTap != 0 && cgSource != 0 {
			C.CFRunLoopAddSource(runLoop, cgSource, C.kCFRunLoopDefaultMode)
			C.CGEventTapEnable(cgTap, C.bool(true))
			defer func() {
				C.CGEventTapEnable(cgTap, C.bool(false))
				C.CFRunLoopRemoveSource(runLoop, cgSource, C.kCFRunLoopDefaultMode)
				C.CFRelease(C.CFTypeRef(cgSource))
				C.CFRelease(C.CFTypeRef(cgTap))
			}()
		}
	}

	if s.opts.Mode == ModeHID || s.opts.Mode == ModeBoth {
		hidManager = C.createAndOpenHIDManager()
		if hidManager != 0 {
			C.IOHIDManagerScheduleWithRunLoop(hidManager, runLoop, C.kCFRunLoopDefaultMode)
			defer func() {
				C.IOHIDManagerUnscheduleFromRunLoop(hidManager, runLoop, C.kCFRunLoopDefaultMode)
				C.IOHIDManagerClose(hidManager, C.kIOHIDOptionsTypeNone)
				C.CFRelease(C.CFTypeRef(hidManager))
			}()
		}
	}

	if cgTap == 0 && hidManager == 0 {
		return fmt.Errorf("failed to initialize both CoreGraphics EventTap and IOHIDManager (check Accessibility permissions)")
	}

	// Run the run loop in short slices to check context cancellation, emitting the
	// HID reports completed during each slice.
	for {
		select {
		case <-ctx.Done():
			s.flushHID()
			return nil
		default:
			C.CFRunLoopRunInMode(C.kCFRunLoopDefaultMode, 0.05, C.false)
			s.flushHID()
		}
	}
}
