// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package structs

import (
	"reflect"
)

// FieldByTag retrieves the value of a field by its tag type and value from a struct.
// Returns the field value and a boolean indicating if the field was found.
func FieldByTag(obj any, tagType, tagValue string) (any, bool) {
	v := reflect.ValueOf(obj)
	t := reflect.TypeOf(obj)

	if v.Kind() == reflect.Ptr {
		v = v.Elem()
		t = t.Elem()
	}

	if t.Kind() != reflect.Struct {
		return nil, false
	}

	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		tag := field.Tag.Get(tagType)
		if tag == tagValue {
			return v.Field(i).Interface(), true
		}
	}
	return nil, false
}
