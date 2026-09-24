package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/alexgorbatchev/mouse-issues/internal/analyzer"
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
