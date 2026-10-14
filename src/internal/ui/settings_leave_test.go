package ui

import "testing"

func TestSettingsLeaveDecision(t *testing.T) {
	for _, tc := range []struct {
		name                    string
		choice                  int
		saved, leave            bool
		saveCalls, discardCalls int
	}{
		{"save", 6, true, true, 1, 0}, {"save failure", 6, false, false, 1, 0}, {"discard", 7, false, true, 0, 1}, {"cancel", 2, false, false, 0, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, d := 0, 0
			got := settingsLeaveDecision(tc.choice, func() bool { s++; return tc.saved }, func() { d++ })
			if got != tc.leave || s != tc.saveCalls || d != tc.discardCalls {
				t.Fatalf("leave=%t save=%d discard=%d", got, s, d)
			}
		})
	}
}
