package ui

import "testing"

func TestRetryOptionsUsesSelectedTask(t *testing.T) {
	previous := Options{Input: "old.exe", Output: "old.json", Recursive: true, TimeoutSec: 123, Lock: true}
	record := taskRecord{Input: "selected.exe", Result: "selected.json", Config: "selected.conf", Execution: &previous}
	got := retryOptions(record, Options{Input: "draft.exe", TimeoutSec: 999})
	if got.Input != record.Input || got.Output != record.Result || got.Config != record.Config || got.TimeoutSec != 123 || !got.Recursive || !got.Lock {
		t.Fatalf("retry options: %+v", got)
	}
	if previous.Input != "old.exe" || previous.Output != "old.json" {
		t.Fatal("historical options modified")
	}
}

func TestRetryOldRecordUsesCurrentSettings(t *testing.T) {
	got := retryOptions(taskRecord{Input: "selected.exe", Result: "selected.json"}, Options{Input: "draft.exe", Config: "current.conf", TimeoutSec: 300})
	if got.Input != "selected.exe" || got.Config != "current.conf" || got.TimeoutSec != 300 {
		t.Fatalf("legacy retry: %+v", got)
	}
}
