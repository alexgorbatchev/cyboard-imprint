//go:build darwin

package device

/*
#cgo LDFLAGS: -framework IOKit -framework CoreFoundation -framework ApplicationServices
#include <CoreFoundation/CoreFoundation.h>
#include <IOKit/hid/IOHIDManager.h>
#include <ApplicationServices/ApplicationServices.h>
#include <stdlib.h>
#include <string.h>

typedef struct {
    char name[128];
    char manufacturer[128];
    char serial[128];
    char transport[64];
    uint32_t vendorID;
    uint32_t productID;
    uint32_t locationID;
} C_DeviceInfo;

static void setMouseMatchingCriteria(IOHIDManagerRef manager) {
    CFMutableArrayRef matchArray = CFArrayCreateMutable(kCFAllocatorDefault, 2, &kCFTypeArrayCallBacks);
    int usages[2] = {2, 1}; // Mouse (2), Pointer (1)
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
}

static void extractDeviceProps(IOHIDDeviceRef dev, C_DeviceInfo *info) {
    memset(info, 0, sizeof(C_DeviceInfo));

    CFStringRef prod = (CFStringRef)IOHIDDeviceGetProperty(dev, CFSTR(kIOHIDProductKey));
    CFStringRef man = (CFStringRef)IOHIDDeviceGetProperty(dev, CFSTR(kIOHIDManufacturerKey));
    CFStringRef ser = (CFStringRef)IOHIDDeviceGetProperty(dev, CFSTR(kIOHIDSerialNumberKey));
    CFStringRef trans = (CFStringRef)IOHIDDeviceGetProperty(dev, CFSTR(kIOHIDTransportKey));
    CFNumberRef vid = (CFNumberRef)IOHIDDeviceGetProperty(dev, CFSTR(kIOHIDVendorIDKey));
    CFNumberRef pid = (CFNumberRef)IOHIDDeviceGetProperty(dev, CFSTR(kIOHIDProductIDKey));
    CFNumberRef loc = (CFNumberRef)IOHIDDeviceGetProperty(dev, CFSTR(kIOHIDLocationIDKey));

    if (prod) CFStringGetCString(prod, info->name, sizeof(info->name), kCFStringEncodingUTF8);
    if (man) CFStringGetCString(man, info->manufacturer, sizeof(info->manufacturer), kCFStringEncodingUTF8);
    if (ser) CFStringGetCString(ser, info->serial, sizeof(info->serial), kCFStringEncodingUTF8);
    if (trans) CFStringGetCString(trans, info->transport, sizeof(info->transport), kCFStringEncodingUTF8);

    if (vid) CFNumberGetValue(vid, kCFNumberSInt32Type, &info->vendorID);
    if (pid) CFNumberGetValue(pid, kCFNumberSInt32Type, &info->productID);
    if (loc) CFNumberGetValue(loc, kCFNumberSInt32Type, &info->locationID);
}
*/
import "C"

import (
	"fmt"
	"strings"
	"unsafe"

	"github.com/alexgorbatchev/mouse-issues/internal/analyzer"
)

