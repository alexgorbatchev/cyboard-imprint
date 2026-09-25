package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/alexgorbatchev/mouse-issues/internal/analyzer"
	"github.com/alexgorbatchev/mouse-issues/internal/firmware/firmwaretest"
	"github.com/alexgorbatchev/mouse-issues/internal/recorder"
)

func executeCommand(args ...string) (string, error) {
	buf := new(bytes.Buffer)
	cmd := newRootCommand()
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs(args)

	err := cmd.Execute()
	return buf.String(), err
}

func TestRootCommand_HelpAndVersion(t *testing.T) {
	t.Setenv("AGENT", "0")
	out, err := executeCommand("--help")
	if err != nil {
		t.Fatalf("help failed: %v", err)
	}
	if len(out) == 0 {
		t.Fatal("expected help output")
	}

	outVer, errVer := executeCommand("--version")
	if errVer != nil {
		t.Fatalf("version failed: %v", errVer)
	}
	if outVer != version+"\n" {
		t.Errorf("expected '%s\\n', got %q", version, outVer)
	}
}

func TestDeviceListHelp_MentionsFirmwareVersion(t *testing.T) {
	t.Setenv("AGENT", "0")
	out, err := executeCommand("device", "list", "--help")
	if err != nil {
		t.Fatalf("device list --help failed: %v", err)
	}
	if !strings.Contains(out, "firmware version") {
		t.Errorf("expected help to mention the firmware version, got: %s", out)
	}
}

func TestStatusCommand_HumanMode(t *testing.T) {
	t.Setenv("AGENT", "0")
	out, err := executeCommand("status")
	if err != nil {
		t.Fatalf("status failed: %v", err)
	}
	if !strings.Contains(out, "[OK] System status: ready") {
		t.Errorf("expected human status output, got %q", out)
	}
}

func TestStatusCommand_AgentMode(t *testing.T) {
	t.Setenv("AGENT", "1")
	out, err := executeCommand("status")
	if err != nil {
		t.Fatalf("status failed: %v", err)
	}
	if !strings.Contains(out, "status: ready") {
		t.Errorf("expected agent status output, got %q", out)
	}
}

func TestDeviceListCommand_HumanAndAgent(t *testing.T) {
	t.Setenv("AGENT", "0")
	out, err := executeCommand("device", "list")
	if err != nil {
		t.Fatalf("device list failed: %v", err)
	}
	if !strings.Contains(out, "Connected Pointing Devices:") {
		t.Errorf("expected 'Connected Pointing Devices:' in human output, got: %s", out)
	}
	if strings.Contains(out, "Vendor ID") && !strings.Contains(out, "Version      :") {
		t.Errorf("expected a Version line for each listed device, got: %s", out)
	}

	t.Setenv("AGENT", "1")
	agentOut, err := executeCommand("device", "list")
	if err != nil {
		t.Fatalf("device list (agent) failed: %v", err)
	}
	if !strings.Contains(agentOut, "pointing_devices:") {
		t.Errorf("expected 'pointing_devices:' in agent output, got: %s", agentOut)
	}
}

func TestDeviceInspectCommand_Default(t *testing.T) {
	t.Setenv("AGENT", "0")
	out, err := executeCommand("device", "inspect")
	if err != nil {
		t.Fatalf("device inspect failed: %v", err)
	}
	if !strings.Contains(out, "HID Report Elements:") {
		t.Errorf("expected 'HID Report Elements:' in output, got: %s", out)
	}
	if !strings.Contains(out, "Version      :") {
		t.Errorf("expected device Version line in output, got: %s", out)
	}
	if strings.Contains(out, "[NOTE] X/Y Motion") && !strings.Contains(out, "too-high DPI") {
		t.Errorf("expected the 8-bit note to explain report saturation, got: %s", out)
	}
}

