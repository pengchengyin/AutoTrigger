//go:build windows

package windows

import "testing"

func TestAssociationCannotExecuteDocumentItself(t *testing.T) {
	path := `C:\samples\disguised.exe`
	for _, program := range []string{"%1", "%L", `C:\samples\disguised.exe`, `c:\SAMPLES\DISGUISED.EXE`, `C:\Windows\System32\OpenWith.exe`} {
		if associationHasRealHost(program, path) {
			t.Fatalf("invalid host allowed: %s", program)
		}
	}
	for _, program := range []string{`C:\Reader\reader.exe`, `C:\Windows\System32\notepad.exe`} {
		if !associationHasRealHost(program, path) {
			t.Fatalf("real host blocked: %s", program)
		}
	}
	// The real Windows .exe association is normally "%1" %*.
	if (Opener{}).HasAssociation(path) {
		t.Skip("host association overridden on this Windows installation; helper cases verified")
	}
	if _, err := (Opener{}).OpenCommand(path); err == nil {
		t.Fatal("disguised document executed through exefile association")
	}
}
