package trun

import (
	"fmt"
	"testing"
)

func TestTableFormat(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{name: "blank_subject", input: " "},
		{"trimmed_subject", " a "},
		{name: "Blank Subject", input: " "}, // want "t.Run name must be lower_snake_case"
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {})
	}
}

func TestTableInline(t *testing.T) {
	for _, tt := range []struct{ name string }{
		{name: "empty_subject"},
		{name: "異常系/空"}, // want "t.Run name must be lower_snake_case"
	} {
		t.Run(tt.name, func(t *testing.T) {})
	}
}

func TestTableMap(t *testing.T) {
	tests := map[string]struct{ name string }{
		"a": {name: "empty_subject"},
		"b": {name: "empty_subject"}, // want "t.Run name must be unique"
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {})
	}
}

func TestTableDuplicated(t *testing.T) {
	tests := []struct {
		name string
	}{
		{name: "empty_subject"},
		{name: "empty_subject"}, // want "t.Run name must be unique"
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {})
	}
}

// 文字列リテラルでない値や、関数で作ったテーブルは調べない
func TestTableNotLiteralIsNotChecked(t *testing.T) {
	tests := []struct {
		name string
	}{
		{name: fmt.Sprintf("Input %d", 1)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {})
	}
	for _, tt := range helperCases() {
		t.Run(tt.name, func(t *testing.T) {})
	}
}

func helperCases() []struct{ name string } {
	return []struct{ name string }{{name: "Not Checked"}}
}
