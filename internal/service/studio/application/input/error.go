// 入力の誤り。コマンド・問い合わせのどちらも返し、presentation 層が理由の型ごとに GraphQL の型（ValidationErrors の FieldError）へ写す
package input

import (
	"errors"
	"fmt"
	"strings"
)

// 理由ごとに型を分け、その理由に要る値だけを持つ。上限などの値を持たせるのは、presentation 層や studio がドメインの決まりを書き写さずに済むようにするため
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

// 上限の無い値が下限より小さい（既定のルールの魔力等）。範囲の上限が決まっている値は OutOfRange
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

type InvalidFormat struct{ Field string }

func (e *InvalidFormat) Error() string {
	return fmt.Sprintf("%s has an invalid format", e.Field)
}

func (e *InvalidFormat) InputField() string {
	return e.Field
}

// 入力の ID が指すもの（プリセットデッキに入れるカード等）が無い
type NotFound struct{ Field string }

func (e *NotFound) Error() string {
	return fmt.Sprintf("%s refers to nothing", e.Field)
}

func (e *NotFound) InputField() string {
	return e.Field
}

// 入力の ID が指すものが、使えない状態（準備中・使用不可のカード等）。無い（NotFound）のとは違い、あるが今は使えない
type Unavailable struct{ Field string }

func (e *Unavailable) Error() string {
	return fmt.Sprintf("%s refers to something unavailable", e.Field)
}

func (e *Unavailable) InputField() string {
	return e.Field
}

// 一覧の中で、同じもの（同じカード等）がすでに出てきた
type Duplicated struct{ Field string }

func (e *Duplicated) Error() string {
	return fmt.Sprintf("%s is duplicated", e.Field)
}

func (e *Duplicated) InputField() string {
	return e.Field
}

// 入力を当てた後の合計（プリセットデッキのカードの枚数の合計等）が上限を超える。入力の値そのものではなく、
// 今の状態と合わせた結果の誤りなので、OutOfRange と分ける（画面が言い分けられるように）
type TotalTooLarge struct {
	Field string
	Max   int
}

func (e *TotalTooLarge) Error() string {
	return fmt.Sprintf("total of %s is larger than %d", e.Field, e.Max)
}

func (e *TotalTooLarge) InputField() string {
	return e.Field
}

// 合計がちょうど決まった値でない（公開する配布デッキの枚数の合計等）。多すぎるのも足りないのも同じ理由なので、
// TotalTooLarge と分ける
type TotalMismatch struct {
	Field    string
	Required int
}

func (e *TotalMismatch) Error() string {
	return fmt.Sprintf("total of %s is not %d", e.Field, e.Required)
}

func (e *TotalMismatch) InputField() string {
	return e.Field
}

// つながりの両端が隣どうし（上下左右）でない（同じ座標を指すものを含む）
type NotAdjacent struct{ Field string }

func (e *NotAdjacent) Error() string {
	return fmt.Sprintf("%s connects tiles that are not adjacent", e.Field)
}

func (e *NotAdjacent) InputField() string {
	return e.Field
}

// 対の端のマスが、転送円でも橋でもない
type NotPairable struct{ Field string }

func (e *NotPairable) Error() string {
	return fmt.Sprintf("%s is not a warp or a bridge", e.Field)
}

func (e *NotPairable) InputField() string {
	return e.Field
}

// 対の両端が、転送円と橋（組めるのは同じ種類どうしだけ）
type PairKindMismatch struct{ Field string }

func (e *PairKindMismatch) Error() string {
	return fmt.Sprintf("%s pairs tiles of different kinds", e.Field)
}

func (e *PairKindMismatch) InputField() string {
	return e.Field
}

// 対の両端が同じ座標（自分自身と組んでいる）
type PairedWithSelf struct{ Field string }

func (e *PairedWithSelf) Error() string {
	return fmt.Sprintf("%s pairs a tile with itself", e.Field)
}

func (e *PairedWithSelf) InputField() string {
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
