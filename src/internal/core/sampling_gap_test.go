package core

import (
	"testing"
	"time"
)

func TestUnexpectedSamplingGap(t *testing.T) {
	start := time.Date(2026, 10, 14, 0, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		elapsed, interval time.Duration
		want              bool
	}{{time.Second, 500 * time.Millisecond, false}, {30 * time.Second, 500 * time.Millisecond, false}, {7 * time.Hour, 500 * time.Millisecond, true}, {40 * time.Second, 10 * time.Second, false}, {51 * time.Second, 10 * time.Second, true}, {-time.Second, time.Second, false}} {
		gap, got := unexpectedSamplingGap(start, start.Add(tc.elapsed), tc.interval)
		if gap != tc.elapsed || got != tc.want {
			t.Fatalf("%+v gap=%v unexpected=%t", tc, gap, got)
		}
	}
}
