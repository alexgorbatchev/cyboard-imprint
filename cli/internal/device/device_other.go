//go:build !darwin

package device

import (
	"fmt"

	"github.com/alexgorbatchev/mouse-issues/internal/analyzer"
)

// ListPointingDevices returns empty list on non-Darwin platforms.
func ListPointingDevices() ([]Info, error) {
	return nil, fmt.Errorf("device enumeration is currently supported on macOS")
}

// InspectDevice returns error on non-Darwin platforms.
func InspectDevice(query string) (*Info, error) {
	return nil, fmt.Errorf("device inspection is currently supported on macOS")
}

// ListDisplays returns stub display list on non-Darwin platforms.
func ListDisplays() ([]analyzer.Display, error) {
	return nil, fmt.Errorf("display enumeration is currently supported on macOS")
}
