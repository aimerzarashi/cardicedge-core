package main

import (
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/board"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/npcavatar"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/starterset"
)

func TestSampleMaster_IsValid(t *testing.T) {
	_, err := loadMaster(sampleMasterJSON)

	require.NoError(t, err)
}

// すべての見本のボードが、公開の条件を満たす（始める前に確かめる流れが、見本で必ず通るように）
func TestSampleMaster_BoardsArePublishable(t *testing.T) {
	m, err := loadMaster(sampleMasterJSON)
	require.NoError(t, err)
	require.NotEmpty(t, m.Boards)

	for _, b := range m.Boards {
		layout, err := b.layout()
		require.NoError(t, err)
		require.Empty(t, layout.Readiness().Problems())
	}
}

// 8×8・チェックポイント 2 以上・分かれ道（3 方向以上につながるマス）が、見本のどこかのボードにある（開発の画面で、それぞれを見て確かめられるように）
func TestSampleMaster_HasLargeBoardWithCheckpointsAndFork(t *testing.T) {
	m, err := loadMaster(sampleMasterJSON)
	require.NoError(t, err)

	found := false
	for _, b := range m.Boards {
		layout, err := b.layout()
		require.NoError(t, err)
		degrees := map[masterBoardPosition]int{}
		for _, c := range b.Connections {
			degrees[c.A]++
			degrees[c.B]++
		}
		fork := false
		for _, degree := range degrees {
			fork = fork || degree >= 3
		}
		found = found || (b.Width == 8 && b.Height == 8 && layout.Readiness().CheckpointCount >= 2 && fork)
	}
	require.True(t, found)
}

// 登録の直後の既定の値と違う値のボードが、見本のどこかにある（開発の画面で、既定のままか変えた値かを見分けられ、変える操作も流れるように）
func TestSampleMaster_HasDefaultRulesDifferentFromInitialOnes(t *testing.T) {
	m, err := loadMaster(sampleMasterJSON)
	require.NoError(t, err)

	initial := board.InitialDefaultRules()
	found := false
	for _, b := range m.Boards {
		found = found || b.DefaultRules.TargetMagic != initial.TargetMagic ||
			b.DefaultRules.StartingMagic != initial.StartingMagic ||
			b.DefaultRules.LapBonus != initial.LapBonus ||
			b.DefaultRules.CheckpointBonus != initial.CheckpointBonus ||
			b.DefaultRules.RoundLimit != initial.RoundLimit ||
			b.DefaultRules.DiceMin != initial.DiceMin ||
			b.DefaultRules.DiceMax != initial.DiceMax
	}
	require.True(t, found)
}

// 6 つの属性が見本のどこかの土地に出てくる（studio・game の画面で、どの属性も見て確かめられるように）
func TestSampleMaster_HasEveryLandElement(t *testing.T) {
	m, err := loadMaster(sampleMasterJSON)
	require.NoError(t, err)

	elements := map[string]bool{}
	for _, b := range m.Boards {
		for _, tile := range b.Tiles {
			elements[tile.Element] = true
		}
	}

	for _, element := range []string{"fire", "earth", "water", "wind", "neutral", "multi"} {
		require.True(t, elements[element])
	}
}

// 転送円の対 1 つ・橋の対 1 つ・施設 4 つのボードが、見本のどこかにある（studio・game の画面で、どの特殊なマスも見て確かめられるように）
func TestSampleMaster_HasEverySpecialTile(t *testing.T) {
	m, err := loadMaster(sampleMasterJSON)
	require.NoError(t, err)

	found := false
	for _, b := range m.Boards {
		kinds := map[string]int{}
		for _, tile := range b.Tiles {
			kinds[tile.Kind]++
		}
		all := kinds["warp"] == 2 && kinds["bridge"] == 2 && len(b.Pairs) == 2
		for _, facility := range []string{"sanctuary", "shrine", "prophecy_shop", "card_shop"} {
			all = all && kinds[facility] == 1
		}
		found = found || all
	}
	require.True(t, found)
}

// 見本のファイルに NPC のアバターが 4 つあり（ゴリガンは、スターターセットが解放する）、どれも名前の決まりを満たし、見本の NPC のデッキを使う（始める前に確かめる流れが、見本で必ず通るように）
func TestSampleMaster_HasFourNPCAvatarsUsingNPCDecks(t *testing.T) {
	m, err := loadMaster(sampleMasterJSON)
	require.NoError(t, err)

	require.Equal(t, []masterNPCAvatar{
		{Name: "見習いの魔術師", Decks: []string{"水・風属性デッキ"}},
		{Name: "森の番人", Decks: []string{"火・地属性デッキ"}},
		{Name: "炎の剣士", Decks: []string{"火・地属性デッキ"}},
		{Name: "ゴリガン", Decks: []string{"火・地属性デッキ", "水・風属性デッキ"}},
	}, m.NPCAvatars)
}

