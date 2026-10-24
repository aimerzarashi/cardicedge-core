package readmodel

import (
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/player"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/startergrant"
)

// players の 1 行のうち、player_starter_grants の投影が書く部分（配布の状態の列）。
// 配布のイベントはプレイヤーを内部 ID で持ち、Evolve は DB を読めないため、書くときに repository が公開用の ID に引き直す
type PlayerStarterGrant struct {
	Player player.ID
	Status StarterGrantStatus
}

// 配布のイベントだけを扱い、ほかは状態をそのまま返す。プレイヤーは開始のイベントだけが持つ（同じ集約の全イベントを
// 順に当てるため、記録・完了のときも開始で入れた値が残る）
func (state PlayerStarterGrant) Evolve(event any) PlayerStarterGrant {
	switch e := event.(type) {
	case *startergrant.StartedEvent:
		state.Player = e.Player
		state.Status = StarterGrantInProgress
	case *startergrant.CollectionRecordedEvent, *startergrant.DeckRecordedEvent:
		state.Status = StarterGrantInProgress
	case *startergrant.CompletedEvent:
		state.Status = StarterGrantCompleted
	}
	return state
}

// 配布の状態を消すイベントは無い
func (state PlayerStarterGrant) IsTerminal() bool {
	return false
}
