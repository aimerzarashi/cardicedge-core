package repository_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/identifier"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/orm/sqlboiler/models"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/rdb"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/game/application/query"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/game/application/query/readmodel"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/game/infrastructure/repository"
)

func TestDeckReader_ListsOwnDecksInRegistrationOrderWithCardsInTypeThenNameOrder(t *testing.T) {
	f := newPlayerFixture(t)
	playerID, ownerPublicID := f.issuePlayer(t)
	other, otherPublicID := f.issuePlayer(t)
	writer := repository.NewDeckWriter(identifier.NewEntityRefResolver(f.registry))
	spell := insertDeckCard(t, f.ctx, "a", query.CardTypeSpell)
	itemB := insertDeckCard(t, f.ctx, "b", query.CardTypeItem)
	itemA := insertDeckCard(t, f.ctx, "a", query.CardTypeItem)
	creature := insertDeckCard(t, f.ctx, "z", query.CardTypeCreature)
	base := time.Date(2026, 11, 3, 9, 0, 0, 0, time.UTC)
	olderID, olderPublicID := f.issueDeck(t)
	newerID, newerPublicID := f.issueDeck(t)
	othersID, othersPublicID := f.issueDeck(t)
	older := readmodel.Deck{Deck: olderID, Player: playerID, Name: "先", RegisteredAt: base, Cards: []readmodel.DeckCard{
		{CardID: spell, Count: 1}, {CardID: itemB, Count: 2}, {CardID: creature, Count: 3}, {CardID: itemA, Count: 4},
	}}
	newer := readmodel.Deck{Deck: newerID, Player: playerID, Name: "後", RegisteredAt: base.Add(time.Second), Cards: []readmodel.DeckCard{{CardID: creature, Count: 5}}}
	others := readmodel.Deck{Deck: othersID, Player: other, Name: "他", RegisteredAt: base, Cards: []readmodel.DeckCard{{CardID: creature, Count: 1}}}
	// 作った順と逆に書いても、作った順に読む
	for _, state := range []readmodel.Deck{newer, others, older} {
		require.NoError(t, writer.Upsert(f.ctx, state))
	}

	got, err := repository.NewDeckReader().ListByPlayer(f.ctx, ownerPublicID)
	require.NoError(t, err)
	gotOther, err := repository.NewDeckReader().ListByPlayer(f.ctx, otherPublicID)
	require.NoError(t, err)

	require.Equal(t, []query.Deck{
		{ID: olderPublicID, Name: "先", Cards: []query.DeckCard{
			{CardID: creature, Name: "z", Type: query.CardTypeCreature, Count: 3},
			{CardID: itemA, Name: "a", Type: query.CardTypeItem, Count: 4},
			{CardID: itemB, Name: "b", Type: query.CardTypeItem, Count: 2},
			{CardID: spell, Name: "a", Type: query.CardTypeSpell, Count: 1},
		}},
		{ID: newerPublicID, Name: "後", Cards: []query.DeckCard{{CardID: creature, Name: "z", Type: query.CardTypeCreature, Count: 5}}},
	}, got)
	require.Equal(t, []query.Deck{{ID: othersPublicID, Name: "他", Cards: []query.DeckCard{{CardID: creature, Name: "z", Type: query.CardTypeCreature, Count: 1}}}}, gotOther)
}

func TestDeckReader_GetReturnsOwnDeckWithCardsInTypeThenNameOrder(t *testing.T) {
	f := newPlayerFixture(t)
	playerID, ownerPublicID := f.issuePlayer(t)
	writer := repository.NewDeckWriter(identifier.NewEntityRefResolver(f.registry))
	spell := insertDeckCard(t, f.ctx, "a", query.CardTypeSpell)
	creature := insertDeckCard(t, f.ctx, "z", query.CardTypeCreature)
	deckID, deckPublicID := f.issueDeck(t)
	require.NoError(t, writer.Upsert(f.ctx, readmodel.Deck{Deck: deckID, Player: playerID, Name: "火", RegisteredAt: time.Now(), Cards: []readmodel.DeckCard{
		{CardID: spell, Count: 1}, {CardID: creature, Count: 3},
	}}))

	got, err := repository.NewDeckReader().Get(f.ctx, ownerPublicID, deckPublicID)

	require.NoError(t, err)
	require.Equal(t, query.Deck{ID: deckPublicID, Name: "火", Cards: []query.DeckCard{
		{CardID: creature, Name: "z", Type: query.CardTypeCreature, Count: 3},
		{CardID: spell, Name: "a", Type: query.CardTypeSpell, Count: 1},
	}}, got)
}

func TestDeckReader_GetReturnsNotFoundForOtherPlayersDeck(t *testing.T) {
	f := newPlayerFixture(t)
	other, _ := f.issuePlayer(t)
	_, requesterPublicID := f.issuePlayer(t)
	writer := repository.NewDeckWriter(identifier.NewEntityRefResolver(f.registry))
	card := insertDeckCard(t, f.ctx, "a", query.CardTypeCreature)
	deckID, deckPublicID := f.issueDeck(t)
	require.NoError(t, writer.Upsert(f.ctx, readmodel.Deck{Deck: deckID, Player: other, Name: "他", RegisteredAt: time.Now(), Cards: []readmodel.DeckCard{{CardID: card, Count: 1}}}))

	_, err := repository.NewDeckReader().Get(f.ctx, requesterPublicID, deckPublicID)

	require.ErrorIs(t, err, query.ErrDeckNotFound)
}

