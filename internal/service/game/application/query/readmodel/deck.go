package readmodel

import (
	"time"

	"github.com/google/uuid"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/deck"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/player"
)

// decks の 1 行。デッキの公開 ID と持ち主は、イベントが内部 ID で持つ（イベントに公開 ID は無い）ため、書くときに repository が公開用の ID に引き直す
type Deck struct {
	Deck         deck.ID
	Player       player.ID
	Name         string
	Cards        []DeckCard
	RegisteredAt time.Time
}

// 入れたカード 1 種類。カードは公開 ID で持ち、名前・種類は書き写さない（読むときに写した cards から引く）
type DeckCard struct {
	CardID uuid.UUID `json:"card_id"`
	Count  int       `json:"count"`
}

// 日時はイベントの発生時刻から作る（DB の現在時刻は使わない）
type DeckMetadata struct {
	// 空のまま。書くときに repository が公開 ID を入れる
	ID           uuid.UUID  `json:"id"`
	Name         string     `json:"name"`
	Cards        []DeckCard `json:"cards"`
	RegisteredAt time.Time  `json:"registered_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
	// 操作した人は持ち主のプレイヤー（アバターと同じ。デッキは配布の流れが作るが、プレイヤーのものとして作られ、更新も無い）。
	// 空のまま。書くときに repository が持ち主の公開 ID を入れる
	RegisteredBy uuid.UUID `json:"registered_by"`
	UpdatedBy    uuid.UUID `json:"updated_by"`
}

func (state Deck) Metadata() DeckMetadata {
	return DeckMetadata{Name: state.Name, Cards: state.Cards, RegisteredAt: state.RegisteredAt, UpdatedAt: state.RegisteredAt}
}

// デッキのイベントだけを扱い、ほかは状態をそのまま返す
func (state Deck) Evolve(event any) Deck {
	if e, ok := event.(*deck.CreatedEvent); ok {
		state.Deck = e.GetMeta().EntityID
		state.Player = e.Player
		state.Name = e.Name.String()
		state.Cards = make([]DeckCard, len(e.Cards))
		for i, card := range e.Cards {
			state.Cards[i] = DeckCard{CardID: card.CardID, Count: card.Count.Int()}
		}
		state.RegisteredAt = e.GetMeta().OccurredAt
	}
	return state
}

// デッキを消すイベントは無い
func (state Deck) IsTerminal() bool {
	return false
}
