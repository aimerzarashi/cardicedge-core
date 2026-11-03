package domaindef_test

import (
	"context"
	"slices"
	"testing"

	"github.com/stretchr/testify/require"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/avatar"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/board"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/card"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/collection"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/creature"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/deck"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/gamerules"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/grantdeck"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/item"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/npcavatar"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/npcdeck"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/operator"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/player"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/presetdeck"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/spell"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/startergrant"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/studiorules"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/rdb"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/rdb/rdbtest"
)

// 集約を足したら、ここにその集約の Types を足す（マイグレーションの種別の足し忘れをテストで検出する）
func aggregates() []domain.AggregateTypes {
	return []domain.AggregateTypes{operator.Types(), creature.Types(), item.Types(), spell.Types(), presetdeck.Types(), grantdeck.Types(), player.Types(), startergrant.Types(), collection.Types(), deck.Types(), gamerules.Types(), avatar.Types(), board.Types(), studiorules.Types(), npcavatar.Types(), npcdeck.Types()}
}

func entityTypes() []domain.EntityType {
	var entityTypes []domain.EntityType
	for _, aggregate := range aggregates() {
		entityTypes = append(entityTypes, aggregate.EntityType)
	}
	return entityTypes
}

// テストのトランザクションの中だけで、DB に行を足す（コードに無い種別を作る）
func insert(t *testing.T, ctx context.Context, query string) {
	t.Helper()
	exec, err := rdb.GetExecutor(ctx)
	require.NoError(t, err)
	_, err = exec.ExecContext(ctx, query)
	require.NoError(t, err)
}

// 一意な key の種別を足したら、ここにその種別を宣言するドメインの UniqueKeyTypes を足す
func uniqueKeyTypes() []domain.UniqueKeyType {
	return slices.Concat(operator.UniqueKeyTypes(), card.UniqueKeyTypes(), presetdeck.UniqueKeyTypes(), grantdeck.UniqueKeyTypes(), player.UniqueKeyTypes(), startergrant.UniqueKeyTypes(), collection.UniqueKeyTypes(), deck.UniqueKeyTypes(), gamerules.UniqueKeyTypes(), avatar.UniqueKeyTypes(), board.UniqueKeyTypes(), studiorules.UniqueKeyTypes(), npcavatar.UniqueKeyTypes(), npcdeck.UniqueKeyTypes())
}

// 参照の種別を足したら、ここにその種別を宣言するドメインの ReferenceTypes を足す
func referenceTypes() []domain.ReferenceType {
	return slices.Concat(presetdeck.ReferenceTypes(), grantdeck.ReferenceTypes(), npcdeck.ReferenceTypes(), npcavatar.ReferenceTypes())
}

// 上限の種別を足したら、ここにその種別を宣言するドメインの QuotaTypes を足す
func quotaTypes() []domain.QuotaType {
	return slices.Concat(deck.QuotaTypes(), grantdeck.QuotaTypes(), avatar.QuotaTypes())
}

// テンプレートから複製した自分だけの DB（一覧の種別の行が入っている）で動かす（行を足したり、食い違う行を入れたりするため）
func isolated(t *testing.T) context.Context {
	t.Helper()
	db, _ := rdbtest.NewDatabase(t)
	return rdb.WithDB(t.Context(), db)
}

func count(t *testing.T, ctx context.Context, table string) int {
	t.Helper()
	exec, err := rdb.GetExecutor(ctx)
	require.NoError(t, err)
	var n int
	require.NoError(t, exec.QueryRowContext(ctx, `SELECT count(*) FROM `+table).Scan(&n))
	return n
}
