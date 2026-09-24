package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/alexgorbatchev/mouse-issues/internal/agent"
	"github.com/alexgorbatchev/mouse-issues/internal/analyzer"
	"github.com/alexgorbatchev/mouse-issues/internal/capture"
	"github.com/alexgorbatchev/mouse-issues/internal/device"
	"github.com/alexgorbatchev/mouse-issues/internal/recorder"
	"github.com/spf13/cobra"
)

func newCursorCommand() *cobra.Command {
	cursorCmd := &cobra.Command{
		Use:   "cursor",
		Short: "Capture, stream, record, and analyze cursor movement",
		Long:  "Tools to detect teleportation spikes, sign-extension glitches, and multi-monitor leap anomalies.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}

	cursorCmd.AddCommand(newCursorMonitorCommand())
	cursorCmd.AddCommand(newCursorRecordCommand())
	cursorCmd.AddCommand(newCursorAnalyzeCommand())
	cursorCmd.AddCommand(newCursorTimelineCommand())
	cursorCmd.AddCommand(newCursorSpikesCommand())
	cursorCmd.AddCommand(newCursorDiagnoseCommand())

	return cursorCmd
}

// readRecording parses a recorded NDJSON session file.
func readRecording(path string) (recorder.Recording, error) {
	f, err := os.Open(path)
	if err != nil {
		return recorder.Recording{}, fmt.Errorf("opening file %q: %w", path, err)
	}
	defer f.Close()

	rec, err := recorder.Read(f)
	if err != nil {
		return recorder.Recording{}, fmt.Errorf("reading recorded events from %q: %w", path, err)
	}
	return rec, nil
}

func newCursorMonitorCommand() *cobra.Command {
	var (
		modeStr       string
		deviceFilter  string
		threshold     int64
		onlyAnomalies bool
	)

	cmd := &cobra.Command{
		Use:   "monitor",
		Short: "Live stream cursor motion and highlight jump anomalies",
		Long:  "Listens in real time to CoreGraphics and/or IOHID pointing events, detecting sudden deltas and boundary jumps.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			displays, _ := device.ListDisplays()
			cfg := analyzer.Config{
				JumpThreshold: threshold,
				Displays:      displays,
			}
			az := analyzer.New(cfg)

			opts := capture.Options{
				Mode:         capture.Mode(modeStr),
				DeviceFilter: deviceFilter,
			}
			session := capture.NewSession(opts)

			ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
			defer stop()

			out := cmd.OutOrStdout()
			isAgent := agent.IsAgentMode()

			if !isAgent {
				fmt.Fprintln(out, "[INFO] Listening for cursor movement... (Press Ctrl+C to stop)")
				if onlyAnomalies {
					fmt.Fprintln(out, "[INFO] Mode: only anomalies will be displayed.")
				}
				fmt.Fprintf(out, "[INFO] Jump threshold: %d counts/pixels\n\n", threshold)
			}

			err := session.Start(ctx, func(ev analyzer.Event) {
				anomalies := az.Process(ev)

				if onlyAnomalies && len(anomalies) == 0 {
					return
				}

				if isAgent {
					if len(anomalies) > 0 {
						for _, an := range anomalies {
							fmt.Fprintf(out, "anomaly: %s | severity: %s | desc: %s | src: %s | dx: %d | dy: %d\n",
								an.Kind, an.Severity, an.Description, ev.Source, ev.DeltaX, ev.DeltaY)
						}
					} else {
						fmt.Fprintf(out, "event: %s | dx: %d | dy: %d | x: %.0f | y: %.0f\n",
							ev.Source, ev.DeltaX, ev.DeltaY, ev.CursorX, ev.CursorY)
					}
					return
				}

				// Human Mode
				if len(anomalies) > 0 {
					for _, an := range anomalies {
						fmt.Fprintf(out, "[ANOMALY: %s] %s\n", strings.ToUpper(string(an.Kind)), an.Description)
						fmt.Fprintf(out, "          Source: %-4s | Delta: (%+4d, %+4d) | Coord: (%.0f, %.0f)\n",
							ev.Source, ev.DeltaX, ev.DeltaY, ev.CursorX, ev.CursorY)
					}
				} else {
					fmt.Fprintf(out, "  [%-4s] Delta: (%+4d, %+4d) | Pos: (%4.0f, %4.0f) | Device: %s\n",
						ev.Source, ev.DeltaX, ev.DeltaY, ev.CursorX, ev.CursorY, ev.DeviceName)
				}
			})

			if err != nil && ctx.Err() == nil {
				return fmt.Errorf("monitor stream: %w", err)
			}

			summary := az.Summary()
			if !isAgent {
				fmt.Fprintln(out, "\nSummary:")
				fmt.Fprintf(out, "  Total Events  : %d\n", summary.TotalEvents)
				fmt.Fprintf(out, "  Total Anomalies: %d\n", summary.AnomalyCount)
				for _, diag := range summary.Diagnoses {
					fmt.Fprintf(out, "  * %s\n", diag)
				}
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&modeStr, "mode", "both", "Capture layer: 'both', 'hid', or 'cg'")
	cmd.Flags().StringVarP(&deviceFilter, "device", "d", "", "Filter device by name, VID:PID, or substring")
	cmd.Flags().Int64VarP(&threshold, "threshold", "t", 50, "Delta magnitude to trigger a jump alert")
	cmd.Flags().BoolVar(&onlyAnomalies, "only-anomalies", false, "Only print detected anomalies, suppress normal movement")

	return cmd
}

func newCursorRecordCommand() *cobra.Command {
	var (
		outputPath   string
		duration     time.Duration
		modeStr      string
		deviceFilter string
		threshold    int64
	)

	cmd := &cobra.Command{
		Use:   "record",
		Short: "Record cursor motion and raw HID deltas to a file",
		Long:  "Records all cursor and pointing events with microsecond timestamps and anomaly tags to an NDJSON file.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if outputPath == "" {
				outputPath = "mouse-session.ndjson"
			}

			f, err := os.Create(outputPath)
			if err != nil {
				return fmt.Errorf("creating record file %q: %w", outputPath, err)
			}
			defer f.Close()

			rec := recorder.NewWriter(f)
			displays, _ := device.ListDisplays()
			devices, _ := device.ListPointingDevices()
			if err := rec.WriteSession(recorder.SessionHeader{
				StartedAt:    time.Now(),
				Mode:         modeStr,
				DeviceFilter: deviceFilter,
				Threshold:    threshold,
				Displays:     displays,
				Devices:      devices,
			}); err != nil {
				return fmt.Errorf("writing %q: %w", outputPath, err)
			}
			az := analyzer.New(analyzer.Config{
				JumpThreshold: threshold,
				Displays:      displays,
			})

			opts := capture.Options{
				Mode:         capture.Mode(modeStr),
				DeviceFilter: deviceFilter,
			}
			session := capture.NewSession(opts)

			var ctx context.Context
			var stop context.CancelFunc
			if duration > 0 {
				ctx, stop = context.WithTimeout(context.Background(), duration)
			} else {
				ctx, stop = signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
			}
			defer stop()

			out := cmd.OutOrStdout()
			isAgent := agent.IsAgentMode()

			if !isAgent {
				if duration > 0 {
					fmt.Fprintf(out, "[INFO] Recording to %s for %s... (move your trackball)\n", outputPath, duration)
				} else {
					fmt.Fprintf(out, "[INFO] Recording to %s... (Press Ctrl+C to stop)\n", outputPath)
				}
			}

			eventCount := 0
			anomalyCount := 0
			var writeErr error
			keepFirst := func(err error) {
				if writeErr == nil {
					writeErr = err
				}
			}

			err = session.Start(ctx, func(ev analyzer.Event) {
				eventCount++
				if err := rec.WriteEvent(ev); err != nil {
					keepFirst(err)
				}
				anomalies := az.Process(ev)
				for _, an := range anomalies {
					anomalyCount++
					if err := rec.WriteAnomaly(an); err != nil {
						keepFirst(err)
					}
				}
			})

			if err != nil && ctx.Err() == nil {
				return fmt.Errorf("record session: %w", err)
			}
			if writeErr != nil {
				return fmt.Errorf("writing %q: %w", outputPath, writeErr)
			}

			if isAgent {
				fmt.Fprintf(out, "file: %s\nevents: %d\nanomalies: %d\n", outputPath, eventCount, anomalyCount)
				return nil
			}

			fmt.Fprintf(out, "[OK] Finished recording %d events (%d anomalies) to %s\n",
				eventCount, anomalyCount, outputPath)
			return nil
		},
	}

	cmd.Flags().StringVarP(&outputPath, "output", "o", "mouse-session.ndjson", "Destination NDJSON log file path")
	cmd.Flags().DurationVar(&duration, "duration", 0, "Record duration (e.g. '10s', default: run until Ctrl+C)")
	cmd.Flags().StringVar(&modeStr, "mode", "both", "Capture layer: 'both', 'hid', or 'cg'")
	cmd.Flags().StringVarP(&deviceFilter, "device", "d", "", "Filter device by name or VID:PID")
	cmd.Flags().Int64VarP(&threshold, "threshold", "t", 50, "Jump threshold")

	return cmd
}

