package model

import (
	"time"

	"github.com/google/uuid"
)

// gqlgen.yml でこの型を StarterDeck に使わせる（生成させない）。cards は resolver がカードを引いて並べてから返すため、
// read model から読んだ公開 ID と枚数を、resolver に渡すまで CardCounts に持つ（PresetDeck と同じ。スキーマの項目ではない）
type StarterDeck struct {
	ID           uuid.UUID
	Name         string
	Status       StarterDeckStatus
	TotalCount   int
	Source       *StarterDeckSource
	Revision     int
	RegisteredAt time.Time
	UpdatedAt    time.Time
	CardCounts   []DeckCardCount
}

func (StarterDeck) IsStarterDeckResult() {}
