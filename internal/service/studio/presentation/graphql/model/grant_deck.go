package model

import (
	"time"

	"github.com/google/uuid"
)

// gqlgen.yml でこの型を GrantDeck に使わせる（生成させない）。cards は resolver がカードを引いて並べてから返すため、
// read model から読んだ公開 ID と枚数を、resolver に渡すまで CardCounts に持つ（PresetDeck と同じ。スキーマの項目ではない）
type GrantDeck struct {
	ID           uuid.UUID
	Name         string
	Status       GrantDeckStatus
	TotalCount   int
	Revision     int
	RegisteredAt time.Time
	UpdatedAt    time.Time
	CardCounts   []DeckCardCount
}

func (GrantDeck) IsGrantDeckResult() {}
