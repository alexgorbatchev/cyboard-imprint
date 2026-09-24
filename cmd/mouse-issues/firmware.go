package main

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/alexgorbatchev/mouse-issues/internal/agent"
	"github.com/alexgorbatchev/mouse-issues/internal/firmware"
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
		Long:  "Parses the 32-byte UF2 block header, verifies magic bytes, target architecture (e.g. RP2040), and payload dimensions.",
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

			return nil
		},
	}
}
