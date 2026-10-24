package readmodel

import (
	"github.com/google/uuid"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/player"
)

// players の 1 行のうち、players の投影が書く部分（行の存在とプレイヤーの公開用の ID）
type Player struct {
	PlayerID uuid.UUID
}

// プレイヤーのイベントだけを扱い、ほかは状態をそのまま返す
func (state Player) Evolve(event any) Player {
	if e, ok := event.(*player.RegisteredEvent); ok {
		state.PlayerID = e.PublicID
	}
	return state
}

// プレイヤーを消すイベントは無い
func (state Player) IsTerminal() bool {
	return false
}

// スターターの配布の状態。値は表の CHECK と同じ
type StarterGrantStatus string

const (
	StarterGrantInProgress StarterGrantStatus = "IN_PROGRESS"
	StarterGrantCompleted  StarterGrantStatus = "COMPLETED"
)
