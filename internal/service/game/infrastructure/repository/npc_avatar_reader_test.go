package repository_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/aarondl/sqlboiler/v4/boil"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/orm/sqlboiler/models"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/rdb"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/rdb/rdbtest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/game/application/query"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/game/infrastructure/repository"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/studio/application/query/readmodel"
)

// 公開は studio の read model の行をそのまま写すため、書き手の型で JSON を作り、読み手が同じ形を読めることを確かめる
func insertNPCAvatar(t *testing.T, ctx context.Context, sortName string, metadata readmodel.NPCAvatarMetadata) {
	t.Helper()
	raw, err := json.Marshal(metadata)
	require.NoError(t, err)
	insertNPCAvatarRow(t, ctx, metadata.ID, sortName, string(metadata.Status), raw)
}

func insertNPCAvatarRow(t *testing.T, ctx context.Context, id uuid.UUID, sortName, status string, metadata []byte) {
	t.Helper()
	row := models.NPCAvatar{NPCAvatarID: id.String(), SortName: sortName, Status: status, Metadata: metadata}
	exec, err := rdb.GetExecutor(ctx)
	require.NoError(t, err)
	require.NoError(t, row.Insert(ctx, exec, boil.Infer()))
}

func npcAvatarMetadata(name string, status readmodel.NPCAvatarStatus, decks ...uuid.UUID) readmodel.NPCAvatarMetadata {
	return readmodel.NPCAvatarMetadata{ID: uuid.New(), Name: name, Status: status, Revision: 3, Decks: append([]uuid.UUID{}, decks...)}
}

func insertNPCDeck(t *testing.T, ctx context.Context, name string, status readmodel.NPCDeckStatus, cards ...readmodel.DeckCard) uuid.UUID {
	t.Helper()
	metadata := readmodel.NPCDeckMetadata{ID: uuid.New(), Name: name, Status: status, Revision: 2, Cards: cards}
	raw, err := json.Marshal(metadata)
	require.NoError(t, err)
	row := models.NPCDeck{NPCDeckID: metadata.ID.String(), SortName: name, Status: string(status), Metadata: raw}
	exec, err := rdb.GetExecutor(ctx)
	require.NoError(t, err)
	require.NoError(t, row.Insert(ctx, exec, boil.Infer()))
	return metadata.ID
}

func insertDeckCard(t *testing.T, ctx context.Context, name string, cardType query.CardType) uuid.UUID {
	t.Helper()
	id := uuid.New()
	insertCardRow(t, ctx, id, name, cardType)
	return id
}

// insertCardRow が写すクリーチャーの性能
func defaultCreatureSpec() *query.CreatureSpec {
	return &query.CreatureSpec{Elements: []string{"neutral"}, Attack: 10, HitPoints: 10, ItemRestrictions: []string{}, PlacementRestrictions: []string{}, Abilities: []query.CreatureAbility{}}
}

// insertCardRow が写すアイテムの性能
func defaultItemSpec() *query.ItemSpec {
	return &query.ItemSpec{Category: "weapon", AttackModifier: 30, HitPointsModifier: 0}
}

// insertCardRow が写すスペルの性能（効果なし）
func defaultSpellSpec() *query.SpellSpec {
	return &query.SpellSpec{}
}

func insertCardRow(t *testing.T, ctx context.Context, id uuid.UUID, name string, cardType query.CardType) {
	t.Helper()
	metadata := readmodel.CardMetadata{ID: id, Name: name, Status: readmodel.CardStatusPublished, Cost: 10, Rarity: "normal"}
	if cardType == query.CardTypeCreature {
		metadata.Creature = &readmodel.CreatureMetadata{Elements: []string{"neutral"}, Attack: 10, HitPoints: 10, ItemRestrictions: []string{}, PlacementRestrictions: []string{}, Abilities: []readmodel.CreatureAbilityMetadata{}}
	}
	if cardType == query.CardTypeItem {
		metadata.Item = &readmodel.ItemMetadata{Category: "weapon", AttackModifier: 30}
	}
	if cardType == query.CardTypeSpell {
		metadata.Spell = &readmodel.SpellMetadata{}
	}
	raw, err := json.Marshal(metadata)
	require.NoError(t, err)
	row := models.Card{CardID: id.String(), CardType: int16(cardType), SortName: name, Status: string(readmodel.CardStatusPublished), Metadata: raw}
	exec, err := rdb.GetExecutor(ctx)
	require.NoError(t, err)
	require.NoError(t, row.Insert(ctx, exec, boil.Infer()))
}

