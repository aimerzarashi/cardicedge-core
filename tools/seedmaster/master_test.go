package main

import (
	"testing"

	"github.com/stretchr/testify/require"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/board"
)

func TestSampleMaster_IsValid(t *testing.T) {
	_, err := loadMaster(sampleMasterJSON)

	require.NoError(t, err)
}

// 見本のボードは、公開の条件を満たし、拠点 1・チェックポイント 2 以上・分かれ道（3 方向以上につながるマス）が 1 つ以上ある 8×8
func TestSampleMaster_HasPublishableLoopBoardWithFork(t *testing.T) {
	m, err := loadMaster(sampleMasterJSON)
	require.NoError(t, err)
	require.NotEmpty(t, m.Boards)

	for _, b := range m.Boards {
		layout, err := b.layout()
		require.NoError(t, err)
		readiness := layout.Readiness()
		degrees := map[masterBoardPosition]int{}
		for _, c := range b.Connections {
			degrees[c.A]++
			degrees[c.B]++
		}
		fork := false
		for _, degree := range degrees {
			fork = fork || degree >= 3
		}
		require.Equal(t, 8, b.Width)
		require.Equal(t, 8, b.Height)
		require.Equal(t, 1, readiness.BaseCount)
		require.GreaterOrEqual(t, readiness.CheckpointCount, 2)
		require.True(t, fork)
	}
}

// 登録の直後の既定の値と違う値を入れる（開発の画面で、既定のままか変えた値かを見分けられ、変える操作も流れるように）
func TestSampleMaster_HasDefaultRulesDifferentFromInitialOnes(t *testing.T) {
	m, err := loadMaster(sampleMasterJSON)
	require.NoError(t, err)

	for _, b := range m.Boards {
		initial := board.InitialDefaultRules()
		require.NotEqual(t, []int{initial.TargetMagic(), initial.StartingMagic(), initial.LapBonus(), initial.CheckpointBonus()},
			[]int{b.DefaultRules.TargetMagic, b.DefaultRules.StartingMagic, b.DefaultRules.LapBonus, b.DefaultRules.CheckpointBonus})
	}
}

// 6 つの属性が見本のどこかの土地に出てくる（studio・game の画面で、どの属性も見て確かめられるように）
func TestSampleMaster_HasEveryLandAttribute(t *testing.T) {
	m, err := loadMaster(sampleMasterJSON)
	require.NoError(t, err)

	attributes := map[string]bool{}
	for _, b := range m.Boards {
		for _, tile := range b.Tiles {
			attributes[tile.Attribute] = true
		}
	}

	for _, attribute := range []string{"fire", "earth", "water", "wind", "neutral", "multi"} {
		require.True(t, attributes[attribute])
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
		{name: "board_with_target_magic_not_above_starting_magic", json: boardWithRulesJSON(`{"target_magic":600,"starting_magic":600,"lap_bonus":300,"checkpoint_bonus":100}`)},
		{name: "board_with_negative_lap_bonus", json: boardWithRulesJSON(`{"target_magic":8000,"starting_magic":600,"lap_bonus":-1,"checkpoint_bonus":100}`)},
		{name: "no_starter_decks", json: `{"cards":[],"starter_decks":[]}`},
		{name: "unknown_preset_deck_for_starter_deck", json: `{"cards":[],"starter_decks":["x"]}`},
		{name: "blank_board_name", json: boardJSON("", `{"x":0,"y":0,"kind":"base"},{"x":1,"y":0,"kind":"checkpoint"}`, `{"a":{"x":0,"y":0},"b":{"x":1,"y":0}}`)},
		{name: "board_tile_outside_grid", json: boardJSON("b", `{"x":8,"y":0,"kind":"base"}`, ``)},
		{name: "board_without_checkpoint", json: boardJSON("b", `{"x":0,"y":0,"kind":"base"}`, ``)},
		{name: "board_with_unreachable_tile", json: boardJSON("b", `{"x":0,"y":0,"kind":"base"},{"x":1,"y":0,"kind":"checkpoint"}`, ``)},
		{name: "unknown_board_tile_kind", json: boardJSON("b", `{"x":0,"y":0,"kind":"water"}`, ``)},
		{name: "land_without_attribute", json: boardJSON("b", `{"x":0,"y":0,"kind":"base"},{"x":1,"y":0,"kind":"land"},{"x":2,"y":0,"kind":"checkpoint"}`, `{"a":{"x":0,"y":0},"b":{"x":1,"y":0}},{"a":{"x":1,"y":0},"b":{"x":2,"y":0}}`)},
		{name: "base_with_attribute", json: boardJSON("b", `{"x":0,"y":0,"kind":"base","attribute":"fire"},{"x":1,"y":0,"kind":"checkpoint"}`, `{"a":{"x":0,"y":0},"b":{"x":1,"y":0}}`)},
		{name: "unknown_board_tile_attribute", json: boardJSON("b", `{"x":0,"y":0,"kind":"base"},{"x":1,"y":0,"kind":"land","attribute":"light"},{"x":2,"y":0,"kind":"checkpoint"}`, `{"a":{"x":0,"y":0},"b":{"x":1,"y":0}},{"a":{"x":1,"y":0},"b":{"x":2,"y":0}}`)},
		{name: "duplicated_board_name", json: `{"cards":[],"starter_decks":[],"boards":[` + boardBody("b") + `,` + boardBody("b") + `]}`},
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
	return `{"cards":[],"starter_decks":[],"boards":[` + boardBodyWith(name, tiles, connections) + `]}`
}

func boardWithRulesJSON(defaultRules string) string {
	body := boardBodyWithRules("b", `{"x":0,"y":0,"kind":"base"},{"x":1,"y":0,"kind":"checkpoint"}`, `{"a":{"x":0,"y":0},"b":{"x":1,"y":0}}`, defaultRules)
	return `{"cards":[],"starter_decks":[],"boards":[` + body + `]}`
}

func boardBody(name string) string {
	return boardBodyWith(name, `{"x":0,"y":0,"kind":"base"},{"x":1,"y":0,"kind":"checkpoint"}`, `{"a":{"x":0,"y":0},"b":{"x":1,"y":0}}`)
}

func boardBodyWith(name, tiles, connections string) string {
	return boardBodyWithRules(name, tiles, connections, `{"target_magic":8000,"starting_magic":600,"lap_bonus":300,"checkpoint_bonus":100}`)
}

func boardBodyWithRules(name, tiles, connections, defaultRules string) string {
	return `{"name":"` + name + `","width":8,"height":8,"default_rules":` + defaultRules + `,"tiles":[` + tiles + `],"connections":[` + connections + `]}`
}
