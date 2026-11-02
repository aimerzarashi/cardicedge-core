package readmodel

import (
	"fmt"
	"maps"
	"time"

	"github.com/google/uuid"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/operator"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/presetdeck"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/starterdeck"
)

// starter_decks.status の値（準備中・公開中・使用不可）。domain の starterdeck.Status を使わない理由は CardStatus と同じ
type StarterDeckStatus string

const (
	StarterDeckStatusDraft     StarterDeckStatus = "draft"
	StarterDeckStatusPublished StarterDeckStatus = "published"
	StarterDeckStatusRetired   StarterDeckStatus = "retired"
)

// DB の starter_decks.status など、外から来た値を状態にする
func NewStarterDeckStatus(value string) (StarterDeckStatus, error) {
	status := StarterDeckStatus(value)
	switch status {
	case StarterDeckStatusDraft, StarterDeckStatusPublished, StarterDeckStatusRetired:
		return status, nil
	default:
		return "", fmt.Errorf("readmodel: unknown starter deck status: %q", value)
	}
}

// starter_decks の 1 行。個別の列（SortName・Status・TotalCount）は並べ替え・絞り込み用、Metadata は表示用
type StarterDeck struct {
	StarterDeckID uuid.UUID
	SortName      string
	Status        StarterDeckStatus
	Metadata      StarterDeckMetadata
	// イベントはオペレーターを内部 ID で持つため、内部 ID で持ち、書くときに公開 ID に引き直す（NPCAvatar と同じ）
	RegisteredBy operator.ID
	UpdatedBy    operator.ID
	// 入れたカードと枚数（0 のカードは持たない）。書くときに repository が公開 ID に引き直して Metadata.Cards にする（PresetDeck と同じ）
	Cards map[presetdeck.CardRef]int
	// 集約が削除された。DB には持たず、投影が行を消すかを決めるのに使う
	IsDeleted bool
}

// 日時はイベントの発生時刻から作る（DB の現在時刻は使わない）。Cards は repository が書くときに埋める
type StarterDeckMetadata struct {
	ID       uuid.UUID         `json:"id"`
	Name     string            `json:"name"`
	Status   StarterDeckStatus `json:"status"`
	Revision int64             `json:"revision"`
	// カード名・種類は書き写さない（PresetDeckMetadata と同じ）
	Cards []DeckCard `json:"cards"`
	// プリセットデッキから写して作ったときだけ。元は参照にしない（消してよい）ため、公開 ID で持つ
	Source       *StarterDeckSource `json:"source,omitempty"`
	RegisteredAt time.Time          `json:"registered_at"`
	UpdatedAt    time.Time          `json:"updated_at"`
	// 投影では空のまま。書くときに repository が RegisteredBy・UpdatedBy から公開 ID を入れる
	RegisteredBy uuid.UUID `json:"registered_by"`
	UpdatedBy    uuid.UUID `json:"updated_by"`
}

type StarterDeckSource struct {
	PresetDeckID uuid.UUID `json:"preset_deck_id"`
	Revision     int64     `json:"revision"`
}

func (state StarterDeck) TotalCount() int {
	total := 0
	for _, count := range state.Cards {
		total += count
	}
	return total
}

// スターターデッキのイベントだけを扱い、ほかは状態をそのまま返す。状態を書き換えないよう、カードの一覧は写してから変える
func (state StarterDeck) Evolve(event any) StarterDeck {
	switch e := event.(type) {
	case *starterdeck.RegisteredEvent:
		occurredAt := e.GetMeta().OccurredAt
		state.StarterDeckID = e.PublicID
		state.SortName = domain.NormalizeName(e.Name.String())
		state.Status = StarterDeckStatusDraft
		state.Metadata = StarterDeckMetadata{
			ID: e.PublicID, Name: e.Name.String(), Status: StarterDeckStatusDraft, Revision: e.GetMeta().AggregateVersion,
			RegisteredAt: occurredAt, UpdatedAt: occurredAt,
		}
		state.RegisteredBy = e.RegisteredBy
		state.UpdatedBy = e.RegisteredBy
		if e.Source != nil {
			state.Metadata.Source = &StarterDeckSource{PresetDeckID: e.Source.PresetDeckID(), Revision: e.Source.Revision()}
		}
		state.Cards = make(map[presetdeck.CardRef]int, len(e.Cards))
		for _, entry := range e.Cards {
			state.Cards[entry.Card] = entry.Count.Int()
		}
	case *starterdeck.RenamedEvent:
		state.SortName = domain.NormalizeName(e.Name.String())
		state.Metadata.Name = e.Name.String()
		state = state.touched(e.GetMeta().AggregateVersion, e.GetMeta().OccurredAt, e.RenamedBy)
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
		state = state.touched(e.GetMeta().AggregateVersion, e.GetMeta().OccurredAt, e.ChangedBy)
	case *starterdeck.PublishedEvent:
		state = state.changeStatus(StarterDeckStatusPublished, e.GetMeta().AggregateVersion, e.GetMeta().OccurredAt, e.PublishedBy)
	case *starterdeck.RetiredEvent:
		state = state.changeStatus(StarterDeckStatusRetired, e.GetMeta().AggregateVersion, e.GetMeta().OccurredAt, e.RetiredBy)
	case *starterdeck.DeletedEvent:
		state.IsDeleted = true
	}
	return state
}

func (state StarterDeck) changeStatus(status StarterDeckStatus, revision int64, at time.Time, by operator.ID) StarterDeck {
	state.Status = status
	state.Metadata.Status = status
	return state.touched(revision, at, by)
}

func (state StarterDeck) touched(revision int64, at time.Time, by operator.ID) StarterDeck {
	state.Metadata.Revision = revision
	state.Metadata.UpdatedAt = at
	state.UpdatedBy = by
	return state
}

func (state StarterDeck) IsTerminal() bool {
	return state.IsDeleted
}
