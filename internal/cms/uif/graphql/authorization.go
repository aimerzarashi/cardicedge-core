package graphql

import (
	"context"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/cms/app/authentication"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/cms/uif/graphql/model"
)

// 認可で断るときは GraphQL の errors ではなく、操作の結果の型（ForbiddenError）で返す（呼び出し側が型で分岐できるように）。
// 操作が増えても同じ判定を書き写さないよう、ここにまとめる
func requireOperator(ctx context.Context) *model.ForbiddenError {
	if !authentication.HasOperator(ctx) {
		return &model.ForbiddenError{Message: "not an operator"}
	}
	return nil
}
