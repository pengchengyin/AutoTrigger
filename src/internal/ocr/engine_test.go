package ocr

import "testing"

func TestNativeConfidenceAndBoxParsing(t *testing.T) {
	data := []byte(`{"code":100,"data":[{"text":"安装","box":[[10,20],[80,20],[80,40],[10,40]],"score":0.94},{"text":"bad","box":[[1,1]],"score":1}]}`)
	got, err := Parse(data)
	if err != nil || len(got) != 1 || got[0].Score != .94 || got[0].Box[2].Y != 40 {
		t.Fatalf("%v %v", got, err)
	}
	if _, err = Parse([]byte(`{"code":200,"data":"missing"}`)); err == nil {
		t.Fatal("engine failure swallowed")
	}
	if got, err = Parse([]byte(`{"code":101,"data":[]}`)); err != nil || len(got) != 0 {
		t.Fatal("empty recognition mishandled")
	}
}
func TestTesseractTSV(t *testing.T) {
	got, err := ParseTSV("level\tpage_num\tblock_num\tpar_num\tline_num\tword_num\tleft\ttop\twidth\theight\tconf\ttext\n5\t1\t1\t1\t1\t1\t10\t20\t30\t15\t90\tInstall\n")
	if err != nil || len(got) != 1 || got[0].Score != .9 || got[0].Text != "Install" {
		t.Fatalf("%+v %v", got, err)
	}
}