func newCursorAnalyzeCommand() *cobra.Command {
	var (
		threshold    int64
		showTimeline bool
		showSpikes   bool
	)

	cmd := &cobra.Command{
		Use:   "analyze <file>",
		Short: "Analyze recorded cursor session file and output diagnosis",
		Long:  "Parses a recorded NDJSON file, calculates velocity, delta histograms, overflow boundaries, and diagnostic findings.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			filePath := args[0]
			recording, err := readRecording(filePath)
			if err != nil {
				return err
			}
			events := recording.Events

			// Display crossings are judged against the layout the session was recorded with.
			var displays []analyzer.Display
			if recording.Session != nil {
				displays = recording.Session.Displays
			}
			az := analyzer.New(analyzer.Config{
				JumpThreshold: threshold,
				Displays:      displays,
			})

			for _, ev := range events {
				az.Process(ev)
			}

			summary := az.Summary()
			out := cmd.OutOrStdout()
			isAgent := agent.IsAgentMode()

			devicesSeen := analyzer.DevicesSeen(events)

			if isAgent {
				if recording.Session != nil {
					fmt.Fprintf(out, "recorded_at: %s\n", recording.Session.StartedAt.Format(time.RFC3339))
				}
				fmt.Fprintf(out, "displays_recorded: %d\n", len(displays))
				for _, d := range devicesSeen {
					fmt.Fprintf(out, "device: %s | vid: 0x%04x | pid: 0x%04x | version: %s | reports: %d\n",
						d.Name, d.VID, d.PID, device.FormatBCDVersion(d.Version), d.Reports)
				}
				fmt.Fprintf(out, "events_total: %d\n", summary.TotalEvents)
				fmt.Fprintf(out, "hid_events: %d\n", summary.HIDEvents)
				fmt.Fprintf(out, "cg_events: %d\n", summary.CGEvents)
				fmt.Fprintf(out, "duration_sec: %.2f\n", summary.Duration.Seconds())
				fmt.Fprintf(out, "sample_rate_hz: %.1f\n", summary.AvgHz)
				fmt.Fprintf(out, "anomalies_total: %d\n", summary.AnomalyCount)
				fmt.Fprintf(out, "x_min: %d | x_max: %d | x_mean: %.2f\n", summary.XStats.Min, summary.XStats.Max, summary.XStats.Mean)
				fmt.Fprintf(out, "y_min: %d | y_max: %d | y_mean: %.2f\n", summary.YStats.Min, summary.YStats.Max, summary.YStats.Mean)
				fmt.Fprintln(out, "diagnoses:")
				for _, d := range summary.Diagnoses {
					fmt.Fprintf(out, "  - %s\n", d)
				}
				if showTimeline {
					fmt.Fprintln(out, "timeline:")
					for _, b := range analyzer.Timeline(events, threshold) {
						fmt.Fprintf(out, "  - sec: %s | events: %d | avg: %.2f | max: %d | spikes: %d\n",
							b.Second, b.Events, b.AvgDelta, b.MaxDelta, b.SpikesCount)
					}
				}
				if showSpikes {
					sp := analyzer.AnalyzeSpikes(events, threshold)
					fmt.Fprintf(out, "spikes_total: %d | neg_x: %d | pos_x: %d | neg_y: %d | pos_y: %d\n",
						sp.TotalSpikes, sp.NegativeX, sp.PositiveX, sp.NegativeY, sp.PositiveY)
				}
				return nil
			}

			// Human Mode
			fmt.Fprintf(out, "Analysis Report for: %s\n", filePath)
			if recording.Session != nil {
				fmt.Fprintf(out, "  Recorded At    : %s\n", recording.Session.StartedAt.Format(time.RFC3339))
			}
			fmt.Fprintf(out, "  Displays       : %d recorded\n", len(displays))
			for _, d := range devicesSeen {
				fmt.Fprintf(out, "  HID Device     : %s [0x%04x:0x%04x] firmware %s (%d reports)\n",
					d.Name, d.VID, d.PID, device.FormatBCDVersion(d.Version), d.Reports)
			}
			fmt.Fprintf(out, "  Total Events   : %d (HID: %d, CG: %d)\n", summary.TotalEvents, summary.HIDEvents, summary.CGEvents)
			fmt.Fprintf(out, "  Duration       : %s (avg %.1f Hz)\n", summary.Duration.Round(time.Millisecond), summary.AvgHz)
			fmt.Fprintf(out, "  Delta X Range  : min=%d, max=%d, mean=%.2f, stddev=%.2f\n",
				summary.XStats.Min, summary.XStats.Max, summary.XStats.Mean, summary.XStats.StdDev)
			fmt.Fprintf(out, "  Delta Y Range  : min=%d, max=%d, mean=%.2f, stddev=%.2f\n",
				summary.YStats.Min, summary.YStats.Max, summary.YStats.Mean, summary.YStats.StdDev)

			fmt.Fprintln(out, "\nDelta Magnitude Distribution:")
			for _, b := range summary.Buckets {
				pct := bucketPercent(summary, b)
				fmt.Fprintf(out, "  * [%3d - %-5d] : %6d (%5.1f%%)\n", b.Min, b.Max, b.Count, pct)
			}

			fmt.Fprintf(out, "\nAnomalies Detected: %d\n", summary.AnomalyCount)
			for kind, count := range summary.AnomalyBreakdown {
				fmt.Fprintf(out, "  * %-24s: %d\n", kind, count)
			}

			if showTimeline {
				buckets := analyzer.Timeline(events, threshold)
				fmt.Fprintf(out, "\nTimeline Breakdown (Threshold: > %d counts):\n", threshold)
				fmt.Fprintf(out, "  %-20s | %-7s | %-10s | %-10s | Deltas > %d\n", "Second", "Events", "Avg Delta", "Max Delta", threshold)
				fmt.Fprintln(out, "  "+strings.Repeat("-", 66))
				for _, b := range buckets {
					fmt.Fprintf(out, "  %-20s | %-7d | %-10.2f | %-10d | %d\n",
						b.Second, b.Events, b.AvgDelta, b.MaxDelta, b.SpikesCount)
				}
			}

			if showSpikes {
				sp := analyzer.AnalyzeSpikes(events, threshold)
				fmt.Fprintf(out, "\nDirectional Spike Breakdown (Threshold: > %d counts):\n", threshold)
				fmt.Fprintf(out, "  Total Spikes: %d (Left: %d, Right: %d, Up: %d, Down: %d)\n",
					sp.TotalSpikes, sp.NegativeX, sp.PositiveX, sp.NegativeY, sp.PositiveY)
			}

			fmt.Fprintln(out, "\nDiagnostic Findings & Recommendations:")
			for i, d := range summary.Diagnoses {
				fmt.Fprintf(out, "  [%d] %s\n\n", i+1, d)
			}

			return nil
		},
	}

	cmd.Flags().Int64VarP(&threshold, "threshold", "t", 50, "Jump threshold")
	cmd.Flags().BoolVar(&showTimeline, "timeline", false, "Include second-by-second timeline in report")
	cmd.Flags().BoolVar(&showSpikes, "spikes", false, "Include directional spike analysis in report")
	return cmd
}

