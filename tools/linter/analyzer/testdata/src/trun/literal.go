package trun

import (
	"fmt"
	"testing"
)

func TestLiteralFormat(t *testing.T) {
	t.Run("blank_subject", func(t *testing.T) {})
	t.Run("rejects_blank_issuer", func(t *testing.T) {})
	t.Run("utf8_4byte", func(t *testing.T) {})
	t.Run("blank subject", func(t *testing.T) {})  // want "t.Run name must be lower_snake_case"
	t.Run("正常系/空文字", func(t *testing.T) {})        // want "t.Run name must be lower_snake_case"
	t.Run("BlankSubject", func(t *testing.T) {})   // want "t.Run name must be lower_snake_case"
	t.Run("blank-subject", func(t *testing.T) {})  // want "t.Run name must be lower_snake_case"
	t.Run("blank__subject", func(t *testing.T) {}) // want "t.Run name must be lower_snake_case"
	t.Run("_blank_subject", func(t *testing.T) {}) // want "t.Run name must be lower_snake_case"
	t.Run("", func(t *testing.T) {})               // want "t.Run name must be lower_snake_case"
	t.Run("空白だけは拒否する", func(t *testing.T) {})      // want "t.Run name must be lower_snake_case"
}

func TestLiteralDuplicated(t *testing.T) {
	t.Run("blank_subject", func(t *testing.T) {
		t.Run("full_width_space", func(t *testing.T) {})
	})
	t.Run("blank_subject", func(t *testing.T) {}) // want "t.Run name must be unique"
	// 親のテストが違えば、同じ名前でもよい
	t.Run("full_width_space", func(t *testing.T) {})
}

// 文字列リテラルでない名前（入力から作る等）は調べない
func TestNotLiteralIsNotChecked(t *testing.T) {
	name := "Not Checked"
	t.Run(name, func(t *testing.T) {})
	t.Run(fmt.Sprintf("input %d", 1), func(t *testing.T) {})
}
