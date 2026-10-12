package core

import (
	"autotrigger/internal/verify"
	"autotrigger/pkg/schema"
	"testing"
)

func TestLaterSuccessRetainsRollbackConflicts(t *testing.T) {
	for _, id := range []string{"N2", "N3", "N4", "N5", "N6"} {
		negative := resolveAttemptFailures([]schema.Evidence{{ID: "N1", Level: "N"}, {ID: id, Level: "N", Source: "original-resource"}})
		if len(negative) != 1 || negative[0].ID != id || negative[0].Source != "original-resource" {
			t.Fatal(negative)
		}
		ev := append([]schema.Evidence{{ID: "S1", Level: "S"}}, negative...)
		d := verify.Decide("loose", ev, true, false, true, false, "ok")
		if d.Status != "install_partial" || len(d.Conflicts) != 1 {
			t.Fatal(d)
		}
	}
	if len(resolveAttemptFailures([]schema.Evidence{{ID: "N1", Level: "N"}})) != 0 {
		t.Fatal("earlier parameter exit failure was not resolved")
	}
}
