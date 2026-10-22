package authentication

import (
	"context"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/operator"
)

type operatorKey struct{}

// JWT の (iss, sub) に紐づいたオペレーターを載せる。入力ではなく JWT だけから決めるのは、他人として操作させないため
func WithOperator(ctx context.Context, id operator.ID) context.Context {
	return context.WithValue(ctx, operatorKey{}, id)
}

// application 層のコマンドが、操作した人として使う
func Operator(ctx context.Context) (operator.ID, bool) {
	id, ok := ctx.Value(operatorKey{}).(operator.ID)
	return id, ok
}

// presentation 層の認可が使う。presentation 層にドメインの型（operator.ID）を見せないため、オペレーターかどうかだけを返す
func HasOperator(ctx context.Context) bool {
	_, ok := Operator(ctx)
	return ok
}
