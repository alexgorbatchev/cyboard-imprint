package main

import (
	cobrahelptree "github.com/alexgorbatchev/cobra-help-tree"
	"github.com/spf13/cobra"
)

var techCatalog = cobrahelptree.TechCatalog{
	"mouse-issues": {
		Summary:     "Diagnostic tool for mouse and trackball cursor teleportation issues",
		Description: "Diagnostic tool for mouse and trackball cursor teleportation issues - command-line interface.",
	},
	"mouse-issues status": {
		Summary:     "Display system and environment status",
		Description: "Outputs current runtime readiness and version information.",
	},
	"mouse-issues device": {
		Summary:     "Inspect pointing devices and connected displays",
		Description: "Discover and inspect connected pointing devices, HID report elements, and display geometry.",
	},
	"mouse-issues device list": {
		Summary:     "List connected pointing devices and active displays",
		Description: "Enumerate USB/Bluetooth pointing devices with vendor, product, firmware version, and transport info, along with monitor bounds.",
	},
	"mouse-issues device inspect": {
		Summary:     "Inspect HID report descriptor elements for a pointing device",
		Description: "Inspect report IDs, element sizes, counts, and logical bounds to see how much motion one report can carry before it saturates.",
		Args:        "[device]",
	},
	"mouse-issues cursor": {
		Summary:     "Capture, stream, record, and analyze cursor movement",
		Description: "Tools to stream, record, and analyze cursor motion, detecting report saturation, large deltas, direction flips, and cursor teleports.",
	},
	"mouse-issues cursor monitor": {
		Summary:     "Live stream cursor motion and highlight jump anomalies",
		Description: "Real-time stream of raw HID deltas and CoreGraphics cursor events with automatic anomaly detection.",
	},
	"mouse-issues cursor record": {
		Summary:     "Record cursor motion and raw HID deltas to a file",
		Description: "Streams a session header (displays, devices, firmware versions) followed by all motion events and anomaly tags into an NDJSON file for offline analysis.",
	},
	"mouse-issues cursor analyze": {
		Summary:     "Analyze recorded cursor session file and output diagnosis",
		Description: "Parses an NDJSON recording, reports the devices and firmware versions it came from, computes delta histograms, and outputs diagnoses.",
		Args:        "<file>",
	},
	"mouse-issues cursor timeline": {
		Summary:     "Display second-by-second timeline of event counts and peak deltas",
		Description: "Parses an NDJSON recording, buckets events into 1-second slices, and shows average deltas, peak values, and spike counts.",
		Args:        "<file>",
	},
	"mouse-issues cursor spikes": {
		Summary:     "Analyze directional distribution and timestamps of delta spikes",
		Description: "Extracts all pointing events exceeding threshold, groups them by direction (Left, Right, Up, Down), and shows chronological samples.",
		Args:        "<file>",
	},
	"mouse-issues cursor diagnose": {
		Summary:     "Run an automated diagnostic session while moving the trackball",
		Description: "Guides you through gentle trackball movement, captures high-resolution sensor metrics, and prints automated root-cause diagnosis.",
	},
	"mouse-issues firmware": {
		Summary:     "Inspect and validate keyboard firmware binaries",
		Description: "Commands for validating UF2 firmware files, target microcontroller architectures, and payload dimensions.",
	},
	"mouse-issues firmware inspect": {
		Summary:     "Inspect a UF2 binary header and validate target architecture",
		Description: "Parses the 32-byte UF2 block header, verifies magic bytes, target architecture (e.g. RP2040), and payload dimensions.",
		Args:        "<file>",
	},
}

func setupHelp(cmd *cobra.Command) {
	cobrahelptree.Setup(cmd, cobrahelptree.TreeOptions{
		TechCatalog: techCatalog,
	})
}