func TestCursorAnalyzeCommand_RecordedSession(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "test-session.ndjson")

	f, err := os.Create(logPath)
	if err != nil {
		t.Fatalf("creating test file: %v", err)
	}

	rec := recorder.NewWriter(f)
	t0 := time.Now()
	for i := 0; i < 5; i++ {
		_ = rec.WriteEvent(analyzer.Event{
			ID:        uint64(i + 1),
			Timestamp: t0.Add(time.Duration(i*8) * time.Millisecond),
			Source:    analyzer.SourceHID,
			DeltaX:    2,
			DeltaY:    1,
		})
	}
	// Add an overflow event
	_ = rec.WriteEvent(analyzer.Event{
		ID:        6,
		Timestamp: t0.Add(48 * time.Millisecond),
		Source:    analyzer.SourceHID,
		DeltaX:    255,
		DeltaY:    0,
	})
	f.Close()

	t.Setenv("AGENT", "0")
	out, err := executeCommand("cursor", "analyze", logPath)
	if err != nil {
		t.Fatalf("cursor analyze failed: %v", err)
	}

	if !strings.Contains(out, "Analysis Report for:") {
		t.Errorf("expected 'Analysis Report for:' in output, got: %s", out)
	}
	if !strings.Contains(out, "Firmware Integer Overflow Detected") {
		t.Errorf("expected 'Firmware Integer Overflow Detected' in output, got: %s", out)
	}

	t.Setenv("AGENT", "1")
	agentOut, err := executeCommand("cursor", "analyze", logPath)
	if err != nil {
		t.Fatalf("cursor analyze (agent) failed: %v", err)
	}
	if !strings.Contains(agentOut, "events_total: 6") {
		t.Errorf("expected 'events_total: 6' in agent output, got: %s", agentOut)
	}
}

func TestCursorAnalyzeCommand_SessionHeader(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "header-session.ndjson")
	f, err := os.Create(logPath)
	if err != nil {
		t.Fatalf("creating test file: %v", err)
	}

	t0 := time.Date(2026, 9, 24, 12, 0, 0, 0, time.UTC)
	rec := recorder.NewWriter(f)
	_ = rec.WriteSession(recorder.SessionHeader{
		StartedAt: t0,
		Mode:      "both",
		Threshold: 40,
		Displays: []analyzer.Display{
			{ID: 2, Bounds: analyzer.Rect{Width: 2560, Height: 1440}, IsMain: true},
			{ID: 3, Bounds: analyzer.Rect{X: 511, Y: 1440, Width: 1440, Height: 900}},
		},
	})
	_ = rec.WriteEvent(analyzer.Event{ID: 1, Timestamp: t0, Source: analyzer.SourceHID, DeviceName: "Imprint (Patched)", DeviceVID: 0x4359, DeviceVersion: 0x0022, DeltaX: 3})
	_ = rec.WriteEvent(analyzer.Event{ID: 2, Timestamp: t0, Source: analyzer.SourceCG, CursorX: 1000, CursorY: 1300, DeltaX: 3})
	// Jumps onto display 3 with a delta that cannot explain the move.
	_ = rec.WriteEvent(analyzer.Event{ID: 3, Timestamp: t0.Add(time.Millisecond), Source: analyzer.SourceCG, CursorX: 800, CursorY: 2000, DeltaX: 3})
	f.Close()

	t.Setenv("AGENT", "0")
	out, err := executeCommand("cursor", "analyze", logPath)
	if err != nil {
		t.Fatalf("cursor analyze failed: %v", err)
	}
	for _, want := range []string{"Recorded At    : 2026-09-24T12:00:00Z", "Imprint (Patched)", "firmware 0.2.2", "display_cross", "Cursor Teleports (1 occurrences, 1 across displays", "Cursor Teleport Leaps:"} {
		if !strings.Contains(out, want) {
			t.Errorf("expected %q in human output, got: %s", want, out)
		}
	}

	t.Setenv("AGENT", "1")
	agentOut, err := executeCommand("cursor", "analyze", logPath)
	if err != nil {
		t.Fatalf("cursor analyze (agent) failed: %v", err)
	}
	for _, want := range []string{"hid_events: 1", "cg_events: 2", "displays_recorded: 2", "device: Imprint (Patched) | vid: 0x4359 | pid: 0x0000 | version: 0.2.2 | reports: 1", "cursor_leaps:"} {
		if !strings.Contains(agentOut, want) {
			t.Errorf("expected %q in agent output, got: %s", want, agentOut)
		}
	}
}

