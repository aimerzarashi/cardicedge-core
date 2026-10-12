package authentication

import (
	"context"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/operator"
)

type operatorKey struct{}

// 認証ミドルウェアが、JWT の (iss, sub) に紐づいたオペレーターを載せる。入力ではなく JWT だけから決めるのは、他人として操作させないため
func WithOperator(ctx context.Context, id operator.ID) context.Context {
	return context.WithValue(ctx, operatorKey{}, id)
}

// JWT は正しいがオペレーターに紐づかないときは false（認可で ForbiddenError にする）
func Operator(ctx context.Context) (operator.ID, bool) {
	id, ok := ctx.Value(operatorKey{}).(operator.ID)
	return id, ok
}
