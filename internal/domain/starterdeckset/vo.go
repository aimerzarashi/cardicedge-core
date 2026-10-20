package starterdeckset

import (
	"cmp"
	"encoding/json"
	"errors"
	"slices"

	"github.com/google/uuid"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/presetdeck"
)

type ID int64

var entityType = domain.NewEntityType("starter_deck_set")

func (ID) EntityType() domain.EntityType {
	return entityType
}

func (id ID) Int64() int64 {
	return int64(id)
}

func NewID(value int64) (ID, error) {
	if value <= 0 {
		return 0, errors.New("starter deck set id must be positive")
	}
	return ID(value), nil
}

// スターターデッキの数。調整できるようにするのは core#23（それまでは決まった値）
const RequiredDeckCount = 2

// プリセットデッキから写したスターターデッキ。写したときの中身を持ち、元のプリセットデッキを後で直しても変わらない（core#25）。
// 作れるのは NewDeck だけ（スターターに選べないプリセットデッキから作らせないため）。項目を外に出さないのも同じ理由
type Deck struct {
	source Source
	name   presetdeck.Name
	// カードの順に並べる（同じ中身の写しを、同じ値として比べるため）
	cards []DeckCard
}

// どのプリセットデッキの、どの時点から写したか。元のプリセットデッキは消してよい（参照にしない）ため、
// 消えた後も読めるよう、内部 ID ではなく公開 ID で持つ（core#25）
type Source struct {
	presetDeckID uuid.UUID
	revision     int64
}

type DeckCard struct {
	card  presetdeck.CardRef
	count presetdeck.CardCount
}

// 選べるのは、合計がちょうど presetdeck.MaxTotalCount のプリセットデッキだけ。ほかの集約の状態なので Decide では見ず、
// app 層がプリセットデッキを Load して写すときに確かめる（カードの存在の確かめと同じ分け方）
func CheckSelectable(deck presetdeck.PresetDeck) error {
	if deck.TotalCount() != presetdeck.MaxTotalCount {
		return ErrDeckNotSelectable
	}
	return nil
}

// 選べなければ ErrDeckNotSelectable
func NewDeck(deck presetdeck.PresetDeck) (Deck, error) {
	if err := CheckSelectable(deck); err != nil {
		return Deck{}, err
	}
	cards := make([]DeckCard, 0, len(deck.Cards))
	for card, count := range deck.Cards {
		cards = append(cards, DeckCard{card: card, count: count})
	}
	slices.SortFunc(cards, compareDeckCards)
	return Deck{source: Source{presetDeckID: deck.PublicID, revision: deck.Version}, name: deck.Name, cards: cards}, nil
}

func (d Deck) Source() Source {
	return d.source
}

func (d Deck) Name() presetdeck.Name {
	return d.name
}

func (d Deck) Cards() []DeckCard {
	return slices.Clone(d.cards)
}

func (d Deck) equal(other Deck) bool {
	return d.source == other.source && d.name == other.name && slices.Equal(d.cards, other.cards)
}

func (s Source) PresetDeckID() uuid.UUID {
	return s.presetDeckID
}

func (s Source) Revision() int64 {
	return s.revision
}

func (c DeckCard) Card() presetdeck.CardRef {
	return c.card
}

func (c DeckCard) Count() presetdeck.CardCount {
	return c.count
}

func compareDeckCards(a, b DeckCard) int {
	return cmp.Or(cmp.Compare(a.card.EntityType(), b.card.EntityType()), cmp.Compare(a.card.ID(), b.card.ID()))
}

type deckJSON struct {
	Source sourceJSON      `json:"source"`
	Name   presetdeck.Name `json:"name"`
	Cards  []deckCardJSON  `json:"cards"`
}

type sourceJSON struct {
	PresetDeckID uuid.UUID `json:"preset_deck_id"`
	Revision     int64     `json:"revision"`
}

type deckCardJSON struct {
	Card  presetdeck.CardRef   `json:"card"`
	Count presetdeck.CardCount `json:"count"`
}

func (d Deck) MarshalJSON() ([]byte, error) {
	cards := make([]deckCardJSON, 0, len(d.cards))
	for _, c := range d.cards {
		cards = append(cards, deckCardJSON{Card: c.card, Count: c.count})
	}
	source := sourceJSON{PresetDeckID: d.source.presetDeckID, Revision: d.source.revision}
	return json.Marshal(deckJSON{Source: source, Name: d.name, Cards: cards})
}

// 保存済みのイベントから読み直すときは、選べるかを確かめ直さない（写したときに確かめた事実として読む。元はもう変わっているかもしれない）
func (d *Deck) UnmarshalJSON(data []byte) error {
	var v deckJSON
	if err := json.Unmarshal(data, &v); err != nil {
		return err
	}
	cards := make([]DeckCard, 0, len(v.Cards))
	for _, c := range v.Cards {
		cards = append(cards, DeckCard{card: c.Card, count: c.Count})
	}
	slices.SortFunc(cards, compareDeckCards)
	source := Source{presetDeckID: v.Source.PresetDeckID, revision: v.Source.Revision}
	*d = Deck{source: source, name: v.Name, cards: cards}
	return nil
}