func TestCursorAnalyzeCommand_SaturationRuns(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "saturation-session.ndjson")
	f, err := os.Create(logPath)
	if err != nil {
		t.Fatalf("creating test file: %v", err)
	}

	t0 := time.Date(2026, 9, 24, 12, 0, 0, 0, time.UTC)
	rec := recorder.NewWriter(f)
	_ = rec.WriteEvent(analyzer.Event{ID: 1, Timestamp: t0, Source: analyzer.SourceHID, DeviceName: "Imprint", DeltaX: -127, DeltaY: 127})
	_ = rec.WriteEvent(analyzer.Event{ID: 2, Timestamp: t0.Add(time.Millisecond), Source: analyzer.SourceHID, DeviceName: "Imprint", DeltaX: -127, DeltaY: 127})
	_ = rec.WriteEvent(analyzer.Event{ID: 3, Timestamp: t0.Add(2 * time.Millisecond), Source: analyzer.SourceHID, DeviceName: "Imprint", DeltaX: 5, DeltaY: 2})
	f.Close()

	t.Setenv("AGENT", "0")
	out, err := executeCommand("cursor", "analyze", logPath)
	if err != nil {
		t.Fatalf("cursor analyze failed: %v", err)
	}
	if !strings.Contains(out, "Report Saturation Runs:") {
		t.Errorf("expected 'Report Saturation Runs:' in human output, got: %s", out)
	}

	t.Setenv("AGENT", "1")
	agentOut, err := executeCommand("cursor", "analyze", logPath)
	if err != nil {
		t.Fatalf("cursor analyze (agent) failed: %v", err)
	}
	if !strings.Contains(agentOut, "saturation_runs:") {
		t.Errorf("expected 'saturation_runs:' in agent output, got: %s", agentOut)
	}
}

func TestCursorTimelineCommand(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "timeline-session.ndjson")

	f, err := os.Create(logPath)
	if err != nil {
		t.Fatalf("creating test file: %v", err)
	}

	rec := recorder.NewWriter(f)
	t0, _ := time.Parse(time.RFC3339, "2026-09-23T12:00:00Z")
	_ = rec.WriteEvent(analyzer.Event{ID: 1, Timestamp: t0, Source: analyzer.SourceHID, DeltaX: 2, DeltaY: 1})
	_ = rec.WriteEvent(analyzer.Event{ID: 2, Timestamp: t0.Add(500 * time.Millisecond), Source: analyzer.SourceHID, DeltaX: 45, DeltaY: 0})
	f.Close()

	t.Setenv("AGENT", "0")
	out, err := executeCommand("cursor", "timeline", logPath)
	if err != nil {
		t.Fatalf("cursor timeline failed: %v", err)
	}
	if !strings.Contains(out, "Timeline Analysis for:") {
		t.Errorf("expected 'Timeline Analysis for:' in output, got: %s", out)
	}

	t.Setenv("AGENT", "1")
	agentOut, err := executeCommand("cursor", "timeline", logPath)
	if err != nil {
		t.Fatalf("cursor timeline (agent) failed: %v", err)
	}
	if !strings.Contains(agentOut, "sec:") {
		t.Errorf("expected 'sec:' in agent output, got: %s", agentOut)
	}
}

