package query

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/pagination"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/studio/application/input"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/studio/application/query/readmodel"
)

// 公開 ID に当たる NPC のデッキが read model に無い（無い・削除した・まだ投影されていない）
var ErrNPCDeckNotFound = errors.New("query: npc deck not found")

// presentation 層に read model の作りを見せず、表示に要る値だけを返す
type NPCDeck struct {
	ID     uuid.UUID
	Name   string
	Status readmodel.NPCDeckStatus
	// カード名・種類は持たない（presentation 層が CardHandler.Get で引く）
	Cards      []PresetDeckCard
	TotalCount int
	// 集約の版（画面が変える操作に渡す）。投影が遅れているあいだは古いことがある（internal/eventstore/README.md）
	Revision     int64
	RegisteredAt time.Time
	// 操作したオペレーターの公開 ID。GraphQL にはまだ出さない（オペレーターの read model と、何を見せるかが決まっていないため）
	RegisteredBy uuid.UUID
	UpdatedAt    time.Time
	UpdatedBy    uuid.UUID
}

// Statuses が nil なら絞り込まない。空のリストは入力の誤り（CardFilter と同じ）
type NPCDeckFilter struct {
	NameContains *string
	Statuses     []readmodel.NPCDeckStatus
}

// 並び順（正規化した名前 → 公開 ID）の位置。項目名を 1 文字にする理由は CardListCursor と同じ
type NPCDeckListCursor struct {
	SortName  string    `json:"n"`
	NPCDeckID uuid.UUID `json:"i"`
}

func (c NPCDeckListCursor) Validate() error {
	if c.SortName == "" || c.NPCDeckID == uuid.Nil {
		return errors.New("sort name and npc deck id are required")
	}
	return nil
}

// 読み出しの関数に渡す条件。値は application 層で確かめ・正規化した後のもの
type ListNPCDecksCondition struct {
	NameContains string
	Statuses     []readmodel.NPCDeckStatus
	After        *NPCDeckListCursor
	Limit        int
}

type NPCDeckListRow struct {
	NPCDeck NPCDeck
	Cursor  NPCDeckListCursor
}

type ListNPCDecksFunc func(ctx context.Context, condition ListNPCDecksCondition) ([]NPCDeckListRow, error)

// 無ければ ErrNPCDeckNotFound
type GetNPCDeckFunc func(ctx context.Context, id uuid.UUID) (NPCDeck, error)

type NPCDeckHandler struct {
	listNPCDecks ListNPCDecksFunc
	getNPCDeck   GetNPCDeckFunc
}

func NewNPCDeckHandler(listNPCDecks ListNPCDecksFunc, getNPCDeck GetNPCDeckFunc) *NPCDeckHandler {
	return &NPCDeckHandler{listNPCDecks: listNPCDecks, getNPCDeck: getNPCDeck}
}

// 入力の誤りは input.Errors（まとめて返す）。項目名は GraphQL の引数・入力の項目名
func (q *NPCDeckHandler) List(ctx context.Context, filter NPCDeckFilter, first *int, after *string) (pagination.Connection[NPCDeck], error) {
	invalid := input.Errors{}
	page, err := pagination.NewCondition[NPCDeckListCursor](first, after)
	if errors.Is(err, pagination.ErrFirstOutOfRange) {
		invalid = append(invalid, &input.OutOfRange{Field: "first", Min: pagination.MinFirst, Max: pagination.MaxFirst})
	}
	if errors.Is(err, pagination.ErrInvalidCursor) {
		invalid = append(invalid, &input.InvalidFormat{Field: "after"})
	}
	if err != nil && len(invalid) == 0 {
		return pagination.Connection[NPCDeck]{}, err
	}
	nameContains, nameErr := normalizeNameContains(filter.NameContains)
	if nameErr != nil {
		invalid = append(invalid, nameErr)
	}
	if filter.Statuses != nil && len(filter.Statuses) == 0 {
		invalid = append(invalid, &input.Required{Field: "filter.statuses"})
	}
	if len(invalid) > 0 {
		return pagination.Connection[NPCDeck]{}, invalid
	}
	rows, err := q.listNPCDecks(ctx, ListNPCDecksCondition{
		NameContains: nameContains, Statuses: filter.Statuses, After: page.After, Limit: page.FetchLimit(),
	})
	if err != nil {
		return pagination.Connection[NPCDeck]{}, err
	}
	rowConnection, err := pagination.BuildConnection(rows, page, func(row NPCDeckListRow) NPCDeckListCursor {
		return row.Cursor
	})
	if err != nil {
		return pagination.Connection[NPCDeck]{}, err
	}
	connection := pagination.Connection[NPCDeck]{
		Edges:    make([]pagination.Edge[NPCDeck], 0, len(rowConnection.Edges)),
		PageInfo: rowConnection.PageInfo,
	}
	for _, edge := range rowConnection.Edges {
		connection.Edges = append(connection.Edges, pagination.Edge[NPCDeck]{Node: edge.Node.NPCDeck, Cursor: edge.Cursor})
	}
	return connection, nil
}

// 無ければ ErrNPCDeckNotFound
func (q *NPCDeckHandler) Get(ctx context.Context, id uuid.UUID) (NPCDeck, error) {
	return q.getNPCDeck(ctx, id)
}
