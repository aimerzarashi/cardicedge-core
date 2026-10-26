// read model の状態の型と、イベントから状態を作る Evolve（投影の宣言は internal/service/studio、DB の読み書きは infrastructure/repository）
package readmodel

import (
	"fmt"
	"time"

	"github.com/google/uuid"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain"
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

// cards.status の値（準備中・公開中・使用不可）。domain の card.Status を使わないのは、read model の型を application 層の
// 外（presentation 層）に見せるため、ドメインの型への依存を持ち込まないように（CardType と同じ）
type CardStatus string

const (
	CardStatusDraft     CardStatus = "draft"
	CardStatusPublished CardStatus = "published"
	CardStatusRetired   CardStatus = "retired"
)

// DB の cards.status など、外から来た値を状態にする
func NewCardStatus(value string) (CardStatus, error) {
	status := CardStatus(value)
	switch status {
	case CardStatusDraft, CardStatusPublished, CardStatusRetired:
		return status, nil
	default:
		return "", fmt.Errorf("readmodel: unknown card status: %q", value)
	}
}

// cards の 1 行。個別の列（CardType・Status・SortName）は並べ替え・絞り込み用、Metadata は表示用。
// 種類は列だけに持つ（並べ替えに必ず要り、読み出しで毎回取るため。表示用にも持つと、同じ事実が 2 か所になる）。
// 状態は絞り込みに使う列と、表示に使う metadata の両方に持つ（0008 の決まり）
type Card struct {
	CardID   uuid.UUID
	CardType CardType
	Status   CardStatus
	SortName string
	Metadata CardMetadata
	// 集約が削除された。DB には持たず、投影が行を消すかを決めるのに使う。削除のイベントを足したら Evolve で立てる
	IsDeleted bool
}

// 日時はイベントの発生時刻から作る（DB の現在時刻は使わない）
type CardMetadata struct {
	ID     uuid.UUID  `json:"id"`
	Name   string     `json:"name"`
	Status CardStatus `json:"status"`
	// 画面が操作（公開する・使用不可にする）の入力に渡す、集約の版
	Revision     int64     `json:"revision"`
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
		state.SortName = domain.NormalizeName(e.Name.String())
		state.Status = CardStatusDraft
		state.Metadata = CardMetadata{
			ID: e.PublicID, Name: e.Name.String(), Status: CardStatusDraft, Revision: e.GetMeta().AggregateVersion,
			RegisteredAt: occurredAt, UpdatedAt: occurredAt,
		}
	case *item.RegisteredEvent:
		occurredAt := e.GetMeta().OccurredAt
		state.CardID = e.PublicID
		state.CardType = CardTypeItem
		state.SortName = domain.NormalizeName(e.Name.String())
		state.Status = CardStatusDraft
		state.Metadata = CardMetadata{
			ID: e.PublicID, Name: e.Name.String(), Status: CardStatusDraft, Revision: e.GetMeta().AggregateVersion,
			RegisteredAt: occurredAt, UpdatedAt: occurredAt,
		}
	case *spell.RegisteredEvent:
		occurredAt := e.GetMeta().OccurredAt
		state.CardID = e.PublicID
		state.CardType = CardTypeSpell
		state.SortName = domain.NormalizeName(e.Name.String())
		state.Status = CardStatusDraft
		state.Metadata = CardMetadata{
			ID: e.PublicID, Name: e.Name.String(), Status: CardStatusDraft, Revision: e.GetMeta().AggregateVersion,
			RegisteredAt: occurredAt, UpdatedAt: occurredAt,
		}
	case *creature.PublishedEvent:
		state = state.changeStatus(CardStatusPublished, e.GetMeta().AggregateVersion, e.GetMeta().OccurredAt)
	case *creature.RetiredEvent:
		state = state.changeStatus(CardStatusRetired, e.GetMeta().AggregateVersion, e.GetMeta().OccurredAt)
	case *item.PublishedEvent:
		state = state.changeStatus(CardStatusPublished, e.GetMeta().AggregateVersion, e.GetMeta().OccurredAt)
	case *item.RetiredEvent:
		state = state.changeStatus(CardStatusRetired, e.GetMeta().AggregateVersion, e.GetMeta().OccurredAt)
	case *spell.PublishedEvent:
		state = state.changeStatus(CardStatusPublished, e.GetMeta().AggregateVersion, e.GetMeta().OccurredAt)
	case *spell.RetiredEvent:
		state = state.changeStatus(CardStatusRetired, e.GetMeta().AggregateVersion, e.GetMeta().OccurredAt)
	}
	return state
}

// 3 種類のイベントの EventMeta は型が別（集約の ID の型が違う）なので、使う値（版と発生時刻）だけ受け取る
func (state Card) changeStatus(status CardStatus, revision int64, occurredAt time.Time) Card {
	state.Status = status
	state.Metadata.Status = status
	state.Metadata.Revision = revision
	state.Metadata.UpdatedAt = occurredAt
	return state
}

func (state Card) IsTerminal() bool {
	return state.IsDeleted
}
