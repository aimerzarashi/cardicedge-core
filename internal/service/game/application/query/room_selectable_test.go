package query_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/game/application/query"
)

// 画面の選択肢を出すハンドラー（Player.avatars・decks・npcAvatars・boards の query）に、決めた値を返す読み出しの関数を渡して作る
func selectableOf(avatars []query.Avatar, decks []query.Deck, npcAvatars []query.NPCAvatar, boards []query.Board) *query.RoomSelectable {
	return query.NewRoomSelectable(
		query.NewAvatarHandler(func(context.Context, uuid.UUID) ([]query.Avatar, error) { return avatars, nil }),
		query.NewDeckHandler(nil, func(_ context.Context, _ uuid.UUID, deckID uuid.UUID) (query.Deck, error) {
			for _, deck := range decks {
				if deck.ID == deckID {
					return deck, nil
				}
			}
			return query.Deck{}, query.ErrDeckNotFound
		}),
		query.NewNPCAvatarHandler(
			func(context.Context, uuid.UUID) ([]query.NPCAvatarRecord, error) {
				records := []query.NPCAvatarRecord{}
				for _, avatar := range npcAvatars {
					record := query.NPCAvatarRecord{ID: avatar.ID, Name: avatar.Name}
					for _, deck := range avatar.Decks {
						record.DeckIDs = append(record.DeckIDs, deck.ID)
					}
					for _, mindset := range avatar.Mindsets {
						record.MindsetIDs = append(record.MindsetIDs, mindset.ID)
					}
					records = append(records, record)
				}
				return records, nil
			},
			func(_ context.Context, ids []uuid.UUID) ([]query.NPCDeck, error) {
				found := []query.NPCDeck{}
				for _, avatar := range npcAvatars {
					for _, deck := range avatar.Decks {
						for _, id := range ids {
							if deck.ID == id {
								found = append(found, deck)
							}
						}
					}
				}
				return found, nil
			},
			func(_ context.Context, ids []uuid.UUID) ([]query.NPCMindset, error) {
				found := []query.NPCMindset{}
				for _, avatar := range npcAvatars {
					for _, mindset := range avatar.Mindsets {
						for _, id := range ids {
							if mindset.ID == id {
								found = append(found, mindset)
							}
						}
					}
				}
				return found, nil
			},
		),
		query.NewUnlockHandler(func(context.Context, uuid.UUID) ([]query.Board, error) { return boards, nil }),
	)
}

func TestRoomSelectable_AcceptsOnlyWhatTheSelectionQueriesShow(t *testing.T) {
	avatar := query.Avatar{ID: uuid.New(), Name: "アリス"}
	deck := query.Deck{ID: uuid.New(), Name: "火"}
	npcDeck := query.NPCDeck{ID: uuid.New(), Name: "ゴリガンのデッキ"}
	npcMindset := query.NPCMindset{ID: uuid.New(), Name: "攻めがち"}
	npcAvatar := query.NPCAvatar{ID: uuid.New(), Name: "ゴリガン", Decks: []query.NPCDeck{npcDeck}, Mindsets: []query.NPCMindset{npcMindset}}
	board := query.Board{ID: uuid.New(), Name: "チュートリアル"}
	selectable := selectableOf([]query.Avatar{avatar}, []query.Deck{deck}, []query.NPCAvatar{npcAvatar}, []query.Board{board})
	playerID := uuid.New()
	unknown := uuid.New()
	tests := []struct {
		name string
		call func() (bool, error)
		want bool
	}{
		{name: "own_avatar", call: func() (bool, error) { return selectable.HasAvatar(t.Context(), playerID, avatar.ID) }, want: true},
		{name: "unknown_avatar", call: func() (bool, error) { return selectable.HasAvatar(t.Context(), playerID, unknown) }},
		{name: "own_deck", call: func() (bool, error) { return selectable.HasDeck(t.Context(), playerID, deck.ID) }, want: true},
		{name: "unknown_deck", call: func() (bool, error) { return selectable.HasDeck(t.Context(), playerID, unknown) }},
		{name: "unlocked_npc_avatar", call: func() (bool, error) { return selectable.HasNPCAvatar(t.Context(), playerID, npcAvatar.ID) }, want: true},
		{name: "unknown_npc_avatar", call: func() (bool, error) { return selectable.HasNPCAvatar(t.Context(), playerID, unknown) }},
		{name: "assigned_npc_deck", call: func() (bool, error) { return selectable.HasNPCDeck(t.Context(), playerID, npcAvatar.ID, npcDeck.ID) }, want: true},
		{name: "npc_deck_of_unknown_avatar", call: func() (bool, error) { return selectable.HasNPCDeck(t.Context(), playerID, unknown, npcDeck.ID) }},
		{name: "own_deck_as_npc_deck", call: func() (bool, error) { return selectable.HasNPCDeck(t.Context(), playerID, npcAvatar.ID, deck.ID) }},
		{name: "assigned_npc_mindset", call: func() (bool, error) {
			return selectable.HasNPCMindset(t.Context(), playerID, npcAvatar.ID, npcMindset.ID)
		}, want: true},
		{name: "npc_mindset_of_unknown_avatar", call: func() (bool, error) { return selectable.HasNPCMindset(t.Context(), playerID, unknown, npcMindset.ID) }},
		{name: "unknown_npc_mindset", call: func() (bool, error) { return selectable.HasNPCMindset(t.Context(), playerID, npcAvatar.ID, unknown) }},
		{name: "unlocked_board", call: func() (bool, error) { return selectable.HasBoard(t.Context(), playerID, board.ID) }, want: true},
		{name: "unknown_board", call: func() (bool, error) { return selectable.HasBoard(t.Context(), playerID, unknown) }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.call()

			require.NoError(t, err)
			require.Equal(t, tt.want, got)
		})
	}
}
