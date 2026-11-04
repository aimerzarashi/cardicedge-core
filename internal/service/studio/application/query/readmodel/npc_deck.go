package readmodel

import (
	"fmt"
	"maps"
	"time"

	"github.com/google/uuid"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/card"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/npcdeck"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/operator"
)

// npc_decks.status の値（準備中・公開中・使用不可）。domain の npcdeck.Status を使わない理由は CardStatus と同じ
type NPCDeckStatus string

const (
	NPCDeckStatusDraft     NPCDeckStatus = "draft"
	NPCDeckStatusPublished NPCDeckStatus = "published"
	NPCDeckStatusRetired   NPCDeckStatus = "retired"
)

// DB の npc_decks.status など、外から来た値を状態にする
func NewNPCDeckStatus(value string) (NPCDeckStatus, error) {
	status := NPCDeckStatus(value)
	switch status {
	case NPCDeckStatusDraft, NPCDeckStatusPublished, NPCDeckStatusRetired:
		return status, nil
	default:
		return "", fmt.Errorf("readmodel: unknown npc deck status: %q", value)
	}
}

// npc_decks の 1 行。個別の列（SortName・Status・TotalCount）は並べ替え・絞り込み用、Metadata は表示用
type NPCDeck struct {
	NPCDeckID uuid.UUID
	SortName  string
	Status    NPCDeckStatus
	Metadata  NPCDeckMetadata
	// イベントはオペレーターを内部 ID で持つため、内部 ID で持ち、書くときに公開 ID に引き直す（AvatarWriter と同じ）
	RegisteredBy operator.ID
	UpdatedBy    operator.ID
	// 入れたカードと枚数（0 のカードは持たない）。書くときに repository が公開 ID に引き直して Metadata.Cards にする（PresetDeck と同じ）
	Cards map[card.Ref]int
	// 集約が削除された。DB には持たず、投影が行を消すかを決めるのに使う
	IsDeleted bool
}

// 日時はイベントの発生時刻から作る（DB の現在時刻は使わない）。Cards は repository が書くときに埋める
type NPCDeckMetadata struct {
	ID       uuid.UUID     `json:"id"`
	Name     string        `json:"name"`
	Status   NPCDeckStatus `json:"status"`
	Revision int64         `json:"revision"`
	// カード名・種類は書き写さない（PresetDeckMetadata と同じ）
	Cards        []DeckCard `json:"cards"`
	RegisteredAt time.Time  `json:"registered_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
	// 投影では空のまま。書くときに repository が RegisteredBy・UpdatedBy から入れる
	RegisteredBy uuid.UUID `json:"registered_by"`
	UpdatedBy    uuid.UUID `json:"updated_by"`
}

func (state NPCDeck) TotalCount() int {
	total := 0
	for _, count := range state.Cards {
		total += count
	}
	return total
}

// NPC のデッキのイベントだけを扱い、ほかは状態をそのまま返す。状態を書き換えないよう、カードの一覧は写してから変える
func (state NPCDeck) Evolve(event any) NPCDeck {
	switch e := event.(type) {
	case *npcdeck.RegisteredEvent:
		occurredAt := e.GetMeta().OccurredAt
		state.NPCDeckID = e.PublicID
		state.SortName = domain.NormalizeName(e.Name.String())
		state.Status = NPCDeckStatusDraft
		state.Metadata = NPCDeckMetadata{
			ID: e.PublicID, Name: e.Name.String(), Status: NPCDeckStatusDraft, Revision: e.GetMeta().AggregateVersion,
			RegisteredAt: occurredAt, UpdatedAt: occurredAt,
		}
		state.RegisteredBy = e.RegisteredBy
		state.UpdatedBy = e.RegisteredBy
		state.Cards = make(map[card.Ref]int, len(e.Cards))
		for _, entry := range e.Cards {
			state.Cards[entry.Card] = entry.Quantity.Int()
		}
	case *npcdeck.RenamedEvent:
		state.SortName = domain.NormalizeName(e.Name.String())
		state.Metadata.Name = e.Name.String()
		state = state.touched(e.GetMeta().AggregateVersion, e.GetMeta().OccurredAt, e.RenamedBy)
	case *npcdeck.CardCountsChangedEvent:
		cards := maps.Clone(state.Cards)
		for _, change := range e.Changes {
			if count := cards[change.Card] + change.Delta; count == 0 {
				delete(cards, change.Card)
			} else {
				cards[change.Card] = count
			}
		}
		state.Cards = cards
		state = state.touched(e.GetMeta().AggregateVersion, e.GetMeta().OccurredAt, e.ChangedBy)
	case *npcdeck.PublishedEvent:
		state = state.changeStatus(NPCDeckStatusPublished, e.GetMeta().AggregateVersion, e.GetMeta().OccurredAt, e.PublishedBy)
	case *npcdeck.RetiredEvent:
		state = state.changeStatus(NPCDeckStatusRetired, e.GetMeta().AggregateVersion, e.GetMeta().OccurredAt, e.RetiredBy)
	case *npcdeck.DeletedEvent:
		state.IsDeleted = true
	}
	return state
}

func (state NPCDeck) changeStatus(status NPCDeckStatus, revision int64, at time.Time, by operator.ID) NPCDeck {
	state.Status = status
	state.Metadata.Status = status
	return state.touched(revision, at, by)
}

func (state NPCDeck) touched(revision int64, at time.Time, by operator.ID) NPCDeck {
	state.Metadata.Revision = revision
	state.Metadata.UpdatedAt = at
	state.UpdatedBy = by
	return state
}

func (state NPCDeck) IsTerminal() bool {
	return state.IsDeleted
}