// 解放の行を、投影が書く形（metadata の npc_avatars は解放した順の公開 ID）で直接入れる
func insertUnlockedNPCAvatars(t *testing.T, ctx context.Context, playerPublicID uuid.UUID, npcAvatars ...uuid.UUID) {
	t.Helper()
	id := uuid.New()
	metadata, err := json.Marshal(map[string]any{"id": id, "boards": []uuid.UUID{}, "npc_avatars": npcAvatars})
	require.NoError(t, err)
	row := models.Unlock{UnlockID: id.String(), PlayerID: playerPublicID.String(), RegisteredAt: time.Now(), Metadata: metadata}
	exec, err := rdb.GetExecutor(ctx)
	require.NoError(t, err)
	require.NoError(t, row.Insert(ctx, exec, boil.Infer()))
}

// 解放した順に、公開中のものだけを読む。使用不可・準備中・写していないものは飛ばし、解放していないもの（公開中でも）と
// ほかのプレイヤーの解放は混ざらない。デッキの公開 ID は割り当てた順のまま
func TestNPCAvatarReader_ListsOwnUnlockedPublishedAvatarsInUnlockOrderWithDeckIDsInAssignedOrder(t *testing.T) {
	db, _ := rdbtest.NewDatabase(t)
	ctx := rdb.WithDB(t.Context(), db)
	d1, d2 := uuid.New(), uuid.New()
	b := npcAvatarMetadata("B", readmodel.NPCAvatarStatusPublished)
	a := npcAvatarMetadata("A", readmodel.NPCAvatarStatusPublished, d2, d1)
	notUnlocked := npcAvatarMetadata("未解放", readmodel.NPCAvatarStatusPublished)
	retired := npcAvatarMetadata("使用不可", readmodel.NPCAvatarStatusRetired)
	draft := npcAvatarMetadata("準備中", readmodel.NPCAvatarStatusDraft)
	for i, metadata := range []readmodel.NPCAvatarMetadata{a, b, notUnlocked, retired, draft} {
		insertNPCAvatar(t, ctx, string(rune('a'+i)), metadata)
	}
	player, other := uuid.New(), uuid.New()
	insertUnlockedNPCAvatars(t, ctx, player, b.ID, retired.ID, uuid.New(), draft.ID, a.ID)
	insertUnlockedNPCAvatars(t, ctx, other, notUnlocked.ID)

	got, err := repository.NewNPCAvatarReader().ListUnlockedByPlayer(ctx, player)
	gotOther, otherErr := repository.NewNPCAvatarReader().ListUnlockedByPlayer(ctx, other)

	require.NoError(t, err)
	require.Equal(t, []query.NPCAvatarRecord{
		{ID: b.ID, Name: "B", DeckIDs: []uuid.UUID{}},
		{ID: a.ID, Name: "A", DeckIDs: []uuid.UUID{d2, d1}},
	}, got)
	require.NoError(t, otherErr)
	require.Equal(t, []uuid.UUID{notUnlocked.ID}, []uuid.UUID{gotOther[0].ID})
}

func TestNPCAvatarReader_ListsNothingForPlayerWithoutUnlockOrUnlockedAvatars(t *testing.T) {
	db, _ := rdbtest.NewDatabase(t)
	ctx := rdb.WithDB(t.Context(), db)
	insertNPCAvatar(t, ctx, "a", npcAvatarMetadata("A", readmodel.NPCAvatarStatusPublished))
	withoutAvatars := uuid.New()
	insertUnlockedNPCAvatars(t, ctx, withoutAvatars)

	none, noneErr := repository.NewNPCAvatarReader().ListUnlockedByPlayer(ctx, uuid.New())
	empty, emptyErr := repository.NewNPCAvatarReader().ListUnlockedByPlayer(ctx, withoutAvatars)

	require.NoError(t, noneErr)
	require.Empty(t, none)
	require.NoError(t, emptyErr)
	require.Empty(t, empty)
}

// decks が無い行（デッキを持たせる前に写した行）を、デッキが無いものとして返さず、読み出しを失敗させる
func TestNPCAvatarReader_ListFailsWhenDecksIsMissing(t *testing.T) {
	db, _ := rdbtest.NewDatabase(t)
	ctx := rdb.WithDB(t.Context(), db)
	id, player := uuid.New(), uuid.New()
	insertNPCAvatarRow(t, ctx, id, "a", "published", []byte(`{"name":"A"}`))
	insertUnlockedNPCAvatars(t, ctx, player, id)

	_, err := repository.NewNPCAvatarReader().ListUnlockedByPlayer(ctx, player)

	require.ErrorContains(t, err, "decks is missing")
}