func TestLoadMaster_RejectsInvalidNPCAvatars(t *testing.T) {
	fireDeck := []string{"火・地属性デッキ"}
	tests := []struct {
		name       string
		npcAvatars []masterNPCAvatar
	}{
		{name: "blank_name", npcAvatars: []masterNPCAvatar{{Name: " ", Decks: fireDeck}}},
		{name: "too_long_name", npcAvatars: []masterNPCAvatar{{Name: strings.Repeat("あ", npcavatar.MaxNameLength+1), Decks: fireDeck}}},
		{name: "duplicated_name", npcAvatars: []masterNPCAvatar{{Name: "森の番人", Decks: fireDeck}, {Name: "森の番人", Decks: fireDeck}}},
		{name: "duplicated_after_normalization", npcAvatars: []masterNPCAvatar{{Name: "ＡＢＣ", Decks: fireDeck}, {Name: " abc", Decks: fireDeck}}},
		{name: "no_decks", npcAvatars: []masterNPCAvatar{{Name: "森の番人"}}},
		{name: "unknown_deck", npcAvatars: []masterNPCAvatar{{Name: "森の番人", Decks: []string{"x"}}}},
		{name: "duplicated_deck", npcAvatars: []masterNPCAvatar{{Name: "森の番人", Decks: []string{fireDeck[0], fireDeck[0]}}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, err := loadMaster(sampleMasterJSON)
			require.NoError(t, err)
			m.NPCAvatars = tt.npcAvatars

			require.Error(t, m.validate())
		})
	}
}

// 見本のファイルの NPC のデッキは、プリセットデッキの名前で書く（同じ名前で写して公開する）
func TestSampleMaster_HasNPCDecksFromPresetDecks(t *testing.T) {
	m, err := loadMaster(sampleMasterJSON)
	require.NoError(t, err)

	require.Equal(t, []string{"火・地属性デッキ", "水・風属性デッキ"}, m.NPCDecks)
}

func TestLoadMaster_RejectsInvalidNPCDecks(t *testing.T) {
	tests := []struct {
		name     string
		npcDecks []string
	}{
		{name: "unknown_preset_deck", npcDecks: []string{"x"}},
		{name: "duplicated_preset_deck", npcDecks: []string{"火・地属性デッキ", "火・地属性デッキ"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, err := loadMaster(sampleMasterJSON)
			require.NoError(t, err)
			m.NPCDecks = tt.npcDecks

			require.Error(t, m.validate())
		})
	}
}

// 見本のスターターセットは、見本の配布デッキを今の順にすべて配り、チュートリアルのボードを解放する
func TestSampleMaster_HasStarterSetWithGrantDecksAndTutorialBoard(t *testing.T) {
	m, err := loadMaster(sampleMasterJSON)
	require.NoError(t, err)

	require.Equal(t, []masterStarterSet{
		{Name: "スターターセット", GrantDecks: []string{"火・地属性デッキ", "水・風属性デッキ"}, UnlockBoards: []string{"チュートリアル"}, UnlockNPCAvatars: []string{"ゴリガン"}},
	}, m.StarterSets)
	require.Equal(t, m.GrantDecks, m.StarterSets[0].GrantDecks)
}

