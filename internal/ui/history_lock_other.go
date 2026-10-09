//go:build !windows

package ui

import "sync"

// Non-Windows builds expose no GUI; serialize the helper's tests in process.
var historyLock sync.Mutex

func acquireHistoryLock(string) (func(), error) {
	historyLock.Lock()
	return historyLock.Unlock, nil
}
