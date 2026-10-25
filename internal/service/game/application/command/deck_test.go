package command_test

import (
	"context"
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
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domaindef"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/eventstore"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/rdb"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/game/application/rules"
)

func TestCreateDeck_CreatesDeckOfOwnerWithNameAndCardsOfSource(t *testing.T) {
	ctx := isolatedDatabase(t)
	owner := playertest.ID(t, issuePlayerID(t, ctx))
	grant := startergranttest.ID(t, 10)
	number := startergranttest.DeckNumber(t, 2)
	source := startergranttest.Deck(t, "水", startergranttest.CardCount(t, cardA, 2), startergranttest.CardCount(t, cardC, 2))

	require.NoError(t, newDeckHandler(t, ctx).Create(ctx, owner, grant, number, source))

	registry, err := domaindef.Load(ctx)
	require.NoError(t, err)
	id, err := deck.NewID(constrainttest.Owner(t, ctx, deck.StarterKey(grant, number)))
	require.NoError(t, err)
	state, err := eventstore.New(registry, deck.NewID, deck.NewEventType, deck.InitialState, deck.Evolve).Load(ctx, id)
	require.NoError(t, err)
	created, ok := state.(deck.Deck)
	require.True(t, ok)
	require.Equal(t, owner, created.Owner)
	require.Equal(t, decktest.Name(t, "水"), created.Name)
	require.Equal(t, source.Cards, created.Cards)
	require.Equal(t, grant, created.Grant)
	require.Equal(t, number, created.Number)
}

// 同じ配布の同じ番号のデッキは 1 つ。2 回目はエラーにせず何もしない（ポリシーのやり直しで同じ作成が来ても害が無いように）
func TestCreateDeck_DoesNothingWhenStarterDeckAlreadyExists(t *testing.T) {
	ctx := isolatedDatabase(t)
	handler := newDeckHandler(t, ctx)
	owner := playertest.ID(t, issuePlayerID(t, ctx))
	grant := startergranttest.ID(t, 10)
	number := startergranttest.DeckNumber(t, 1)
	source := startergranttest.Deck(t, "火", startergranttest.CardCount(t, cardA, 4))
	require.NoError(t, handler.Create(ctx, owner, grant, number, source))

	err := handler.Create(ctx, owner, grant, number, source)

	require.NoError(t, err)
	require.Equal(t, 1, countEventsOf(t, ctx, deckTypeID))
}

func reservedDecks(t *testing.T, ctx context.Context, owner player.ID) int {
	t.Helper()
	exec, err := rdb.GetExecutor(ctx)
	require.NoError(t, err)
	var used int
	err = exec.QueryRowContext(ctx, `
		SELECT COALESCE(SUM(q.used), 0) FROM domain_quotas q JOIN domain_quota_types t ON t.id = q.quota_type_id
		WHERE t.name = $1 AND q.owner_id = $2`, deck.PlayerQuotaType.String(), owner.Int64()).Scan(&used)
	require.NoError(t, err)
	return used
}

func TestCreateDeck_ReservesQuotaOfOwner(t *testing.T) {
	ctx := isolatedDatabase(t)
	owner := playertest.ID(t, issuePlayerID(t, ctx))
	source := startergranttest.Deck(t, "火", startergranttest.CardCount(t, cardA, 4))

	require.NoError(t, newDeckHandler(t, ctx).Create(ctx, owner, startergranttest.ID(t, 10), startergranttest.DeckNumber(t, 1), source))

	require.Equal(t, 1, reservedDecks(t, ctx, owner))
}

// 配布のやり直しで同じ作成が来ても、全体が戻るので数は増えない
func TestCreateDeck_DoesNotReserveQuotaAgainWhenStarterDeckAlreadyExists(t *testing.T) {
	ctx := isolatedDatabase(t)
	handler := newDeckHandler(t, ctx)
	owner := playertest.ID(t, issuePlayerID(t, ctx))
	grant := startergranttest.ID(t, 10)
	number := startergranttest.DeckNumber(t, 1)
	source := startergranttest.Deck(t, "火", startergranttest.CardCount(t, cardA, 4))
	require.NoError(t, handler.Create(ctx, owner, grant, number, source))

	require.NoError(t, handler.Create(ctx, owner, grant, number, source))

	require.Equal(t, 1, reservedDecks(t, ctx, owner))
}

func TestCreateDeck_RejectsDeckOverLimit(t *testing.T) {
	ctx := isolatedDatabase(t)
	limit, err := domain.NewQuotaLimit(1)
	require.NoError(t, err)
	handler := newDeckHandlerWithLimit(t, ctx, limit)
	owner := playertest.ID(t, issuePlayerID(t, ctx))
	grant := startergranttest.ID(t, 10)
	require.NoError(t, handler.Create(ctx, owner, grant, startergranttest.DeckNumber(t, 1), startergranttest.Deck(t, "火", startergranttest.CardCount(t, cardA, 4))))

	err = handler.Create(ctx, owner, grant, startergranttest.DeckNumber(t, 2), startergranttest.Deck(t, "水", startergranttest.CardCount(t, cardA, 4)))

	require.ErrorIs(t, err, deck.ErrLimitReached)
	require.Equal(t, 1, countEventsOf(t, ctx, deckTypeID))
	require.Equal(t, 1, reservedDecks(t, ctx, owner))
}

// 写されていないルールの代わりに既定の値で作らない。何も残さない（トランザクションごと戻る）
func TestCreateDeck_RejectsWhenRulesNotPublished(t *testing.T) {
	ctx := isolatedDatabase(t)
	handler := newDeckHandlerWithRules(t, ctx, func(context.Context) (gamerules.Rules, bool, error) {
		return gamerules.Rules{}, false, nil
	})
	owner := playertest.ID(t, issuePlayerID(t, ctx))

	err := handler.Create(ctx, owner, startergranttest.ID(t, 10), startergranttest.DeckNumber(t, 1), startergranttest.Deck(t, "火", startergranttest.CardCount(t, cardA, 4)))

	require.ErrorIs(t, err, rules.ErrNotPublished)
	require.Zero(t, countEventsOf(t, ctx, deckTypeID))
	require.Zero(t, reservedDecks(t, ctx, owner))
}

func TestCreateDeck_RejectsSameNameOfSameOwner(t *testing.T) {
	ctx := isolatedDatabase(t)
	handler := newDeckHandler(t, ctx)
	owner := playertest.ID(t, issuePlayerID(t, ctx))
	grant := startergranttest.ID(t, 10)
	require.NoError(t, handler.Create(ctx, owner, grant, startergranttest.DeckNumber(t, 1), startergranttest.Deck(t, "火", startergranttest.CardCount(t, cardA, 4))))

	err := handler.Create(ctx, owner, grant, startergranttest.DeckNumber(t, 2), startergranttest.Deck(t, "火", startergranttest.CardCount(t, cardA, 4)))

	require.ErrorIs(t, err, deck.ErrNameAlreadyInUse)
	require.Equal(t, 1, reservedDecks(t, ctx, owner))
}
