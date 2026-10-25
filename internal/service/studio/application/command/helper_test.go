package command_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/constraint"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/creature"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/gamerules"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/item"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/operator"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/operator/operatortest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/presetdeck"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/spell"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/starterdeck"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domaindef"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/eventstore"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/identifier"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/rdb"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/rdb/rdbtest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/studio/application/authentication"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/studio/application/command"
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

var defaultRules = gamerules.Defaults().Deck

// 既定のルールで動かす。値を小さくして確かめるときは newPresetDeckHandlerWithRules
func newPresetDeckHandler(t *testing.T, ctx context.Context) *command.PresetDeckHandler {
	t.Helper()
	return newPresetDeckHandlerWithRules(t, ctx, gamerules.Defaults())
}

func newPresetDeckHandlerWithRules(t *testing.T, ctx context.Context, rules gamerules.Rules) *command.PresetDeckHandler {
	t.Helper()
	registry, err := domaindef.Load(ctx)
	require.NoError(t, err)
	return command.NewPresetDeckHandler(
		presetDeckStore(registry),
		identifier.New(registry, presetdeck.NewID),
		identifier.NewEntityRefResolver(registry),
		constraint.UpdatePresetDeckName,
		constraint.UpdatePresetDeckCardReferences,
		newCardStatusReader(registry).Statuses,
		fixedRules(rules),
		// wiring と同じく、DB に残る精度（マイクロ秒）にそろえる（保存の後に読み直した日時と、結果の日時を同じにするため）
		func() time.Time { return time.Now().UTC().Truncate(time.Microsecond) },
	)
}

func newGameRulesHandler(t *testing.T, ctx context.Context) *command.GameRulesHandler {
	t.Helper()
	registry, err := domaindef.Load(ctx)
	require.NoError(t, err)
	return command.NewGameRulesHandler(
		eventstore.New(registry, gamerules.NewID, gamerules.NewEventType, gamerules.InitialState, gamerules.Evolve),
		identifier.New(registry, gamerules.NewID),
		constraint.FindGameRules,
		constraint.CreateGameRules,
		func() time.Time { return time.Now().UTC().Truncate(time.Microsecond) },
	)
}

// 読むルールを固定する（イベントストアに作らずに、テストで値を小さくして確かめるため）
func fixedRules(rules gamerules.Rules) command.GetGameRulesFunc {
	return func(context.Context) (gamerules.Rules, error) {
		return rules, nil
	}
}

func newCardStatusReader(registry *domaindef.Registry) *command.CardStatusReader {
	return command.NewCardStatusReader(
		eventstore.New(registry, creature.NewID, creature.NewEventType, creature.InitialState, creature.Evolve),
		eventstore.New(registry, item.NewID, item.NewEventType, item.InitialState, item.Evolve),
		eventstore.New(registry, spell.NewID, spell.NewEventType, spell.InitialState, spell.Evolve),
	)
}

func newCardHandler(t *testing.T, ctx context.Context) *command.CardHandler {
	t.Helper()
	registry, err := domaindef.Load(ctx)
	require.NoError(t, err)
	return command.NewCardHandler(
		identifier.NewEntityRefResolver(registry),
		newCreatureHandler(t, ctx),
		newItemHandler(t, ctx),
		newSpellHandler(t, ctx),
	)
}

// プリセットデッキに入れられるカード（公開中）を、登録して公開して作る。名前は重ならないよう毎回変える
func publishedCreature(t *testing.T, ctx context.Context) uuid.UUID {
	t.Helper()
	registered, err := newCreatureHandler(t, ctx).Register(ctx, "クリーチャー-"+uuid.NewString())
	require.NoError(t, err)
	_, err = newCardHandler(t, ctx).Publish(ctx, registered.ID, registered.Revision)
	require.NoError(t, err)
	return registered.ID
}

func publishedItem(t *testing.T, ctx context.Context) uuid.UUID {
	t.Helper()
	registered, err := newItemHandler(t, ctx).Register(ctx, "アイテム-"+uuid.NewString())
	require.NoError(t, err)
	_, err = newCardHandler(t, ctx).Publish(ctx, registered.ID, registered.Revision)
	require.NoError(t, err)
	return registered.ID
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

func starterDeckStore(registry *domaindef.Registry) domain.EventStore[starterdeck.ID, starterdeck.Event, starterdeck.State] {
	return eventstore.New(registry, starterdeck.NewID, starterdeck.NewEventType, starterdeck.InitialState, starterdeck.Evolve)
}

// 公開中の数の持ち主（ゲーム全体のルール）が要るため、無ければ作る。ルールの値は固定する（値を小さくして確かめるため）
func newStarterDeckHandlerWithRules(t *testing.T, ctx context.Context, rules gamerules.Rules) *command.StarterDeckHandler {
	t.Helper()
	_, err := newGameRulesHandler(t, ctx).CreateInitial(ctx)
	require.NoError(t, err)
	registry, err := domaindef.Load(ctx)
	require.NoError(t, err)
	return command.NewStarterDeckHandler(
		starterDeckStore(registry),
		identifier.New(registry, starterdeck.NewID),
		identifier.NewEntityRefResolver(registry),
		constraint.UpdateStarterDeckName,
		constraint.UpdateStarterDeckCardReferences,
		newCardStatusReader(registry).Statuses,
		presetDeckStore(registry),
		identifier.New(registry, presetdeck.NewID),
		constraint.FindGameRules,
		constraint.ReserveStarterDeckQuota,
		constraint.ReleaseStarterDeckQuota,
		fixedRules(rules),
		func() time.Time { return time.Now().UTC().Truncate(time.Microsecond) },
	)
}

func newStarterDeckHandler(t *testing.T, ctx context.Context) *command.StarterDeckHandler {
	t.Helper()
	return newStarterDeckHandlerWithRules(t, ctx, gamerules.Defaults())
}

func newCreatureHandler(t *testing.T, ctx context.Context) *command.CreatureHandler {
	t.Helper()
	registry, err := domaindef.Load(ctx)
	require.NoError(t, err)
	return command.NewCreatureHandler(
		eventstore.New(registry, creature.NewID, creature.NewEventType, creature.InitialState, creature.Evolve),
		identifier.New(registry, creature.NewID),
		constraint.UpdateCreatureName,
		func() time.Time { return time.Now().UTC() },
	)
}

func newItemHandler(t *testing.T, ctx context.Context) *command.ItemHandler {
	t.Helper()
	registry, err := domaindef.Load(ctx)
	require.NoError(t, err)
	return command.NewItemHandler(
		eventstore.New(registry, item.NewID, item.NewEventType, item.InitialState, item.Evolve),
		identifier.New(registry, item.NewID),
		constraint.UpdateItemName,
		func() time.Time { return time.Now().UTC() },
	)
}
