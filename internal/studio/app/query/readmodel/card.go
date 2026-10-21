// read model の状態の型と、イベントから状態を作る Evolve（投影の宣言は cmd/app、DB の読み書きは inf/repository）
package readmodel

import (
	"fmt"
	"time"

	"github.com/google/uuid"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/card"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/creature"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/item"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/spell"
)

// cards.card_type の値。並び順を表す（集約の種別の id の大小に並び順の決まりを隠さないため。migrations/0010）。
// 素の数にしないのは、件数などほかの数と取り違えてもコンパイルが通らないように
type CardType int16

const (
	CardTypeCreature CardType = 1
	CardTypeItem     CardType = 2
	CardTypeSpell    CardType = 3
)

// DB の cards.card_type など、外から来た数を種類にする
func NewCardType(value int16) (CardType, error) {
	cardType := CardType(value)
	if cardType < CardTypeCreature || cardType > CardTypeSpell {
		return 0, fmt.Errorf("readmodel: unknown card type: %d", value)
	}
	return cardType, nil
}

// cards の 1 行。個別の列（CardType・SortName）は並べ替え・絞り込み用、Metadata は表示用。
// 種類は列だけに持つ（並べ替えに必ず要り、読み出しで毎回取るため。表示用にも持つと、同じ事実が 2 か所になる）
type Card struct {
	CardID   uuid.UUID
	CardType CardType
	SortName string
	Metadata CardMetadata
	// 集約が削除された。DB には持たず、投影が行を消すかを決めるのに使う。削除のイベントを足したら Evolve で立てる
	IsDeleted bool
}

// 日時はイベントの発生時刻から作る（DB の現在時刻は使わない）
type CardMetadata struct {
	ID           uuid.UUID `json:"id"`
	Name         string    `json:"name"`
	RegisteredAt time.Time `json:"registered_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// Creature・Item・Spell のイベントだけを扱い、ほかは状態をそのまま返す
func (state Card) Evolve(event any) Card {
	switch e := event.(type) {
	case *creature.RegisteredEvent:
		occurredAt := e.GetMeta().OccurredAt
		state.CardID = e.PublicID
		state.CardType = CardTypeCreature
		state.SortName = card.NormalizeName(e.Name.String())
		state.Metadata = CardMetadata{ID: e.PublicID, Name: e.Name.String(), RegisteredAt: occurredAt, UpdatedAt: occurredAt}
	case *item.RegisteredEvent:
		occurredAt := e.GetMeta().OccurredAt
		state.CardID = e.PublicID
		state.CardType = CardTypeItem
		state.SortName = card.NormalizeName(e.Name.String())
		state.Metadata = CardMetadata{ID: e.PublicID, Name: e.Name.String(), RegisteredAt: occurredAt, UpdatedAt: occurredAt}
	case *spell.RegisteredEvent:
		occurredAt := e.GetMeta().OccurredAt
		state.CardID = e.PublicID
		state.CardType = CardTypeSpell
		state.SortName = card.NormalizeName(e.Name.String())
		state.Metadata = CardMetadata{ID: e.PublicID, Name: e.Name.String(), RegisteredAt: occurredAt, UpdatedAt: occurredAt}
	}
	return state
}

func (state Card) IsTerminal() bool {
	return state.IsDeleted
}
