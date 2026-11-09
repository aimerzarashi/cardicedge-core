package repository_test

import (
	"encoding/json"
	"maps"
	"testing"
	"time"

	"github.com/aarondl/sqlboiler/v4/boil"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/card"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/collection"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/identifier"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/orm/sqlboiler/models"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/rdb"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/game/application/query"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/game/application/query/readmodel"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/game/infrastructure/repository"
)

func (f playerFixture) issueCollection(t *testing.T) collection.ID {
	t.Helper()
	id, _, err := identifier.New(f.registry, collection.NewID).Issue(f.ctx)
	require.NoError(t, err)
	return id
}

type collectionCards struct {
	creatureZ, itemA, itemB, spellA uuid.UUID
	refs                            map[uuid.UUID]card.Ref
}

// 種類をまたいで並びを確かめられる 4 枚（Creature の z、Item の a・b、Spell の a）を写す
func (f playerFixture) insertCollectionCards(t *testing.T) collectionCards {
	t.Helper()
	var cards collectionCards
	cards.refs = map[uuid.UUID]card.Ref{}
	insert := func(name string, cardType query.CardType) uuid.UUID {
		id, ref := f.insertPublishedCard(t, name, cardType)
		cards.refs[id] = ref
		return id
	}
	cards.spellA = insert("a", query.CardTypeSpell)
	cards.itemB = insert("b", query.CardTypeItem)
	cards.itemA = insert("a", query.CardTypeItem)
	cards.creatureZ = insert("z", query.CardTypeCreature)
	return cards
}

func (c collectionCards) owned(id uuid.UUID, count int) readmodel.CollectionCard {
	return readmodel.CollectionCard{Card: c.refs[id], Count: count}
}

func writeCollectionOf(t *testing.T, f playerFixture, state readmodel.Collection) {
	t.Helper()
	require.NoError(t, repository.NewCollectionWriter(identifier.NewEntityRefResolver(f.registry)).Upsert(f.ctx, state))
}

func TestCollectionReader_ListsOwnCardsInTypeThenNameOrderWithCounts(t *testing.T) {
	f := newPlayerFixture(t)
	owner, ownerPublicID := f.issuePlayer(t)
	other, otherPublicID := f.issuePlayer(t)
	cards := f.insertCollectionCards(t)
	at := time.Date(2026, 11, 3, 9, 0, 0, 0, time.UTC)
	// 持った順と並びが違っても、種類 → 名前 → 公開 ID の順に読む
	writeCollectionOf(t, f, readmodel.Collection{Collection: f.issueCollection(t), Player: owner, RegisteredAt: at, UpdatedAt: at, Cards: []readmodel.CollectionCard{
		cards.owned(cards.spellA, 1), cards.owned(cards.itemB, 2), cards.owned(cards.creatureZ, 3), cards.owned(cards.itemA, 4),
	}})
	writeCollectionOf(t, f, readmodel.Collection{Collection: f.issueCollection(t), Player: other, RegisteredAt: at, UpdatedAt: at, Cards: []readmodel.CollectionCard{cards.owned(cards.creatureZ, 9)}})

	got, err := repository.NewCollectionReader().List(f.ctx, ownerPublicID, query.ListCollectionCondition{Limit: 10})
	require.NoError(t, err)
	gotOther, err := repository.NewCollectionReader().List(f.ctx, otherPublicID, query.ListCollectionCondition{Limit: 10})
	require.NoError(t, err)

	require.Equal(t, []query.CollectionListRow{
		expectedCollectionRow(cards.creatureZ, "z", query.CardTypeCreature, 3),
		expectedCollectionRow(cards.itemA, "a", query.CardTypeItem, 4),
		expectedCollectionRow(cards.itemB, "b", query.CardTypeItem, 2),
		expectedCollectionRow(cards.spellA, "a", query.CardTypeSpell, 1),
	}, got)
	require.Equal(t, []query.CollectionListRow{expectedCollectionRow(cards.creatureZ, "z", query.CardTypeCreature, 9)}, gotOther)
}

