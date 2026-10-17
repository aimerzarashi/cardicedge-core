package command_test

import (
	"context"
	"testing"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/rdb"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/rdb/rdbtest"
)

// このパッケージのテストは、テンプレートから複製した自分専用の DB で動かす（同時に呼ぶテストは commit が要るため、
// ほかのテストもそろえて、パッケージの中の DB の使い方を 1 つにする）
func isolatedDatabase(t *testing.T) context.Context {
	t.Helper()
	db, _ := rdbtest.NewDatabase(t)
	return rdb.WithDB(t.Context(), db)
}