func newCursorTimelineCommand() *cobra.Command {
	var threshold int64

	cmd := &cobra.Command{
		Use:   "timeline <file>",
		Short: "Display second-by-second timeline of event counts and peak deltas",
		Long:  "Parses a recorded NDJSON file, buckets pointing events by second, and displays average deltas and spike counts.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			filePath := args[0]
			recording, err := readRecording(filePath)
			if err != nil {
				return err
			}
			events := recording.Events

			buckets := analyzer.Timeline(events, threshold)
			out := cmd.OutOrStdout()
			isAgent := agent.IsAgentMode()

			if isAgent {
				for _, b := range buckets {
					fmt.Fprintf(out, "sec: %s | events: %d | avg: %.2f | max: %d | spikes: %d\n",
						b.Second, b.Events, b.AvgDelta, b.MaxDelta, b.SpikesCount)
				}
				return nil
			}

			// Human Mode
			fmt.Fprintf(out, "\nTimeline Analysis for: %s (Threshold: > %d counts)\n", filePath, threshold)
			fmt.Fprintf(out, "%-20s | %-7s | %-10s | %-10s | Deltas > %d\n", "Second", "Events", "Avg Delta", "Max Delta", threshold)
			fmt.Fprintln(out, strings.Repeat("-", 68))
			for _, b := range buckets {
				fmt.Fprintf(out, "%-20s | %-7d | %-10.2f | %-10d | %d\n",
					b.Second, b.Events, b.AvgDelta, b.MaxDelta, b.SpikesCount)
			}
			fmt.Fprintln(out, strings.Repeat("-", 68))
			return nil
		},
	}

	cmd.Flags().Int64VarP(&threshold, "threshold", "t", 30, "Spike threshold for flagging count anomalies")
	return cmd
}