// 項目に入れる値は生成済みの変数で渡す（factory_enforcer）ため、行は変数を経由して作る。性能は写したカードの既定（コスト 10・希少度 normal）
func expectedCollectionRow(id uuid.UUID, name string, cardType query.CardType, count int) query.CollectionListRow {
	card := query.Card{ID: id, Name: name, Type: cardType, Cost: 10, Rarity: "normal"}
	if cardType == query.CardTypeCreature {
		card.Creature = defaultCreatureSpec()
	}
	if cardType == query.CardTypeItem {
		card.Item = defaultItemSpec()
	}
	owned := query.CollectionCard{Card: card, Count: count}
	cursor := query.CollectionListCursor{CardType: cardType, SortName: name, CardID: id}
	return query.CollectionListRow{Card: owned, Cursor: cursor}
}

func TestCollectionReader_FiltersByCardTypes(t *testing.T) {
	f := newPlayerFixture(t)
	owner, ownerPublicID := f.issuePlayer(t)
	cards := f.insertCollectionCards(t)
	writeCollectionOf(t, f, readmodel.Collection{Collection: f.issueCollection(t), Player: owner, RegisteredAt: time.Now(), UpdatedAt: time.Now(), Cards: []readmodel.CollectionCard{
		cards.owned(cards.spellA, 1), cards.owned(cards.itemB, 2), cards.owned(cards.creatureZ, 3), cards.owned(cards.itemA, 4),
	}})

	got, err := repository.NewCollectionReader().List(f.ctx, ownerPublicID, query.ListCollectionCondition{Types: []query.CardType{query.CardTypeSpell, query.CardTypeCreature}, Limit: 10})

	require.NoError(t, err)
	ids := make([]uuid.UUID, len(got))
	for i, row := range got {
		ids[i] = row.Card.Card.ID
	}
	require.Equal(t, []uuid.UUID{cards.creatureZ, cards.spellA}, ids)
}

// 続きは前のページの最後の位置から、重ならず抜けずに取れる
func TestCollectionReader_PagesByKeysetWithoutOverlap(t *testing.T) {
	f := newPlayerFixture(t)
	owner, ownerPublicID := f.issuePlayer(t)
	cards := f.insertCollectionCards(t)
	writeCollectionOf(t, f, readmodel.Collection{Collection: f.issueCollection(t), Player: owner, RegisteredAt: time.Now(), UpdatedAt: time.Now(), Cards: []readmodel.CollectionCard{
		cards.owned(cards.spellA, 1), cards.owned(cards.itemB, 2), cards.owned(cards.creatureZ, 3), cards.owned(cards.itemA, 4),
	}})
	reader := repository.NewCollectionReader()
	want := []uuid.UUID{cards.creatureZ, cards.itemA, cards.itemB, cards.spellA}

	var got []uuid.UUID
	var after *query.CollectionListCursor
	for range 4 {
		rows, err := reader.List(f.ctx, ownerPublicID, query.ListCollectionCondition{After: after, Limit: 2})
		require.NoError(t, err)
		if len(rows) == 0 {
			break
		}
		got = append(got, rows[0].Card.Card.ID)
		if len(rows) > 1 {
			got = append(got, rows[1].Card.Card.ID)
		}
		cursor := rows[min(1, len(rows)-1)].Cursor
		after = &cursor
	}

	require.Equal(t, want, got)
}

func TestCollectionReader_ListsNothingForPlayerWithoutCollection(t *testing.T) {
	f := newPlayerFixture(t)
	_, publicID := f.issuePlayer(t)

	got, err := repository.NewCollectionReader().List(f.ctx, publicID, query.ListCollectionCondition{Limit: 10})

	require.NoError(t, err)
	require.Empty(t, got)
}

// 写していないカードを黙って捨てず、読み出しを失敗させる（デッキと同じ）。種類の絞り込みの外でも失敗する
func TestCollectionReader_FailsForCardThatWasNotPublishedToGame(t *testing.T) {
	f := newPlayerFixture(t)
	owner, publicID := f.issuePlayer(t)
	cards := f.insertCollectionCards(t)
	_, unpublished := f.issueCard(t, query.CardTypeCreature)
	writeCollectionOf(t, f, readmodel.Collection{Collection: f.issueCollection(t), Player: owner, RegisteredAt: time.Now(), UpdatedAt: time.Now(), Cards: []readmodel.CollectionCard{
		cards.owned(cards.spellA, 1), {Card: unpublished, Count: 1},
	}})

	_, err := repository.NewCollectionReader().List(f.ctx, publicID, query.ListCollectionCondition{Types: []query.CardType{query.CardTypeSpell}, Limit: 10})

	require.ErrorContains(t, err, "is not published to game")
}

