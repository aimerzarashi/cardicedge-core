package model

import (
	"time"

	"github.com/google/uuid"
)

// gqlgen.yml でこの型を NpcAvatar に使わせる（生成させない）。decks は resolver が NPC のデッキを引いて並べてから返すため、
// read model から読んだ公開 ID を、resolver に渡すまで DeckIDs・MindsetIDs に持つ（NpcDeck の CardCounts と同じ。スキーマの項目ではない）
type NpcAvatar struct {
	ID           uuid.UUID
	Name         string
	Status       NpcAvatarStatus
	Revision     int
	RegisteredAt time.Time
	UpdatedAt    time.Time
	DeckIDs      []uuid.UUID
	MindsetIDs   []uuid.UUID
}

func (NpcAvatar) IsNpcAvatarResult() {}
