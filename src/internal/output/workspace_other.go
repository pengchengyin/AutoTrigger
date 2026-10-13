//go:build !windows

package output

func ownerExited(pid int) bool { return false }
