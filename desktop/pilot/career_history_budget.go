// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"encoding/json"
	"reflect"
	"strconv"
)

// Bound typed inputs before any whole-object marshal. All valid History strings
// are at most the inherited256-byte origin; rows and slots are the only arrays.
func historyBudget(v reflect.Value, depth int, left *int) bool {
	spend := func(n int) bool {
		if n < 0 || n > *left {
			return false
		}
		*left -= n
		return true
	}
	switch v.Kind() {
	case reflect.Pointer:
		if v.IsNil() {
			return spend(4)
		}
		return historyBudget(v.Elem(), depth, left)
	case reflect.Struct:
		if depth >= 12 || !spend(2) {
			return false
		}
		t := v.Type()
		for i := 0; i < v.NumField(); i++ {
			f := t.Field(i)
			if f.PkgPath != "" {
				continue
			}
			if !spend(len(f.Tag.Get("json"))+4) || !historyBudget(v.Field(i), depth+1, left) {
				return false
			}
		}
	case reflect.Array, reflect.Slice:
		limit := v.Len()
		if v.Kind() == reflect.Slice {
			limit = 8
		}
		if depth >= 12 || v.Len() > limit || !spend(2) {
			return false
		}
		for i := 0; i < v.Len(); i++ {
			if !spend(1) || !historyBudget(v.Index(i), depth+1, left) {
				return false
			}
		}
	case reflect.String:
		if v.Len() > 256 {
			return false
		}
		b, e := json.Marshal(v.String())
		return e == nil && spend(len(b))
	case reflect.Bool:
		return spend(5)
	case reflect.Int, reflect.Int32, reflect.Int64:
		return spend(len(strconv.FormatInt(v.Int(), 10)))
	case reflect.Uint, reflect.Uint32, reflect.Uint64:
		return spend(len(strconv.FormatUint(v.Uint(), 10)))
	default:
		return false
	}
	return true
}
