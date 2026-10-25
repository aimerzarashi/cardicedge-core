package readmodel

import (
	"time"

	"github.com/google/uuid"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/avatar"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/player"
)

// avatars の 1 行。持ち主は、イベントが内部 ID で持つため、書くときに repository が公開用の ID に引き直す
type Avatar struct {
	AvatarID     uuid.UUID
	Owner        player.ID
	Name         string
	RegisteredAt time.Time
}

// 日時はイベントの発生時刻から作る（DB の現在時刻は使わない）
type AvatarMetadata struct {
	ID           uuid.UUID `json:"id"`
	Name         string    `json:"name"`
	RegisteredAt time.Time `json:"registered_at"`
}

func (state Avatar) Metadata() AvatarMetadata {
	return AvatarMetadata{ID: state.AvatarID, Name: state.Name, RegisteredAt: state.RegisteredAt}
}

// アバターのイベントだけを扱い、ほかは状態をそのまま返す
func (state Avatar) Evolve(event any) Avatar {
	if e, ok := event.(*avatar.RegisteredEvent); ok {
		state.AvatarID = e.PublicID
		state.Owner = e.Owner
		state.Name = e.Name.String()
		state.RegisteredAt = e.GetMeta().OccurredAt
	}
	return state
}

// アバターを消すイベントは無い
func (state Avatar) IsTerminal() bool {
	return false
}
