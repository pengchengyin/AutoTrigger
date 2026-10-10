// schema-gen generates the additive JSON contract from the public Go model.
package main

import (
	"autotrigger/internal/output"
	"autotrigger/pkg/schema"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"reflect"
	"strings"
	"time"
)

func main() {
	path := flag.String("out", "pkg/schema/result.schema.json", "schema path")
	flag.Parse()
	s := derive(reflect.TypeOf(schema.Result{}))
	s["$schema"] = "https://json-schema.org/draft/2020-12/schema"
	s["$id"] = "urn:autotrigger:result:2.8"
	s["title"] = "AutoTrigger result schema 2.8"
	b, err := json.MarshalIndent(s, "", "  ")
	if err == nil {
		err = output.Atomic(*path, b)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func derive(t reflect.Type) map[string]interface{} {
	if t == reflect.TypeOf(time.Time{}) {
		return map[string]interface{}{"type": "string", "format": "date-time"}
	}
	switch t.Kind() {
	case reflect.Pointer:
		return map[string]interface{}{"anyOf": []interface{}{derive(t.Elem()), map[string]interface{}{"type": "null"}}}
	case reflect.Struct:
		properties := map[string]interface{}{}
		required := []string{}
		for i := 0; i < t.NumField(); i++ {
			f := t.Field(i)
			name := strings.Split(f.Tag.Get("json"), ",")[0]
			if name == "" || name == "-" {
				continue
			}
			field := derive(f.Type)
			switch name {
			case "final_status":
				field["enum"] = schema.Statuses
			case "judgment_profile":
				field["enum"] = []string{"loose", "strict"}
			case "install_confidence":
				field = map[string]interface{}{"enum": []interface{}{"high", "medium", "low", nil}}
			case "schema_version":
				field["const"] = schema.Version
			}
			properties[name] = field
			if !strings.Contains(f.Tag.Get("json"), ",omitempty") {
				required = append(required, name)
			}
		}
		return map[string]interface{}{"type": "object", "properties": properties, "required": required, "additionalProperties": true}
	case reflect.Slice:
		return map[string]interface{}{"type": "array", "items": derive(t.Elem())}
	case reflect.Map:
		return map[string]interface{}{"type": "object", "additionalProperties": derive(t.Elem())}
	case reflect.String:
		return map[string]interface{}{"type": "string"}
	case reflect.Bool:
		return map[string]interface{}{"type": "boolean"}
	case reflect.Int, reflect.Int32, reflect.Int64, reflect.Uint32, reflect.Uint64:
		return map[string]interface{}{"type": "integer"}
	case reflect.Float32, reflect.Float64:
		return map[string]interface{}{"type": "number"}
	default:
		return map[string]interface{}{}
	}
}
