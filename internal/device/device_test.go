package device

import (
	"testing"
)

func TestGetUsageName(t *testing.T) {
	tests := []struct {
		page     uint32
		usage    uint32
		expected string
	}{
		{page: 0x1, usage: 0x30, expected: "X (Pointer X-axis Delta)"},
		{page: 0x1, usage: 0x31, expected: "Y (Pointer Y-axis Delta)"},
		{page: 0x9, usage: 1, expected: "Button 1"},
		{page: 0xc, usage: 0x238, expected: "AC Pan (Horizontal Scroll)"},
		{page: 0x99, usage: 0x99, expected: ""},
	}

	for _, tt := range tests {
		got := getUsageName(tt.page, tt.usage)
		if got != tt.expected {
			t.Errorf("getUsageName(0x%x, 0x%x) = %q, want %q", tt.page, tt.usage, got, tt.expected)
		}
	}
}

func TestListDisplays(t *testing.T) {
	displays, err := ListDisplays()
	if err != nil {
		t.Fatalf("ListDisplays error: %v", err)
	}
	if len(displays) == 0 {
		t.Logf("no displays returned (may happen in headless CI)")
		return
	}
	for i, d := range displays {
		t.Logf("Display %d: ID=%d Main=%v Bounds=(%.0f, %.0f, %.0f x %.0f)",
			i, d.ID, d.IsMain, d.Bounds.X, d.Bounds.Y, d.Bounds.Width, d.Bounds.Height)
	}
}

func TestListPointingDevices(t *testing.T) {
	devices, err := ListPointingDevices()
	if err != nil {
		t.Fatalf("ListPointingDevices error: %v", err)
	}
	if len(devices) == 0 {
		t.Logf("no pointing devices found")
		return
	}
	for _, dev := range devices {
		t.Logf("Device: %s [%s] VID=0x%04x PID=0x%04x Serial=%s",
			dev.Name, dev.Manufacturer, dev.VendorID, dev.ProductID, dev.SerialNumber)
	}
}