// ListPointingDevices returns all pointing devices connected to the system.
func ListPointingDevices() ([]Info, error) {
	manager := C.IOHIDManagerCreate(C.kCFAllocatorDefault, C.kIOHIDOptionsTypeNone)
	if manager == 0 {
		return nil, fmt.Errorf("failed to create IOHIDManager")
	}
	defer C.CFRelease(C.CFTypeRef(manager))

	C.setMouseMatchingCriteria(manager)
	ret := C.IOHIDManagerOpen(manager, C.kIOHIDOptionsTypeNone)
	if ret != C.kIOReturnSuccess {
		return nil, fmt.Errorf("IOHIDManagerOpen returned error 0x%x", uint32(ret))
	}
	defer C.IOHIDManagerClose(manager, C.kIOHIDOptionsTypeNone)

	devSet := C.IOHIDManagerCopyDevices(manager)
	if devSet == 0 {
		return nil, nil
	}
	defer C.CFRelease(C.CFTypeRef(devSet))

	count := int(C.CFSetGetCount(devSet))
	if count == 0 {
		return nil, nil
	}

	devices := make([]unsafe.Pointer, count)
	C.CFSetGetValues(devSet, (*unsafe.Pointer)(unsafe.Pointer(&devices[0])))

	seen := make(map[string]bool)
	var result []Info

	for _, devPtr := range devices {
		dev := C.IOHIDDeviceRef(devPtr)
		var cInfo C.C_DeviceInfo
		C.extractDeviceProps(dev, &cInfo)

		name := C.GoString(&cInfo.name[0])
		if name == "" {
			name = "Pointing Device"
		}
		man := C.GoString(&cInfo.manufacturer[0])
		serial := C.GoString(&cInfo.serial[0])
		trans := C.GoString(&cInfo.transport[0])
		key := fmt.Sprintf("%04x:%04x:%08x:%s", cInfo.vendorID, cInfo.productID, cInfo.locationID, name)

		if seen[key] {
			continue
		}
		seen[key] = true

		info := Info{
			ID:           key,
			Name:         name,
			Manufacturer: man,
			VendorID:     uint32(cInfo.vendorID),
			ProductID:    uint32(cInfo.productID),
			SerialNumber: serial,
			LocationID:   uint32(cInfo.locationID),
			Transport:    trans,
		}
		result = append(result, info)
	}

	return result, nil
}

// InspectDevice finds a device matching query and loads all its HID elements.
func InspectDevice(query string) (*Info, error) {
	manager := C.IOHIDManagerCreate(C.kCFAllocatorDefault, C.kIOHIDOptionsTypeNone)
	if manager == 0 {
		return nil, fmt.Errorf("failed to create IOHIDManager")
	}
	defer C.CFRelease(C.CFTypeRef(manager))

	C.setMouseMatchingCriteria(manager)
	if ret := C.IOHIDManagerOpen(manager, C.kIOHIDOptionsTypeNone); ret != C.kIOReturnSuccess {
		return nil, fmt.Errorf("failed to open IOHIDManager: 0x%x", uint32(ret))
	}
	defer C.IOHIDManagerClose(manager, C.kIOHIDOptionsTypeNone)

	devSet := C.IOHIDManagerCopyDevices(manager)
	if devSet == 0 {
		return nil, fmt.Errorf("no pointing devices found")
	}
	defer C.CFRelease(C.CFTypeRef(devSet))

	count := int(C.CFSetGetCount(devSet))
	devices := make([]unsafe.Pointer, count)
	C.CFSetGetValues(devSet, (*unsafe.Pointer)(unsafe.Pointer(&devices[0])))

	var matchedDev C.IOHIDDeviceRef
	var matchedInfo Info

	queryLower := strings.ToLower(query)
	for _, devPtr := range devices {
		dev := C.IOHIDDeviceRef(devPtr)
		var cInfo C.C_DeviceInfo
		C.extractDeviceProps(dev, &cInfo)

		name := C.GoString(&cInfo.name[0])
		man := C.GoString(&cInfo.manufacturer[0])
		serial := C.GoString(&cInfo.serial[0])
		trans := C.GoString(&cInfo.transport[0])
		id := fmt.Sprintf("%04x:%04x:%08x:%s", cInfo.vendorID, cInfo.productID, cInfo.locationID, name)

		matches := false
		if query == "" {
			// default to non-Apple device if available, else first
			if !strings.Contains(strings.ToLower(name), "apple") {
				matches = true
			}
		} else {
			matches = strings.Contains(strings.ToLower(name), queryLower) ||
				strings.Contains(strings.ToLower(id), queryLower) ||
				strings.Contains(strings.ToLower(man), queryLower)
		}

		if matches {
			matchedDev = dev
			matchedInfo = Info{
				ID:           id,
				Name:         name,
				Manufacturer: man,
				VendorID:     uint32(cInfo.vendorID),
				ProductID:    uint32(cInfo.productID),
				SerialNumber: serial,
				LocationID:   uint32(cInfo.locationID),
				Transport:    trans,
			}
			break
		}
	}

	if matchedDev == 0 && count > 0 && query == "" {
		matchedDev = C.IOHIDDeviceRef(devices[0])
		var cInfo C.C_DeviceInfo
		C.extractDeviceProps(matchedDev, &cInfo)
		matchedInfo = Info{
			Name:         C.GoString(&cInfo.name[0]),
			Manufacturer: C.GoString(&cInfo.manufacturer[0]),
			VendorID:     uint32(cInfo.vendorID),
			ProductID:    uint32(cInfo.productID),
			SerialNumber: C.GoString(&cInfo.serial[0]),
			LocationID:   uint32(cInfo.locationID),
			Transport:    C.GoString(&cInfo.transport[0]),
		}
	}

	if matchedDev == 0 {
		return nil, fmt.Errorf("no matching device found for query %q", query)
	}

	// Copy and parse matching elements
	elements := C.IOHIDDeviceCopyMatchingElements(matchedDev, 0, C.kIOHIDOptionsTypeNone)
	if elements != 0 {
		defer C.CFRelease(C.CFTypeRef(elements))
		elemCount := int(C.CFArrayGetCount(elements))
		for i := 0; i < elemCount; i++ {
			elem := C.IOHIDElementRef(C.CFArrayGetValueAtIndex(elements, C.CFIndex(i)))
			eType := uint32(C.IOHIDElementGetType(elem))
			page := uint32(C.IOHIDElementGetUsagePage(elem))
			usage := uint32(C.IOHIDElementGetUsage(elem))
			repID := uint32(C.IOHIDElementGetReportID(elem))
			size := uint32(C.IOHIDElementGetReportSize(elem))
			rCount := uint32(C.IOHIDElementGetReportCount(elem))
			min := int64(C.IOHIDElementGetLogicalMin(elem))
			max := int64(C.IOHIDElementGetLogicalMax(elem))

			usageName := getUsageName(page, usage)

			matchedInfo.Elements = append(matchedInfo.Elements, ElementInfo{
				Type:        eType,
				UsagePage:   page,
				Usage:       usage,
				ReportID:    repID,
				ReportSize:  size,
				ReportCount: rCount,
				LogicalMin:  min,
				LogicalMax:  max,
				UsageName:   usageName,
			})
		}
	}

	return &matchedInfo, nil
}

