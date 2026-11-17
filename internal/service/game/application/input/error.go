// 入力の誤り。コマンドが返し、presentation 層が理由の型ごとに GraphQL の型（ValidationErrors の FieldError）へ写す。
// studio の input と同じ形（サービスごとに縦に切る決まりなので import せず置く）。理由は、game で要るものから足す
package input

import (
	"errors"
	"fmt"
	"strings"
)

// 理由ごとに型を分け、その理由に要る値だけを持つ。上限などの値を持たせるのは、presentation 層や game がドメインの決まりを書き写さずに済むようにするため
type Error interface {
	error
	InputField() string
}

type Required struct{ Field string }

func (e *Required) Error() string {
	return fmt.Sprintf("%s is required", e.Field)
}

func (e *Required) InputField() string {
	return e.Field
}

type TooLong struct {
	Field string
	Max   int
}

func (e *TooLong) Error() string {
	return fmt.Sprintf("%s is longer than %d", e.Field, e.Max)
}

func (e *TooLong) InputField() string {
	return e.Field
}

type OutOfRange struct {
	Field string
	Min   int
	Max   int
}

func (e *OutOfRange) Error() string {
	return fmt.Sprintf("%s is out of range %d..%d", e.Field, e.Min, e.Max)
}

func (e *OutOfRange) InputField() string {
	return e.Field
}

// 上限の無い値が下限より小さい（対戦ルールの魔力等）。範囲の上限が決まっている値は OutOfRange
type TooSmall struct {
	Field string
	Min   int
}

func (e *TooSmall) Error() string {
	return fmt.Sprintf("%s is smaller than %d", e.Field, e.Min)
}

func (e *TooSmall) InputField() string {
	return e.Field
}

// 別の入力の項目の値より大きくなければならない値が、大きくない（等しいのも含む）。Than はその項目の名前。
// 下限の値は、相手の項目の値によって変わるので持たない
type NotGreaterThan struct {
	Field string
	Than  string
}

func (e *NotGreaterThan) Error() string {
	return fmt.Sprintf("%s is not greater than %s", e.Field, e.Than)
}

func (e *NotGreaterThan) InputField() string {
	return e.Field
}

// 入力の ID が指すものが、選べるものの中に無い（無い・ほかのプレイヤーのもの・解放していない・公開中でない等。存在を明かさないため区別しない）
type NotFound struct{ Field string }

func (e *NotFound) Error() string {
	return fmt.Sprintf("%s is not found", e.Field)
}

func (e *NotFound) InputField() string {
	return e.Field
}

// 一覧の中の、ある種類の個数が上限より多い（部屋のプレイヤーの席等）。Max は上限の個数
type TooMany struct {
	Field string
	Max   int
}

func (e *TooMany) Error() string {
	return fmt.Sprintf("%s has more than %d", e.Field, e.Max)
}

func (e *TooMany) InputField() string {
	return e.Field
}

type InvalidFormat struct{ Field string }

func (e *InvalidFormat) Error() string {
	return fmt.Sprintf("%s has an invalid format", e.Field)
}

func (e *InvalidFormat) InputField() string {
	return e.Field
}

// 項目が増えても、最初の 1 つで止めずにまとめて返すため
type Errors []Error

func (e Errors) Error() string {
	messages := make([]string, len(e))
	for i, err := range e {
		messages[i] = err.Error()
	}
	return "input: invalid input: " + strings.Join(messages, "; ")
}

// 呼ぶ側に空の Errors を宣言させないため（factory_enforcer）
func AsErrors(err error) (Errors, bool) {
	var invalid Errors
	ok := errors.As(err, &invalid)
	return invalid, ok
}
