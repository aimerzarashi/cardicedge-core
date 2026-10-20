package readmodel

import (
	"maps"
	"time"

	"github.com/google/uuid"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/card"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/starterdeck"
)

// starter_deck_lists の 1 行。個別の列（SortName・TotalCount）は並べ替え・絞り込み用、Metadata は表示用
type StarterDeckList struct {
	StarterDeckID uuid.UUID
	SortName      string
	Metadata      StarterDeckListMetadata
	// 入れたカードと枚数（0 のカードは持たない）。イベントはカードを内部 ID で持ち、Evolve は DB を読めないため、
	// 書くときに repository が公開 ID に引き直して Metadata.Cards にする
	Cards map[starterdeck.CardRef]int
	// 集約が削除された。DB には持たず、投影が行を消すかを決めるのに使う
	IsDeleted bool
}

// 日時はイベントの発生時刻から作る（DB の現在時刻は使わない）。Cards は repository が書くときに埋める
type StarterDeckListMetadata struct {
	ID       uuid.UUID `json:"id"`
	Name     string    `json:"name"`
	Revision int64     `json:"revision"`
	// カード名・種類は書き写さない（カードの名前の変更のたびに直す必要が出ないよう、読むときに card_lists から引く。core#24）
	Cards        []StarterDeckListCard `json:"cards"`
	RegisteredAt time.Time             `json:"registered_at"`
	UpdatedAt    time.Time             `json:"updated_at"`
}

type StarterDeckListCard struct {
	CardID uuid.UUID `json:"card_id"`
	Count  int       `json:"count"`
}

func (state StarterDeckList) TotalCount() int {
	total := 0
	for _, count := range state.Cards {
		total += count
	}
	return total
}

// スターターデッキのイベントだけを扱い、ほかは状態をそのまま返す。状態を書き換えないよう、カードの一覧は写してから変える
func (state StarterDeckList) Evolve(event any) StarterDeckList {
	switch e := event.(type) {
	case *starterdeck.RegisteredEvent:
		occurredAt := e.GetMeta().OccurredAt
		state.StarterDeckID = e.PublicID
		state.SortName = card.NormalizeName(e.Name.String())
		state.Metadata = StarterDeckListMetadata{
			ID: e.PublicID, Name: e.Name.String(), Revision: e.GetMeta().AggregateVersion,
			RegisteredAt: occurredAt, UpdatedAt: occurredAt,
		}
		state.Cards = map[starterdeck.CardRef]int{}
	case *starterdeck.RenamedEvent:
		state.SortName = card.NormalizeName(e.Name.String())
		state.Metadata.Name = e.Name.String()
		state = state.touched(e.GetMeta().AggregateVersion, e.GetMeta().OccurredAt)
	case *starterdeck.CardCountsChangedEvent:
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
	case *starterdeck.DeletedEvent:
		state.IsDeleted = true
	}
	return state
}

func (state StarterDeckList) touched(revision int64, at time.Time) StarterDeckList {
	state.Metadata.Revision = revision
	state.Metadata.UpdatedAt = at
	return state
}

func (state StarterDeckList) IsTerminal() bool {
	return state.IsDeleted
}
