package query

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/pagination"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/studio/app/input"
)

// 公開 ID に当たるスターターデッキが read model に無い（無い・削除した・まだ投影されていない）
var ErrStarterDeckNotFound = errors.New("query: starter deck not found")

// uif に read model の作りを見せず、表示に要る値だけを返す
type StarterDeck struct {
	ID         uuid.UUID
	Name       string
	Cards      []StarterDeckCard
	TotalCount int
	// 集約の版（画面が変える操作に渡す）。投影が遅れているあいだは古いことがある（internal/eventstore/README.md）
	Revision     int64
	RegisteredAt time.Time
	UpdatedAt    time.Time
}

// カード名・種類は持たない（uif が CardList.Get で引く）
type StarterDeckCard struct {
	CardID uuid.UUID
	Count  int
}

type StarterDeckFilter struct {
	NameContains *string
}

// 並び順（正規化した名前 → 公開 ID）の位置。項目名を 1 文字にする理由は CardListCursor と同じ
type StarterDeckListCursor struct {
	SortName      string    `json:"n"`
	StarterDeckID uuid.UUID `json:"i"`
}

func (c StarterDeckListCursor) Validate() error {
	if c.SortName == "" || c.StarterDeckID == uuid.Nil {
		return errors.New("sort name and starter deck id are required")
	}
	return nil
}

// 読み出しの関数に渡す条件。値は app 層で確かめ・正規化した後のもの
type ListStarterDecksCondition struct {
	NameContains string
	After        *StarterDeckListCursor
	Limit        int
}

type StarterDeckListRow struct {
	StarterDeck StarterDeck
	Cursor      StarterDeckListCursor
}

type ListStarterDecksFunc func(ctx context.Context, condition ListStarterDecksCondition) ([]StarterDeckListRow, error)

// 無ければ ErrStarterDeckNotFound
type GetStarterDeckFunc func(ctx context.Context, id uuid.UUID) (StarterDeck, error)

type StarterDeckList struct {
	listStarterDecks ListStarterDecksFunc
	getStarterDeck   GetStarterDeckFunc
}

func NewStarterDeckList(listStarterDecks ListStarterDecksFunc, getStarterDeck GetStarterDeckFunc) *StarterDeckList {
	return &StarterDeckList{listStarterDecks: listStarterDecks, getStarterDeck: getStarterDeck}
}

// 入力の誤りは input.Errors（まとめて返す）。項目名は GraphQL の引数・入力の項目名
func (q *StarterDeckList) List(ctx context.Context, filter StarterDeckFilter, first *int, after *string) (pagination.Connection[StarterDeck], error) {
	invalid := input.Errors{}
	page, err := pagination.NewCondition[StarterDeckListCursor](first, after)
	if errors.Is(err, pagination.ErrFirstOutOfRange) {
		invalid = append(invalid, &input.OutOfRange{Field: "first", Min: pagination.MinFirst, Max: pagination.MaxFirst})
	}
	if errors.Is(err, pagination.ErrInvalidCursor) {
		invalid = append(invalid, &input.InvalidFormat{Field: "after"})
	}
	if err != nil && len(invalid) == 0 {
		return pagination.Connection[StarterDeck]{}, err
	}
	// 名前の比べ方・上限はカード名と同じ（starterdeck.NameKey・MaxNameLength）なので、カードの一覧と同じ確かめ方を使う
	nameContains, nameErr := normalizeNameContains(filter.NameContains)
	if nameErr != nil {
		invalid = append(invalid, nameErr)
	}
	if len(invalid) > 0 {
		return pagination.Connection[StarterDeck]{}, invalid
	}
	condition := ListStarterDecksCondition{
		NameContains: nameContains,
		After:        page.After,
		Limit:        page.FetchLimit(),
	}
	rows, err := q.listStarterDecks(ctx, condition)
	if err != nil {
		return pagination.Connection[StarterDeck]{}, err
	}
	rowConnection, err := pagination.BuildConnection(rows, page, func(row StarterDeckListRow) StarterDeckListCursor {
		return row.Cursor
	})
	if err != nil {
		return pagination.Connection[StarterDeck]{}, err
	}
	connection := pagination.Connection[StarterDeck]{
		Edges:    make([]pagination.Edge[StarterDeck], 0, len(rowConnection.Edges)),
		PageInfo: rowConnection.PageInfo,
	}
	for _, edge := range rowConnection.Edges {
		connection.Edges = append(connection.Edges, pagination.Edge[StarterDeck]{
			Node:   edge.Node.StarterDeck,
			Cursor: edge.Cursor,
		})
	}
	return connection, nil
}

// 無ければ ErrStarterDeckNotFound
func (q *StarterDeckList) Get(ctx context.Context, id uuid.UUID) (StarterDeck, error) {
	return q.getStarterDeck(ctx, id)
}
