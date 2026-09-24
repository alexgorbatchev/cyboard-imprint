package analyzer

import (
	"testing"
	"time"
)

func TestTimeline(t *testing.T) {
	t0, _ := time.Parse(time.RFC3339, "2026-09-23T12:00:00Z")
	events := []Event{
		{Timestamp: t0, Source: SourceHID, DeltaX: 2, DeltaY: 3},
		{Timestamp: t0.Add(200 * time.Millisecond), Source: SourceHID, DeltaX: 4, DeltaY: 1},
		{Timestamp: t0.Add(500 * time.Millisecond), Source: SourceHID, DeltaX: 50, DeltaY: 0}, // spike > 30
		{Timestamp: t0.Add(1500 * time.Millisecond), Source: SourceHID, DeltaX: 5, DeltaY: 2}, // next second
	}

	buckets := Timeline(events, 30)
	if len(buckets) != 2 {
		t.Fatalf("expected 2 second buckets, got %d", len(buckets))
	}

	b0 := buckets[0]
	if b0.Events != 3 {
		t.Errorf("bucket 0: expected 3 events, got %d", b0.Events)
	}
	if b0.MaxDelta != 50 {
		t.Errorf("bucket 0: expected max delta 50, got %d", b0.MaxDelta)
	}
	if b0.SpikesCount != 1 {
		t.Errorf("bucket 0: expected 1 spike, got %d", b0.SpikesCount)
	}

	b1 := buckets[1]
	if b1.Events != 1 {
		t.Errorf("bucket 1: expected 1 event, got %d", b1.Events)
	}
	if b1.MaxDelta != 5 {
		t.Errorf("bucket 1: expected max delta 5, got %d", b1.MaxDelta)
	}
	if b1.SpikesCount != 0 {
		t.Errorf("bucket 1: expected 0 spikes, got %d", b1.SpikesCount)
	}
}

func TestAnalyzeSpikes(t *testing.T) {
	t0 := time.Now()
	events := []Event{
		{Timestamp: t0, Source: SourceHID, DeltaX: 5, DeltaY: 2},                              // normal
		{Timestamp: t0.Add(10 * time.Millisecond), Source: SourceHID, DeltaX: -45, DeltaY: 0}, // NegX spike
		{Timestamp: t0.Add(20 * time.Millisecond), Source: SourceHID, DeltaX: 60, DeltaY: 0},  // PosX spike
		{Timestamp: t0.Add(30 * time.Millisecond), Source: SourceHID, DeltaX: 0, DeltaY: -70}, // NegY spike
		{Timestamp: t0.Add(40 * time.Millisecond), Source: SourceHID, DeltaX: 0, DeltaY: 55},  // PosY spike
	}

	spikes := AnalyzeSpikes(events, 30)
	if spikes.TotalSpikes != 4 {
		t.Errorf("expected 4 total spikes, got %d", spikes.TotalSpikes)
	}
	if spikes.NegativeX != 1 {
		t.Errorf("expected 1 NegX spike, got %d", spikes.NegativeX)
	}
	if spikes.PositiveX != 1 {
		t.Errorf("expected 1 PosX spike, got %d", spikes.PositiveX)
	}
	if spikes.NegativeY != 1 {
		t.Errorf("expected 1 NegY spike, got %d", spikes.NegativeY)
	}
	if spikes.PositiveY != 1 {
		t.Errorf("expected 1 PosY spike, got %d", spikes.PositiveY)
	}
	if len(spikes.Samples) != 4 {
		t.Errorf("expected 4 samples, got %d", len(spikes.Samples))
	}
}
