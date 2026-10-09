package cli

import (
	"autotrigger/internal/config"
	"autotrigger/internal/core/ports"
	"autotrigger/internal/platform/stub"
	"autotrigger/internal/resources"
	"autotrigger/pkg/schema"
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func stubDeps() Dependencies {
	a := stub.Adapter{}
	return Dependencies{Runner: a, Trigger: func(config.Config, resources.Pack) ports.InstallerTrigger { return a }, Opener: a, Mounter: a, Evidence: a, NativeGUI: a, Lock: a, Prereqs: a}
}

type checkModeProbe struct{ readOnly bool }

func (p *checkModeProbe) Check(context.Context, config.Config) (schema.Prereq, map[string]interface{}, error) {
	p.readOnly = false
	r := schema.EmptyPrereq()
	r.ArchiveBackend = schema.String("test-7z.exe")
	return r, nil, nil
}
func (p *checkModeProbe) CheckOnly(ctx context.Context, c config.Config) (schema.Prereq, map[string]interface{}, error) {
	r, meta, err := p.Check(ctx, c)
	p.readOnly = true
	return r, meta, err
}
func TestPrereqDeploymentCanBeDisabled(t *testing.T) {
	for _, readOnly := range []bool{false, true} {
		probe := &checkModeProbe{}
		deps := stubDeps()
		deps.Prereqs = probe
		args := []string{"--check-prereq", "--json", "--output=-", "--log-file", filepath.Join(t.TempDir(), "check.log")}
		if readOnly {
			args = append(args, "--no-deploy-deps")
		}
		var stdout, stderr bytes.Buffer
		if code := Run(context.Background(), args, &stdout, &stderr, deps); code != 0 || probe.readOnly != readOnly || !json.Valid(stdout.Bytes()) {
			t.Fatalf("readonly=%v got=%v code=%d stderr=%s", readOnly, probe.readOnly, code, stderr.String())
		}
	}
}
func TestInformationStdoutIsOneJSONDocument(t *testing.T) {
	for _, args := range [][]string{{"--dump-config"}, {"--version"}, {"--dump-config", "--timeout=0"}} {
		var stdout, stderr bytes.Buffer
		code := Run(context.Background(), args, &stdout, &stderr, stubDeps())
		if code != 0 || !json.Valid(stdout.Bytes()) {
			t.Fatalf("%v code=%d output=%s error=%s", args, code, stdout.String(), stderr.String())
		}
	}
}
func TestFailureArtifactsAreComplete(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing")
	configPath := filepath.Join(t.TempDir(), "bad.json")
	os.WriteFile(configPath, []byte(`{"ocr":{"min_score":3}}`), 0600)
	for _, tc := range []struct {
		args []string
		code int
	}{{[]string{"--input", missing}, 2}, {[]string{"--input", missing, "--typo"}, 2}, {[]string{"--config", configPath, "--input", missing}, 3}, {[]string{"--timeout=-1", "--input", missing}, 3}, {[]string{"--input", missing, "unexpected"}, 2}} {
		var stdout, stderr bytes.Buffer
		args := append([]string{"--log-file", filepath.Join(t.TempDir(), "result.log")}, tc.args...)
		args = append(args, "--output=-")
		code := Run(context.Background(), args, &stdout, &stderr, stubDeps())
		if code != tc.code {
			t.Fatalf("%v => %d stderr=%s", args, code, stderr.String())
		}
		var parsed schema.Result
		if err := json.Unmarshal(stdout.Bytes(), &parsed); err != nil {
			t.Fatal(err, stdout.String())
		}
		if parsed.Error == nil || parsed.Error.Code != tc.code || parsed.Results == nil || parsed.FinishedAt == nil {
			t.Fatalf("incomplete failure: %+v", parsed)
		}
	}
}
func TestCLIOverridesPartialConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	os.WriteFile(path, []byte(`{"timeout_sec":300,"ocr":{"min_score":0.8}}`), 0600)
	var out, errout bytes.Buffer
	code := Run(context.Background(), []string{"--config", path, "--timeout", "60", "--dump-config"}, &out, &errout, stubDeps())
	if code != 0 {
		t.Fatal(errout.String())
	}
	var c config.Config
	json.Unmarshal(out.Bytes(), &c)
	if c.TimeoutSec != 60 || c.OCR.MinScore != .8 || c.OCR.Engine != "paddleocr_json" {
		t.Fatalf("%+v", c)
	}
}
func TestCannotOverwriteInputWithFailureOutput(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sample.exe")
	original := []byte("MZoriginal")
	os.WriteFile(path, original, 0600)
	var out, errout bytes.Buffer
	code := Run(context.Background(), []string{"--input", path, "--output", path, "--invalid"}, &out, &errout, stubDeps())
	data, _ := os.ReadFile(path)
	if code != 2 || !bytes.Equal(data, original) || !json.Valid(out.Bytes()) {
		t.Fatal("failure output corrupted sample")
	}
}
func TestTemplateSelfTestDoesNotPretendReady(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Run(context.Background(), []string{"--self-test", "templates"}, &stdout, &stderr, stubDeps())
	if code != 1 || !json.Valid(stdout.Bytes()) || !strings.Contains(stdout.String(), `"ok": false`) {
		t.Fatal("empty pack reported usable", stdout.String(), stderr.String())
	}
}
func TestHelpGoesToStdoutWithZeroExit(t *testing.T) {
	for _, args := range [][]string{{"--help"}, {"-h"}, {"-help"}, {"--help", "--input", "x"}} {
		var stdout, stderr bytes.Buffer
		code := Run(context.Background(), args, &stdout, &stderr, stubDeps())
		if code != 0 {
			t.Fatalf("%v => code %d stderr=%s", args, code, stderr.String())
		}
		if !strings.Contains(stdout.String(), "用法:") || !strings.Contains(stdout.String(), "--check-prereq") || !strings.Contains(stdout.String(), "退出码") {
			t.Fatalf("%v helped incompletely: %s", args, stdout.String())
		}
		if stderr.Len() != 0 {
			t.Fatalf("%v wrote help to stderr: %s", args, stderr.String())
		}
	}
}
func TestNoArgumentsPrintsUsageWithoutArtifacts(t *testing.T) {
	dir := t.TempDir()
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err = os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(cwd)
	var stdout, stderr bytes.Buffer
	code := Run(context.Background(), []string{}, &stdout, &stderr, stubDeps())
	if code != 2 || stdout.Len() != 0 || !strings.Contains(stderr.String(), "用法:") {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var left []string
	for _, e := range entries {
		left = append(left, e.Name())
	}
	if len(left) != 0 {
		t.Fatalf("no-argument run left artifacts behind: %v", left)
	}
}
func TestUnknownFlagPointsToHelp(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Run(context.Background(), []string{"--input", "x", "--typo"}, &stdout, &stderr, stubDeps())
	if code != 2 || !strings.Contains(stderr.String(), "typo") || !strings.Contains(stderr.String(), "--help") {
		t.Fatalf("code=%d stderr=%s", code, stderr.String())
	}
}
