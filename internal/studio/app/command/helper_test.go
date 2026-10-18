package command_test

import (
	"context"
	"testing"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/operator"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/operator/operatortest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/rdb"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/rdb/rdbtest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/studio/app/authentication"
)

// このパッケージのテストは、テンプレートから複製した自分専用の DB で動かす（同時に呼ぶテストは commit が要るため、
// ほかのテストもそろえて、パッケージの中の DB の使い方を 1 つにする）
func isolatedDatabase(t *testing.T) context.Context {
	t.Helper()
	db, _ := rdbtest.NewDatabase(t)
	return rdb.WithDB(t.Context(), db)
}

// 認証ミドルウェアが載せるのと同じく、操作した人を context に載せる。
// イベントに内部 ID を残すだけなので、オペレーターは実在しなくてよい
func signedIn(t *testing.T, ctx context.Context, by operator.ID) context.Context {
	t.Helper()
	return authentication.WithOperator(ctx, by)
}

func registrant(t *testing.T) operator.ID {
	t.Helper()
	return operatortest.ID(t, 10)
}
