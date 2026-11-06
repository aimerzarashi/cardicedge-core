package query

import (
	"context"
	"errors"

	"github.com/google/uuid"
)

// 公開 ID に当たるデッキが、そのプレイヤーのものとして無い（無い・ほかのプレイヤーのもの。存在を明かさないため区別しない）
var ErrDeckNotFound = errors.New("query: deck not found")

// プレイヤーのデッキ。Cards は種類（Creature → Item → Spell）→ 名前の順（NPC のデッキと同じ並び）
type Deck struct {
	ID    uuid.UUID
	Name  string
	Cards []DeckCard
}

func (d Deck) TotalCount() int {
	total := 0
	for _, card := range d.Cards {
		total += card.Count
	}
	return total
}

// カードの種類ごとの枚数の合計（種類の数ではない）。入っていない種類は 0
type DeckTypeCounts struct {
	Creature int
	Item     int
	Spell    int
}

func NewDeckTypeCounts(creature, item, spell int) DeckTypeCounts {
	return DeckTypeCounts{Creature: creature, Item: item, Spell: spell}
}

// 画面で数え直さなくてよいように、ここで数える
func (d Deck) TypeCounts() DeckTypeCounts {
	var counts DeckTypeCounts
	for _, card := range d.Cards {
		switch card.Type {
		case CardTypeCreature:
			counts.Creature += card.Count
		case CardTypeItem:
			counts.Item += card.Count
		case CardTypeSpell:
			counts.Spell += card.Count
		}
	}
	return counts
}

// プレイヤーのデッキを作った順に返す
type ListDecksByPlayerFunc func(ctx context.Context, playerID uuid.UUID) ([]Deck, error)

// そのプレイヤーのデッキ 1 件。無い・ほかのプレイヤーのものなら ErrDeckNotFound
type GetDeckFunc func(ctx context.Context, playerID uuid.UUID, deckID uuid.UUID) (Deck, error)

type DeckHandler struct {
	listByPlayer ListDecksByPlayerFunc
	get          GetDeckFunc
}

func NewDeckHandler(listByPlayer ListDecksByPlayerFunc, get GetDeckFunc) *DeckHandler {
	return &DeckHandler{listByPlayer: listByPlayer, get: get}
}

// ほかのプレイヤーのデッキも無いのと同じ ErrDeckNotFound にする
func (q *DeckHandler) Get(ctx context.Context, playerID uuid.UUID, deckID uuid.UUID) (Deck, error) {
	return q.get(ctx, playerID, deckID)
}

// 投影が遅れていて 1 件も無いときも、空の一覧を返す（nil にしない。GraphQL の [Deck!]! は null を返せないため）
func (q *DeckHandler) ListByPlayer(ctx context.Context, playerID uuid.UUID) ([]Deck, error) {
	decks, err := q.listByPlayer(ctx, playerID)
	if err != nil {
		return nil, err
	}
	if decks == nil {
		return []Deck{}, nil
	}
	return decks, nil
}