func TestCursorSpikesCommand(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "spikes-session.ndjson")

	f, err := os.Create(logPath)
	if err != nil {
		t.Fatalf("creating test file: %v", err)
	}

	rec := recorder.NewWriter(f)
	t0 := time.Now()
	_ = rec.WriteEvent(analyzer.Event{ID: 1, Timestamp: t0, Source: analyzer.SourceHID, DeltaX: -55, DeltaY: 0})
	_ = rec.WriteEvent(analyzer.Event{ID: 2, Timestamp: t0.Add(10 * time.Millisecond), Source: analyzer.SourceHID, DeltaX: 45, DeltaY: 60})
	f.Close()

	t.Setenv("AGENT", "0")
	out, err := executeCommand("cursor", "spikes", logPath)
	if err != nil {
		t.Fatalf("cursor spikes failed: %v", err)
	}
	if !strings.Contains(out, "Spike Direction Analysis for:") {
		t.Errorf("expected 'Spike Direction Analysis for:' in output, got: %s", out)
	}

	t.Setenv("AGENT", "1")
	agentOut, err := executeCommand("cursor", "spikes", logPath)
	if err != nil {
		t.Fatalf("cursor spikes (agent) failed: %v", err)
	}
	if !strings.Contains(agentOut, "total_spikes: 2") {
		t.Errorf("expected 'total_spikes: 2' in agent output, got: %s", agentOut)
	}
}

func TestFirmwareInspectCommand_USBIdentity(t *testing.T) {
	path := firmwaretest.WriteUF2(t, firmwaretest.ImprintImage(), -1)

	t.Setenv("AGENT", "0")
	out, err := executeCommand("firmware", "inspect", path)
	if err != nil {
		t.Fatalf("firmware inspect failed: %v", err)
	}
	for _, want := range []string{"USB Device          : 0x4359:0x0000, version 0.2.3", "USB Strings         : Cyboard | Imprint (Patched) | vial:f64c2b3c"} {
		if !strings.Contains(out, want) {
			t.Errorf("expected %q in human output, got: %s", want, out)
		}
	}

	t.Setenv("AGENT", "1")
	agentOut, err := executeCommand("firmware", "inspect", path)
	if err != nil {
		t.Fatalf("firmware inspect (agent) failed: %v", err)
	}
	for _, want := range []string{"usb_vid: 0x4359\n", "usb_pid: 0x0000\n", "usb_version: 0.2.3\n", "usb_string: Imprint (Patched)\n"} {
		if !strings.Contains(agentOut, want) {
			t.Errorf("expected %q in agent output, got: %s", want, agentOut)
		}
	}
}

func TestFirmwareInspectCommand(t *testing.T) {
	tmpDir := t.TempDir()
	badPath := filepath.Join(tmpDir, "dummy.uf2")
	_ = os.WriteFile(badPath, []byte("not a real uf2"), 0644)

	t.Setenv("AGENT", "0")
	_, err := executeCommand("firmware", "inspect", badPath)
	if err == nil {
		t.Fatalf("expected error on bad uf2 file")
	}

	realUF2 := "firmware/bin/cyboard-imprint-uf2/cyboard_imprint_imprint_number_row_5key_bottom_row_vial.uf2"
	if _, err := os.Stat(realUF2); err == nil {
		out, err := executeCommand("firmware", "inspect", realUF2)
		if err != nil {
			t.Fatalf("inspect real uf2 failed: %v", err)
		}
		if !strings.Contains(out, "Raspberry Pi RP2040") {
			t.Errorf("expected 'Raspberry Pi RP2040' in output, got: %s", out)
		}

		t.Setenv("AGENT", "1")
		agentOut, err := executeCommand("firmware", "inspect", realUF2)
		if err != nil {
			t.Fatalf("inspect real uf2 (agent) failed: %v", err)
		}
		if !strings.Contains(agentOut, "architecture: Raspberry Pi RP2040") {
			t.Errorf("expected 'architecture: Raspberry Pi RP2040' in agent output, got: %s", agentOut)
		}
	}
}
