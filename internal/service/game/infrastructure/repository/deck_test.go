package repository_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/card"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/card/cardtest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/creature"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/deck"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/deck/decktest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/item"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/spell"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/identifier"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/orm/sqlboiler/models"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/rdb"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/game/application/query"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/game/application/query/readmodel"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/game/infrastructure/repository"
)

// 写したカードを、make publish-master と同じく cards の行と game の内部 ID（domain_entities の行）で用意する。
// 公開 ID は識別子の発行が振るので、cards の行はその公開 ID で入れる
func (f playerFixture) insertPublishedCard(t *testing.T, name string, cardType query.CardType) (uuid.UUID, card.Ref) {
	t.Helper()
	publicID, ref := f.issueCard(t, cardType)
	insertCardRow(t, f.ctx, publicID, name, cardType)
	return publicID, ref
}

// cards の行は入れず、game の内部 ID（domain_entities の行）だけ用意する
func (f playerFixture) issueCard(t *testing.T, cardType query.CardType) (uuid.UUID, card.Ref) {
	t.Helper()
	switch cardType {
	case query.CardTypeCreature:
		return issueCardOf(t, f, creature.NewID)
	case query.CardTypeItem:
		return issueCardOf(t, f, item.NewID)
	default:
		return issueCardOf(t, f, spell.NewID)
	}
}

func issueCardOf[I domain.ID](t *testing.T, f playerFixture, newID domain.NewIDFunc[I]) (uuid.UUID, card.Ref) {
	t.Helper()
	id, publicID, err := identifier.New(f.registry, newID).Issue(f.ctx)
	require.NoError(t, err)
	return publicID, cardtest.Ref(t, id.EntityType(), id.Int64())
}

func TestDeckReader_ListsOwnDecksInRegistrationOrderWithCardsInTypeThenNameOrder(t *testing.T) {
	f := newPlayerFixture(t)
	playerID, ownerPublicID := f.issuePlayer(t)
	other, otherPublicID := f.issuePlayer(t)
	writer := repository.NewDeckWriter(identifier.NewEntityRefResolver(f.registry))
	spell, spellRef := f.insertPublishedCard(t, "a", query.CardTypeSpell)
	itemB, itemBRef := f.insertPublishedCard(t, "b", query.CardTypeItem)
	itemA, itemARef := f.insertPublishedCard(t, "a", query.CardTypeItem)
	creature, creatureRef := f.insertPublishedCard(t, "z", query.CardTypeCreature)
	base := time.Date(2026, 11, 3, 9, 0, 0, 0, time.UTC)
	olderID, olderPublicID := f.issueDeck(t)
	newerID, newerPublicID := f.issueDeck(t)
	othersID, othersPublicID := f.issueDeck(t)
	older := readmodel.Deck{Deck: olderID, Player: playerID, Name: "先", RegisteredAt: base, Cards: []deck.Card{
		decktest.Card(t, spellRef, 1), decktest.Card(t, itemBRef, 2), decktest.Card(t, creatureRef, 3), decktest.Card(t, itemARef, 4),
	}}
	newer := readmodel.Deck{Deck: newerID, Player: playerID, Name: "後", RegisteredAt: base.Add(time.Second), Cards: []deck.Card{decktest.Card(t, creatureRef, 4)}}
	others := readmodel.Deck{Deck: othersID, Player: other, Name: "他", RegisteredAt: base, Cards: []deck.Card{decktest.Card(t, creatureRef, 1)}}
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
			{CardID: creature, Name: "z", Type: query.CardTypeCreature, Cost: 10, Rarity: "normal", Creature: defaultCreatureSpec(), Count: 3},
			{CardID: itemA, Name: "a", Type: query.CardTypeItem, Cost: 10, Rarity: "normal", Item: defaultItemSpec(), Count: 4},
			{CardID: itemB, Name: "b", Type: query.CardTypeItem, Cost: 10, Rarity: "normal", Item: defaultItemSpec(), Count: 2},
			{CardID: spell, Name: "a", Type: query.CardTypeSpell, Cost: 10, Rarity: "normal", Count: 1},
		}},
		{ID: newerPublicID, Name: "後", Cards: []query.DeckCard{{CardID: creature, Name: "z", Type: query.CardTypeCreature, Cost: 10, Rarity: "normal", Creature: defaultCreatureSpec(), Count: 4}}},
	}, got)
	require.Equal(t, []query.Deck{{ID: othersPublicID, Name: "他", Cards: []query.DeckCard{{CardID: creature, Name: "z", Type: query.CardTypeCreature, Cost: 10, Rarity: "normal", Creature: defaultCreatureSpec(), Count: 1}}}}, gotOther)
}

