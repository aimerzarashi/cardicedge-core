package readmodel

import (
	"slices"
	"time"

	"github.com/google/uuid"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/board"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/npcavatar"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/player"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/unlock"
)

// unlocks の 1 行。解放の公開 ID と持ち主は、イベントが内部 ID で持つ（イベントに公開 ID は無い）ため、書くときに repository が公開用の ID に引き直す
type Unlock struct {
	Unlock unlock.ID
	Player player.ID
	// 解放したボードの game の内部 ID（解放した順）。read model には公開 ID で書く（書くときに repository が引き直す）。
	// ボードの中身は書き写さず、読むときに写した boards から公開 ID で引く
	Boards []board.ID
	// 解放した NPC のアバターの game の内部 ID（解放した順）。ボードと同じく、read model には公開 ID で書く
	NPCAvatars   []npcavatar.ID
	RegisteredAt time.Time
	UpdatedAt    time.Time
}

// 日時はイベントの発生時刻から作る（DB の現在時刻は使わない）
type UnlockMetadata struct {
	// 空のまま。書くときに repository が公開 ID を入れる
	ID uuid.UUID `json:"id"`
	// 解放したボードの公開 ID（解放した順）。空のまま。書くときに repository が Boards から入れる
	Boards []uuid.UUID `json:"boards"`
	// 解放した NPC のアバターの公開 ID（解放した順）。空のまま。書くときに repository が NPCAvatars から入れる
	NPCAvatars   []uuid.UUID `json:"npc_avatars"`
	RegisteredAt time.Time   `json:"registered_at"`
	UpdatedAt    time.Time   `json:"updated_at"`
	// 操作した人は持ち主のプレイヤー（デッキと同じ。解放は配布の流れが作るが、プレイヤーのものとして作られ、更新も無い）。
	// 空のまま。書くときに repository が持ち主の公開 ID を入れる
	RegisteredBy uuid.UUID `json:"registered_by"`
	UpdatedBy    uuid.UUID `json:"updated_by"`
}

func (state Unlock) Metadata() UnlockMetadata {
	return UnlockMetadata{RegisteredAt: state.RegisteredAt, UpdatedAt: state.UpdatedAt}
}

// 解放のイベントだけを扱い、ほかは状態をそのまま返す。作ったときは何も持たず、解放するたびに解放した順に後ろへ足す
func (state Unlock) Evolve(event any) Unlock {
	switch e := event.(type) {
	case *unlock.CreatedEvent:
		state.Unlock = e.GetMeta().EntityID
		state.Player = e.Player
		state.Boards = []board.ID{}
		state.NPCAvatars = []npcavatar.ID{}
		state.RegisteredAt = e.GetMeta().OccurredAt
		state.UpdatedAt = e.GetMeta().OccurredAt
	case *unlock.BoardsUnlockedEvent:
		state.Boards = append(slices.Clone(state.Boards), e.Boards...)
		state.UpdatedAt = e.GetMeta().OccurredAt
	case *unlock.NPCAvatarsUnlockedEvent:
		state.NPCAvatars = append(slices.Clone(state.NPCAvatars), e.NPCAvatars...)
		state.UpdatedAt = e.GetMeta().OccurredAt
	}
	return state
}

// 解放を消すイベントは無い
func (state Unlock) IsTerminal() bool {
	return false
}
