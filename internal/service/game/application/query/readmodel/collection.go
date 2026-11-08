package readmodel

import (
	"slices"
	"time"

	"github.com/google/uuid"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/card"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/collection"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/player"
)

// collections の 1 行。所持品の公開 ID と持ち主は、イベントが内部 ID で持つ（イベントに公開 ID は無い）ため、書くときに repository が公開用の ID に引き直す
type Collection struct {
	Collection collection.ID
	Player     player.ID
	// 持っているカードと枚数。同じカードは 1 つにまとめ（足すたびに枚数を足す）、初めて持った順に並べる。
	// 枚数を int で持つのは、足した合計をイベントの枚数の型（card.OwnedQuantity）の生成関数に通さず、そのまま足すため
	Cards        []CollectionCard
	RegisteredAt time.Time
	UpdatedAt    time.Time
}

// 持っているカード 1 種類。カードは game の内部 ID で持ち、書くときに repository が公開 ID に引き直す
type CollectionCard struct {
	Card  card.Ref
	Count int
}

// 持っているカード 1 種類の、書く形。カードは公開 ID で持ち、名前・種類は書き写さない（読むときに写した cards から引く）
type CollectionCardMetadata struct {
	CardID uuid.UUID `json:"card_id"`
	Count  int       `json:"count"`
}

// 日時はイベントの発生時刻から作る（DB の現在時刻は使わない）
type CollectionMetadata struct {
	// 空のまま。書くときに repository が公開 ID を入れる
	ID uuid.UUID `json:"id"`
	// 持っているカードの公開 ID と枚数（初めて持った順）。空のまま。書くときに repository が Cards から入れる
	Cards        []CollectionCardMetadata `json:"cards"`
	RegisteredAt time.Time                `json:"registered_at"`
	UpdatedAt    time.Time                `json:"updated_at"`
	// 操作した人は持ち主のプレイヤー（デッキと同じ。所持品は配布や戦利品の流れが変えるが、プレイヤーのものとして扱う）。
	// 空のまま。書くときに repository が持ち主の公開 ID を入れる
	RegisteredBy uuid.UUID `json:"registered_by"`
	UpdatedBy    uuid.UUID `json:"updated_by"`
}

func (state Collection) Metadata() CollectionMetadata {
	return CollectionMetadata{RegisteredAt: state.RegisteredAt, UpdatedAt: state.UpdatedAt}
}

// 所持品のイベントだけを扱い、ほかは状態をそのまま返す。渡された状態の Cards は書き換えない
func (state Collection) Evolve(event any) Collection {
	switch e := event.(type) {
	case *collection.CreatedEvent:
		state.Collection = e.GetMeta().EntityID
		state.Player = e.Player
		state.Cards = acquire(nil, e.Cards)
		state.RegisteredAt = e.GetMeta().OccurredAt
		state.UpdatedAt = e.GetMeta().OccurredAt
	case *collection.CardsAcquiredEvent:
		state.Cards = acquire(state.Cards, e.Cards)
		state.UpdatedAt = e.GetMeta().OccurredAt
	}
	return state
}

func acquire(current []CollectionCard, acquired []collection.Card) []CollectionCard {
	cards := slices.Clone(current)
	if cards == nil {
		cards = []CollectionCard{}
	}
	for _, c := range acquired {
		index := slices.IndexFunc(cards, func(held CollectionCard) bool {
			return held.Card == c.Card
		})
		if index < 0 {
			cards = append(cards, CollectionCard{Card: c.Card, Count: c.Quantity.Int()})
			continue
		}
		cards[index].Count += c.Quantity.Int()
	}
	return cards
}

// 所持品を消すイベントは無い
func (state Collection) IsTerminal() bool {
	return false
}
