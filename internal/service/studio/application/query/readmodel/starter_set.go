package readmodel

import (
	"fmt"
	"slices"
	"time"

	"github.com/google/uuid"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/board"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/grantdeck"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/npcavatar"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/operator"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/starterset"
)

// starter_sets.status の値（準備中・公開中・使用不可）。domain の starterset.Status を使わない理由は CardStatus と同じ
type StarterSetStatus string

const (
	StarterSetStatusDraft     StarterSetStatus = "draft"
	StarterSetStatusPublished StarterSetStatus = "published"
	StarterSetStatusRetired   StarterSetStatus = "retired"
)

// DB の starter_sets.status など、外から来た値を状態にする
func NewStarterSetStatus(value string) (StarterSetStatus, error) {
	status := StarterSetStatus(value)
	switch status {
	case StarterSetStatusDraft, StarterSetStatusPublished, StarterSetStatusRetired:
		return status, nil
	default:
		return "", fmt.Errorf("readmodel: unknown starter set status: %q", value)
	}
}

// starter_sets の 1 行。個別の列（SortName・Status）は並べ替え・絞り込み用、Metadata は表示用
type StarterSet struct {
	StarterSetID uuid.UUID
	SortName     string
	Status       StarterSetStatus
	Metadata     StarterSetMetadata
	// イベントはオペレーターを内部 ID で持つため、内部 ID で持ち、書くときに公開 ID に引き直す（AvatarWriter と同じ）
	RegisteredBy operator.ID
	UpdatedBy    operator.ID
	// 配るデッキと解放するボード（集約が持つ順）。イベントは別の集約を内部 ID で持つため、内部 ID で持ち、書くときに公開 ID に引き直す
	GrantDecks   []grantdeck.ID
	UnlockBoards []board.ID
	// 解放する NPC のアバター（集約が持つ順）。同じく内部 ID で持ち、書くときに公開 ID に引き直す
	UnlockNPCAvatars []npcavatar.ID
	// 集約が削除された。DB には持たず、投影が行を消すかを決めるのに使う
	IsDeleted bool
}

// 日時はイベントの発生時刻から作る（DB の現在時刻は使わない）
type StarterSetMetadata struct {
	ID           uuid.UUID        `json:"id"`
	Name         string           `json:"name"`
	Status       StarterSetStatus `json:"status"`
	Revision     int64            `json:"revision"`
	RegisteredAt time.Time        `json:"registered_at"`
	UpdatedAt    time.Time        `json:"updated_at"`
	// 投影では空のまま。書くときに repository が RegisteredBy・UpdatedBy から入れる
	RegisteredBy uuid.UUID `json:"registered_by"`
	UpdatedBy    uuid.UUID `json:"updated_by"`
	// 配るデッキ・解放するボードの公開 ID（集約が持つ順）。投影では空のまま。書くときに repository が GrantDecks・UnlockBoards から入れる
	GrantDecks   []uuid.UUID `json:"grant_decks"`
	UnlockBoards []uuid.UUID `json:"unlock_boards"`
	// 解放する NPC のアバターの公開 ID（集約が持つ順）。投影では空のまま。書くときに repository が UnlockNPCAvatars から入れる
	UnlockNPCAvatars []uuid.UUID `json:"unlock_npc_avatars"`
}

// スターターセットのイベントだけを扱い、ほかは状態をそのまま返す
func (state StarterSet) Evolve(event any) StarterSet {
	switch e := event.(type) {
	case *starterset.RegisteredEvent:
		occurredAt := e.GetMeta().OccurredAt
		state.StarterSetID = e.PublicID
		state.SortName = domain.NormalizeName(e.Name.String())
		state.Status = StarterSetStatusDraft
		state.Metadata = StarterSetMetadata{
			ID: e.PublicID, Name: e.Name.String(), Status: StarterSetStatusDraft, Revision: e.GetMeta().AggregateVersion,
			RegisteredAt: occurredAt, UpdatedAt: occurredAt,
		}
		state.RegisteredBy = e.RegisteredBy
		state.UpdatedBy = e.RegisteredBy
	case *starterset.RenamedEvent:
		state.SortName = domain.NormalizeName(e.Name.String())
		state.Metadata.Name = e.Name.String()
		state = state.touched(e.GetMeta().AggregateVersion, e.GetMeta().OccurredAt, e.RenamedBy)
	case *starterset.GrantDecksAssignedEvent:
		state.GrantDecks = append(slices.Clone(state.GrantDecks), e.GrantDecks...)
		state = state.touched(e.GetMeta().AggregateVersion, e.GetMeta().OccurredAt, e.AssignedBy)
	case *starterset.GrantDecksUnassignedEvent:
		state.GrantDecks = slices.DeleteFunc(slices.Clone(state.GrantDecks), func(id grantdeck.ID) bool { return slices.Contains(e.GrantDecks, id) })
		state = state.touched(e.GetMeta().AggregateVersion, e.GetMeta().OccurredAt, e.UnassignedBy)
	case *starterset.UnlockBoardsAssignedEvent:
		state.UnlockBoards = append(slices.Clone(state.UnlockBoards), e.UnlockBoards...)
		state = state.touched(e.GetMeta().AggregateVersion, e.GetMeta().OccurredAt, e.AssignedBy)
	case *starterset.UnlockBoardsUnassignedEvent:
		state.UnlockBoards = slices.DeleteFunc(slices.Clone(state.UnlockBoards), func(id board.ID) bool { return slices.Contains(e.UnlockBoards, id) })
		state = state.touched(e.GetMeta().AggregateVersion, e.GetMeta().OccurredAt, e.UnassignedBy)
	case *starterset.UnlockNPCAvatarsAssignedEvent:
		state.UnlockNPCAvatars = append(slices.Clone(state.UnlockNPCAvatars), e.UnlockNPCAvatars...)
		state = state.touched(e.GetMeta().AggregateVersion, e.GetMeta().OccurredAt, e.AssignedBy)
	case *starterset.UnlockNPCAvatarsUnassignedEvent:
		state.UnlockNPCAvatars = slices.DeleteFunc(slices.Clone(state.UnlockNPCAvatars), func(id npcavatar.ID) bool { return slices.Contains(e.UnlockNPCAvatars, id) })
		state = state.touched(e.GetMeta().AggregateVersion, e.GetMeta().OccurredAt, e.UnassignedBy)
	case *starterset.PublishedEvent:
		state = state.changeStatus(StarterSetStatusPublished, e.GetMeta().AggregateVersion, e.GetMeta().OccurredAt, e.PublishedBy)
	case *starterset.RetiredEvent:
		state = state.changeStatus(StarterSetStatusRetired, e.GetMeta().AggregateVersion, e.GetMeta().OccurredAt, e.RetiredBy)
	case *starterset.DeletedEvent:
		state.IsDeleted = true
	}
	return state
}

func (state StarterSet) changeStatus(status StarterSetStatus, revision int64, at time.Time, by operator.ID) StarterSet {
	state.Status = status
	state.Metadata.Status = status
	return state.touched(revision, at, by)
}

func (state StarterSet) touched(revision int64, at time.Time, by operator.ID) StarterSet {
	state.Metadata.Revision = revision
	state.Metadata.UpdatedAt = at
	state.UpdatedBy = by
	return state
}

func (state StarterSet) IsTerminal() bool {
	return state.IsDeleted
}
