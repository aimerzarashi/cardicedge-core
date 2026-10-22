package command_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/constraint"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/presetdeck"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domaindef"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/eventstore"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/identifier"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/studio/application/command"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/operator"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/operator/operatortest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/rdb"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/rdb/rdbtest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/studio/application/authentication"
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

func newPresetDeckHandler(t *testing.T, ctx context.Context) *command.PresetDeckHandler {
	t.Helper()
	registry, err := domaindef.Load(ctx)
	require.NoError(t, err)
	return command.NewPresetDeckHandler(
		presetDeckStore(registry),
		identifier.New(registry, presetdeck.NewID),
		identifier.NewEntityRefResolver(registry),
		constraint.UpdatePresetDeckName,
		constraint.UpdatePresetDeckCardReferences,
		// wiring と同じく、DB に残る精度（マイクロ秒）にそろえる（保存の後に読み直した日時と、結果の日時を同じにするため）
		func() time.Time { return time.Now().UTC().Truncate(time.Microsecond) },
	)
}

func presetDeckStore(registry *domaindef.Registry) domain.EventStore[presetdeck.ID, presetdeck.Event, presetdeck.State] {
	return eventstore.New(registry, presetdeck.NewID, presetdeck.NewEventType, presetdeck.InitialState, presetdeck.Evolve)
}

// カードがあるとは、識別子が発行されていること（カードの集約の中身は、プリセットデッキのコマンドは見ない）
func issueCard[I domain.ID](t *testing.T, ctx context.Context, newID domain.NewIDFunc[I]) uuid.UUID {
	t.Helper()
	registry, err := domaindef.Load(ctx)
	require.NoError(t, err)
	_, publicID, err := identifier.New(registry, newID).Issue(ctx)
	require.NoError(t, err)
	return publicID
}

// application 層は presentation 層に内部 ID を返さないため、識別子から引く
func internalID(t *testing.T, ctx context.Context, publicID uuid.UUID) int64 {
	t.Helper()
	registry, err := domaindef.Load(ctx)
	require.NoError(t, err)
	ref, err := identifier.NewEntityRefResolver(registry).Resolve(ctx, publicID)
	require.NoError(t, err)
	return ref.ID()
}

func registerPresetDeck(t *testing.T, ctx context.Context, name string) command.ChangedPresetDeck {
	t.Helper()
	registered, err := newPresetDeckHandler(t, ctx).Register(ctx, name)
	require.NoError(t, err)
	return registered
}