// 性能を足す前に写した行を、0 や空にせず読み出しの誤りにする
func TestCollectionReader_FailsForCardWithoutCostOrRarity(t *testing.T) {
	f := newPlayerFixture(t)
	owner, publicID := f.issuePlayer(t)
	cardID, ref := f.issueCard(t, query.CardTypeCreature)
	raw, err := json.Marshal(map[string]any{"id": cardID, "name": "古い行", "status": "published"})
	require.NoError(t, err)
	exec, err := rdb.GetExecutor(f.ctx)
	require.NoError(t, err)
	row := models.Card{CardID: cardID.String(), CardType: int16(query.CardTypeCreature), SortName: "古い行", Status: "published", Metadata: raw}
	require.NoError(t, row.Insert(f.ctx, exec, boil.Infer()))
	writeCollectionOf(t, f, readmodel.Collection{Collection: f.issueCollection(t), Player: owner, RegisteredAt: time.Now(), UpdatedAt: time.Now(), Cards: []readmodel.CollectionCard{{Card: ref, Count: 1}}})

	_, err = repository.NewCollectionReader().List(f.ctx, publicID, query.ListCollectionCondition{Limit: 10})

	require.ErrorContains(t, err, "has no cost or rarity")
}

func TestCollectionWriter_UpsertTwiceKeepsOneRowWithLatestCards(t *testing.T) {
	f := newPlayerFixture(t)
	owner, ownerPublicID := f.issuePlayer(t)
	cards := f.insertCollectionCards(t)
	state := readmodel.Collection{Collection: f.issueCollection(t), Player: owner, RegisteredAt: time.Now(), UpdatedAt: time.Now(), Cards: []readmodel.CollectionCard{cards.owned(cards.itemA, 1)}}
	writeCollectionOf(t, f, state)
	state.Cards = []readmodel.CollectionCard{cards.owned(cards.itemA, 3)}
	writeCollectionOf(t, f, state)

	got, err := repository.NewCollectionReader().List(f.ctx, ownerPublicID, query.ListCollectionCondition{Limit: 10})

	require.NoError(t, err)
	require.Len(t, got, 1)
	require.Equal(t, 3, got[0].Card.Count)
}

func TestCollectionWriter_DeleteRemovesRowAndToleratesMissing(t *testing.T) {
	f := newPlayerFixture(t)
	owner, ownerPublicID := f.issuePlayer(t)
	cards := f.insertCollectionCards(t)
	state := readmodel.Collection{Collection: f.issueCollection(t), Player: owner, RegisteredAt: time.Now(), UpdatedAt: time.Now(), Cards: []readmodel.CollectionCard{cards.owned(cards.itemA, 1)}}
	writeCollectionOf(t, f, state)
	writer := repository.NewCollectionWriter(identifier.NewEntityRefResolver(f.registry))

	require.NoError(t, writer.Delete(f.ctx, state))
	require.NoError(t, writer.Delete(f.ctx, state))
	got, err := repository.NewCollectionReader().List(f.ctx, ownerPublicID, query.ListCollectionCondition{Limit: 10})

	require.NoError(t, err)
	require.Empty(t, got)
}

// 所持品は持ち主のプレイヤーのものとして記録し、登録も更新も持ち主の公開 ID になる。日時はイベントの発生時刻
func TestCollectionWriter_RecordsOwnerPublicIDAndTimesInMetadata(t *testing.T) {
	f := newPlayerFixture(t)
	owner, ownerPublicID := f.issuePlayer(t)
	collectionID := f.issueCollection(t)
	cards := f.insertCollectionCards(t)
	registeredAt := time.Date(2026, 11, 3, 9, 0, 0, 0, time.UTC)
	updatedAt := registeredAt.Add(time.Hour)
	writeCollectionOf(t, f, readmodel.Collection{Collection: collectionID, Player: owner, RegisteredAt: registeredAt, UpdatedAt: updatedAt, Cards: []readmodel.CollectionCard{cards.owned(cards.itemA, 2)}})

	exec, err := rdb.GetExecutor(f.ctx)
	require.NoError(t, err)
	row, err := models.Collections(models.CollectionWhere.PlayerID.EQ(ownerPublicID.String())).One(f.ctx, exec)
	require.NoError(t, err)
	var metadata readmodel.CollectionMetadata
	require.NoError(t, json.Unmarshal(row.Metadata, &metadata))

	require.Equal(t, row.CollectionID, metadata.ID.String())
	require.Equal(t, []readmodel.CollectionCardMetadata{{CardID: cards.itemA, Count: 2}}, metadata.Cards)
	require.Equal(t, ownerPublicID, metadata.RegisteredBy)
	require.Equal(t, ownerPublicID, metadata.UpdatedBy)
	require.Equal(t, registeredAt, metadata.RegisteredAt.UTC())
	require.Equal(t, updatedAt, metadata.UpdatedAt.UTC())
}

