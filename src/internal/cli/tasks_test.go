package cli

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

func TestTaskCommandHelpAndValidation(t *testing.T) {
	for _, test := range []struct {
		args []string
		code int
	}{{[]string{"task", "--help"}, 0}, {[]string{"task", "submit"}, 2}, {[]string{"task", "cancel", "--id", "../bad"}, 2}, {[]string{"task", "unknown"}, 2}, {[]string{"task", "status", "--config", "other.json"}, 2}} {
		var stdout, stderr bytes.Buffer
		if code := Run(context.Background(), test.args, &stdout, &stderr, Dependencies{}); code != test.code {
			t.Fatal(test.args, code, stderr.String())
		}
		if test.code == 0 && !strings.Contains(stdout.String(), "桌面端") {
			t.Fatal(stdout.String())
		}
	}
}
