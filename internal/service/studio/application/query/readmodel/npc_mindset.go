package readmodel

import (
	"fmt"
	"time"

	"github.com/google/uuid"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/matchaction"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/npcmindset"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/operator"
)

// npc_mindsets.status の値（準備中・公開中・使用不可）。domain の npcmindset.Status を使わない理由は CardStatus と同じ
type NPCMindsetStatus string

const (
	NPCMindsetStatusDraft     NPCMindsetStatus = "draft"
	NPCMindsetStatusPublished NPCMindsetStatus = "published"
	NPCMindsetStatusRetired   NPCMindsetStatus = "retired"
)

// DB の npc_mindsets.status など、外から来た値を状態にする
func NewNPCMindsetStatus(value string) (NPCMindsetStatus, error) {
	status := NPCMindsetStatus(value)
	switch status {
	case NPCMindsetStatusDraft, NPCMindsetStatusPublished, NPCMindsetStatusRetired:
		return status, nil
	default:
		return "", fmt.Errorf("readmodel: unknown npc mindset status: %q", value)
	}
}

// npc_mindsets の 1 行。個別の列（SortName・Status）は並べ替え・絞り込み用、Metadata は表示用
type NPCMindset struct {
	NPCMindsetID uuid.UUID
	SortName     string
	Status       NPCMindsetStatus
	Metadata     NPCMindsetMetadata
	// イベントはオペレーターを内部 ID で持つため、内部 ID で持ち、書くときに公開 ID に引き直す（AvatarWriter と同じ）
	RegisteredBy operator.ID
	UpdatedBy    operator.ID
	// 集約が削除された。DB には持たず、投影が行を消すかを決めるのに使う
	IsDeleted bool
}

// 日時はイベントの発生時刻から作る（DB の現在時刻は使わない）
type NPCMindsetMetadata struct {
	ID           uuid.UUID        `json:"id"`
	Name         string           `json:"name"`
	Status       NPCMindsetStatus `json:"status"`
	Revision     int64            `json:"revision"`
	RegisteredAt time.Time        `json:"registered_at"`
	UpdatedAt    time.Time        `json:"updated_at"`
	// 投影では空のまま。書くときに repository が RegisteredBy・UpdatedBy から入れる
	RegisteredBy uuid.UUID `json:"registered_by"`
	UpdatedBy    uuid.UUID `json:"updated_by"`
	// 行動の種類ごとの重み（JSON のキーは行動の種類の値。種類のすべてを持つ）と、採点基準ごとの重み（基準のすべてを持つ）。
	// game もこの形を契約として読む（make publish-master がそのまま写す）
	ActionWeights    map[matchaction.Action]int   `json:"action_weights"`
	CriterionWeights map[npcmindset.Criterion]int `json:"criterion_weights"`
}

// NPC の思考パターンのイベントだけを扱い、ほかは状態をそのまま返す
func (state NPCMindset) Evolve(event any) NPCMindset {
	switch e := event.(type) {
	case *npcmindset.RegisteredEvent:
		occurredAt := e.GetMeta().OccurredAt
		state.NPCMindsetID = e.PublicID
		state.SortName = domain.NormalizeName(e.Name.String())
		state.Status = NPCMindsetStatusDraft
		state.Metadata = NPCMindsetMetadata{
			ID: e.PublicID, Name: e.Name.String(), Status: NPCMindsetStatusDraft, Revision: e.GetMeta().AggregateVersion,
			RegisteredAt: occurredAt, UpdatedAt: occurredAt,
			ActionWeights: actionWeightValues(e.ActionWeights), CriterionWeights: criterionWeightValues(e.CriterionWeights),
		}
		state.RegisteredBy = e.RegisteredBy
		state.UpdatedBy = e.RegisteredBy
	case *npcmindset.RenamedEvent:
		state.SortName = domain.NormalizeName(e.Name.String())
		state.Metadata.Name = e.Name.String()
		state = state.touched(e.GetMeta().AggregateVersion, e.GetMeta().OccurredAt, e.RenamedBy)
	case *npcmindset.ActionWeightsChangedEvent:
		state.Metadata.ActionWeights = actionWeightValues(e.Weights)
		state = state.touched(e.GetMeta().AggregateVersion, e.GetMeta().OccurredAt, e.ChangedBy)
	case *npcmindset.CriterionWeightsChangedEvent:
		state.Metadata.CriterionWeights = criterionWeightValues(e.Weights)
		state = state.touched(e.GetMeta().AggregateVersion, e.GetMeta().OccurredAt, e.ChangedBy)
	case *npcmindset.PublishedEvent:
		state = state.changeStatus(NPCMindsetStatusPublished, e.GetMeta().AggregateVersion, e.GetMeta().OccurredAt, e.PublishedBy)
	case *npcmindset.RetiredEvent:
		state = state.changeStatus(NPCMindsetStatusRetired, e.GetMeta().AggregateVersion, e.GetMeta().OccurredAt, e.RetiredBy)
	case *npcmindset.DeletedEvent:
		state.IsDeleted = true
	}
	return state
}

// 重みの値は JSON の読み書きで確かめない値なので、素の整数で持つ（read model は表示用の値）
func actionWeightValues(weights npcmindset.ActionWeights) map[matchaction.Action]int {
	values := make(map[matchaction.Action]int, len(weights))
	for action, weight := range weights {
		values[action] = weight.Int()
	}
	return values
}

func criterionWeightValues(weights npcmindset.CriterionWeights) map[npcmindset.Criterion]int {
	values := make(map[npcmindset.Criterion]int, len(weights))
	for criterion, weight := range weights {
		values[criterion] = weight.Int()
	}
	return values
}

func (state NPCMindset) changeStatus(status NPCMindsetStatus, revision int64, at time.Time, by operator.ID) NPCMindset {
	state.Status = status
	state.Metadata.Status = status
	return state.touched(revision, at, by)
}

func (state NPCMindset) touched(revision int64, at time.Time, by operator.ID) NPCMindset {
	state.Metadata.Revision = revision
	state.Metadata.UpdatedAt = at
	state.UpdatedBy = by
	return state
}

func (state NPCMindset) IsTerminal() bool {
	return state.IsDeleted
}
