package gui

import (
	"context"
	"testing"
	"time"

	"autotrigger/internal/config"
	"autotrigger/internal/core/ports"
	"autotrigger/internal/resources"
)

func TestEnglishLicenseSelectionAndNext(t *testing.T) {
	labels := []string{
		"I &accept the terms of the license agreement",
		"I accept the terms in this agreement",
		"I accept the terms of this agreement",
		"I accept the license agreement",
		"I accept the terms and conditions",
		"I agree to the license agreement",
		"I agree to the terms and conditions",
		"I agree to the terms of the license agreement",
		"I agree to the terms of this agreement",
	}
	for _, role := range []string{"radio", "checkbox"} {
		for _, label := range labels {
			t.Run(role+"/"+label, func(t *testing.T) {
				n := &native{controls: []ports.Control{
					{ID: "decline", Text: "I do not accept the terms of the license agreement", Role: role, Enabled: true, Checked: true},
					{ID: "accept", Text: label, Role: role, Enabled: true},
					{ID: "next", Text: "&Next >", Role: "button"},
				}}
				a := Automator{Native: n, Config: config.Defaults(), Pack: resources.Pack{Mode: "builtin"}}
				if err := a.Init(); err != nil {
					t.Fatal(err)
				}
				if _, err := a.Step(context.Background(), []int{1}); err != nil || len(n.clicked) != 1 || n.clicked[0].ID != "accept" {
					t.Fatalf("agreement selection: %v, %+v", err, n.clicked)
				}
				n.controls[0].Checked = false
				n.controls[1].Checked = true
				n.controls[2].Enabled = true
				a.states[1].lastClick = time.Now().Add(-time.Second)
				if _, err := a.Step(context.Background(), []int{1}); err != nil || len(n.clicked) != 2 || n.clicked[1].ID != "next" {
					t.Fatalf("next after agreement: %v, %+v", err, n.clicked)
				}
			})
		}
	}
}

func TestEnglishDeclineIsNeverAccept(t *testing.T) {
	a := Automator{Config: config.Defaults(), Pack: resources.Pack{Mode: "builtin"}}
	if err := a.Init(); err != nil {
		t.Fatal(err)
	}
	for _, label := range []string{
		"I do not accept", "I do not agree", "I decline",
		"I do not accept the agreement", "I do not accept the terms in the license agreement",
		"I do not accept the terms of the license agreement", "I do not accept the terms of this agreement",
		"I do not agree to the terms and conditions", "I decline the license agreement",
	} {
		if got := a.semantic(label); got != "deny" {
			t.Errorf("%q: got %q, want deny", label, got)
		}
	}
	if got := a.semantic("I accept optional offers and install additional software"); got != "" {
		t.Fatalf("unrecognized offer must not be accepted: %s", got)
	}
}
