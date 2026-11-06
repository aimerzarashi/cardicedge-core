package readmodel

import (
	"time"

	"github.com/google/uuid"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/studiorules"
)

// studio_rules の 1 行（全体で 1 行）。値は studiorules.Rules をそのまま持つので、値の項目を足しても変えない。
// game には写さない（game_rules と違い、metadata の形は studio の中だけの約束）
type StudioRules struct {
	StudioRulesID uuid.UUID
	Revision      int64
	RegisteredAt  time.Time
	UpdatedAt     time.Time
	Rules         studiorules.Rules
}

// 表示に使う値。日時はイベントの発生時刻から作る。起動時に作る集約で誰も操作していないため、registered_by・updated_by は持たない（書かない）
type StudioRulesMetadata struct {
	ID           uuid.UUID         `json:"id"`
	Revision     int64             `json:"revision"`
	RegisteredAt time.Time         `json:"registered_at"`
	UpdatedAt    time.Time         `json:"updated_at"`
	Rules        studiorules.Rules `json:"rules"`
}

// studio_rules のイベントだけを扱い、ほかは状態をそのまま返す
func (state StudioRules) Evolve(event any) StudioRules {
	if e, ok := event.(*studiorules.CreatedEvent); ok {
		state.StudioRulesID = e.PublicID
		state.Revision = e.GetMeta().AggregateVersion
		state.RegisteredAt = e.GetMeta().OccurredAt
		state.UpdatedAt = e.GetMeta().OccurredAt
		state.Rules = e.Rules
	}
	return state
}

// 削除のイベントは無い
func (state StudioRules) IsTerminal() bool {
	return false
}
