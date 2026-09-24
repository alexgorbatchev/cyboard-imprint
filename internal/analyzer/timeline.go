package analyzer

import (
	"sort"
	"time"
)

// SecondBucket summarizes events occurring within a single second.
type SecondBucket struct {
	Second      string  `json:"second"`
	Events      int     `json:"events"`
	AvgDelta    float64 `json:"avg_delta"`
	MaxDelta    int64   `json:"max_delta"`
	SpikesCount int     `json:"spikes_count"`
}

// SpikeSample records timestamp and deltas for a detected spike.
type SpikeSample struct {
	Timestamp time.Time `json:"timestamp"`
	DeltaX    int64     `json:"delta_x"`
	DeltaY    int64     `json:"delta_y"`
}

// DirectionalSpikes groups spikes by direction vectors.
type DirectionalSpikes struct {
	TotalSpikes int           `json:"total_spikes"`
	NegativeX   int           `json:"negative_x"` // Left
	PositiveX   int           `json:"positive_x"` // Right
	NegativeY   int           `json:"negative_y"` // Up
	PositiveY   int           `json:"positive_y"` // Down
	Samples     []SpikeSample `json:"samples"`
}

type secondAccumulator struct {
	events      int
	totalDelta  int64
	maxDelta    int64
	spikesCount int
}

// Timeline groups events into second-by-second buckets and computes statistics.
func Timeline(events []Event, threshold int64) []SecondBucket {
	accumMap := make(map[string]*secondAccumulator)

	for _, ev := range events {
		if ev.Source != SourceHID {
			continue
		}
		secKey := ev.Timestamp.Format("2006-01-02T15:04:05")
		acc := accumMap[secKey]
		if acc == nil {
			acc = &secondAccumulator{}
			accumMap[secKey] = acc
		}

		mag := abs(ev.DeltaX)
		if abs(ev.DeltaY) > mag {
			mag = abs(ev.DeltaY)
		}

		acc.events++
		acc.totalDelta += mag
		if mag > acc.maxDelta {
			acc.maxDelta = mag
		}
		if mag > threshold {
			acc.spikesCount++
		}
	}

	keys := make([]string, 0, len(accumMap))
	for k := range accumMap {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	buckets := make([]SecondBucket, 0, len(keys))
	for _, k := range keys {
		acc := accumMap[k]
		var avg float64
		if acc.events > 0 {
			avg = float64(acc.totalDelta) / float64(acc.events)
		}
		buckets = append(buckets, SecondBucket{
			Second:      k,
			Events:      acc.events,
			AvgDelta:    avg,
			MaxDelta:    acc.maxDelta,
			SpikesCount: acc.spikesCount,
		})
	}
	return buckets
}

// AnalyzeSpikes extracts and categorizes delta spikes exceeding threshold.
func AnalyzeSpikes(events []Event, threshold int64) DirectionalSpikes {
	var result DirectionalSpikes

	for _, ev := range events {
		if ev.Source != SourceHID {
			continue
		}
		dx, dy := ev.DeltaX, ev.DeltaY
		if abs(dx) > threshold || abs(dy) > threshold {
			result.TotalSpikes++
			if dx < -threshold {
				result.NegativeX++
			}
			if dx > threshold {
				result.PositiveX++
			}
			if dy < -threshold {
				result.NegativeY++
			}
			if dy > threshold {
				result.PositiveY++
			}
			result.Samples = append(result.Samples, SpikeSample{
				Timestamp: ev.Timestamp,
				DeltaX:    dx,
				DeltaY:    dy,
			})
		}
	}

	return result
}
