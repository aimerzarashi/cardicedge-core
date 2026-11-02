package readmodel

import (
	"fmt"
	"slices"
	"time"

	"github.com/google/uuid"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/npcavatar"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/npcdeck"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/operator"
)

// npc_avatars.status の値（準備中・公開中・使用不可）。domain の npcavatar.Status を使わない理由は CardStatus と同じ
type NPCAvatarStatus string

const (
	NPCAvatarStatusDraft     NPCAvatarStatus = "draft"
	NPCAvatarStatusPublished NPCAvatarStatus = "published"
	NPCAvatarStatusRetired   NPCAvatarStatus = "retired"
)

// DB の npc_avatars.status など、外から来た値を状態にする
func NewNPCAvatarStatus(value string) (NPCAvatarStatus, error) {
	status := NPCAvatarStatus(value)
	switch status {
	case NPCAvatarStatusDraft, NPCAvatarStatusPublished, NPCAvatarStatusRetired:
		return status, nil
	default:
		return "", fmt.Errorf("readmodel: unknown npc avatar status: %q", value)
	}
}

// npc_avatars の 1 行。個別の列（SortName・Status）は並べ替え・絞り込み用、Metadata は表示用
type NPCAvatar struct {
	NPCAvatarID uuid.UUID
	SortName    string
	Status      NPCAvatarStatus
	Metadata    NPCAvatarMetadata
	// イベントはオペレーターを内部 ID で持つため、内部 ID で持ち、書くときに公開 ID に引き直す（AvatarWriter と同じ）
	RegisteredBy operator.ID
	UpdatedBy    operator.ID
	// 使うデッキ（集約が持つ順）。イベントは別の集約を内部 ID で持つため、内部 ID で持ち、書くときに公開 ID に引き直す
	Decks []npcdeck.ID
	// 集約が削除された。DB には持たず、投影が行を消すかを決めるのに使う
	IsDeleted bool
}

// 日時はイベントの発生時刻から作る（DB の現在時刻は使わない）
type NPCAvatarMetadata struct {
	ID           uuid.UUID       `json:"id"`
	Name         string          `json:"name"`
	Status       NPCAvatarStatus `json:"status"`
	Revision     int64           `json:"revision"`
	RegisteredAt time.Time       `json:"registered_at"`
	UpdatedAt    time.Time       `json:"updated_at"`
	// 投影では空のまま。書くときに repository が RegisteredBy・UpdatedBy から入れる
	RegisteredBy uuid.UUID `json:"registered_by"`
	UpdatedBy    uuid.UUID `json:"updated_by"`
	// 使うデッキの公開 ID（集約が持つ順）。投影では空のまま。書くときに repository が Decks から入れる
	Decks []uuid.UUID `json:"decks"`
}

// NPC のアバターのイベントだけを扱い、ほかは状態をそのまま返す
func (state NPCAvatar) Evolve(event any) NPCAvatar {
	switch e := event.(type) {
	case *npcavatar.RegisteredEvent:
		occurredAt := e.GetMeta().OccurredAt
		state.NPCAvatarID = e.PublicID
		state.SortName = domain.NormalizeName(e.Name.String())
		state.Status = NPCAvatarStatusDraft
		state.Metadata = NPCAvatarMetadata{
			ID: e.PublicID, Name: e.Name.String(), Status: NPCAvatarStatusDraft, Revision: e.GetMeta().AggregateVersion,
			RegisteredAt: occurredAt, UpdatedAt: occurredAt,
		}
		state.RegisteredBy = e.RegisteredBy
		state.UpdatedBy = e.RegisteredBy
	case *npcavatar.RenamedEvent:
		state.SortName = domain.NormalizeName(e.Name.String())
		state.Metadata.Name = e.Name.String()
		state = state.touched(e.GetMeta().AggregateVersion, e.GetMeta().OccurredAt, e.RenamedBy)
	case *npcavatar.DecksAssignedEvent:
		state.Decks = append(slices.Clone(state.Decks), e.Decks...)
		state = state.touched(e.GetMeta().AggregateVersion, e.GetMeta().OccurredAt, e.AssignedBy)
	case *npcavatar.DecksUnassignedEvent:
		state.Decks = slices.DeleteFunc(slices.Clone(state.Decks), func(deck npcdeck.ID) bool { return slices.Contains(e.Decks, deck) })
		state = state.touched(e.GetMeta().AggregateVersion, e.GetMeta().OccurredAt, e.UnassignedBy)
	case *npcavatar.PublishedEvent:
		state = state.changeStatus(NPCAvatarStatusPublished, e.GetMeta().AggregateVersion, e.GetMeta().OccurredAt, e.PublishedBy)
	case *npcavatar.RetiredEvent:
		state = state.changeStatus(NPCAvatarStatusRetired, e.GetMeta().AggregateVersion, e.GetMeta().OccurredAt, e.RetiredBy)
	case *npcavatar.DeletedEvent:
		state.IsDeleted = true
	}
	return state
}

func (state NPCAvatar) changeStatus(status NPCAvatarStatus, revision int64, at time.Time, by operator.ID) NPCAvatar {
	state.Status = status
	state.Metadata.Status = status
	return state.touched(revision, at, by)
}

func (state NPCAvatar) touched(revision int64, at time.Time, by operator.ID) NPCAvatar {
	state.Metadata.Revision = revision
	state.Metadata.UpdatedAt = at
	state.UpdatedBy = by
	return state
}

func (state NPCAvatar) IsTerminal() bool {
	return state.IsDeleted
}