func TestNPCAvatarReader_ListDecksReturnsOnlyPublishedRequestedDecksWithCardsInTypeThenNameOrder(t *testing.T) {
	db, _ := rdbtest.NewDatabase(t)
	ctx := rdb.WithDB(t.Context(), db)
	spell := insertDeckCard(t, ctx, "a", query.CardTypeSpell)
	itemB := insertDeckCard(t, ctx, "b", query.CardTypeItem)
	itemA := insertDeckCard(t, ctx, "a", query.CardTypeItem)
	creature := insertDeckCard(t, ctx, "z", query.CardTypeCreature)
	published := insertNPCDeck(t, ctx, "公開中", readmodel.NPCDeckStatusPublished,
		readmodel.DeckCard{CardID: spell, Count: 1}, readmodel.DeckCard{CardID: itemB, Count: 2},
		readmodel.DeckCard{CardID: creature, Count: 3}, readmodel.DeckCard{CardID: itemA, Count: 4})
	draft := insertNPCDeck(t, ctx, "準備中", readmodel.NPCDeckStatusDraft, readmodel.DeckCard{CardID: creature, Count: 1})
	retired := insertNPCDeck(t, ctx, "使用不可", readmodel.NPCDeckStatusRetired, readmodel.DeckCard{CardID: creature, Count: 1})
	insertNPCDeck(t, ctx, "頼まれていない", readmodel.NPCDeckStatusPublished, readmodel.DeckCard{CardID: creature, Count: 1})

	got, err := repository.NewNPCAvatarReader().ListPublishedDecks(ctx, []uuid.UUID{published, draft, retired, uuid.New()})

	require.NoError(t, err)
	require.Equal(t, []query.NPCDeck{{
		ID: published, Name: "公開中",
		Cards: []query.DeckCard{
			{CardID: creature, Name: "z", Type: query.CardTypeCreature, Cost: 10, Rarity: "normal", Creature: defaultCreatureSpec(), Count: 3},
			{CardID: itemA, Name: "a", Type: query.CardTypeItem, Cost: 10, Rarity: "normal", Item: defaultItemSpec(), Count: 4},
			{CardID: itemB, Name: "b", Type: query.CardTypeItem, Cost: 10, Rarity: "normal", Item: defaultItemSpec(), Count: 2},
			{CardID: spell, Name: "a", Type: query.CardTypeSpell, Cost: 10, Rarity: "normal", Spell: defaultSpellSpec(), Count: 1},
		},
	}}, got)
}

func TestNPCAvatarReader_ListDecksReturnsNothingForNoIDs(t *testing.T) {
	db, _ := rdbtest.NewDatabase(t)
	ctx := rdb.WithDB(t.Context(), db)

	got, err := repository.NewNPCAvatarReader().ListPublishedDecks(ctx, nil)

	require.NoError(t, err)
	require.Empty(t, got)
}

// 写していないカードを黙って捨てず、読み出しを失敗させる（枚数の合計が実際と食い違って見えるより、気づけるようにする）
func TestNPCAvatarReader_ListDecksFailsForCardThatWasNotPublishedToGame(t *testing.T) {
	db, _ := rdbtest.NewDatabase(t)
	ctx := rdb.WithDB(t.Context(), db)
	deck := insertNPCDeck(t, ctx, "公開中", readmodel.NPCDeckStatusPublished, readmodel.DeckCard{CardID: uuid.New(), Count: 1})

	_, err := repository.NewNPCAvatarReader().ListPublishedDecks(ctx, []uuid.UUID{deck})

	require.ErrorContains(t, err, "is not published to game")
}

// 性能を足す前に投影した行は、0 や空にせず読み出しの誤りにする（投影を作り直して写し直せば入る）
func TestNPCAvatarReader_ListDecksRejectsCardsWithoutSpec(t *testing.T) {
	db, _ := rdbtest.NewDatabase(t)
	ctx := rdb.WithDB(t.Context(), db)
	id := uuid.New()
	raw, err := json.Marshal(map[string]any{"id": id, "name": "古い行", "status": "published"})
	require.NoError(t, err)
	row := models.Card{CardID: id.String(), CardType: int16(query.CardTypeCreature), SortName: "古い行", Status: string(readmodel.CardStatusPublished), Metadata: raw}
	exec, err := rdb.GetExecutor(ctx)
	require.NoError(t, err)
	require.NoError(t, row.Insert(ctx, exec, boil.Infer()))
	deck := insertNPCDeck(t, ctx, "古いカードのデッキ", readmodel.NPCDeckStatusPublished, readmodel.DeckCard{CardID: id, Count: 1})

	_, err = repository.NewNPCAvatarReader().ListPublishedDecks(ctx, []uuid.UUID{deck})

	require.Error(t, err)
}
