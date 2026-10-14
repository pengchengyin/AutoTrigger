// Package archive implements bounded extraction with an external 7-Zip executable.
package archive

import (
	"autotrigger/internal/core/layout"
	"autotrigger/internal/core/ports"
	"autotrigger/internal/procutil"
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type SevenZip struct {
	Path   string
	Runner ports.ProcessRunner
}

func (s SevenZip) List(ctx context.Context, path string) ([]ports.ArchiveEntry, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	e, err := procutil.Run(ctx, s.Runner, ports.Command{Program: s.Path, HideWindow: true, Args: []string{"l", "-slt", "-ba", "-sccUTF-8", "-p", "--", path}})
	if err != nil {
		return nil, extractionError("list", e, err)
	}
	entries, err := ParseList(e.Stdout)
	if err != nil {
		return nil, extractionError("list", e, err)
	}
	return entries, nil
}
func ParseList(output string) ([]ports.ArchiveEntry, error) {
	out := []ports.ArchiveEntry{}
	current := map[string]string{}
	flush := func() error {
		path := current["Path"]
		if path == "" {
			current = map[string]string{}
			return nil
		}
		if !layout.SafeRelative(path) {
			return fmt.Errorf("unsafe archive path: %s", path)
		}
		size := int64(0)
		if current["Size"] != "" {
			n, err := strconv.ParseInt(current["Size"], 10, 64)
			if err != nil || n < 0 {
				return fmt.Errorf("invalid archive entry size: %s", path)
			}
			size = n
		}
		attrs := current["Attributes"]
		link := current["Symbolic Link"] != "" || current["Hard Link"] != "" || strings.Contains(attrs, "l") || strings.HasPrefix(attrs, "L")
		if link {
			return fmt.Errorf("archive links are not supported: %s", path)
		}
		out = append(out, ports.ArchiveEntry{Path: path, Size: size, Directory: current["Folder"] == "+" || strings.HasPrefix(attrs, "D"), Link: link})
		current = map[string]string{}
		return nil
	}
	scan := bufio.NewScanner(strings.NewReader(output))
	scan.Buffer(make([]byte, 4096), 1024*1024)
	for scan.Scan() {
		line := strings.TrimSuffix(scan.Text(), "\r")
		if line == "" {
			if err := flush(); err != nil {
				return nil, err
			}
			continue
		}
		parts := strings.SplitN(line, " = ", 2)
		if len(parts) == 2 {
			current[parts[0]] = parts[1]
		}
	}
	if err := scan.Err(); err != nil {
		return nil, err
	}
	if err := flush(); err != nil {
		return nil, err
	}
	return out, nil
}
func Size(entries []ports.ArchiveEntry, limit int64) error {
	total := int64(0)
	for _, e := range entries {
		if e.Size > limit-total {
			return fmt.Errorf("extract_size_exceeded: declared size exceeds %d bytes", limit)
		}
		total += e.Size
	}
	return nil
}
func (s SevenZip) Extract(ctx context.Context, path, dest string, limit int64) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	done := make(chan error, 1)
	go func() {
		exit, err := procutil.Run(ctx, s.Runner, ports.Command{Program: s.Path, HideWindow: true, Args: []string{"x", "-y", "-p", "-sccUTF-8", "-o" + dest, "--", path}})
		done <- extractionError("extract", exit, err)
	}()
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case err := <-done:
			if err != nil {
				return err
			}
			return ValidateTree(dest, limit)
		case <-ctx.Done():
			select {
			case <-done:
			case <-time.After(time.Second):
			}
			return ctx.Err()
		case <-ticker.C:
			if err := ValidateTree(dest, limit); err != nil {
				cancel()
				select {
				case <-done:
				case <-time.After(time.Second):
				}
				return err
			}
		}
	}
}

type extractionFailure struct {
	cause      error
	diagnostic string
}

func (e extractionFailure) Error() string { return e.diagnostic }
func (e extractionFailure) Unwrap() error { return e.cause }

func extractionError(stage string, exit ports.Exit, err error) error {
	if err == nil || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	detail := strings.TrimSpace(err.Error() + "\n" + exit.Stdout + "\n" + exit.Stderr)
	runes := []rune(detail)
	if len(runes) > 4096 {
		detail = string(runes[:3072]) + "\n[diagnostics truncated]\n" + string(runes[len(runes)-1024:])
	}
	return extractionFailure{cause: err, diagnostic: "extract_failed: " + stage + ": " + detail}
}
func ValidateTree(root string, limit int64) error {
	var size int64
	return filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return err
		}
		if d.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("extract_failed: link detected at %s", path)
		}
		if d.IsDir() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return err
		}
		if info.Size() > limit-size {
			return fmt.Errorf("extract_size_exceeded: extracted size exceeds %d bytes", limit)
		}
		size += info.Size()
		return nil
	})
}
