package main

import (
	"fmt"
	"strings"

	"github.com/alexgorbatchev/mouse-issues/internal/agent"
	"github.com/alexgorbatchev/mouse-issues/internal/device"
	"github.com/alexgorbatchev/mouse-issues/internal/firmware"
	"github.com/spf13/cobra"
)

func newFirmwareCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "firmware",
		Short: "Inspect and validate keyboard firmware binaries",
		Long:  "Tools to validate UF2 firmware binaries, target architectures, flash addresses, and block sizes.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}

	cmd.AddCommand(newFirmwareInspectCommand())
	return cmd
}

func newFirmwareInspectCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "inspect <file>",
		Short: "Inspect a UF2 binary header and validate target architecture",
		Long:  "Validates every UF2 block, the target architecture (e.g. RP2040), and payload dimensions, and reports the USB vendor/product ID, firmware version, and USB strings compiled into the image.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			filePath := args[0]
			info, err := firmware.InspectUF2(filePath)
			if err != nil {
				return fmt.Errorf("inspecting UF2 binary: %w", err)
			}

			out := cmd.OutOrStdout()
			isAgent := agent.IsAgentMode()

			if isAgent {
				fmt.Fprintf(out, "file: %s\nvalid_uf2: %v\narchitecture: %s\nfamily_id: 0x%08x\ntarget_addr: 0x%08x\nblocks: %d\nfile_size_bytes: %d\n",
					info.FilePath, info.IsValidMagic, info.Architecture, info.FamilyID, info.TargetAddr, info.BlockCount, info.FileSize)
				if usb := info.USB; usb != nil {
					fmt.Fprintf(out, "usb_vid: 0x%04x\nusb_pid: 0x%04x\nusb_version: %s\n", usb.VendorID, usb.ProductID, device.FormatBCDVersion(usb.Version))
					for _, str := range usb.Strings {
						fmt.Fprintf(out, "usb_string: %s\n", str)
					}
				}
				return nil
			}

			// Human Mode
			validStr := "[OK] Valid UF2 Magic Header"
			if !info.IsValidMagic {
				validStr = "[ERROR] Invalid UF2 Header"
			}

			fmt.Fprintf(out, "UF2 Binary Inspection: %s\n", info.FilePath)
			fmt.Fprintf(out, "  Status              : %s\n", validStr)
			fmt.Fprintf(out, "  Target Architecture : %s (Family ID: 0x%08x)\n", info.Architecture, info.FamilyID)
			fmt.Fprintf(out, "  Target Flash Base   : 0x%08x\n", info.TargetAddr)
			fmt.Fprintf(out, "  Payload Block Size  : %d bytes\n", info.PayloadSize)
			fmt.Fprintf(out, "  Total UF2 Blocks    : %d blocks\n", info.BlockCount)
			fmt.Fprintf(out, "  Total File Size     : %.1f KB (%d bytes)\n", float64(info.FileSize)/1024.0, info.FileSize)
			if usb := info.USB; usb != nil {
				fmt.Fprintf(out, "  USB Device          : 0x%04x:0x%04x, version %s\n", usb.VendorID, usb.ProductID, device.FormatBCDVersion(usb.Version))
				fmt.Fprintf(out, "  USB Strings         : %s\n", strings.Join(usb.Strings, " | "))
			} else {
				fmt.Fprintln(out, "  USB Device          : no USB device descriptor found")
			}

			return nil
		},
	}
}
