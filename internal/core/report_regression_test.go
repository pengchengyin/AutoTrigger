package core

import (
	"context"
	"strings"
	"testing"
)

func TestD01DirectBlacklistedInputsNeverStart(t *testing.T) {
	for _, name := range []string{"unins000.exe", "卸载.exe", "update.exe", "crashpad_handler.exe", "checkupdate.exe", "repair.exe", "modify.exe"} {
		t.Run(name, func(t *testing.T) {
			e, r := engineFor(t)
			e.Run(context.Background(), sample(t, name, []byte("MZfixture")))
			f := e.Result.Results[0]
			if len(r.commands) != 0 || len(f.Attempts) != 0 || f.FinalStatus != "skipped" || !strings.HasPrefix(f.Notes, "excluded_blacklist:") {
				t.Fatalf("%+v commands=%v", f, r.commands)
			}
		})
	}
}