func TestLoadMaster_RejectsInvalidStarterSets(t *testing.T) {
	grantDecks := []string{"火・地属性デッキ"}
	tests := []struct {
		name        string
		starterSets []masterStarterSet
	}{
		{name: "blank_name", starterSets: []masterStarterSet{{Name: " ", GrantDecks: grantDecks}}},
		{name: "too_long_name", starterSets: []masterStarterSet{{Name: strings.Repeat("あ", starterset.MaxNameLength+1), GrantDecks: grantDecks}}},
		{name: "duplicated_name", starterSets: []masterStarterSet{{Name: "a", GrantDecks: grantDecks}, {Name: "a", GrantDecks: grantDecks}}},
		{name: "more_than_publishable_at_once", starterSets: []masterStarterSet{{Name: "a", GrantDecks: grantDecks}, {Name: "b", GrantDecks: grantDecks}}},
		{name: "no_grant_decks", starterSets: []masterStarterSet{{Name: "a"}}},
		{name: "unknown_grant_deck", starterSets: []masterStarterSet{{Name: "a", GrantDecks: []string{"x"}}}},
		{name: "duplicated_grant_deck", starterSets: []masterStarterSet{{Name: "a", GrantDecks: []string{grantDecks[0], grantDecks[0]}}}},
		{name: "unknown_unlock_board", starterSets: []masterStarterSet{{Name: "a", GrantDecks: grantDecks, UnlockBoards: []string{"x"}}}},
		{name: "unknown_unlock_npc_avatar", starterSets: []masterStarterSet{{Name: "a", GrantDecks: grantDecks, UnlockNPCAvatars: []string{"x"}}}},
		{name: "duplicated_unlock_npc_avatar", starterSets: []masterStarterSet{{Name: "a", GrantDecks: grantDecks, UnlockNPCAvatars: []string{"ゴリガン", "ゴリガン"}}}},
		{name: "duplicated_unlock_board", starterSets: []masterStarterSet{{Name: "a", GrantDecks: grantDecks, UnlockBoards: []string{"チュートリアル", "チュートリアル"}}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, err := loadMaster(sampleMasterJSON)
			require.NoError(t, err)
			m.StarterSets = tt.starterSets

			require.Error(t, m.validate())
		})
	}
}

