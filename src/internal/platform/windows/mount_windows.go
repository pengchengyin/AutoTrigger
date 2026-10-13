//go:build windows

package windows

import (
	"autotrigger/internal/core/ports"
	"autotrigger/internal/platform/windows/winver"
	"autotrigger/internal/procutil"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type Mounter struct {
	Runner ports.ProcessRunner
	Tool   string
}

func psQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", "''") + "'" }
func (m Mounter) Mount(ctx context.Context, path string) (string, error) {
	if strings.HasPrefix(path, "\\\\") {
		return "", fmt.Errorf("mount requires a local image path")
	}
	if !winver.Current().NativeMount() {
		return "", fmt.Errorf("Win7 mounting requires a separately deployed and validated WinCDEmu adapter: %w", ports.ErrUnsupported)
	}
	script := "$ErrorActionPreference='Stop';$p=" + psQuote(path) + ";$old=Get-DiskImage -ImagePath $p -ErrorAction SilentlyContinue;if($old -and $old.Attached){throw 'image already mounted; ownership unknown'};$disk=Mount-DiskImage -ImagePath $p -PassThru;$v=$disk|Get-Volume;if(!$v.DriveLetter){Dismount-DiskImage -ImagePath $p;throw 'no drive letter'};[Console]::Out.Write(($v.DriveLetter+':\\'))"
	e, err := procutil.Run(ctx, m.Runner, ports.Command{Program: systemExe("WindowsPowerShell\\v1.0\\powershell.exe"), HideWindow: true, Args: []string{"-NoProfile", "-NonInteractive", "-Command", script}})
	if err != nil {
		return "", err
	}
	drive := strings.TrimSpace(e.Stdout)
	if len(drive) != 3 || drive[1] != ':' || drive[2] != '\\' {
		return "", fmt.Errorf("invalid mounted volume: %q", drive)
	}
	return drive, nil
}
func (m Mounter) Dismount(ctx context.Context, path string) error {
	_, err := procutil.Run(ctx, m.Runner, ports.Command{Program: systemExe("WindowsPowerShell\\v1.0\\powershell.exe"), HideWindow: true, Args: []string{"-NoProfile", "-NonInteractive", "-Command", "$ErrorActionPreference='Stop';Dismount-DiskImage -ImagePath " + psQuote(path)}})
	return err
}
func (m Mounter) MotW(imagePath, root, policy string) error {
	if policy == "native" {
		return nil
	}
	var zone []byte
	if policy == "preserve" {
		var err error
		zone, err = os.ReadFile(imagePath + ":Zone.Identifier")
		if os.IsNotExist(err) {
			return nil
		}
		if err != nil {
			return err
		}
	}
	return filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if policy == "clear" {
			err = os.Remove(path + ":Zone.Identifier")
			if os.IsNotExist(err) {
				return nil
			}
			return err
		}
		return os.WriteFile(path+":Zone.Identifier", zone, 0600)
	})
}
