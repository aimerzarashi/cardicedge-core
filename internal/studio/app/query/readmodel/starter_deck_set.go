package readmodel

import (
	"time"

	"github.com/google/uuid"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/starterdeckset"
)

// starter_deck_sets の 1 行（ゲーム全体で 1 行）
type StarterDeckSet struct {
	StarterDeckSetID uuid.UUID
	Revision         int64
	UpdatedAt        time.Time
	// 写したスターターデッキ。イベントはカードを内部 ID で持ち、Evolve は DB を読めないため、書くときに repository が
	// カードを公開 ID に引き直して metadata にする（プリセットデッキの read model と同じ）
	Decks []starterdeckset.Deck
}

// 表示に使う値。日時はイベントの発生時刻から作る
type StarterDeckSetMetadata struct {
	ID        uuid.UUID            `json:"id"`
	Revision  int64                `json:"revision"`
	UpdatedAt time.Time            `json:"updated_at"`
	Decks     []StarterDeckSetDeck `json:"decks"`
}

// カード名・種類は書き写さない（読むときに cards から引く。プリセットデッキと同じ）
type StarterDeckSetDeck struct {
	Name   string               `json:"name"`
	Cards  []DeckCard           `json:"cards"`
	Source StarterDeckSetSource `json:"source"`
}

type StarterDeckSetSource struct {
	PresetDeckID uuid.UUID `json:"preset_deck_id"`
	Revision     int64     `json:"revision"`
}

// スターターデッキセットのイベントだけを扱い、ほかは状態をそのまま返す
func (state StarterDeckSet) Evolve(event any) StarterDeckSet {
	if e, ok := event.(*starterdeckset.DecksSetEvent); ok {
		state.StarterDeckSetID = e.PublicID
		state.Revision = e.GetMeta().AggregateVersion
		state.UpdatedAt = e.GetMeta().OccurredAt
		state.Decks = e.Decks
	}
	return state
}

// 削除のイベントは無い（未設定に戻す操作は作らない。core#25）
func (state StarterDeckSet) IsTerminal() bool {
	return false
}
