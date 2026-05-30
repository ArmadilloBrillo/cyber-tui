// Package sanitize neutralises untrusted text before it is rendered to a
// terminal. Server-supplied strings (usernames, titles, bios, and similar) may
// contain control characters, including ESC (0x1B), that a terminal interprets
// as escape sequences. Stripping them at the API boundary prevents a remote user
// from injecting cursor, colour, window-title, or clipboard sequences into a
// viewer's terminal.
package sanitize

import (
	"reflect"
	"strings"
	"unicode"
)

// Strip removes control characters (C0, C1, and DEL) from s while preserving tab
// and newline, which are legitimate layout whitespace.
func Strip(s string) string {
	if !strings.ContainsFunc(s, isControl) {
		return s
	}
	return strings.Map(func(r rune) rune {
		if isControl(r) {
			return -1
		}
		return r
	}, s)
}

func isControl(r rune) bool {
	if r == '\t' || r == '\n' {
		return false
	}
	return unicode.IsControl(r)
}

// Strings walks v, which must be a non-nil pointer, and applies Strip to every
// settable string field, recursing through nested structs, pointers, slices, and
// arrays. It lets the API layer sanitise a whole decoded wire value in one call
// so no individual display field can be forgotten.
func Strings(v any) {
	rv := reflect.ValueOf(v)
	if rv.Kind() != reflect.Pointer || rv.IsNil() {
		return
	}
	walk(rv.Elem())
}

func walk(v reflect.Value) {
	switch v.Kind() {
	case reflect.String:
		if v.CanSet() {
			v.SetString(Strip(v.String()))
		}
	case reflect.Pointer, reflect.Interface:
		if !v.IsNil() {
			walk(v.Elem())
		}
	case reflect.Struct:
		for i := 0; i < v.NumField(); i++ {
			walk(v.Field(i))
		}
	case reflect.Slice, reflect.Array:
		for i := 0; i < v.Len(); i++ {
			walk(v.Index(i))
		}
	}
}
