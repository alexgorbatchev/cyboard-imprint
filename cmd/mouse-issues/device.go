package main

import (
	"fmt"
	"strings"

	"github.com/alexgorbatchev/mouse-issues/internal/agent"
	"github.com/alexgorbatchev/mouse-issues/internal/device"
	"github.com/spf13/cobra"
)

func newDeviceCommand() *cobra.Command {
	deviceCmd := &cobra.Command{
		Use:   "device",
		Short: "Inspect pointing devices and connected displays",
		Long:  "Commands for discovering and inspecting HID pointing devices, trackball descriptors, and monitor layouts.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}

	deviceCmd.AddCommand(newDeviceListCommand())
	deviceCmd.AddCommand(newDeviceInspectCommand())

	return deviceCmd
}

func newDeviceListCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List connected pointing devices and active displays",
		Long:  "Enumerate all USB/Bluetooth pointing devices and active monitor boundaries.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			devices, devErr := device.ListPointingDevices()
			displays, dispErr := device.ListDisplays()

			out := cmd.OutOrStdout()
			isAgent := agent.IsAgentMode()

			if isAgent {
				fmt.Fprintln(out, "displays:")
				if dispErr != nil {
					fmt.Fprintf(out, "  error: %s\n", dispErr.Error())
				} else {
					for _, d := range displays {
						fmt.Fprintf(out, "  - id: %d\n    main: %v\n    origin_x: %.0f\n    origin_y: %.0f\n    width: %.0f\n    height: %.0f\n",
							d.ID, d.IsMain, d.Bounds.X, d.Bounds.Y, d.Bounds.Width, d.Bounds.Height)
					}
				}

				fmt.Fprintln(out, "pointing_devices:")
				if devErr != nil {
					fmt.Fprintf(out, "  error: %s\n", devErr.Error())
				} else {
					for _, dev := range devices {
						fmt.Fprintf(out, "  - name: %s\n    manufacturer: %s\n    vid: 0x%04x\n    pid: 0x%04x\n    version: %s\n    serial: %s\n    transport: %s\n",
							dev.Name, dev.Manufacturer, dev.VendorID, dev.ProductID, device.FormatBCDVersion(dev.VersionNumber), dev.SerialNumber, dev.Transport)
					}
				}
				return nil
			}

			// Human Mode
			fmt.Fprintln(out, "Active Displays:")
			if dispErr != nil {
				fmt.Fprintf(out, "  [ERROR] Failed to query displays: %v\n", dispErr)
			} else if len(displays) == 0 {
				fmt.Fprintln(out, "  No displays detected.")
			} else {
				for i, d := range displays {
					mainTag := ""
					if d.IsMain {
						mainTag = " [MAIN]"
					}
					fmt.Fprintf(out, "  Display %d (ID: %d)%s: bounds=(%.0f, %.0f) size=(%.0f x %.0f)\n",
						i, d.ID, mainTag, d.Bounds.X, d.Bounds.Y, d.Bounds.Width, d.Bounds.Height)
				}
			}

			fmt.Fprintln(out, "")
			fmt.Fprintln(out, "Connected Pointing Devices:")
			if devErr != nil {
				fmt.Fprintf(out, "  [ERROR] Failed to query pointing devices: %v\n", devErr)
			} else if len(devices) == 0 {
				fmt.Fprintln(out, "  No pointing devices detected.")
			} else {
				for i, dev := range devices {
					isKeyboardTrackball := strings.Contains(strings.ToLower(dev.Name), "imprint") ||
						strings.Contains(strings.ToLower(dev.Manufacturer), "cyboard") ||
						strings.Contains(strings.ToLower(dev.SerialNumber), "vial")

					badge := ""
					if isKeyboardTrackball {
						badge = " [KEYBOARD TRACKBALL]"
					}

					fmt.Fprintf(out, "  [%d] %s%s\n", i+1, dev.Name, badge)
					fmt.Fprintf(out, "      Manufacturer : %s\n", dev.Manufacturer)
					fmt.Fprintf(out, "      Vendor ID    : 0x%04x\n", dev.VendorID)
					fmt.Fprintf(out, "      Product ID   : 0x%04x\n", dev.ProductID)
					fmt.Fprintf(out, "      Version      : %s\n", device.FormatBCDVersion(dev.VersionNumber))
					if dev.SerialNumber != "" {
						fmt.Fprintf(out, "      Serial Number: %s\n", dev.SerialNumber)
					}
					if dev.Transport != "" {
						fmt.Fprintf(out, "      Transport    : %s\n", dev.Transport)
					}
				}
			}

			return nil
		},
	}
}