func TestDeckReader_GetReturnsOwnDeckWithCardsInTypeThenNameOrder(t *testing.T) {
	f := newPlayerFixture(t)
	playerID, ownerPublicID := f.issuePlayer(t)
	writer := repository.NewDeckWriter(identifier.NewEntityRefResolver(f.registry))
	spell, spellRef := f.insertPublishedCard(t, "a", query.CardTypeSpell)
	creature, creatureRef := f.insertPublishedCard(t, "z", query.CardTypeCreature)
	deckID, deckPublicID := f.issueDeck(t)
	require.NoError(t, writer.Upsert(f.ctx, readmodel.Deck{Deck: deckID, Player: playerID, Name: "火", RegisteredAt: time.Now(), Cards: []deck.Card{
		decktest.Card(t, spellRef, 1), decktest.Card(t, creatureRef, 3),
	}}))

	got, err := repository.NewDeckReader().Get(f.ctx, ownerPublicID, deckPublicID)

	require.NoError(t, err)
	require.Equal(t, query.Deck{ID: deckPublicID, Name: "火", Cards: []query.DeckCard{
		{CardID: creature, Name: "z", Type: query.CardTypeCreature, Cost: 10, Rarity: "normal", Creature: defaultCreatureSpec(), Count: 3},
		{CardID: spell, Name: "a", Type: query.CardTypeSpell, Cost: 10, Rarity: "normal", Count: 1},
	}}, got)
}

func TestDeckReader_GetReturnsNotFoundForOtherPlayersDeck(t *testing.T) {
	f := newPlayerFixture(t)
	other, _ := f.issuePlayer(t)
	_, requesterPublicID := f.issuePlayer(t)
	writer := repository.NewDeckWriter(identifier.NewEntityRefResolver(f.registry))
	_, cardRef := f.insertPublishedCard(t, "a", query.CardTypeCreature)
	deckID, deckPublicID := f.issueDeck(t)
	require.NoError(t, writer.Upsert(f.ctx, readmodel.Deck{Deck: deckID, Player: other, Name: "他", RegisteredAt: time.Now(), Cards: []deck.Card{decktest.Card(t, cardRef, 1)}}))

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
	_, unpublished := f.issueCard(t, query.CardTypeCreature)
	state := readmodel.Deck{Deck: deckID, Player: playerID, Name: "火", RegisteredAt: time.Now(), Cards: []deck.Card{decktest.Card(t, unpublished, 1)}}
	require.NoError(t, writer.Upsert(f.ctx, state))

	_, err := repository.NewDeckReader().ListByPlayer(f.ctx, publicID)

	require.ErrorContains(t, err, "is not published to game")
}

func TestDeckWriter_UpsertTwiceKeepsOneRow(t *testing.T) {
	f := newPlayerFixture(t)
	playerID, ownerPublicID := f.issuePlayer(t)
	deckID, _ := f.issueDeck(t)
	writer := repository.NewDeckWriter(identifier.NewEntityRefResolver(f.registry))
	_, cardRef := f.insertPublishedCard(t, "a", query.CardTypeCreature)
	state := readmodel.Deck{Deck: deckID, Player: playerID, Name: "火", RegisteredAt: time.Date(2026, 11, 3, 9, 0, 0, 0, time.UTC), Cards: []deck.Card{decktest.Card(t, cardRef, 1)}}

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
	_, cardRef := f.insertPublishedCard(t, "a", query.CardTypeCreature)
	state := readmodel.Deck{Deck: deckID, Player: playerID, Name: "火", RegisteredAt: time.Date(2026, 11, 3, 9, 0, 0, 0, time.UTC), Cards: []deck.Card{decktest.Card(t, cardRef, 1)}}
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
	card, cardRef := f.insertPublishedCard(t, "a", query.CardTypeCreature)
	state := readmodel.Deck{Deck: deckID, Player: playerID, Name: "火", RegisteredAt: registeredAt, Cards: []deck.Card{decktest.Card(t, cardRef, 2)}}
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
