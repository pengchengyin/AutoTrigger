//go:build windows

package windows

import (
	"autotrigger/internal/config"
	"autotrigger/internal/core/ports"
	"autotrigger/internal/resources"
	"debug/pe"
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"unsafe"
)

type Trigger struct {
	Config config.Config
	Pack   resources.Pack
}

func (t Trigger) Commands(path string, ft ports.FileType, temp string) ([]ports.Command, error) {
	dir := filepath.Dir(path)
	out := []ports.Command{}
	add := func(program string, args []string, method string, msi bool) {
		out = append(out, ports.Command{Program: program, Args: args, Dir: dir, Method: method, MSI: msi})
	}
	switch ft.Kind {
	case "pe_executable":
		if ext := strings.ToLower(filepath.Ext(path)); ext == ".com" || ext == ".scr" {
			add(path, nil, "no_param_run", false)
			out[0].NonInstall = true
			return out, nil
		}
		rules, err := t.Pack.Rules("params/silent_params.windows.json")
		if err != nil {
			return nil, err
		}
		if ft.Family != "" {
			for i, r := range rules {
				if r.ID == ft.Family {
					rules = append([]resources.Rule{r}, append(rules[:i], rules[i+1:]...)...)
					break
				}
			}
		}
		max := t.Config.MaxAttemptsPerFile - 1
		if max > len(rules) {
			max = len(rules)
		}
		for i := 0; i < max; i++ {
			add(path, rules[i].Args, "silent_install", false)
		}
		add(path, nil, "no_param_run", false)
	case "pe_dll":
		names, err := dllExports(path, t.Config.DLLExportProbeCount)
		if err != nil {
			return nil, err
		}
		for _, name := range names {
			add(systemExe("rundll32.exe"), []string{path + "," + name}, "dll_probe", false)
		}
	case "msi":
		flag := "/i"
		if strings.EqualFold(filepath.Ext(path), ".msp") {
			flag = "/p"
		}
		for i, extra := range [][]string{{"/quiet", "/norestart"}, {"/passive", "/norestart"}, {"/norestart"}} {
			logPath := filepath.Join(temp, fmt.Sprintf("MSI-%d.log", i))
			args := append([]string{flag, path}, extra...)
			args = append(args, "/L*v", logPath)
			add(systemExe("msiexec.exe"), args, "msi", true)
			out[len(out)-1].LogPath = logPath
		}
	case "script":
		switch strings.ToLower(filepath.Ext(path)) {
		case ".bat", ".cmd":
			converted, err := normalizeBatch(path, temp)
			if err != nil {
				return nil, err
			}
			path = converted
			add(systemExe("cmd.exe"), []string{"/d", "/s", "/c", "\"" + path + "\""}, "script", false)
			// cmd.exe parses its shell tail differently from CommandLineToArgvW.
			out[len(out)-1].CommandLine = syscall.EscapeArg(systemExe("cmd.exe")) + " /d /s /c \"\"" + path + "\"\""
		case ".ps1":
			add(systemExe("WindowsPowerShell\\v1.0\\powershell.exe"), []string{"-NoProfile", "-ExecutionPolicy", "Bypass", "-File", path}, "script", false)
		case ".hta":
			add(systemExe("mshta.exe"), []string{path}, "script", false)
		default:
			add(systemExe("wscript.exe"), []string{path}, "script", false)
			add(systemExe("cscript.exe"), []string{"//nologo", path}, "script", false)
		}
	default:
		return nil, ports.ErrUnsupported
	}
	if len(out) > t.Config.MaxAttemptsPerFile {
		out = out[:t.Config.MaxAttemptsPerFile]
	}
	return out, nil
}

func dllExports(path string, limit int) ([]string, error) {
	f, err := pe.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var rva, size uint32
	switch h := f.OptionalHeader.(type) {
	case *pe.OptionalHeader32:
		rva = h.DataDirectory[0].VirtualAddress
		size = h.DataDirectory[0].Size
	case *pe.OptionalHeader64:
		rva = h.DataDirectory[0].VirtualAddress
		size = h.DataDirectory[0].Size
	default:
		return nil, fmt.Errorf("invalid PE optional header")
	}
	offset := func(addr uint32) (int, bool) {
		for _, s := range f.Sections {
			if addr >= s.VirtualAddress && uint64(addr) < uint64(s.VirtualAddress)+uint64(s.Size) {
				o := int(addr - s.VirtualAddress + s.Offset)
				return o, o >= 0 && o < len(data)
			}
		}
		return 0, false
	}
	o, ok := offset(rva)
	if !ok || o+40 > len(data) {
		return nil, fmt.Errorf("DLL has no valid export directory")
	}
	n := binary.LittleEndian.Uint32(data[o+24:])
	namesRVA := binary.LittleEndian.Uint32(data[o+32:])
	no, ok := offset(namesRVA)
	if !ok {
		return nil, fmt.Errorf("invalid DLL export names")
	}
	out := []string{}
	for i := uint32(0); i < n && len(out) < limit; i++ {
		at := no + int(i)*4
		if at+4 > len(data) {
			break
		}
		addr := binary.LittleEndian.Uint32(data[at:])
		p, ok := offset(addr)
		if !ok {
			continue
		}
		end := p
		for end < len(data) && end-p < 256 && data[end] != 0 {
			end++
		}
		if end > p && end < len(data) {
			name := string(data[p:end])
			if !strings.ContainsAny(name, " ,\"\r\n") {
				out = append(out, name)
			}
		}
	}
	_ = size
	if len(out) == 0 {
		return nil, fmt.Errorf("DLL has no named exports")
	}
	return out, nil
}

