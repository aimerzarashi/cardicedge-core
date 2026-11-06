package query_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/game/application/query"
)

// 決めた行を返す読み出しの関数
type fakeNPCAvatarReader struct {
	records []query.NPCAvatarRecord
	decks   []query.NPCDeck
	// デッキの読み出しに渡された ID の一覧の履歴
	deckCalls [][]uuid.UUID
}

func (r *fakeNPCAvatarReader) listUnlocked(context.Context, uuid.UUID) ([]query.NPCAvatarRecord, error) {
	return r.records, nil
}

func (r *fakeNPCAvatarReader) listDecks(_ context.Context, ids []uuid.UUID) ([]query.NPCDeck, error) {
	r.deckCalls = append(r.deckCalls, ids)
	return r.decks, nil
}

func npcAvatarRecord(name string, deckIDs ...uuid.UUID) query.NPCAvatarRecord {
	return query.NPCAvatarRecord{ID: uuid.New(), Name: name, DeckIDs: deckIDs}
}

func TestNPCAvatarHandler_ListUnlockedReadsDecksOfAllAvatarsOnce(t *testing.T) {
	d1, d2 := uuid.New(), uuid.New()
	reader := &fakeNPCAvatarReader{records: []query.NPCAvatarRecord{npcAvatarRecord("A", d1, d2), npcAvatarRecord("B", d1)}}

	_, err := query.NewNPCAvatarHandler(reader.listUnlocked, reader.listDecks).ListUnlockedByPlayer(t.Context(), uuid.New())

	require.NoError(t, err)
	require.Equal(t, [][]uuid.UUID{{d1, d2}}, reader.deckCalls)
}

func TestNPCAvatarHandler_ListUnlockedReturnsEmptyListWithoutReadingDecksWhenNothingIsUnlocked(t *testing.T) {
	reader := &fakeNPCAvatarReader{}

	got, err := query.NewNPCAvatarHandler(reader.listUnlocked, reader.listDecks).ListUnlockedByPlayer(t.Context(), uuid.New())

	require.NoError(t, err)
	require.Equal(t, []query.NPCAvatar{}, got)
	require.Empty(t, reader.deckCalls)
}

func TestNPCAvatarHandler_ListUnlockedSkipsDeckReadWhenNoDeckIsAssigned(t *testing.T) {
	reader := &fakeNPCAvatarReader{records: []query.NPCAvatarRecord{npcAvatarRecord("A")}}

	got, err := query.NewNPCAvatarHandler(reader.listUnlocked, reader.listDecks).ListUnlockedByPlayer(t.Context(), uuid.New())

	require.NoError(t, err)
	require.Empty(t, reader.deckCalls)
	require.Empty(t, got[0].Decks)
}

// アバターは読み出しの順（解放した順）のまま。読み出しは公開中のデッキだけを順不同で返す。アバターごとに割り当てた順に並べ、
// 公開中でない（読み出しに無い）デッキは飛ばす
func TestNPCAvatarHandler_ListUnlockedKeepsUnlockOrderAndOrdersDecksByAssignmentSkippingUnpublished(t *testing.T) {
	d1, d2, unpublished := uuid.New(), uuid.New(), uuid.New()
	second, first := npcAvatarRecord("B", d1), npcAvatarRecord("A", d2, unpublished, d1)
	reader := &fakeNPCAvatarReader{records: []query.NPCAvatarRecord{second, first}, decks: []query.NPCDeck{{ID: d1, Name: "1"}, {ID: d2, Name: "2"}}}

	got, err := query.NewNPCAvatarHandler(reader.listUnlocked, reader.listDecks).ListUnlockedByPlayer(t.Context(), uuid.New())

	require.NoError(t, err)
	require.Equal(t, []query.NPCAvatar{
		{ID: second.ID, Name: "B", Decks: []query.NPCDeck{{ID: d1, Name: "1"}}},
		{ID: first.ID, Name: "A", Decks: []query.NPCDeck{{ID: d2, Name: "2"}, {ID: d1, Name: "1"}}},
	}, got)
}

func TestNPCAvatarHandler_GetUnlockedReturnsOnlyUnlockedAvatar(t *testing.T) {
	unlocked := npcAvatarRecord("A")
	reader := &fakeNPCAvatarReader{records: []query.NPCAvatarRecord{unlocked}}
	handler := query.NewNPCAvatarHandler(reader.listUnlocked, reader.listDecks)

	got, err := handler.GetUnlocked(t.Context(), uuid.New(), unlocked.ID)
	_, notUnlocked := handler.GetUnlocked(t.Context(), uuid.New(), uuid.New())

	require.NoError(t, err)
	require.Equal(t, query.NPCAvatar{ID: unlocked.ID, Name: "A", Decks: []query.NPCDeck{}}, got)
	require.ErrorIs(t, notUnlocked, query.ErrNPCAvatarNotFound)
}

func TestNPCDeck_TotalCountSumsCardCounts(t *testing.T) {
	deck := query.NPCDeck{Cards: []query.DeckCard{{Count: 3}, {Count: 2}}}

	require.Equal(t, 5, deck.TotalCount())
}