func TestCardReader_GetReturnsPublishedAndRetiredCardsButNotMissing(t *testing.T) {
	f := newPlayerFixture(t)
	published, _ := f.insertPublishedCard(t, "公開中", query.CardTypeItem)
	retired := uuid.New()
	raw, err := json.Marshal(map[string]any{"id": retired, "name": "使用不可", "status": "retired", "cost": 7, "rarity": "rare"})
	require.NoError(t, err)
	exec, err := rdb.GetExecutor(f.ctx)
	require.NoError(t, err)
	row := models.Card{CardID: retired.String(), CardType: int16(query.CardTypeSpell), SortName: "使用不可", Status: "retired", Metadata: raw}
	require.NoError(t, row.Insert(f.ctx, exec, boil.Infer()))
	reader := repository.NewCardReader()

	gotPublished, err := reader.Get(f.ctx, published)
	require.NoError(t, err)
	gotRetired, err := reader.Get(f.ctx, retired)
	require.NoError(t, err)
	_, errMissing := reader.Get(f.ctx, uuid.New())

	require.Equal(t, query.Card{ID: published, Name: "公開中", Type: query.CardTypeItem, Cost: 10, Rarity: "normal", Item: defaultItemSpec()}, gotPublished)
	require.Equal(t, query.Card{ID: retired, Name: "使用不可", Type: query.CardTypeSpell, Cost: 7, Rarity: "rare"}, gotRetired)
	require.ErrorIs(t, errMissing, query.ErrCardNotFound)
}

func TestCardReader_GetFailsForCardWithoutCostOrRarity(t *testing.T) {
	f := newPlayerFixture(t)
	id := uuid.New()
	raw, err := json.Marshal(map[string]any{"id": id, "name": "古い行", "status": "published"})
	require.NoError(t, err)
	exec, err := rdb.GetExecutor(f.ctx)
	require.NoError(t, err)
	row := models.Card{CardID: id.String(), CardType: int16(query.CardTypeCreature), SortName: "古い行", Status: "published", Metadata: raw}
	require.NoError(t, row.Insert(f.ctx, exec, boil.Infer()))

	_, err = repository.NewCardReader().Get(f.ctx, id)

	require.ErrorContains(t, err, "has no cost or rarity")
	require.NotErrorIs(t, err, query.ErrCardNotFound)
}

// クリーチャーの性能（属性・攻撃力・体力）が無い行は、0 や空にせず読み出しの誤りにする
func TestCardReader_GetFailsForCreatureWithoutCreatureSpec(t *testing.T) {
	tests := []struct {
		name     string
		metadata map[string]any
	}{
		{name: "no_creature_object", metadata: map[string]any{}},
		{name: "no_elements", metadata: map[string]any{"creature": map[string]any{"attack": 10, "hit_points": 10}}},
		{name: "no_attack", metadata: map[string]any{"creature": map[string]any{"elements": []string{"fire"}, "hit_points": 10}}},
		{name: "no_hit_points", metadata: map[string]any{"creature": map[string]any{"elements": []string{"fire"}, "attack": 10}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newPlayerFixture(t)
			id := uuid.New()
			metadata := map[string]any{"id": id, "name": "古い行", "status": "published", "cost": 10, "rarity": "normal"}
			maps.Copy(metadata, tt.metadata)
			raw, err := json.Marshal(metadata)
			require.NoError(t, err)
			exec, err := rdb.GetExecutor(f.ctx)
			require.NoError(t, err)
			row := models.Card{CardID: id.String(), CardType: int16(query.CardTypeCreature), SortName: "古い行", Status: "published", Metadata: raw}
			require.NoError(t, row.Insert(f.ctx, exec, boil.Infer()))

			_, err = repository.NewCardReader().Get(f.ctx, id)

			require.ErrorContains(t, err, "has no creature elements, attack or hit_points")
		})
	}
}
