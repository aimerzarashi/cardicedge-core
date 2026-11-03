package readmodel

import (
	"fmt"
	"maps"
	"time"

	"github.com/google/uuid"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/grantdeck"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/operator"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/presetdeck"
)

// grant_decks.status の値（準備中・公開中・使用不可）。domain の grantdeck.Status を使わない理由は CardStatus と同じ
type GrantDeckStatus string

const (
	GrantDeckStatusDraft     GrantDeckStatus = "draft"
	GrantDeckStatusPublished GrantDeckStatus = "published"
	GrantDeckStatusRetired   GrantDeckStatus = "retired"
)

// DB の grant_decks.status など、外から来た値を状態にする
func NewGrantDeckStatus(value string) (GrantDeckStatus, error) {
	status := GrantDeckStatus(value)
	switch status {
	case GrantDeckStatusDraft, GrantDeckStatusPublished, GrantDeckStatusRetired:
		return status, nil
	default:
		return "", fmt.Errorf("readmodel: unknown grant deck status: %q", value)
	}
}

// grant_decks の 1 行。個別の列（SortName・Status・TotalCount）は並べ替え・絞り込み用、Metadata は表示用
type GrantDeck struct {
	GrantDeckID uuid.UUID
	SortName    string
	Status      GrantDeckStatus
	Metadata    GrantDeckMetadata
	// イベントはオペレーターを内部 ID で持つため、内部 ID で持ち、書くときに公開 ID に引き直す（NPCAvatar と同じ）
	RegisteredBy operator.ID
	UpdatedBy    operator.ID
	// 入れたカードと枚数（0 のカードは持たない）。書くときに repository が公開 ID に引き直して Metadata.Cards にする（PresetDeck と同じ）
	Cards map[presetdeck.CardRef]int
	// 集約が削除された。DB には持たず、投影が行を消すかを決めるのに使う
	IsDeleted bool
}

// 日時はイベントの発生時刻から作る（DB の現在時刻は使わない）。Cards は repository が書くときに埋める
type GrantDeckMetadata struct {
	ID       uuid.UUID       `json:"id"`
	Name     string          `json:"name"`
	Status   GrantDeckStatus `json:"status"`
	Revision int64           `json:"revision"`
	// カード名・種類は書き写さない（PresetDeckMetadata と同じ）
	Cards []DeckCard `json:"cards"`
	// プリセットデッキから写して作ったときだけ。元は参照にしない（消してよい）ため、公開 ID で持つ
	Source       *GrantDeckSource `json:"source,omitempty"`
	RegisteredAt time.Time        `json:"registered_at"`
	UpdatedAt    time.Time        `json:"updated_at"`
	// 投影では空のまま。書くときに repository が RegisteredBy・UpdatedBy から公開 ID を入れる
	RegisteredBy uuid.UUID `json:"registered_by"`
	UpdatedBy    uuid.UUID `json:"updated_by"`
}

type GrantDeckSource struct {
	PresetDeckID uuid.UUID `json:"preset_deck_id"`
	Revision     int64     `json:"revision"`
}

func (state GrantDeck) TotalCount() int {
	total := 0
	for _, count := range state.Cards {
		total += count
	}
	return total
}

// 配布デッキのイベントだけを扱い、ほかは状態をそのまま返す。状態を書き換えないよう、カードの一覧は写してから変える
func (state GrantDeck) Evolve(event any) GrantDeck {
	switch e := event.(type) {
	case *grantdeck.RegisteredEvent:
		occurredAt := e.GetMeta().OccurredAt
		state.GrantDeckID = e.PublicID
		state.SortName = domain.NormalizeName(e.Name.String())
		state.Status = GrantDeckStatusDraft
		state.Metadata = GrantDeckMetadata{
			ID: e.PublicID, Name: e.Name.String(), Status: GrantDeckStatusDraft, Revision: e.GetMeta().AggregateVersion,
			RegisteredAt: occurredAt, UpdatedAt: occurredAt,
		}
		state.RegisteredBy = e.RegisteredBy
		state.UpdatedBy = e.RegisteredBy
		if e.Source != nil {
			state.Metadata.Source = &GrantDeckSource{PresetDeckID: e.Source.PresetDeckID(), Revision: e.Source.Revision()}
		}
		state.Cards = make(map[presetdeck.CardRef]int, len(e.Cards))
		for _, entry := range e.Cards {
			state.Cards[entry.Card] = entry.Count.Int()
		}
	case *grantdeck.RenamedEvent:
		state.SortName = domain.NormalizeName(e.Name.String())
		state.Metadata.Name = e.Name.String()
		state = state.touched(e.GetMeta().AggregateVersion, e.GetMeta().OccurredAt, e.RenamedBy)
	case *grantdeck.CardCountsChangedEvent:
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
	case *grantdeck.PublishedEvent:
		state = state.changeStatus(GrantDeckStatusPublished, e.GetMeta().AggregateVersion, e.GetMeta().OccurredAt, e.PublishedBy)
	case *grantdeck.RetiredEvent:
		state = state.changeStatus(GrantDeckStatusRetired, e.GetMeta().AggregateVersion, e.GetMeta().OccurredAt, e.RetiredBy)
	case *grantdeck.DeletedEvent:
		state.IsDeleted = true
	}
	return state
}

func (state GrantDeck) changeStatus(status GrantDeckStatus, revision int64, at time.Time, by operator.ID) GrantDeck {
	state.Status = status
	state.Metadata.Status = status
	return state.touched(revision, at, by)
}

func (state GrantDeck) touched(revision int64, at time.Time, by operator.ID) GrantDeck {
	state.Metadata.Revision = revision
	state.Metadata.UpdatedAt = at
	state.UpdatedBy = by
	return state
}

func (state GrantDeck) IsTerminal() bool {
	return state.IsDeleted
}
