// Package detect identifies files without loading or executing their contents.
package detect

import (
	"autotrigger/internal/core/ports"
	"bytes"
	"debug/pe"
	"io"
	"os"
	"path/filepath"
	"strings"
)

type Detector struct{}

func (Detector) Detect(path string) (ports.FileType, error) {
	ext := strings.ToLower(filepath.Ext(path))
	ft := ports.FileType{Kind: ExtensionKind(ext), Extension: ext}
	f, err := os.Open(path)
	if err != nil {
		return ft, err
	}
	defer f.Close()
	buf := make([]byte, 1024*1024)
	n, err := f.Read(buf)
	if err != nil && err != io.EOF {
		return ft, err
	}
	buf = buf[:n]
	kind := ""
	switch {
	case bytes.HasPrefix(buf, []byte("MZ")):
		kind = "pe_executable"
		pf, e := pe.Open(path)
		if e == nil {
			ft.DLL = pf.Characteristics&0x2000 != 0
			pf.Close()
		}
		if ft.DLL || ext == ".dll" {
			kind = "pe_dll"
		}
		lower := bytes.ToLower(buf)
		for _, p := range []struct{ sig, name string }{{"inno setup", "inno"}, {"nullsoft", "nsis"}, {"installshield", "installshield"}, {"wixburn", "wixburn"}, {"advanced installer", "advinstaller"}} {
			if bytes.Contains(lower, []byte(p.sig)) {
				ft.Family = p.name
				break
			}
		}
	case bytes.HasPrefix(buf, []byte{0x50, 0x4b, 3, 4}), bytes.HasPrefix(buf, []byte{0x50, 0x4b, 5, 6}), bytes.HasPrefix(buf, []byte{0x50, 0x4b, 7, 8}):
		kind = "archive"
		if officeZIP(ext) {
			kind = "document"
		}
	case bytes.HasPrefix(buf, []byte{0xd0, 0xcf, 0x11, 0xe0, 0xa1, 0xb1, 0x1a, 0xe1}):
		kind = "document"
		if ext == ".msi" || ext == ".msp" || bytes.Contains(buf, []byte("_StringPool")) || bytes.Contains(buf, []byte{'_', '\x00', 'S', '\x00', 't', '\x00', 'r', '\x00', 'i', '\x00', 'n', '\x00', 'g', '\x00', 'P', '\x00', 'o', '\x00', 'o', '\x00', 'l', '\x00'}) {
			kind = "msi"
		}
	case bytes.HasPrefix(buf, []byte{0x37, 0x7a, 0xbc, 0xaf, 0x27, 0x1c}), bytes.HasPrefix(buf, []byte("Rar!")), bytes.HasPrefix(buf, []byte{0x1f, 0x8b}), len(buf) > 262 && string(buf[257:262]) == "ustar":
		kind = "archive"
	case bytes.HasPrefix(buf, []byte("%PDF")), bytes.HasPrefix(buf, []byte("{\\rtf")):
		kind = "document"
	case bytes.HasPrefix(buf, []byte{0x4c, 0, 0, 0, 1, 0x14, 2, 0, 0, 0, 0, 0, 0xc0, 0, 0, 0, 0, 0, 0, 0x46}):
		kind = "shortcut"
	case len(buf) >= 0x8006 && string(buf[0x8001:0x8006]) == "CD001":
		kind = "disk_image"
	}
	if kind != "" {
		ft.Disguised = ft.Kind != kind
		ft.Kind = kind
	}
	return ft, nil
}

func officeZIP(ext string) bool {
	return ext == ".docx" || ext == ".xlsx" || ext == ".pptx" || ext == ".docm" || ext == ".xlsm" || ext == ".pptm"
}
func ExtensionKind(ext string) string {
	switch strings.ToLower(ext) {
	case ".exe", ".scr", ".com":
		return "pe_executable"
	case ".dll":
		return "pe_dll"
	case ".msi", ".msp":
		return "msi"
	case ".bat", ".cmd", ".ps1", ".vbs", ".js", ".wsf", ".hta":
		return "script"
	case ".zip", ".rar", ".7z", ".tar", ".gz", ".bz2", ".xz":
		return "archive"
	case ".iso", ".img", ".nrg", ".mds", ".cue":
		return "disk_image"
	case ".lnk":
		return "shortcut"
	case ".doc", ".docx", ".xls", ".xlsx", ".ppt", ".pptx", ".pdf", ".rtf", ".docm", ".xlsm", ".pptm":
		return "document"
	default:
		return "unknown"
	}
}
