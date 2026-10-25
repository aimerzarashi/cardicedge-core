package readmodel

import (
	"maps"
	"time"

	"github.com/google/uuid"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/presetdeck"
)

// preset_decks の 1 行。個別の列（SortName・TotalCount）は並べ替え・絞り込み用、Metadata は表示用
type PresetDeck struct {
	PresetDeckID uuid.UUID
	SortName     string
	Metadata     PresetDeckMetadata
	// 入れたカードと枚数（0 のカードは持たない）。イベントはカードを内部 ID で持ち、Evolve は DB を読めないため、
	// 書くときに repository が公開 ID に引き直して Metadata.Cards にする
	Cards map[presetdeck.CardRef]int
	// 集約が削除された。DB には持たず、投影が行を消すかを決めるのに使う
	IsDeleted bool
}

// 日時はイベントの発生時刻から作る（DB の現在時刻は使わない）。Cards は repository が書くときに埋める
type PresetDeckMetadata struct {
	ID       uuid.UUID `json:"id"`
	Name     string    `json:"name"`
	Revision int64     `json:"revision"`
	// カード名・種類は書き写さない（カードの名前の変更のたびに直す必要が出ないよう、読むときに cards から引く。core#24）
	Cards        []DeckCard `json:"cards"`
	RegisteredAt time.Time  `json:"registered_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
}

type DeckCard struct {
	CardID uuid.UUID `json:"card_id"`
	Count  int       `json:"count"`
}

func (state PresetDeck) TotalCount() int {
	total := 0
	for _, count := range state.Cards {
		total += count
	}
	return total
}

// プリセットデッキのイベントだけを扱い、ほかは状態をそのまま返す。状態を書き換えないよう、カードの一覧は写してから変える
func (state PresetDeck) Evolve(event any) PresetDeck {
	switch e := event.(type) {
	case *presetdeck.RegisteredEvent:
		occurredAt := e.GetMeta().OccurredAt
		state.PresetDeckID = e.PublicID
		state.SortName = domain.NormalizeName(e.Name.String())
		state.Metadata = PresetDeckMetadata{
			ID: e.PublicID, Name: e.Name.String(), Revision: e.GetMeta().AggregateVersion,
			RegisteredAt: occurredAt, UpdatedAt: occurredAt,
		}
		state.Cards = map[presetdeck.CardRef]int{}
	case *presetdeck.RenamedEvent:
		state.SortName = domain.NormalizeName(e.Name.String())
		state.Metadata.Name = e.Name.String()
		state = state.touched(e.GetMeta().AggregateVersion, e.GetMeta().OccurredAt)
	case *presetdeck.CardCountsChangedEvent:
		cards := maps.Clone(state.Cards)
		for _, change := range e.Changes {
			if count := cards[change.Card] + change.Delta; count == 0 {
				delete(cards, change.Card)
			} else {
				cards[change.Card] = count
			}
		}
		state.Cards = cards
		state = state.touched(e.GetMeta().AggregateVersion, e.GetMeta().OccurredAt)
	case *presetdeck.DeletedEvent:
		state.IsDeleted = true
	}
	return state
}

func (state PresetDeck) touched(revision int64, at time.Time) PresetDeck {
	state.Metadata.Revision = revision
	state.Metadata.UpdatedAt = at
	return state
}

func (state PresetDeck) IsTerminal() bool {
	return state.IsDeleted
}