func TestDeckReader_GetReturnsNotFoundForMissingDeck(t *testing.T) {
	f := newPlayerFixture(t)
	_, publicID := f.issuePlayer(t)

	_, err := repository.NewDeckReader().Get(f.ctx, publicID, uuid.New())

	require.ErrorIs(t, err, query.ErrDeckNotFound)
}

func TestDeckReader_ListsNothingForPlayerWithoutDecks(t *testing.T) {
	f := newPlayerFixture(t)
	_, publicID := f.issuePlayer(t)

	got, err := repository.NewDeckReader().ListByPlayer(f.ctx, publicID)

	require.NoError(t, err)
	require.Empty(t, got)
}

// 写していないカードを黙って捨てず、読み出しを失敗させる（NPC のデッキと同じ）
func TestDeckReader_FailsForCardThatWasNotPublishedToGame(t *testing.T) {
	f := newPlayerFixture(t)
	playerID, publicID := f.issuePlayer(t)
	deckID, _ := f.issueDeck(t)
	writer := repository.NewDeckWriter(identifier.NewEntityRefResolver(f.registry))
	state := readmodel.Deck{Deck: deckID, Player: playerID, Name: "火", RegisteredAt: time.Now(), Cards: []readmodel.DeckCard{{CardID: uuid.New(), Count: 1}}}
	require.NoError(t, writer.Upsert(f.ctx, state))

	_, err := repository.NewDeckReader().ListByPlayer(f.ctx, publicID)

	require.ErrorContains(t, err, "is not published to game")
}

func TestDeckWriter_UpsertTwiceKeepsOneRow(t *testing.T) {
	f := newPlayerFixture(t)
	playerID, ownerPublicID := f.issuePlayer(t)
	deckID, _ := f.issueDeck(t)
	writer := repository.NewDeckWriter(identifier.NewEntityRefResolver(f.registry))
	card := insertDeckCard(t, f.ctx, "a", query.CardTypeCreature)
	state := readmodel.Deck{Deck: deckID, Player: playerID, Name: "火", RegisteredAt: time.Date(2026, 11, 3, 9, 0, 0, 0, time.UTC), Cards: []readmodel.DeckCard{{CardID: card, Count: 1}}}

	require.NoError(t, writer.Upsert(f.ctx, state))
	require.NoError(t, writer.Upsert(f.ctx, state))
	got, err := repository.NewDeckReader().ListByPlayer(f.ctx, ownerPublicID)

	require.NoError(t, err)
	require.Len(t, got, 1)
}

func TestDeckWriter_DeleteRemovesRowAndToleratesMissing(t *testing.T) {
	f := newPlayerFixture(t)
	playerID, ownerPublicID := f.issuePlayer(t)
	deckID, _ := f.issueDeck(t)
	writer := repository.NewDeckWriter(identifier.NewEntityRefResolver(f.registry))
	card := insertDeckCard(t, f.ctx, "a", query.CardTypeCreature)
	state := readmodel.Deck{Deck: deckID, Player: playerID, Name: "火", RegisteredAt: time.Date(2026, 11, 3, 9, 0, 0, 0, time.UTC), Cards: []readmodel.DeckCard{{CardID: card, Count: 1}}}
	require.NoError(t, writer.Upsert(f.ctx, state))

	require.NoError(t, writer.Delete(f.ctx, state))
	require.NoError(t, writer.Delete(f.ctx, state))
	got, err := repository.NewDeckReader().ListByPlayer(f.ctx, ownerPublicID)

	require.NoError(t, err)
	require.Empty(t, got)
}

// デッキを作るのは持ち主のプレイヤーのものとして記録し、更新の操作は無いため、登録も更新も持ち主の公開 ID になる
func TestDeckWriter_RecordsOwnerPublicIDAsRegisteredAndUpdatedBy(t *testing.T) {
	f := newPlayerFixture(t)
	playerID, ownerPublicID := f.issuePlayer(t)
	deckID, deckPublicID := f.issueDeck(t)
	writer := repository.NewDeckWriter(identifier.NewEntityRefResolver(f.registry))
	registeredAt := time.Date(2026, 11, 3, 9, 0, 0, 0, time.UTC)
	card := uuid.New()
	state := readmodel.Deck{Deck: deckID, Player: playerID, Name: "火", RegisteredAt: registeredAt, Cards: []readmodel.DeckCard{{CardID: card, Count: 2}}}
	require.NoError(t, writer.Upsert(f.ctx, state))

	exec, err := rdb.GetExecutor(f.ctx)
	require.NoError(t, err)
	row, err := models.FindDeck(f.ctx, exec, deckPublicID.String())
	require.NoError(t, err)
	var metadata readmodel.DeckMetadata
	require.NoError(t, json.Unmarshal(row.Metadata, &metadata))

	require.Equal(t, deckPublicID, metadata.ID)
	require.Equal(t, "火", metadata.Name)
	require.Equal(t, []readmodel.DeckCard{{CardID: card, Count: 2}}, metadata.Cards)
	require.Equal(t, ownerPublicID, metadata.RegisteredBy)
	require.Equal(t, ownerPublicID, metadata.UpdatedBy)
	require.Equal(t, registeredAt, metadata.RegisteredAt.UTC())
	require.Equal(t, registeredAt, metadata.UpdatedAt.UTC())
}
