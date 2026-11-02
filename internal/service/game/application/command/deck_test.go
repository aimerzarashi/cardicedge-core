package command_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/constraint/constrainttest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/deck"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/deck/decktest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/gamerules"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/player"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/player/playertest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/startergrant/startergranttest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/rdb"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/game/application/rules"
)

func TestCreateDeck_CreatesDeckOfOwnerWithNameAndCardsOfSource(t *testing.T) {
	ctx := isolatedDatabase(t)
	playerID := playertest.ID(t, issuePlayerID(t, ctx))
	starterGrantID := startergranttest.ID(t, 10)
	number := startergranttest.DeckNumber(t, 2)
	source := startergranttest.Deck(t, "水", startergranttest.CardCount(t, cardA, 2), startergranttest.CardCount(t, cardC, 2))

	require.NoError(t, newDeckHandler(t, ctx).Create(ctx, playerID, starterGrantID, number, source))

	// State は持ち主・名前・カードなどを持たない（Decide が使わないため）ので、作成のイベントの payload から読む
	id, err := deck.NewID(constrainttest.Owner(t, ctx, deck.StarterKey(starterGrantID, number)))
	require.NoError(t, err)
	exec, err := rdb.GetExecutor(ctx)
	require.NoError(t, err)
	var raw []byte
	require.NoError(t, exec.QueryRowContext(ctx, `SELECT payload FROM domain_events WHERE entity_id = $1 AND aggregate_version = 1`, id.Int64()).Scan(&raw))
	var created deck.CreatedEvent
	require.NoError(t, json.Unmarshal(raw, &created))
	require.Equal(t, playerID, created.Player)
	require.Equal(t, decktest.Name(t, "水"), created.Name)
	require.Equal(t, source.Cards, created.Cards)
	require.Equal(t, starterGrantID, created.StarterGrant)
	require.Equal(t, number, created.Number)
}

// 同じ配布の同じ番号のデッキは 1 つ。2 回目はエラーにせず何もしない（ポリシーのやり直しで同じ作成が来ても害が無いように）
func TestCreateDeck_DoesNothingWhenStarterDeckAlreadyExists(t *testing.T) {
	ctx := isolatedDatabase(t)
	handler := newDeckHandler(t, ctx)
	playerID := playertest.ID(t, issuePlayerID(t, ctx))
	starterGrantID := startergranttest.ID(t, 10)
	number := startergranttest.DeckNumber(t, 1)
	source := startergranttest.Deck(t, "火", startergranttest.CardCount(t, cardA, 4))
	require.NoError(t, handler.Create(ctx, playerID, starterGrantID, number, source))

	err := handler.Create(ctx, playerID, starterGrantID, number, source)

	require.NoError(t, err)
	require.Equal(t, 1, countEventsOf(t, ctx, deckTypeID))
}

func reservedDecks(t *testing.T, ctx context.Context, playerID player.ID) int {
	t.Helper()
	exec, err := rdb.GetExecutor(ctx)
	require.NoError(t, err)
	var used int
	err = exec.QueryRowContext(ctx, `
		SELECT COALESCE(SUM(q.used), 0) FROM domain_quotas q JOIN domain_quota_types t ON t.id = q.quota_type_id
		WHERE t.name = $1 AND q.owner_id = $2`, deck.PlayerQuotaType.String(), playerID.Int64()).Scan(&used)
	require.NoError(t, err)
	return used
}

func TestCreateDeck_ReservesQuotaOfOwner(t *testing.T) {
	ctx := isolatedDatabase(t)
	playerID := playertest.ID(t, issuePlayerID(t, ctx))
	source := startergranttest.Deck(t, "火", startergranttest.CardCount(t, cardA, 4))

	require.NoError(t, newDeckHandler(t, ctx).Create(ctx, playerID, startergranttest.ID(t, 10), startergranttest.DeckNumber(t, 1), source))

	require.Equal(t, 1, reservedDecks(t, ctx, playerID))
}

// 配布のやり直しで同じ作成が来ても、全体が戻るので数は増えない
func TestCreateDeck_DoesNotReserveQuotaAgainWhenStarterDeckAlreadyExists(t *testing.T) {
	ctx := isolatedDatabase(t)
	handler := newDeckHandler(t, ctx)
	playerID := playertest.ID(t, issuePlayerID(t, ctx))
	starterGrantID := startergranttest.ID(t, 10)
	number := startergranttest.DeckNumber(t, 1)
	source := startergranttest.Deck(t, "火", startergranttest.CardCount(t, cardA, 4))
	require.NoError(t, handler.Create(ctx, playerID, starterGrantID, number, source))

	require.NoError(t, handler.Create(ctx, playerID, starterGrantID, number, source))

	require.Equal(t, 1, reservedDecks(t, ctx, playerID))
}

func TestCreateDeck_RejectsDeckOverLimit(t *testing.T) {
	ctx := isolatedDatabase(t)
	limit, err := domain.NewQuotaLimit(1)
	require.NoError(t, err)
	handler := newDeckHandlerWithLimit(t, ctx, limit)
	playerID := playertest.ID(t, issuePlayerID(t, ctx))
	starterGrantID := startergranttest.ID(t, 10)
	require.NoError(t, handler.Create(ctx, playerID, starterGrantID, startergranttest.DeckNumber(t, 1), startergranttest.Deck(t, "火", startergranttest.CardCount(t, cardA, 4))))

	err = handler.Create(ctx, playerID, starterGrantID, startergranttest.DeckNumber(t, 2), startergranttest.Deck(t, "水", startergranttest.CardCount(t, cardA, 4)))

	require.ErrorIs(t, err, deck.ErrLimitReached)
	require.Equal(t, 1, countEventsOf(t, ctx, deckTypeID))
	require.Equal(t, 1, reservedDecks(t, ctx, playerID))
}

// 写されていないルールの代わりに既定の値で作らない。何も残さない（トランザクションごと戻る）
func TestCreateDeck_RejectsWhenRulesNotPublished(t *testing.T) {
	ctx := isolatedDatabase(t)
	handler := newDeckHandlerWithRules(t, ctx, func(context.Context) (gamerules.Rules, bool, error) {
		return gamerules.Rules{}, false, nil
	})
	playerID := playertest.ID(t, issuePlayerID(t, ctx))

	err := handler.Create(ctx, playerID, startergranttest.ID(t, 10), startergranttest.DeckNumber(t, 1), startergranttest.Deck(t, "火", startergranttest.CardCount(t, cardA, 4)))

	require.ErrorIs(t, err, rules.ErrNotPublished)
	require.Zero(t, countEventsOf(t, ctx, deckTypeID))
	require.Zero(t, reservedDecks(t, ctx, playerID))
}

func TestCreateDeck_RejectsSameNameOfSameOwner(t *testing.T) {
	ctx := isolatedDatabase(t)
	handler := newDeckHandler(t, ctx)
	playerID := playertest.ID(t, issuePlayerID(t, ctx))
	starterGrantID := startergranttest.ID(t, 10)
	require.NoError(t, handler.Create(ctx, playerID, starterGrantID, startergranttest.DeckNumber(t, 1), startergranttest.Deck(t, "火", startergranttest.CardCount(t, cardA, 4))))

	err := handler.Create(ctx, playerID, starterGrantID, startergranttest.DeckNumber(t, 2), startergranttest.Deck(t, "火", startergranttest.CardCount(t, cardA, 4)))

	require.ErrorIs(t, err, deck.ErrNameAlreadyInUse)
	require.Equal(t, 1, reservedDecks(t, ctx, playerID))
}
