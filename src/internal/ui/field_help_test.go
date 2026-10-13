package ui

import (
	"autotrigger/internal/config"
	"autotrigger/pkg/schema"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestFieldHelpCoversCurrentContracts(t *testing.T) {
	var walk func(reflect.Type)
	walk = func(typ reflect.Type) {
		if typ.Kind() == reflect.Ptr {
			typ = typ.Elem()
		}
		if typ == reflect.TypeOf(time.Time{}) || typ.Kind() != reflect.Struct {
			return
		}
		for i := 0; i < typ.NumField(); i++ {
			field := typ.Field(i)
			name := strings.Split(field.Tag.Get("json"), ",")[0]
			if name == "" || name == "-" {
				continue
			}
			if fieldMeanings[name] == "" {
				t.Errorf("missing field explanation: %s", name)
			}
			if field.Type.Kind() == reflect.Slice {
				walk(field.Type.Elem())
			} else {
				walk(field.Type)
			}
		}
	}
	walk(reflect.TypeOf(config.Config{}))
	walk(reflect.TypeOf(schema.Result{}))
	for _, want := range []string{"results[].attempts[].exit_code", "prereq.ocr_engine_status", "install_completed", "null"} {
		if !strings.Contains(fieldHelp(false), want) {
			t.Errorf("missing result field %s", want)
		}
	}
	if !strings.Contains(filterFieldHelp(fieldHelp(true), "template_pack"), "merged") {
		t.Fatal("field search failed")
	}
}
