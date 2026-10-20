package query

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/pagination"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/studio/app/input"
)

// 公開 ID に当たるプリセットデッキが read model に無い（無い・削除した・まだ投影されていない）
var ErrPresetDeckNotFound = errors.New("query: starter deck not found")

// uif に read model の作りを見せず、表示に要る値だけを返す
type PresetDeck struct {
	ID         uuid.UUID
	Name       string
	Cards      []PresetDeckCard
	TotalCount int
	// 集約の版（画面が変える操作に渡す）。投影が遅れているあいだは古いことがある（internal/eventstore/README.md）
	Revision     int64
	RegisteredAt time.Time
	UpdatedAt    time.Time
}

// カード名・種類は持たない（uif が CardList.Get で引く）
type PresetDeckCard struct {
	CardID uuid.UUID
	Count  int
}

type PresetDeckFilter struct {
	NameContains *string
}

// 並び順（正規化した名前 → 公開 ID）の位置。項目名を 1 文字にする理由は CardListCursor と同じ
type PresetDeckListCursor struct {
	SortName     string    `json:"n"`
	PresetDeckID uuid.UUID `json:"i"`
}

func (c PresetDeckListCursor) Validate() error {
	if c.SortName == "" || c.PresetDeckID == uuid.Nil {
		return errors.New("sort name and starter deck id are required")
	}
	return nil
}

// 読み出しの関数に渡す条件。値は app 層で確かめ・正規化した後のもの
type ListPresetDecksCondition struct {
	NameContains string
	After        *PresetDeckListCursor
	Limit        int
}

type PresetDeckListRow struct {
	PresetDeck PresetDeck
	Cursor     PresetDeckListCursor
}

type ListPresetDecksFunc func(ctx context.Context, condition ListPresetDecksCondition) ([]PresetDeckListRow, error)

// 無ければ ErrPresetDeckNotFound
type GetPresetDeckFunc func(ctx context.Context, id uuid.UUID) (PresetDeck, error)

type PresetDeckList struct {
	listPresetDecks ListPresetDecksFunc
	getPresetDeck   GetPresetDeckFunc
}

func NewPresetDeckList(listPresetDecks ListPresetDecksFunc, getPresetDeck GetPresetDeckFunc) *PresetDeckList {
	return &PresetDeckList{listPresetDecks: listPresetDecks, getPresetDeck: getPresetDeck}
}

// 入力の誤りは input.Errors（まとめて返す）。項目名は GraphQL の引数・入力の項目名
func (q *PresetDeckList) List(ctx context.Context, filter PresetDeckFilter, first *int, after *string) (pagination.Connection[PresetDeck], error) {
	invalid := input.Errors{}
	page, err := pagination.NewCondition[PresetDeckListCursor](first, after)
	if errors.Is(err, pagination.ErrFirstOutOfRange) {
		invalid = append(invalid, &input.OutOfRange{Field: "first", Min: pagination.MinFirst, Max: pagination.MaxFirst})
	}
	if errors.Is(err, pagination.ErrInvalidCursor) {
		invalid = append(invalid, &input.InvalidFormat{Field: "after"})
	}
	if err != nil && len(invalid) == 0 {
		return pagination.Connection[PresetDeck]{}, err
	}
	// 名前の比べ方・上限はカード名と同じ（presetdeck.NameKey・MaxNameLength）なので、カードの一覧と同じ確かめ方を使う
	nameContains, nameErr := normalizeNameContains(filter.NameContains)
	if nameErr != nil {
		invalid = append(invalid, nameErr)
	}
	if len(invalid) > 0 {
		return pagination.Connection[PresetDeck]{}, invalid
	}
	condition := ListPresetDecksCondition{
		NameContains: nameContains,
		After:        page.After,
		Limit:        page.FetchLimit(),
	}
	rows, err := q.listPresetDecks(ctx, condition)
	if err != nil {
		return pagination.Connection[PresetDeck]{}, err
	}
	rowConnection, err := pagination.BuildConnection(rows, page, func(row PresetDeckListRow) PresetDeckListCursor {
		return row.Cursor
	})
	if err != nil {
		return pagination.Connection[PresetDeck]{}, err
	}
	connection := pagination.Connection[PresetDeck]{
		Edges:    make([]pagination.Edge[PresetDeck], 0, len(rowConnection.Edges)),
		PageInfo: rowConnection.PageInfo,
	}
	for _, edge := range rowConnection.Edges {
		connection.Edges = append(connection.Edges, pagination.Edge[PresetDeck]{
			Node:   edge.Node.PresetDeck,
			Cursor: edge.Cursor,
		})
	}
	return connection, nil
}

// 無ければ ErrPresetDeckNotFound
func (q *PresetDeckList) Get(ctx context.Context, id uuid.UUID) (PresetDeck, error) {
	return q.getPresetDeck(ctx, id)
}
