package query

import (
	"context"
	"errors"

	"github.com/google/uuid"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/pagination"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/game/application/input"
)

// プレイヤーが持っているカード 1 種類と枚数。どこから手に入れたか（出どころ）は見せない
type CollectionCard struct {
	Card  Card
	Count int
}

// Types が nil なら絞り込まない。空のリストは入力の誤り（何も選ばないのは、0 件を返すより選び忘れと考える。studio の cards と同じ）
type CollectionFilter struct {
	Types []CardType
}

// 並び順（種類 → 正規化した名前 → 公開 ID）の位置。項目名を 1 文字にする理由は studio のカーソルと同じ
type CollectionListCursor struct {
	CardType CardType  `json:"t"`
	SortName string    `json:"n"`
	CardID   uuid.UUID `json:"i"`
}

func (c CollectionListCursor) Validate() error {
	if c.CardType < CardTypeCreature || c.CardType > CardTypeSpell {
		return errors.New("card type is out of range")
	}
	if c.SortName == "" || c.CardID == uuid.Nil {
		return errors.New("sort name and card id are required")
	}
	return nil
}

// 読み出しの関数に渡す条件。値は application 層で確かめた後のもの
type ListCollectionCondition struct {
	Types []CardType
	After *CollectionListCursor
	Limit int
}

type CollectionListRow struct {
	Card   CollectionCard
	Cursor CollectionListCursor
}

// プレイヤーの所持カードを、種類 → 正規化した名前 → 公開 ID の順に Limit 件まで返す。
// 写していないカードがあればエラー（デッキと同じ）
type ListCollectionFunc func(ctx context.Context, playerID uuid.UUID, condition ListCollectionCondition) ([]CollectionListRow, error)

type CollectionHandler struct {
	list ListCollectionFunc
}

func NewCollectionHandler(list ListCollectionFunc) *CollectionHandler {
	return &CollectionHandler{list: list}
}

// 入力の誤りは input.Errors（まとめて返す）。項目名は GraphQL の引数・入力の項目名（入れ子は filter.types のように . でつなぐ）。
// 投影が遅れていて 1 枚も無いときも、空のページを返す
func (q *CollectionHandler) List(ctx context.Context, playerID uuid.UUID, filter CollectionFilter, first *int, after *string) (pagination.Connection[CollectionCard], error) {
	invalid := input.Errors{}
	page, err := pagination.NewCondition[CollectionListCursor](first, after)
	if errors.Is(err, pagination.ErrFirstOutOfRange) {
		invalid = append(invalid, &input.OutOfRange{Field: "first", Min: pagination.MinFirst, Max: pagination.MaxFirst})
	}
	if errors.Is(err, pagination.ErrInvalidCursor) {
		invalid = append(invalid, &input.InvalidFormat{Field: "after"})
	}
	if err != nil && len(invalid) == 0 {
		return pagination.Connection[CollectionCard]{}, err
	}
	if filter.Types != nil && len(filter.Types) == 0 {
		invalid = append(invalid, &input.Required{Field: "filter.types"})
	}
	if len(invalid) > 0 {
		return pagination.Connection[CollectionCard]{}, invalid
	}
	rows, err := q.list(ctx, playerID, ListCollectionCondition{Types: filter.Types, After: page.After, Limit: page.FetchLimit()})
	if err != nil {
		return pagination.Connection[CollectionCard]{}, err
	}
	rowConnection, err := pagination.BuildConnection(rows, page, func(row CollectionListRow) CollectionListCursor {
		return row.Cursor
	})
	if err != nil {
		return pagination.Connection[CollectionCard]{}, err
	}
	connection := pagination.Connection[CollectionCard]{
		Edges:    make([]pagination.Edge[CollectionCard], 0, len(rowConnection.Edges)),
		PageInfo: rowConnection.PageInfo,
	}
	for _, edge := range rowConnection.Edges {
		connection.Edges = append(connection.Edges, pagination.Edge[CollectionCard]{Node: edge.Node.Card, Cursor: edge.Cursor})
	}
	return connection, nil
}
