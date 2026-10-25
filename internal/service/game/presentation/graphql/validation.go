package graphql

import (
	"fmt"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/game/application/input"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/game/presentation/graphql/model"
)

// 入力の誤りは GraphQL の errors ではなく、操作の結果の型（ValidationErrors）で返す（呼び出し側が項目ごとに表示できるように）。
// 文言は作らない（game が __typename と値から作る）。err が入力の誤りでなければ nil
func validationErrors(err error) *model.ValidationErrors {
	invalid, ok := input.AsErrors(err)
	if !ok {
		return nil
	}
	fieldErrors := make([]model.FieldError, len(invalid))
	for i, e := range invalid {
		switch e := e.(type) {
		case *input.Required:
			fieldErrors[i] = &model.FieldRequired{Field: e.Field}
		case *input.TooLong:
			fieldErrors[i] = &model.FieldTooLong{Field: e.Field, Max: e.Max}
		default:
			// 理由を application 層に足したのに、ここに写し方を足し忘れた（GraphQL の型を足すのを忘れずに気づけるよう、黙って別の型にしない）
			panic(fmt.Sprintf("unknown input error: %T", e))
		}
	}
	return &model.ValidationErrors{Errors: fieldErrors}
}