func newCursorSpikesCommand() *cobra.Command {
	var (
		threshold   int64
		sampleLimit int
	)

	cmd := &cobra.Command{
		Use:   "spikes <file>",
		Short: "Analyze directional distribution and timestamps of delta spikes",
		Long:  "Identifies all pointing events exceeding threshold, breaks them down by direction (Left, Right, Up, Down), and shows chronological samples.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			filePath := args[0]
			recording, err := readRecording(filePath)
			if err != nil {
				return err
			}
			events := recording.Events

			spikes := analyzer.AnalyzeSpikes(events, threshold)
			out := cmd.OutOrStdout()
			isAgent := agent.IsAgentMode()

			if isAgent {
				fmt.Fprintf(out, "total_spikes: %d\nnegative_x_left: %d\npositive_x_right: %d\nnegative_y_up: %d\npositive_y_down: %d\n",
					spikes.TotalSpikes, spikes.NegativeX, spikes.PositiveX, spikes.NegativeY, spikes.PositiveY)
				for i, s := range spikes.Samples {
					if sampleLimit > 0 && i >= sampleLimit {
						break
					}
					fmt.Fprintf(out, "sample: %s | dx: %d | dy: %d\n", s.Timestamp.Format("2006-01-02T15:04:05.000000-07:00"), s.DeltaX, s.DeltaY)
				}
				return nil
			}

			// Human Mode
			fmt.Fprintf(out, "\nSpike Direction Analysis for: %s (Threshold: > %d counts)\n", filePath, threshold)
			fmt.Fprintf(out, "  Total Spikes Detected : %d\n", spikes.TotalSpikes)
			fmt.Fprintf(out, "    Negative X (Left)   : %d\n", spikes.NegativeX)
			fmt.Fprintf(out, "    Positive X (Right)  : %d\n", spikes.PositiveX)
			fmt.Fprintf(out, "    Negative Y (Up)     : %d\n", spikes.NegativeY)
			fmt.Fprintf(out, "    Positive Y (Down)   : %d\n", spikes.PositiveY)

			if len(spikes.Samples) > 0 {
				limit := sampleLimit
				if limit <= 0 || limit > len(spikes.Samples) {
					limit = len(spikes.Samples)
				}
				fmt.Fprintf(out, "\nChronological Spike Samples (first %d):\n", limit)
				for i := 0; i < limit; i++ {
					s := spikes.Samples[i]
					fmt.Fprintf(out, "  * %s | Delta: (%+5d, %+5d)\n",
						s.Timestamp.Format("2006-01-02T15:04:05.000000-07:00"), s.DeltaX, s.DeltaY)
				}
			}
			return nil
		},
	}

	cmd.Flags().Int64VarP(&threshold, "threshold", "t", 30, "Delta magnitude to classify as a spike")
	cmd.Flags().IntVarP(&sampleLimit, "limit", "l", 15, "Maximum sample spikes to display in human mode")
	return cmd
}

