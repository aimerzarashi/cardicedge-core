package model

import (
	"time"

	"github.com/google/uuid"
)

// gqlgen.yml でこの型を StarterDeck に使わせる（生成させない）。cards は resolver がカードを引いて並べてから返すため、
// read model から読んだ公開 ID と枚数を、resolver に渡すまで CardCounts に持つ（スキーマの項目ではない）
type StarterDeck struct {
	ID           uuid.UUID
	Name         string
	TotalCount   int
	Revision     int
	RegisteredAt time.Time
	UpdatedAt    time.Time
	CardCounts   []StarterDeckCardCount
}

type StarterDeckCardCount struct {
	CardID uuid.UUID
	Count  int
}

func (StarterDeck) IsStarterDeckResult() {}