type Opener struct{}

func (Opener) HasAssociation(path string) bool { _, err := association(path); return err == nil }
func association(path string) (string, error) {
	if filepath.Ext(path) == "" {
		return "", fmt.Errorf("no associated host for %s", path)
	}
	buf := make([]uint16, 32768)
	n := uint32(len(buf))
	p := syscall.NewLazyDLL("shlwapi.dll").NewProc("AssocQueryStringW")
	r, _, _ := p.Call(0, 1, uintptr(unsafe.Pointer(utf(filepath.Ext(path)))), uintptr(unsafe.Pointer(utf("open"))), uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(&n)))
	if int32(r) < 0 || n < 2 {
		return "", fmt.Errorf("no associated host for %s", path)
	}
	command := syscall.UTF16ToString(buf)
	args, err := splitCommand(command)
	if err != nil || len(args) == 0 || strings.EqualFold(filepath.Base(args[0]), "OpenWith.exe") {
		return "", fmt.Errorf("no associated host for %s", path)
	}
	return command, nil
}
func (Opener) OpenCommand(path string) (ports.Command, error) {
	template, err := association(path)
	if err != nil {
		return ports.Command{}, err
	}
	template = strings.ReplaceAll(template, "%1", path)
	template = strings.ReplaceAll(template, "%L", path)
	template = strings.ReplaceAll(template, "%l", path)
	template = strings.ReplaceAll(template, "%*", "")
	args, err := splitCommand(template)
	if err != nil || len(args) == 0 {
		return ports.Command{}, fmt.Errorf("invalid association command")
	}
	return ports.Command{Program: args[0], Args: args[1:], Dir: filepath.Dir(path), Method: "document_host"}, nil
}
func splitCommand(s string) ([]string, error) {
	var n int32
	p, _, err := syscall.NewLazyDLL("shell32.dll").NewProc("CommandLineToArgvW").Call(uintptr(unsafe.Pointer(utf(s))), uintptr(unsafe.Pointer(&n)))
	if p == 0 {
		return nil, err
	}
	defer kernel32.NewProc("LocalFree").Call(p)
	ptrs := unsafe.Slice((**uint16)(nativeAddress(p)), int(n))
	out := make([]string, 0, n)
	for _, v := range ptrs {
		buf := unsafe.Slice(v, 32768)
		end := 0
		for end < len(buf) && buf[end] != 0 {
			end++
		}
		out = append(out, syscall.UTF16ToString(buf[:end]))
	}
	return out, nil
}
func (Opener) ResolveShortcut(path string) (string, []string, error) {
	relative, err := shortcutRelative(path)
	if err != nil {
		return "", nil, err
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	hr, _, _ := ole32.NewProc("CoInitializeEx").Call(0, 2)
	if int32(hr) < 0 {
		return "", nil, fmt.Errorf("initialize shortcut COM")
	}
	defer ole32.NewProc("CoUninitialize").Call()
	clsid := guid{0x00021401, 0, 0, [8]byte{0xc0, 0, 0, 0, 0, 0, 0, 0x46}}
	iid := guid{0x000214f9, 0, 0, [8]byte{0xc0, 0, 0, 0, 0, 0, 0, 0x46}}
	var link uintptr
	hr, _, _ = ole32.NewProc("CoCreateInstance").Call(uintptr(unsafe.Pointer(&clsid)), 0, 1, uintptr(unsafe.Pointer(&iid)), uintptr(unsafe.Pointer(&link)))
	if int32(hr) < 0 {
		return "", nil, fmt.Errorf("create ShellLink")
	}
	defer release(link)
	persistIID := guid{0x0000010b, 0, 0, [8]byte{0xc0, 0, 0, 0, 0, 0, 0, 0x46}}
	var persist uintptr
	if err := comCall(link, 0, uintptr(unsafe.Pointer(&persistIID)), uintptr(unsafe.Pointer(&persist))); err != nil {
		return "", nil, err
	}
	defer release(persist)
	if err := comCall(persist, 5, uintptr(unsafe.Pointer(utf(path))), 0); err != nil {
		return "", nil, err
	}
	target := make([]uint16, 32768)
	args := make([]uint16, 32768)
	if err := comCall(link, 3, uintptr(unsafe.Pointer(&target[0])), uintptr(len(target)), 0, 4); err != nil {
		return "", nil, err
	}
	comCall(link, 10, uintptr(unsafe.Pointer(&args[0])), uintptr(len(args)))
	argText := syscall.UTF16ToString(args)
	tokens, err := splitCommand("dummy " + argText)
	if err != nil {
		return "", nil, err
	}
	resolved := relocatedShortcutTarget(path, config.ExpandEnv(syscall.UTF16ToString(target)), config.ExpandEnv(relative))
	if resolved == "" {
		return "", nil, fmt.Errorf("shortcut has no filesystem target")
	}
	return resolved, tokens[1:], nil
}
