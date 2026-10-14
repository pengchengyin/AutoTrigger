package core

import "time"

func unexpectedSamplingGap(last, now time.Time, interval time.Duration) (time.Duration, bool) {
	threshold := 30 * time.Second
	if 5*interval > threshold {
		threshold = 5 * interval
	}
	gap := now.Sub(last)
	return gap, gap > threshold
}
