package command

import (
	"errors"
	"fmt"
	"strings"
)

// uif にドメインのエラーを見せないため、app 層のエラーに変えて返す
var (
	// 認証ミドルウェアが JWT の値からオペレーターを特定できなかった（値の形が正しくない）
	ErrInvalidCredentials = errors.New("command: invalid credentials")
	// 操作した人が context に無い（uif の認可を通らずに呼ばれた）
	ErrNotOperator = errors.New("command: not an operator")
	// 登録しようとしたメールアドレスを、ほかのオペレーターが使っている
	ErrOperatorEmailAlreadyRegistered = errors.New("command: operator email already registered")
	// Creature・Item・Spell のどれかが同じ名前を使っている
	ErrCardNameAlreadyInUse = errors.New("command: card name already in use")
)

// 入力の誤り。理由ごとに型を分け、その理由に要る値だけを持つ（uif が型ごとに GraphQL の型へ写す）。
// 上限などの値を持たせるのは、uif や cms がドメインの決まりを書き写さずに済むようにするため
type InputError interface {
	error
	InputField() string
}

type RequiredInput struct{ Field string }

func (e *RequiredInput) Error() string {
	return fmt.Sprintf("%s is required", e.Field)
}

func (e *RequiredInput) InputField() string {
	return e.Field
}

type TooLongInput struct {
	Field string
	Max   int
}

func (e *TooLongInput) Error() string {
	return fmt.Sprintf("%s is longer than %d", e.Field, e.Max)
}

func (e *TooLongInput) InputField() string {
	return e.Field
}

type InvalidFormatInput struct{ Field string }

func (e *InvalidFormatInput) Error() string {
	return fmt.Sprintf("%s has an invalid format", e.Field)
}

func (e *InvalidFormatInput) InputField() string {
	return e.Field
}

// 入力の誤りの一覧。項目が増えても、最初の 1 つで止めずにまとめて返すため
type InputErrors []InputError

func (e InputErrors) Error() string {
	messages := make([]string, len(e))
	for i, err := range e {
		messages[i] = err.Error()
	}
	return "command: invalid input: " + strings.Join(messages, "; ")
}

// err が入力の誤りなら、その一覧を返す（呼ぶ側に空の InputErrors を宣言させないため。factory_enforcer）
func AsInputErrors(err error) (InputErrors, bool) {
	var invalid InputErrors
	ok := errors.As(err, &invalid)
	return invalid, ok
}
