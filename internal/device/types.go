package device

import (
	"fmt"

	"github.com/alexgorbatchev/mouse-issues/internal/analyzer"
)

// ElementInfo describes a single HID report descriptor element.
type ElementInfo struct {
	Type        uint32 `json:"type"`
	UsagePage   uint32 `json:"usage_page"`
	Usage       uint32 `json:"usage"`
	ReportID    uint32 `json:"report_id"`
	ReportSize  uint32 `json:"report_size"`
	ReportCount uint32 `json:"report_count"`
	LogicalMin  int64  `json:"logical_min"`
	LogicalMax  int64  `json:"logical_max"`
	UsageName   string `json:"usage_name,omitempty"`
}

// Info holds metadata for a connected pointing device.
type Info struct {
	ID            string        `json:"id"`
	Name          string        `json:"name"`
	Manufacturer  string        `json:"manufacturer"`
	VendorID      uint32        `json:"vendor_id"`
	ProductID     uint32        `json:"product_id"`
	VersionNumber uint32        `json:"version_number"` // USB bcdDevice (QMK usb.device_version)
	SerialNumber  string        `json:"serial_number"`
	LocationID    uint32        `json:"location_id"`
	Transport     string        `json:"transport"`
	Elements      []ElementInfo `json:"elements,omitempty"`
}

// Display is an alias to analyzer.Display for convenience.
type Display = analyzer.Display

// FormatBCDVersion renders a USB bcdDevice value as major.minor.revision, the encoding
// QMK uses for usb.device_version (e.g. "0.2.2" is stored as 0x0022). Zero means the
// device or recording carried no version and renders as "unknown".
func FormatBCDVersion(bcd uint32) string {
	if bcd == 0 {
		return "unknown"
	}
	major := (bcd>>12&0xF)*10 + (bcd >> 8 & 0xF)
	return fmt.Sprintf("%d.%d.%d", major, bcd>>4&0xF, bcd&0xF)
}