// ListDisplays queries online CoreGraphics displays and their rects.
func ListDisplays() ([]analyzer.Display, error) {
	var displays [16]C.CGDirectDisplayID
	var displayCount C.CGDisplayCount

	ret := C.CGGetOnlineDisplayList(16, &displays[0], &displayCount)
	if ret != C.kCGErrorSuccess {
		return nil, fmt.Errorf("CGGetOnlineDisplayList error %d", ret)
	}

	result := make([]analyzer.Display, 0, int(displayCount))
	for i := 0; i < int(displayCount); i++ {
		dispID := displays[i]
		bounds := C.CGDisplayBounds(dispID)
		isMain := C.CGDisplayIsMain(dispID) != 0

		result = append(result, analyzer.Display{
			ID: uint32(dispID),
			Bounds: analyzer.Rect{
				X:      float64(bounds.origin.x),
				Y:      float64(bounds.origin.y),
				Width:  float64(bounds.size.width),
				Height: float64(bounds.size.height),
			},
			IsMain: isMain,
		})
	}
	return result, nil
}

func getUsageName(page, usage uint32) string {
	switch page {
	case 0x1: // Generic Desktop
		switch usage {
		case 0x1:
			return "Pointer"
		case 0x2:
			return "Mouse"
		case 0x30:
			return "X (Pointer X-axis Delta)"
		case 0x31:
			return "Y (Pointer Y-axis Delta)"
		case 0x32:
			return "Z"
		case 0x38:
			return "Wheel (Vertical Scroll)"
		}
	case 0x9: // Button
		return fmt.Sprintf("Button %d", usage)
	case 0xc: // Consumer
		switch usage {
		case 0x238:
			return "AC Pan (Horizontal Scroll)"
		}
	}
	return ""
}
