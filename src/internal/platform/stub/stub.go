// Package stub preserves cross-platform builds while explicitly rejecting execution.
package stub

import (
	"autotrigger/internal/config"
	"autotrigger/internal/core/ports"
	"autotrigger/pkg/schema"
	"context"
	"image"
	"time"
)

type Adapter struct{}

func (Adapter) Start(context.Context, ports.Command) (ports.Process, error) {
	return nil, ports.ErrUnsupported
}
func (Adapter) OpenCommand(string) (ports.Command, error) {
	return ports.Command{}, ports.ErrUnsupported
}
func (Adapter) HasAssociation(string) bool { return false }
func (Adapter) ResolveShortcut(string) (string, []string, error) {
	return "", nil, ports.ErrUnsupported
}
func (Adapter) Commands(string, ports.FileType, string) ([]ports.Command, error) {
	return nil, ports.ErrUnsupported
}
func (Adapter) Mount(context.Context, string) (string, error) { return "", ports.ErrUnsupported }
func (Adapter) Dismount(context.Context, string) error        { return ports.ErrUnsupported }
func (Adapter) MotW(string, string, string) error             { return ports.ErrUnsupported }
func (Adapter) Snapshot(context.Context, config.Verify) (*ports.Baseline, error) {
	return nil, ports.ErrUnsupported
}
func (Adapter) Events(context.Context, uint64) ([]ports.Event, error) {
	return nil, ports.ErrUnsupported
}
func (Adapter) ValidArtifact(string) bool { return false }
func (Adapter) Acquire(context.Context, time.Duration) (func(), error) {
	return nil, ports.ErrUnsupported
}
func (Adapter) Windows([]int) ([]ports.Window, error)          { return nil, ports.ErrUnsupported }
func (Adapter) Controls(ports.Window) ([]ports.Control, error) { return nil, ports.ErrUnsupported }
func (Adapter) Screenshot(ports.Window) (image.Image, error)   { return nil, ports.ErrUnsupported }
func (Adapter) Click(ports.Control) error                      { return ports.ErrUnsupported }
func (Adapter) SetText(ports.Control, string) error            { return ports.ErrUnsupported }
func (Adapter) CloseWindow(ports.Window) error                 { return ports.ErrUnsupported }
func (Adapter) Close() error                                   { return nil }
func (Adapter) Check(context.Context, config.Config) (schema.Prereq, map[string]interface{}, error) {
	r := schema.EmptyPrereq()
	r.Issues = append(r.Issues, "本平台仅有编译占位实现：实际执行能力只在 Windows 提供")
	return r, map[string]interface{}{}, ports.ErrUnsupported
}
