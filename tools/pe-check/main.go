// pe-check enforces system-only imports for the distributable executable.
package main

import (
	"debug/pe"
	"fmt"
	"os"
	"sort"
	"strings"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: pe-check AutoTrigger.exe")
		os.Exit(2)
	}
	if err := check(os.Args[1]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func check(path string) error {
	f, err := pe.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	if f.Machine != pe.IMAGE_FILE_MACHINE_AMD64 {
		return fmt.Errorf("expected AMD64 PE")
	}
	// Go 1.20's ImportedLibraries is a stub; symbols expose the actual import table.
	symbols, err := f.ImportedSymbols()
	if err != nil {
		return err
	}
	imports := map[string]bool{}
	for _, symbol := range symbols {
		if colon := strings.LastIndexByte(symbol, ':'); colon >= 0 {
			imports[strings.ToLower(symbol[colon+1:])] = true
		}
	}
	if len(imports) == 0 {
		return fmt.Errorf("PE import table is empty or unreadable")
	}
	dlls := make([]string, 0, len(imports))
	for dll := range imports {
		dlls = append(dlls, dll)
	}
	sort.Strings(dlls)
	allowed := map[string]bool{"kernel32.dll": true, "user32.dll": true, "advapi32.dll": true, "ole32.dll": true, "oleaut32.dll": true, "shell32.dll": true, "gdi32.dll": true, "ntdll.dll": true, "userenv.dll": true, "ws2_32.dll": true, "mswsock.dll": true, "crypt32.dll": true, "secur32.dll": true, "bcrypt.dll": true}
	for _, dll := range dlls {
		if !allowed[strings.ToLower(dll)] {
			return fmt.Errorf("non-system import rejected: %s", dll)
		}
	}
	s, err := os.Stat(path)
	if err != nil {
		return err
	}
	if s.Size() > 30*1024*1024 {
		return fmt.Errorf("executable exceeds 30 MiB")
	}
	fmt.Fprintf(os.Stderr, "PE check passed: AMD64, %.2f MiB, imports=%v\n", float64(s.Size())/1024/1024, dlls)
	return nil
}
