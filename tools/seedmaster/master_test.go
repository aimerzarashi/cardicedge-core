package main

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSampleMaster_IsValid(t *testing.T) {
	_, err := loadMaster(sampleMasterJSON)

	require.NoError(t, err)
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
		{name: "no_starter_decks", json: `{"cards":[],"starter_decks":[]}`},
		{name: "unknown_preset_deck_for_starter_deck", json: `{"cards":[],"starter_decks":["x"]}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := loadMaster([]byte(tt.json))

			require.Error(t, err)
		})
	}
}
