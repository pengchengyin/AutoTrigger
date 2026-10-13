package main

import (
	"bytes"
	"debug/pe"
	"encoding/binary"
	"os"
	"testing"
)

func TestIconResourceObject(t *testing.T) {
	icon, err := os.ReadFile("../../internal/ui/assets/AutoTrigger.ico")
	if err != nil {
		t.Fatal(err)
	}
	object, err := resourceObject(icon)
	if err != nil {
		t.Fatal(err)
	}
	file, err := pe.NewFile(bytes.NewReader(object))
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	section := file.Section(".rsrc")
	if section == nil {
		t.Fatal("no resource section")
	}
	data, err := section.Data()
	if err != nil {
		t.Fatal(err)
	}
	if binary.LittleEndian.Uint16(data[14:16]) != 2 || binary.LittleEndian.Uint32(data[16:20]) != 3 || binary.LittleEndian.Uint32(data[24:28]) != 14 {
		t.Fatal("icon resource types missing")
	}
	if len(section.Relocs) != int(binary.LittleEndian.Uint16(icon[4:6]))+1 {
		t.Fatal("resource relocations missing")
	}
	for _, invalid := range [][]byte{nil, icon[:6], icon[:len(icon)-1]} {
		if _, err := resourceObject(invalid); err == nil {
			t.Fatal("truncated icon accepted")
		}
	}
}

func TestBuiltExecutableIconResources(t *testing.T) {
	path := os.Getenv("AUTOTRIGGER_ICON_EXE")
	if path == "" {
		t.Skip("set AUTOTRIGGER_ICON_EXE to check built PE resources")
	}
	file, err := pe.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	header, ok := file.OptionalHeader.(*pe.OptionalHeader64)
	if !ok {
		t.Fatal("expected amd64 PE")
	}
	resources := header.DataDirectory[2]
	section := file.Section(".rsrc")
	if section == nil || resources.VirtualAddress == 0 || resources.Size == 0 {
		t.Fatal("exe icon resource missing")
	}
	data, err := section.Data()
	if err != nil {
		t.Fatal(err)
	}
	root := resources.VirtualAddress - section.VirtualAddress
	if binary.LittleEndian.Uint16(data[root+14:]) != 2 {
		t.Fatal("resource root types missing")
	}
	for i, kind := range []uint32{3, 14} {
		entry := root + 16 + uint32(i*8)
		if binary.LittleEndian.Uint32(data[entry:]) != kind {
			t.Fatal("icon resource type missing")
		}
		typeDir := binary.LittleEndian.Uint32(data[entry+4:]) & 0x7fffffff
		language := binary.LittleEndian.Uint32(data[typeDir+20:]) & 0x7fffffff
		leaf := binary.LittleEndian.Uint32(data[language+20:])
		rva := binary.LittleEndian.Uint32(data[leaf:])
		size := binary.LittleEndian.Uint32(data[leaf+4:])
		if rva < section.VirtualAddress || rva+size > section.VirtualAddress+uint32(len(data)) {
			t.Fatal("icon resource relocation invalid")
		}
	}
}
