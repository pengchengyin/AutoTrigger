package ui

// Choices follow the native Yes/No/Cancel dialog result values.
func settingsLeaveDecision(choice int, save func() bool, discard func()) bool {
	switch choice {
	case 6:
		return save()
	case 7:
		discard()
		return true
	default:
		return false
	}
}
