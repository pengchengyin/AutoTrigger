package detect

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMagicPrecedence(t *testing.T) {
	for _, tc := range []struct {
		name      string
		data      []byte
		kind      string
		disguised bool
	}{{"fake.txt", []byte("MZpayload"), "pe_executable", true}, {"fake.exe", []byte("%PDF-1.7"), "document", true}, {"app.msi", []byte{0xd0, 0xcf, 0x11, 0xe0, 0xa1, 0xb1, 0x1a, 0xe1}, "msi", false}, {"a.docx", []byte{0x50, 0x4b, 3, 4}, "document", false}, {"a.zip", []byte{0x50, 0x4b, 3, 4}, "archive", false}, {"a.ps1", []byte("Write-Output 'hello'"), "script", false}, {"a.data", []byte("something"), "unknown", false}} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), tc.name)
			os.WriteFile(path, tc.data, 0600)
			ft, err := (Detector{}).Detect(path)
			if err != nil || ft.Kind != tc.kind || ft.Disguised != tc.disguised {
				t.Fatalf("%+v %v", ft, err)
			}
		})
	}
	iso := make([]byte, 0x8006)
	copy(iso[0x8001:], "CD001")
	path := filepath.Join(t.TempDir(), "fake.bin")
	os.WriteFile(path, iso, 0600)
	ft, _ := (Detector{}).Detect(path)
	if ft.Kind != "disk_image" {
		t.Fatal(ft)
	}
}
