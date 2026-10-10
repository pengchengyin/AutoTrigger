package layout

import (
	"os"
	"path/filepath"
	"testing"
)

func TestD13LurePrioritizesLinksAndRejectsTextSetup(t *testing.T) {
	root:=t.TempDir()
	link:=[]byte{0x4c,0,0,0,1,0x14,2,0,0,0,0,0,0xc0,0,0,0,0,0,0,0x46}
	for name,data:=range map[string][]byte{"invoice.lnk":link,"order.lnk":link,"payload.exe":[]byte("MZ"),"invoice.exe":[]byte("MZ")} { os.WriteFile(filepath.Join(root,name),data,0600) }
	entries,_:=Scan(root,true)
	a:=analyzer()
	kind,err:=a.MediaKind(entries,root)
	if err!=nil || kind!="malware_lure" { t.Fatalf("%s %v",kind,err) }
	c,_,err:=a.Analyze(root,entries,true)
	if err!=nil || c[0].Type.Kind!="shortcut" || c[0].Priority!=0 { t.Fatalf("%+v %v",c,err) }
	dataRoot:=t.TempDir()
	os.WriteFile(filepath.Join(dataRoot,"setup.exe"),[]byte("this is data"),0600)
	os.WriteFile(filepath.Join(dataRoot,"readme.txt"),[]byte("data disc"),0600)
	entries,_=Scan(dataRoot,true)
	kind,err=a.MediaKind(entries,dataRoot)
	if err!=nil || kind!="data_disc" { t.Fatalf("%s %v",kind,err) }
}

func TestD14ReadmeOnlyProvidesHintsNeverEntrance(t *testing.T) {
	root:=t.TempDir()
	for name,text:=range map[string]string{"安装说明.txt":"运行 missing.exe", "readme.txt":"run setup.exe", "autorun.inf":"[autorun]\nopen=setup.exe", "resource.dat":"data", "setup.exe":"MZ"} { os.WriteFile(filepath.Join(root,name),[]byte(text),0600) }
	entries,_:=Scan(root,true)
	c,_,err:=analyzer().Analyze(root,entries,true)
	if err!=nil { t.Fatal(err) }
	for _,v:=range c {
		if v.Relative=="setup.exe" { if v.Priority!=0 { t.Fatal(v) }; continue }
		if v.Reason!="non_executable_entry" || v.Priority!=5 { t.Fatal(v) }
	}
}
