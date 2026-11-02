package readmodel

import (
	"time"

	"github.com/google/uuid"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/gamerules"
)

// game_rules の 1 行（ゲーム全体で 1 行）。値は gamerules.Rules をそのまま持つので、値の項目を足しても変えない。
// 行をそのまま game に写す（make publish-master）ので、metadata の形が studio と game の契約になる
type GameRules struct {
	GameRulesID  uuid.UUID
	Revision     int64
	RegisteredAt time.Time
	UpdatedAt    time.Time
	Rules        gamerules.Rules
}

// 表示に使う値。日時はイベントの発生時刻から作る。起動時に作る集約で誰も操作していないため、registered_by・updated_by は持たない（書かない）
type GameRulesMetadata struct {
	ID           uuid.UUID       `json:"id"`
	Revision     int64           `json:"revision"`
	RegisteredAt time.Time       `json:"registered_at"`
	UpdatedAt    time.Time       `json:"updated_at"`
	Rules        gamerules.Rules `json:"rules"`
}

// ゲーム全体のルールのイベントだけを扱い、ほかは状態をそのまま返す
func (state GameRules) Evolve(event any) GameRules {
	if e, ok := event.(*gamerules.CreatedEvent); ok {
		state.GameRulesID = e.PublicID
		state.Revision = e.GetMeta().AggregateVersion
		state.RegisteredAt = e.GetMeta().OccurredAt
		state.UpdatedAt = e.GetMeta().OccurredAt
		state.Rules = e.Rules
	}
	return state
}

// 削除のイベントは無い
func (state GameRules) IsTerminal() bool {
	return false
}
