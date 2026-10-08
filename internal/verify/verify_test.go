package verify

import (
	"autotrigger/internal/config"
	"autotrigger/internal/core/ports"
	"autotrigger/pkg/schema"
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDecisionProfiles(t *testing.T) {
	ev := func(ids ...string) []schema.Evidence {
		out := []schema.Evidence{}
		for _, id := range ids {
			level := id[:1]
			if level == "N" {
				level = "N"
			}
			out = append(out, schema.Evidence{ID: id, Level: level})
		}
		return out
	}
	cases := []struct {
		name, profile                string
		ids                          []string
		side, pre, started, timeout  bool
		artifact, status, confidence string
	}{{"exit-zero-protection", "loose", []string{"A1"}, false, false, true, false, "unknown", "install_unverified", "low"}, {"duplicate-exit-zero", "loose", []string{"A1", "A1"}, false, false, true, false, "unknown", "install_unverified", "low"}, {"strong", "loose", []string{"S1"}, true, false, true, false, "ok", "install_completed", "high"}, {"artifact-failed", "strict", []string{"S1"}, true, false, true, false, "failed", "install_completed", "medium"}, {"loose-side-effect", "loose", []string{"A3"}, true, false, true, false, "unknown", "install_completed", "medium"}, {"strict-side-effect", "strict", []string{"A3"}, true, false, true, false, "unknown", "install_unverified", "low"}, {"finish-corroboration", "loose", []string{"A1", "A2"}, false, false, true, false, "unknown", "install_completed", "medium"}, {"two-weak", "loose", []string{"B1", "B2"}, false, false, true, false, "unknown", "install_completed", "low"}, {"strict-weak", "strict", []string{"B1", "B2"}, false, false, true, false, "unknown", "install_unverified", "low"}, {"rollback", "loose", []string{"S1", "N2"}, true, false, true, false, "ok", "install_partial", "low"}, {"negative-no-side", "loose", []string{"N1"}, false, false, true, false, "unknown", "failed", ""}, {"already-installed", "loose", []string{"A1"}, false, true, true, false, "ok", "already_installed", "high"}, {"ordinary-trigger", "loose", nil, false, false, true, false, "unknown", "triggered", ""}, {"timeout", "loose", nil, false, false, true, true, "unknown", "killed_on_timeout", ""}, {"timeout-strong", "loose", []string{"S2"}, true, false, true, true, "ok", "install_completed", "high"}, {"timeout-partial", "loose", nil, true, false, true, true, "unknown", "install_partial", "low"}, {"not-started", "loose", nil, false, false, false, false, "unknown", "failed", ""}}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d := Decide(tc.profile, ev(tc.ids...), tc.side, tc.pre, tc.started, tc.timeout, tc.artifact)
			if d.Status != tc.status {
				t.Fatalf("%s != %s", d.Status, tc.status)
			}
			if tc.confidence == "" {
				if d.Confidence != nil {
					t.Fatal("unexpected confidence")
				}
			} else if d.Confidence == nil || *d.Confidence != tc.confidence {
				t.Fatalf("confidence: %v", d.Confidence)
			}
		})
	}
}
func TestFilesystemSnapshot(t *testing.T) {
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, "App"), 0755)
	os.WriteFile(filepath.Join(root, "App", "app.exe"), []byte("MZ"), 0600)
	os.MkdirAll(filepath.Join(root, "Temp"), 0755)
	os.WriteFile(filepath.Join(root, "Temp", "cache"), []byte("ignored"), 0600)
	c := config.Defaults().Verify
	c.WatchDirs = []string{root}
	c.WatchExclude = []string{filepath.Join(root, "Temp")}
	dirs, truncated, _ := Files(context.Background(), c)
	if truncated {
		t.Fatal("unexpected truncation")
	}
	if _, ok := dirs[filepath.Join(root, "Temp")]; ok {
		t.Fatal("temporary directory included")
	}
	if dirs[filepath.Join(root, "App")].Files != 1 {
		t.Fatal("app files not counted")
	}
	c.SnapshotMaxEntries = 1
	_, truncated, _ = Files(context.Background(), c)
	if !truncated {
		t.Fatal("entry limit ignored")
	}
}

type source struct {
	b        *ports.Baseline
	events   []ports.Event
	artifact bool
}

func (s source) Snapshot(context.Context, config.Verify) (*ports.Baseline, error) { return s.b, nil }
func (s source) Events(context.Context, uint64) ([]ports.Event, error)            { return s.events, nil }
func (s source) ValidArtifact(string) bool                                        { return s.artifact }
func TestObserveMSIAndRollback(t *testing.T) {
	base := &ports.Baseline{Registry: map[string]ports.UninstallEntry{}, Dirs: map[string]ports.DirState{}, CapturedAt: time.Now()}
	after := &ports.Baseline{Registry: map[string]ports.UninstallEntry{"hkcu\\app": {DisplayName: "App", DisplayIcon: "app.exe"}}, Dirs: map[string]ports.DirState{}, CapturedAt: time.Now()}
	c := config.Defaults().Verify
	c.SettleMaxSec = 0
	v := Verifier{Source: source{b: after, artifact: true}, Config: c}
	exit := 3010
	got, err := v.Observe(context.Background(), "App_setup.exe", base, ports.Command{MSI: true}, &exit, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	if !got.SideEffects || !got.RequiresReboot || got.Artifact != "ok" {
		t.Fatalf("%+v", got)
	}
	d := Decide("loose", got.Evidence, got.SideEffects, false, true, false, got.Artifact)
	if d.Status != "install_completed" {
		t.Fatal(d)
	}
	exit = 1603
	got, _ = v.Observe(context.Background(), "app.exe", base, ports.Command{MSI: true}, &exit, nil, "")
	d = Decide("loose", got.Evidence, got.SideEffects, false, true, false, got.Artifact)
	if d.Status != "install_partial" {
		t.Fatal(d)
	}
}
