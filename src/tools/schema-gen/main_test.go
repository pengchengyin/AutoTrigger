package main

import (
	"autotrigger/pkg/schema"
	"reflect"
	"testing"
)

func TestOCRInventoryIsOptional(t *testing.T) {
	doc := derive(reflect.TypeOf(schema.Prereq{}))
	properties := doc["properties"].(map[string]interface{})
	if properties["ocr_engines"] == nil {
		t.Fatal("inventory missing from schema")
	}
	for _, name := range doc["required"].([]string) {
		if name == "ocr_engines" {
			t.Fatal("additive inventory made old results invalid")
		}
	}
}
