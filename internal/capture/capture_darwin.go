//go:build darwin

package capture

/*
#cgo LDFLAGS: -framework IOKit -framework CoreFoundation -framework ApplicationServices
#include <CoreFoundation/CoreFoundation.h>
#include <IOKit/hid/IOHIDManager.h>
#include <ApplicationServices/ApplicationServices.h>
#include <stdlib.h>
#include <string.h>

extern void goCGEventCallback(double x, double y, int64_t dx, int64_t dy, uint64_t tsNano);
extern void goHIDValueCallback(uintptr_t devHandle, char *devName, uint32_t vid, uint32_t pid, uint32_t page, uint32_t usage, int64_t val, uint64_t tsNano);

static CGEventRef cEventTapCallback(CGEventTapProxy proxy, CGEventType type, CGEventRef event, void *refcon) {
    if (type == kCGEventMouseMoved || type == kCGEventLeftMouseDragged ||
        type == kCGEventRightMouseDragged || type == kCGEventOtherMouseDragged) {
        CGPoint pt = CGEventGetLocation(event);
        int64_t dx = CGEventGetIntegerValueField(event, kCGMouseEventDeltaX);
        int64_t dy = CGEventGetIntegerValueField(event, kCGMouseEventDeltaY);
        uint64_t ts = CGEventGetTimestamp(event);
        goCGEventCallback(pt.x, pt.y, dx, dy, ts);
    }
    return event;
}

static void cHIDValueCallback(void *context, IOReturn result, void *sender, IOHIDValueRef value) {
    IOHIDElementRef elem = IOHIDValueGetElement(value);
    uint32_t page = IOHIDElementGetUsagePage(elem);
    uint32_t usage = IOHIDElementGetUsage(elem);

    if (page == 1 && (usage == 0x30 || usage == 0x31 || usage == 0x38)) {
        IOHIDDeviceRef dev = (IOHIDDeviceRef)sender;
        CFStringRef prod = (CFStringRef)IOHIDDeviceGetProperty(dev, CFSTR(kIOHIDProductKey));
        char name[128] = "Unknown Device";
        if (prod) {
            CFStringGetCString(prod, name, sizeof(name), kCFStringEncodingUTF8);
        }

        uint32_t vid = 0, pid = 0;
        CFNumberRef vidNum = (CFNumberRef)IOHIDDeviceGetProperty(dev, CFSTR(kIOHIDVendorIDKey));
        CFNumberRef pidNum = (CFNumberRef)IOHIDDeviceGetProperty(dev, CFSTR(kIOHIDProductIDKey));
        if (vidNum) CFNumberGetValue(vidNum, kCFNumberSInt32Type, &vid);
        if (pidNum) CFNumberGetValue(pidNum, kCFNumberSInt32Type, &pid);

        CFIndex intVal = IOHIDValueGetIntegerValue(value);
        uint64_t ts = IOHIDValueGetTimeStamp(value);

        goHIDValueCallback((uintptr_t)dev, name, vid, pid, page, usage, (int64_t)intVal, ts);
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
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/alexgorbatchev/mouse-issues/internal/analyzer"
)

var (
	activeSession *darwinSession
	sessionMutex  sync.Mutex
)

//export goCGEventCallback
func goCGEventCallback(x, y float64, dx, dy int64, tsNano uint64) {
	sessionMutex.Lock()
	s := activeSession
	sessionMutex.Unlock()
	if s == nil || s.handler == nil {
		return
	}

	if s.opts.Mode == ModeHID {
		return
	}

	id := atomic.AddUint64(&s.nextID, 1)
	event := analyzer.Event{
		ID:        id,
		Timestamp: time.Now(),
		Source:    analyzer.SourceCG,
		CursorX:   x,
		CursorY:   y,
		DeltaX:    dx,
		DeltaY:    dy,
	}
	s.handler(event)
}

type double = float64

type pendingDelta struct {
	hasX      bool
	hasY      bool
	dx        int64
	dy        int64
	name      string
	vid       uint32
	pid       uint32
	lastStamp time.Time
}

//export goHIDValueCallback
func goHIDValueCallback(devHandle uintptr, devName *C.char, vid, pid, page, usage uint32, val int64, tsNano uint64) {
	sessionMutex.Lock()
	s := activeSession
	sessionMutex.Unlock()
	if s == nil || s.handler == nil {
		return
	}

	if s.opts.Mode == ModeCG {
		return
	}

	name := C.GoString(devName)
	if s.opts.DeviceFilter != "" {
		filter := strings.ToLower(s.opts.DeviceFilter)
		if !strings.Contains(strings.ToLower(name), filter) &&
			!strings.Contains(fmt.Sprintf("%04x:%04x", vid, pid), filter) {
			return
		}
	}

	s.hidStateLock.Lock()
	state, exists := s.hidState[devHandle]
	if !exists {
		state = &pendingDelta{
			name:      name,
			vid:       vid,
			pid:       pid,
			lastStamp: time.Now(),
		}
		s.hidState[devHandle] = state
	}

	if usage == 0x30 {
		state.dx = val
		state.hasX = true
	} else if usage == 0x31 {
		state.dy = val
		state.hasY = true
	}

	now := time.Now()
	// Emit if we received both X and Y or interval has elapsed
	shouldEmit := (state.hasX && state.hasY) || now.Sub(state.lastStamp) > 2*time.Millisecond
	if shouldEmit {
		id := atomic.AddUint64(&s.nextID, 1)
		ev := analyzer.Event{
			ID:         id,
			Timestamp:  now,
			Source:     analyzer.SourceHID,
			DeviceName: state.name,
			DeviceVID:  state.vid,
			DevicePID:  state.pid,
			DeltaX:     state.dx,
			DeltaY:     state.dy,
		}
		state.hasX = false
		state.hasY = false
		state.dx = 0
		state.dy = 0
		state.lastStamp = now
		s.hidStateLock.Unlock()

		s.handler(ev)
		return
	}

	s.hidStateLock.Unlock()
}

type darwinSession struct {
	opts         Options
	handler      Handler
	nextID       uint64
	hidStateLock sync.Mutex
	hidState     map[uintptr]*pendingDelta
}

// NewSession creates a Darwin native capture session.
func NewSession(opts Options) Session {
	if opts.Mode == "" {
		opts.Mode = ModeBoth
	}
	return &darwinSession{
		opts:     opts,
		hidState: make(map[uintptr]*pendingDelta),
	}
}

// Start runs the event loop on macOS until ctx is cancelled.
func (s *darwinSession) Start(ctx context.Context, handler Handler) error {
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

	// Main loop: run runloop in short slices to check context cancellation
	for {
		select {
		case <-ctx.Done():
			return nil
		default:
			C.CFRunLoopRunInMode(C.kCFRunLoopDefaultMode, 0.05, C.false)
		}
	}
}
