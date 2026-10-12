package ui

import (
	"archive/zip"
	"autotrigger/internal/config"
	"autotrigger/internal/gui/templates"
	"os"
	"path/filepath"
	"testing"
)

func TestTemplateArchiveRoundTripAndNoOverwrite(t *testing.T) {
	root := t.TempDir()
	dest := t.TempDir()
	p, _ := hubFixture(t, root, "模板一")
	hubFixture(t, root, "模板二")
	if err := toggleTemplateProject(root, p.ID, true, false); err != nil {
		t.Fatal(err)
	}
	archive := filepath.Join(t.TempDir(), "templates.zip")
	if err := exportTemplateArchive(root, archive); err != nil {
		t.Fatal(err)
	}
	if err := importTemplateArchive(dest, archive); err != nil {
		t.Fatal(err)
	}
	projects, err := listTemplateProjects(dest)
	if err != nil || len(projects) != 2 {
		t.Fatal(projects, err)
	}
	selection, err := selectedProjects(dest)
	if err != nil || len(selection.Projects) != 0 {
		t.Fatal("import activated rules")
	}
	if err = importTemplateArchive(dest, archive); err == nil {
		t.Fatal("duplicate templates overwritten")
	}
	if err = toggleTemplateProject(dest, p.ID, true, false); err != nil {
		t.Fatal(err)
	}
	lib, err := templates.LoadAt(config.Defaults().GUI, filepath.Dir(dest), dest)
	if err != nil || len(lib.User.Rules) != 1 {
		t.Fatal(err)
	}
}

func TestTemplateArchiveRejectsTraversalAndDamage(t *testing.T) {
	for _, name := range []string{"../escape", "C:/escape", `a\b`, "a/CON.txt", "a/file.", "a/file "} {
		if archivePath(name) {
			t.Fatal("unsafe path accepted", name)
		}
	}
	path := filepath.Join(t.TempDir(), "bad.zip")
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	writer := zip.NewWriter(file)
	entry, _ := writer.Create("../escape")
	entry.Write([]byte("bad"))
	writer.Close()
	file.Close()
	root := t.TempDir()
	if err = importTemplateArchive(root, path); err == nil {
		t.Fatal("traversal accepted")
	}
	projects, err := listTemplateProjects(root)
	if err != nil || len(projects) > 0 {
		t.Fatal("failed import changed data")
	}
	p, c := hubFixture(t, root, "broken")
	if err = os.WriteFile(filepath.Join(projectDir(root, p), filepath.FromSlash(c.File)), []byte("broken"), 0600); err != nil {
		t.Fatal(err)
	}
	if err = exportTemplateArchive(root, filepath.Join(t.TempDir(), "bad-capture.zip")); err == nil {
		t.Fatal("damaged capture exported")
	}
}
