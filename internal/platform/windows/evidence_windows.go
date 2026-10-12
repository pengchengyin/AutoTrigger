//go:build windows

package windows

import (
	"autotrigger/internal/config"
	"autotrigger/internal/core/ports"
	"autotrigger/internal/verify"
	"context"
	"debug/pe"
	"encoding/xml"
	"fmt"
	"strconv"
	"syscall"
	"time"
	"unsafe"
)

type Evidence struct{}

func regOpen(root syscall.Handle, path string) (syscall.Handle, error) {
	var h syscall.Handle
	err := syscall.RegOpenKeyEx(root, utf(path), 0, syscall.KEY_READ, &h)
	return h, err
}
func regString(h syscall.Handle, name string) string {
	var typ, n uint32
	if syscall.RegQueryValueEx(h, utf(name), nil, &typ, nil, &n) != nil || n < 2 || n > 65536 {
		return ""
	}
	b := make([]uint16, n/2+1)
	if syscall.RegQueryValueEx(h, utf(name), nil, &typ, (*byte)(unsafe.Pointer(&b[0])), &n) != nil {
		return ""
	}
	if typ != syscall.REG_SZ && typ != syscall.REG_EXPAND_SZ {
		return ""
	}
	return syscall.UTF16ToString(b)
}
func registrySnapshot(ctx context.Context) (map[string]ports.UninstallEntry, []string) {
	out := map[string]ports.UninstallEntry{}
	warnings := []string{}
	roots := []struct {
		root         syscall.Handle
		prefix, path string
	}{{syscall.HKEY_LOCAL_MACHINE, "hklm", "SOFTWARE\\Microsoft\\Windows\\CurrentVersion\\Uninstall"}, {syscall.HKEY_LOCAL_MACHINE, "hklm_wow6432", "SOFTWARE\\WOW6432Node\\Microsoft\\Windows\\CurrentVersion\\Uninstall"}, {syscall.HKEY_CURRENT_USER, "hkcu", "SOFTWARE\\Microsoft\\Windows\\CurrentVersion\\Uninstall"}}
	for _, r := range roots {
		h, err := regOpen(r.root, r.path)
		if err != nil {
			if err != syscall.ERROR_FILE_NOT_FOUND {
				warnings = append(warnings, r.prefix+": "+err.Error())
			}
			continue
		}
		for i := uint32(0); i < 20000; i++ {
			if ctx.Err() != nil {
				break
			}
			name := make([]uint16, 512)
			n := uint32(len(name))
			err = syscall.RegEnumKeyEx(h, i, &name[0], &n, nil, nil, nil, nil)
			if err == syscall.Errno(259) {
				break
			}
			if err != nil {
				continue
			}
			key := syscall.UTF16ToString(name[:n])
			sub, err := regOpen(h, key)
			if err != nil {
				continue
			}
			id := r.prefix + "\\" + key
			out[id] = ports.UninstallEntry{Publisher: regString(sub, "Publisher"), Key: id, DisplayName: regString(sub, "DisplayName"), DisplayVersion: regString(sub, "DisplayVersion"), InstallLocation: regString(sub, "InstallLocation"), DisplayIcon: regString(sub, "DisplayIcon"), UninstallString: regString(sub, "UninstallString")}
			syscall.RegCloseKey(sub)
		}
		syscall.RegCloseKey(h)
	}
	return out, warnings
}
func (Evidence) Snapshot(ctx context.Context, c config.Verify) (*ports.Baseline, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	reg, warnings := registrySnapshot(ctx)
	dirs, truncated, fileWarnings := verify.Files(ctx, c)
	warnings = append(warnings, fileWarnings...)
	events, err := queryEvents(ctx, 0, 1, true)
	var watermark uint64
	if err == nil && len(events) > 0 {
		watermark = events[0].RecordID
	} else if err != nil {
		warnings = append(warnings, "MSI 安装事件快照采集失败："+err.Error())
	}
	return &ports.Baseline{Registry: reg, Dirs: dirs, EventWatermark: watermark, CapturedAt: time.Now(), Truncated: truncated, Warnings: warnings}, nil
}
func (Evidence) Events(ctx context.Context, watermark uint64) ([]ports.Event, error) {
	return queryEvents(ctx, watermark, 128, false)
}
func (Evidence) ValidArtifact(path string) bool {
	f, err := pe.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()
	return f.OptionalHeader != nil && f.Characteristics&0x2000 == 0
}

var wevtapi = syscall.NewLazyDLL("wevtapi.dll")

func queryEvents(ctx context.Context, watermark uint64, limit int, reverse bool) ([]ports.Event, error) {
	query := fmt.Sprintf("*[System[Provider[@Name='MsiInstaller'] and EventRecordID > %d]]", watermark)
	flags := uintptr(1)
	if reverse {
		flags |= 0x200
	}
	h, _, err := wevtapi.NewProc("EvtQuery").Call(0, uintptr(unsafe.Pointer(utf("Application"))), uintptr(unsafe.Pointer(utf(query))), flags)
	if h == 0 {
		return nil, err
	}
	defer wevtapi.NewProc("EvtClose").Call(h)
	out := []ports.Event{}
	for len(out) < limit {
		if ctx.Err() != nil {
			return out, ctx.Err()
		}
		var event uintptr
		var count uint32
		r, _, err := wevtapi.NewProc("EvtNext").Call(h, 1, uintptr(unsafe.Pointer(&event)), 100, 0, uintptr(unsafe.Pointer(&count)))
		if r == 0 {
			if err == syscall.Errno(259) {
				break
			}
			return out, err
		}
		buf := make([]uint16, 32768)
		var used, properties uint32
		r, _, err = wevtapi.NewProc("EvtRender").Call(0, event, 1, uintptr(len(buf)*2), uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(&used)), uintptr(unsafe.Pointer(&properties)))
		wevtapi.NewProc("EvtClose").Call(event)
		if r == 0 {
			return out, err
		}
		text := syscall.UTF16ToString(buf)
		var data struct {
			System struct {
				RecordID uint64 `xml:"EventRecordID"`
				ID       int    `xml:"EventID"`
				Time     struct {
					Value string `xml:"SystemTime,attr"`
				} `xml:"TimeCreated"`
			} `xml:"System"`
			Data struct {
				Values []string `xml:"Data"`
			} `xml:"EventData"`
		}
		if xml.Unmarshal([]byte(text), &data) != nil {
			continue
		}
		at, _ := time.Parse(time.RFC3339Nano, data.System.Time.Value)
		out = append(out, ports.Event{RecordID: data.System.RecordID, ID: data.System.ID, At: at, Detail: strconv.Itoa(data.System.ID) + " " + fmt.Sprint(data.Data.Values)})
	}
	return out, nil
}
