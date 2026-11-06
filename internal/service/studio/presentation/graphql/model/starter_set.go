package model

import (
	"time"

	"github.com/google/uuid"
)

// gqlgen.yml でこの型を StarterSet に使わせる（生成させない）。grantDecks・unlockBoards は resolver が配布デッキ・ボードを引いて
// 並べてから返すため、read model から読んだ公開 ID を、resolver に渡すまで GrantDeckIDs・UnlockBoardIDs に持つ
// （NpcAvatar の DeckIDs と同じ。スキーマの項目ではない）
type StarterSet struct {
	ID                 uuid.UUID
	Name               string
	Status             StarterSetStatus
	Revision           int
	RegisteredAt       time.Time
	UpdatedAt          time.Time
	GrantDeckIDs       []uuid.UUID
	UnlockBoardIDs     []uuid.UUID
	UnlockNPCAvatarIDs []uuid.UUID
}

func (StarterSet) IsStarterSetResult() {}