func TestLoadMaster_RejectsInvalidMaster(t *testing.T) {
	tests := []struct {
		name string
		json string
	}{
		{name: "unknown_card_type", json: `{"cards":[{"type":"hero","name":"a"}]}`},
		{name: "blank_card_name", json: `{"cards":[{"type":"item","name":""}]}`},
		{name: "duplicated_card_name", json: `{"cards":[{"type":"item","name":"a"},{"type":"spell","name":"a"}]}`},
		{name: "unknown_card_in_deck", json: `{"cards":[],"preset_decks":[{"name":"d","cards":[{"card":"x","count":4}]}]}`},
		{name: "deck_total_not_forty", json: `{"cards":[{"type":"item","name":"a"}],"preset_decks":[{"name":"d","cards":[{"card":"a","count":4}]}]}`},
		{name: "board_without_default_rules", json: boardWithRulesJSON(`{}`)},
		{name: "board_with_target_magic_not_above_starting_magic", json: boardWithRulesJSON(`{"target_magic":600,"starting_magic":600,"lap_bonus":300,"checkpoint_bonus":100,"round_limit":30,"dice_min":1,"dice_max":6}`)},
		{name: "board_with_negative_lap_bonus", json: boardWithRulesJSON(`{"target_magic":8000,"starting_magic":600,"lap_bonus":-1,"checkpoint_bonus":100,"round_limit":30,"dice_min":1,"dice_max":6}`)},
		{name: "board_with_zero_round_limit", json: boardWithRulesJSON(`{"target_magic":8000,"starting_magic":600,"lap_bonus":300,"checkpoint_bonus":100,"round_limit":0,"dice_min":1,"dice_max":6}`)},
		{name: "board_with_dice_max_below_dice_min", json: boardWithRulesJSON(`{"target_magic":8000,"starting_magic":600,"lap_bonus":300,"checkpoint_bonus":100,"round_limit":30,"dice_min":4,"dice_max":3}`)},
		{name: "no_grant_decks", json: `{"cards":[],"grant_decks":[]}`},
		{name: "unknown_preset_deck_for_grant_deck", json: `{"cards":[],"grant_decks":["x"]}`},
		{name: "blank_board_name", json: boardJSON("", `{"x":0,"y":0,"kind":"base"},{"x":1,"y":0,"kind":"checkpoint"}`, `{"a":{"x":0,"y":0},"b":{"x":1,"y":0}}`)},
		{name: "board_tile_outside_grid", json: boardJSON("b", `{"x":8,"y":0,"kind":"base"}`, ``)},
		{name: "board_without_checkpoint", json: boardJSON("b", `{"x":0,"y":0,"kind":"base"}`, ``)},
		{name: "board_with_unreachable_tile", json: boardJSON("b", `{"x":0,"y":0,"kind":"base"},{"x":1,"y":0,"kind":"checkpoint"}`, ``)},
		{name: "unknown_board_tile_kind", json: boardJSON("b", `{"x":0,"y":0,"kind":"water"}`, ``)},
		{name: "land_without_element", json: boardJSON("b", `{"x":0,"y":0,"kind":"base"},{"x":1,"y":0,"kind":"land"},{"x":2,"y":0,"kind":"checkpoint"}`, `{"a":{"x":0,"y":0},"b":{"x":1,"y":0}},{"a":{"x":1,"y":0},"b":{"x":2,"y":0}}`)},
		{name: "base_with_element", json: boardJSON("b", `{"x":0,"y":0,"kind":"base","element":"fire"},{"x":1,"y":0,"kind":"checkpoint"}`, `{"a":{"x":0,"y":0},"b":{"x":1,"y":0}}`)},
		{name: "unknown_board_tile_element", json: boardJSON("b", `{"x":0,"y":0,"kind":"base"},{"x":1,"y":0,"kind":"land","element":"light"},{"x":2,"y":0,"kind":"checkpoint"}`, `{"a":{"x":0,"y":0},"b":{"x":1,"y":0}},{"a":{"x":1,"y":0},"b":{"x":2,"y":0}}`)},
		{name: "duplicated_board_name", json: `{"cards":[],"grant_decks":[],"boards":[` + boardBody("b") + `,` + boardBody("b") + `]}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := loadMaster([]byte(tt.json))

			require.Error(t, err)
		})
	}
}

// 8×8 の盤面を 1 つ持つ見本の JSON。tiles・connections は中身だけを渡す
func boardJSON(name, tiles, connections string) string {
	return `{"cards":[],"grant_decks":[],"boards":[` + boardBodyWith(name, tiles, connections) + `]}`
}

func boardWithRulesJSON(defaultRules string) string {
	body := boardBodyWithRules("b", `{"x":0,"y":0,"kind":"base"},{"x":1,"y":0,"kind":"checkpoint"}`, `{"a":{"x":0,"y":0},"b":{"x":1,"y":0}}`, defaultRules)
	return `{"cards":[],"grant_decks":[],"boards":[` + body + `]}`
}

func boardBody(name string) string {
	return boardBodyWith(name, `{"x":0,"y":0,"kind":"base"},{"x":1,"y":0,"kind":"checkpoint"}`, `{"a":{"x":0,"y":0},"b":{"x":1,"y":0}}`)
}

func boardBodyWith(name, tiles, connections string) string {
	return boardBodyWithRules(name, tiles, connections, `{"target_magic":8000,"starting_magic":600,"lap_bonus":300,"checkpoint_bonus":100,"round_limit":30,"dice_min":1,"dice_max":6}`)
}

func boardBodyWithRules(name, tiles, connections, defaultRules string) string {
	return `{"name":"` + name + `","width":8,"height":8,"default_rules":` + defaultRules + `,"tiles":[` + tiles + `],"connections":[` + connections + `]}`
}

func TestLoadMaster_RejectsInvalidCardSpec(t *testing.T) {
	tests := []struct {
		name string
		card func(c *masterCard)
	}{
		{name: "negative_cost", card: func(c *masterCard) { c.Cost = -1 }},
		{name: "unknown_rarity", card: func(c *masterCard) { c.Rarity = "N" }},
		{name: "missing_rarity", card: func(c *masterCard) { c.Rarity = "" }},
		{name: "missing_elements", card: func(c *masterCard) { c.Elements = nil }},
		{name: "unknown_element", card: func(c *masterCard) { c.Elements = []string{"light"} }},
		{name: "neutral_mixed_elements", card: func(c *masterCard) { c.Elements = []string{"neutral", "fire"} }},
		{name: "missing_attack", card: func(c *masterCard) { c.Attack = nil }},
		{name: "negative_attack", card: func(c *masterCard) { attack := -1; c.Attack = &attack }},
		{name: "missing_hit_points", card: func(c *masterCard) { c.HitPoints = nil }},
		{name: "zero_hit_points", card: func(c *masterCard) { hitPoints := 0; c.HitPoints = &hitPoints }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, err := loadMaster(sampleMasterJSON)
			require.NoError(t, err)
			tt.card(&m.Cards[0])

			require.Error(t, m.validate())
		})
	}
}

func TestLoadMaster_RejectsCreatureSpecOnItemAndSpell(t *testing.T) {
	for _, cardType := range []string{"item", "spell"} {
		t.Run(cardType, func(t *testing.T) {
			m, err := loadMaster(sampleMasterJSON)
			require.NoError(t, err)
			index := slices.IndexFunc(m.Cards, func(c masterCard) bool { return c.Type == cardType })
			m.Cards[index].Elements = []string{"fire"}

			require.Error(t, m.validate())
		})
	}
}
