// icon-gen creates an amd64 COFF resource object without a resource compiler.
package main

import (
	"encoding/binary"
	"flag"
	"fmt"
	"os"
)

func main() {
	input := flag.String("input", "internal/ui/assets/AutoTrigger.ico", "ICO source")
	output := flag.String("output", "cmd/autotrigger/app_windows_amd64.syso", "COFF output")
	flag.Parse()
	data, err := os.ReadFile(*input)
	if err != nil {
		panic(err)
	}
	object, err := resourceObject(data)
	if err != nil {
		panic(err)
	}
	if err = os.WriteFile(*output, object, 0644); err != nil {
		panic(err)
	}
}

func resourceObject(ico []byte) ([]byte, error) {
	if len(ico) < 6 || binary.LittleEndian.Uint16(ico[2:4]) != 1 {
		return nil, fmt.Errorf("invalid ICO")
	}
	count := int(binary.LittleEndian.Uint16(ico[4:6]))
	if count == 0 || len(ico) < 6+16*count {
		return nil, fmt.Errorf("invalid ICO entries")
	}
	images := make([][]byte, count)
	group := make([]byte, 6+14*count)
	copy(group, ico[:6])
	for i := 0; i < count; i++ {
		entry := ico[6+i*16 : 6+(i+1)*16]
		size := int(binary.LittleEndian.Uint32(entry[8:12]))
		offset := int(binary.LittleEndian.Uint32(entry[12:16]))
		if size <= 0 || offset < 0 || offset > len(ico)-size {
			return nil, fmt.Errorf("invalid ICO image")
		}
		images[i] = ico[offset : offset+size]
		copy(group[6+i*14:], entry[:12])
		binary.LittleEndian.PutUint16(group[6+i*14+12:], uint16(i+1))
	}
	var section []byte
	allocate := func(size int) uint32 {
		offset := uint32(len(section))
		section = append(section, make([]byte, size)...)
		return offset
	}
	put32 := func(offset, value uint32) { binary.LittleEndian.PutUint32(section[offset:], value) }
	directory := func(ids []uint32) uint32 {
		offset := allocate(16 + 8*len(ids))
		binary.LittleEndian.PutUint16(section[offset+14:], uint16(len(ids)))
		for i, id := range ids {
			put32(offset+16+uint32(i*8), id)
		}
		return offset
	}
	root := directory([]uint32{3, 14})
	var relocations []uint32
	for typeIndex, kind := range []uint32{3, 14} {
		blobs := images
		if kind == 14 {
			blobs = [][]byte{group}
		}
		ids := make([]uint32, len(blobs))
		for i := range ids {
			ids[i] = uint32(i + 1)
		}
		typeDir := directory(ids)
		put32(root+20+uint32(typeIndex*8), typeDir|0x80000000)
		for i, blob := range blobs {
			language := directory([]uint32{1033})
			put32(typeDir+20+uint32(i*8), language|0x80000000)
			entry := allocate(16)
			put32(language+20, entry)
			for len(section)%4 != 0 {
				allocate(1)
			}
			payload := allocate(len(blob))
			copy(section[payload:], blob)
			put32(entry, payload)
			put32(entry+4, uint32(len(blob)))
			relocations = append(relocations, entry)
		}
	}
	relocStart := 60 + len(section)
	symbolStart := relocStart + 10*len(relocations)
	result := make([]byte, symbolStart+18+4)
	binary.LittleEndian.PutUint16(result, 0x8664)
	binary.LittleEndian.PutUint16(result[2:], 1)
	binary.LittleEndian.PutUint32(result[8:], uint32(symbolStart))
	binary.LittleEndian.PutUint32(result[12:], 1)
	copy(result[20:], ".rsrc")
	binary.LittleEndian.PutUint32(result[36:], uint32(len(section)))
	binary.LittleEndian.PutUint32(result[40:], 60)
	binary.LittleEndian.PutUint32(result[44:], uint32(relocStart))
	binary.LittleEndian.PutUint16(result[52:], uint16(len(relocations)))
	binary.LittleEndian.PutUint32(result[56:], 0x40000040)
	copy(result[60:], section)
	for i, offset := range relocations {
		position := relocStart + i*10
		binary.LittleEndian.PutUint32(result[position:], offset)
		binary.LittleEndian.PutUint16(result[position+8:], 3)
	}
	copy(result[symbolStart:], ".rsrc")
	binary.LittleEndian.PutUint16(result[symbolStart+12:], 1)
	result[symbolStart+16] = 3
	binary.LittleEndian.PutUint32(result[symbolStart+18:], 4)
	return result, nil
}
