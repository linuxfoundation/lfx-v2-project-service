// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package misc

import "testing"

func TestValueOrZero(t *testing.T) {
	str := "value"
	num := 42

	tests := []struct {
		name string
		got  any
		want any
	}{
		{name: "nil string pointer", got: ValueOrZero[string](nil), want: ""},
		{name: "non-nil string pointer", got: ValueOrZero(&str), want: "value"},
		{name: "nil int pointer", got: ValueOrZero[int](nil), want: 0},
		{name: "non-nil int pointer", got: ValueOrZero(&num), want: 42},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.want {
				t.Errorf("ValueOrZero() = %v, want %v", tt.got, tt.want)
			}
		})
	}
}