func newDeviceInspectCommand() *cobra.Command {
	var (
		targetDevice string
		showAll      bool
	)

	cmd := &cobra.Command{
		Use:   "inspect [device]",
		Short: "Inspect HID report descriptor elements for a pointing device",
		Long:  "Deeply inspects report IDs, element sizes, counts, logical min/max values, and firmware coordinate constraints.",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			query := targetDevice
			if len(args) > 0 {
				query = args[0]
			}

			dev, err := device.InspectDevice(query)
			if err != nil {
				return fmt.Errorf("inspecting device %q: %w", query, err)
			}

			out := cmd.OutOrStdout()
			isAgent := agent.IsAgentMode()

			if isAgent {
				fmt.Fprintf(out, "device:\n  name: %s\n  manufacturer: %s\n  vid: 0x%04x\n  pid: 0x%04x\n  version: %s\n  serial: %s\n",
					dev.Name, dev.Manufacturer, dev.VendorID, dev.ProductID, device.FormatBCDVersion(dev.VersionNumber), dev.SerialNumber)
				fmt.Fprintln(out, "elements:")
				for _, e := range dev.Elements {
					if !showAll && e.UsagePage != 1 && e.UsagePage != 9 && e.UsagePage != 0xc {
						continue
					}
					fmt.Fprintf(out, "  - page: 0x%x\n    usage: 0x%x\n    name: %s\n    report_id: %d\n    size_bits: %d\n    min: %d\n    max: %d\n",
						e.UsagePage, e.Usage, e.UsageName, e.ReportID, e.ReportSize, e.LogicalMin, e.LogicalMax)
				}
				return nil
			}

			// Human Mode
			fmt.Fprintf(out, "Device: %s\n", dev.Name)
			fmt.Fprintf(out, "  Manufacturer : %s\n", dev.Manufacturer)
			fmt.Fprintf(out, "  Vendor ID    : 0x%04x\n", dev.VendorID)
			fmt.Fprintf(out, "  Product ID   : 0x%04x\n", dev.ProductID)
			fmt.Fprintf(out, "  Version      : %s\n", device.FormatBCDVersion(dev.VersionNumber))
			if dev.SerialNumber != "" {
				fmt.Fprintf(out, "  Serial Number: %s\n", dev.SerialNumber)
			}
			fmt.Fprintln(out, "")
			fmt.Fprintln(out, "Pointing & Mouse HID Report Elements:")

			has8BitDeltas := false
			otherCount := 0
			for _, e := range dev.Elements {
				if e.UsagePage == 1 && (e.Usage == 0x30 || e.Usage == 0x31) {
					if e.ReportSize == 8 && e.LogicalMin == -127 && e.LogicalMax == 127 {
						has8BitDeltas = true
					}
				}

				isPointerElem := (e.UsagePage == 1 && (e.Usage == 0x30 || e.Usage == 0x31 || e.Usage == 0x38 || e.Usage == 1 || e.Usage == 2)) ||
					(e.UsagePage == 9 && e.Usage <= 16) ||
					(e.UsagePage == 0xc && e.Usage == 0x238)

				if !showAll && !isPointerElem {
					otherCount++
					continue
				}

				name := e.UsageName
				if name == "" {
					name = fmt.Sprintf("Page 0x%x Usage 0x%x", e.UsagePage, e.Usage)
				}
				fmt.Fprintf(out, "  * %-30s : ReportID=%d, Size=%2d bits, LogMin=%-4d, LogMax=%-4d\n",
					name, e.ReportID, e.ReportSize, e.LogicalMin, e.LogicalMax)
			}

			if otherCount > 0 && !showAll {
				fmt.Fprintf(out, "  (and %d other consumer/keyboard elements omitted; use --all to view all)\n", otherCount)
			}

			if has8BitDeltas {
				fmt.Fprintln(out, "")
				fmt.Fprintln(out, "[NOTE] X/Y Motion Descriptors are 8-bit signed (-127 to +127):")
				fmt.Fprintln(out, "  One report carries at most 127 counts per axis and QMK clamps larger motion")
				fmt.Fprintln(out, "  to that limit. Deltas pinned at +/-127 in a capture (report_saturation) mean")
				fmt.Fprintln(out, "  the sensor produces more counts per report than fit, typically a too-high DPI.")
			}

			return nil
		},
	}

	cmd.Flags().StringVarP(&targetDevice, "device", "d", "", "Filter device by name, VID:PID, or serial")
	cmd.Flags().BoolVarP(&showAll, "all", "a", false, "Show all HID elements including raw keyboard scan codes")
	return cmd
}
