package ui

import (
	"os"
	"testing"
)

func TestDeleteTemplateProject(t *testing.T) {
	root := t.TempDir()
	p, err := newTemplateProject(root, "待删除")
	if err != nil {
		t.Fatal(err)
	}
	other, err := newTemplateProject(root, "保留")
	if err != nil {
		t.Fatal(err)
	}
	if err = deleteTemplateProject(root, p.ID, true); err == nil {
		t.Fatal("busy deletion accepted")
	}
	if _, err = os.Stat(projectDir(root, p)); err != nil {
		t.Fatal("busy deletion changed data")
	}
	if err = deleteTemplateProject(root, "../outside", false); err == nil {
		t.Fatal("unknown project accepted")
	}
	if err = deleteTemplateProject(root, p.ID, false); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(projectDir(root, p)); !os.IsNotExist(err) {
		t.Fatal("project not deleted")
	}
	if _, err = os.Stat(projectDir(root, other)); err != nil {
		t.Fatal("other project changed")
	}
	index, err := selectedProjects(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range index.Projects {
		if id == p.ID {
			t.Fatal("deleted project still active")
		}
	}
}