func newCursorDiagnoseCommand() *cobra.Command {
	var (
		duration     time.Duration
		deviceFilter string
		threshold    int64
	)

	cmd := &cobra.Command{
		Use:   "diagnose",
		Short: "Run an automated diagnostic session while moving the trackball",
		Long:  "Guides you through moving the trackball gently, collects raw sensor data, and outputs an automated root-cause diagnosis.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if duration <= 0 {
				duration = 10 * time.Second
			}

			out := cmd.OutOrStdout()
			isAgent := agent.IsAgentMode()

			displays, _ := device.ListDisplays()
			devices, _ := device.ListPointingDevices()

			var targetDev *device.Info
			for i := range devices {
				d := &devices[i]
				if deviceFilter != "" {
					if strings.Contains(strings.ToLower(d.Name), strings.ToLower(deviceFilter)) ||
						strings.Contains(strings.ToLower(d.Manufacturer), strings.ToLower(deviceFilter)) {
						targetDev = d
						break
					}
				} else if strings.Contains(strings.ToLower(d.Name), "imprint") ||
					strings.Contains(strings.ToLower(d.Manufacturer), "cyboard") ||
					strings.Contains(strings.ToLower(d.SerialNumber), "vial") {
					targetDev = d
					break
				}
			}

			if targetDev == nil && len(devices) > 0 {
				targetDev = &devices[0]
			}

			if !isAgent {
				fmt.Fprintln(out, "============================================================")
				fmt.Fprintln(out, "          TRACKBALL / MOUSE DIAGNOSTIC SESSION             ")
				fmt.Fprintln(out, "============================================================")
				if targetDev != nil {
					fmt.Fprintf(out, "Target Device : %s (%s) [VID: 0x%04x, PID: 0x%04x]\n",
						targetDev.Name, targetDev.Manufacturer, targetDev.VendorID, targetDev.ProductID)
				}
				fmt.Fprintf(out, "Test Duration : %s\n", duration)
				fmt.Fprintf(out, "Monitors      : %d active\n", len(displays))
				fmt.Fprintln(out, "------------------------------------------------------------")
				fmt.Fprintln(out, "INSTRUCTIONS:")
				fmt.Fprintln(out, "  1. Move your trackball GENTLY and CONTINUOUSLY.")
				fmt.Fprintln(out, "  2. Roll softly in small circles and slow diagonals.")
				fmt.Fprintln(out, "  3. Especially try rolling in the direction where it jumps.")
				fmt.Fprintln(out, "------------------------------------------------------------")
				fmt.Fprintln(out, "Starting in 2 seconds...")
				time.Sleep(2 * time.Second)
				fmt.Fprintln(out, "[CAPTURING] Move the trackball now...")
			}

			az := analyzer.New(analyzer.Config{
				JumpThreshold: threshold,
				Displays:      displays,
			})

			filter := ""
			if targetDev != nil {
				filter = targetDev.Name
			}
			session := capture.NewSession(capture.Options{
				Mode:         capture.ModeBoth,
				DeviceFilter: filter,
			})

			ctx, cancel := context.WithTimeout(context.Background(), duration)
			defer cancel()

			var capturedEvents []analyzer.Event
			var liveAnomalies []analyzer.Anomaly

			_ = session.Start(ctx, func(ev analyzer.Event) {
				capturedEvents = append(capturedEvents, ev)
				ans := az.Process(ev)
				if len(ans) > 0 {
					liveAnomalies = append(liveAnomalies, ans...)
					if !isAgent {
						for _, an := range ans {
							fmt.Fprintf(out, "  ! SPIKE DETECTED: %s (Delta: %d, %d)\n", an.Title, ev.DeltaX, ev.DeltaY)
						}
					}
				}
			})

			summary := az.Summary()

			if isAgent {
				fmt.Fprintf(out, "events: %d\nanomalies: %d\n", summary.TotalEvents, summary.AnomalyCount)
				for _, d := range summary.Diagnoses {
					fmt.Fprintf(out, "diagnosis: %s\n", d)
				}
				return nil
			}

			fmt.Fprintln(out, "\n============================================================")
			fmt.Fprintln(out, "                    DIAGNOSTIC RESULTS                      ")
			fmt.Fprintln(out, "============================================================")
			fmt.Fprintf(out, "Total Motion Events Sampled: %d\n", summary.TotalEvents)
			fmt.Fprintf(out, "Sample Frequency          : %.1f Hz\n", summary.AvgHz)
			fmt.Fprintf(out, "Total Anomalies Flagged   : %d\n", summary.AnomalyCount)
			fmt.Fprintf(out, "Max Delta Observed        : X=%+d, Y=%+d\n", summary.XStats.Max, summary.YStats.Max)
			fmt.Fprintf(out, "Min Delta Observed        : X=%+d, Y=%+d\n", summary.XStats.Min, summary.YStats.Min)

			fmt.Fprintln(out, "\nHistogram (Magnitude of Movement Counts):")
			for _, b := range summary.Buckets {
				pct := bucketPercent(summary, b)
				fmt.Fprintf(out, "  * [%3d - %-5d counts] : %6d (%5.1f%%)\n", b.Min, b.Max, b.Count, pct)
			}

			fmt.Fprintln(out, "\nROOT-CAUSE DIAGNOSIS & ACTIONABLE FIXES:")
			for i, d := range summary.Diagnoses {
				fmt.Fprintf(out, "\n[%d] %s\n", i+1, d)
			}

			fmt.Fprintln(out, "\n============================================================")
			return nil
		},
	}

	cmd.Flags().DurationVar(&duration, "duration", 10*time.Second, "Diagnostic capture duration (e.g. '10s', '20s')")
	cmd.Flags().StringVarP(&deviceFilter, "device", "d", "", "Filter device by name or VID:PID")
	cmd.Flags().Int64VarP(&threshold, "threshold", "t", 50, "Jump delta threshold")

	return cmd
}

// bucketPercent is the share of a histogram bucket within the stream the histogram was built from.
func bucketPercent(s analyzer.Summary, b analyzer.HistogramBucket) float64 {
	total := s.HIDEvents
	if total == 0 {
		total = s.CGEvents
	}
	if total == 0 {
		return 0
	}
	return float64(b.Count) / float64(total) * 100
}
