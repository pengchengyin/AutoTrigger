//go:build windows

package windows

import "testing"

func TestEnumerationCallbackDoesNotAccumulate(t *testing.T) {
	callback := enumerationCallback
	for i := 0; i < 2500; i++ {
		calls := 0
		if err := enumerateWindows(0, false, func(hwnd, token uintptr) uintptr {
			calls++
			return 1
		}); err != nil {
			t.Fatal(err)
		}
		if calls == 0 || enumerationCallback != callback {
			t.Fatal("enumeration did not use the fixed callback")
		}
	}
	enumerations.Lock()
	defer enumerations.Unlock()
	if len(enumerations.visitors) != 0 {
		t.Fatal("enumeration visitor leaked")
	}
}
