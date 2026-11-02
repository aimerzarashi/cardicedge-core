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

// 公開 ID に当たる NPC のアバターが read model に無い（無い・削除した・まだ投影されていない）
var ErrNPCAvatarNotFound = errors.New("query: npc avatar not found")

// presentation 層に read model の作りを見せず、表示に要る値だけを返す
type NPCAvatar struct {
	ID     uuid.UUID
	Name   string
	Status readmodel.NPCAvatarStatus
	// 集約の版（画面が変える操作に渡す）。投影が遅れているあいだは古いことがある（internal/eventstore/README.md）
	Revision     int64
	RegisteredAt time.Time
	// 操作したオペレーターの公開 ID。GraphQL にはまだ出さない（オペレーターの read model と、何を見せるかが決まっていないため）
	RegisteredBy uuid.UUID
	UpdatedAt    time.Time
	UpdatedBy    uuid.UUID
	// 使う NPC のデッキの公開 ID（集約が持つ順）
	Decks []uuid.UUID
}

// Statuses が nil なら絞り込まない。空のリストは入力の誤り（CardFilter と同じ）
type NPCAvatarFilter struct {
	NameContains *string
	Statuses     []readmodel.NPCAvatarStatus
}

// 並び順（正規化した名前 → 公開 ID）の位置。項目名を 1 文字にする理由は CardListCursor と同じ
type NPCAvatarListCursor struct {
	SortName    string    `json:"n"`
	NPCAvatarID uuid.UUID `json:"i"`
}

func (c NPCAvatarListCursor) Validate() error {
	if c.SortName == "" || c.NPCAvatarID == uuid.Nil {
		return errors.New("sort name and npc avatar id are required")
	}
	return nil
}

// 読み出しの関数に渡す条件。値は application 層で確かめ・正規化した後のもの
type ListNPCAvatarsCondition struct {
	NameContains string
	Statuses     []readmodel.NPCAvatarStatus
	After        *NPCAvatarListCursor
	Limit        int
}

type NPCAvatarListRow struct {
	NPCAvatar NPCAvatar
	Cursor    NPCAvatarListCursor
}

type ListNPCAvatarsFunc func(ctx context.Context, condition ListNPCAvatarsCondition) ([]NPCAvatarListRow, error)

// 無ければ ErrNPCAvatarNotFound
type GetNPCAvatarFunc func(ctx context.Context, id uuid.UUID) (NPCAvatar, error)

type NPCAvatarHandler struct {
	listNPCAvatars ListNPCAvatarsFunc
	getNPCAvatar   GetNPCAvatarFunc
}

func NewNPCAvatarHandler(listNPCAvatars ListNPCAvatarsFunc, getNPCAvatar GetNPCAvatarFunc) *NPCAvatarHandler {
	return &NPCAvatarHandler{listNPCAvatars: listNPCAvatars, getNPCAvatar: getNPCAvatar}
}

// 入力の誤りは input.Errors（まとめて返す）。項目名は GraphQL の引数・入力の項目名
func (q *NPCAvatarHandler) List(ctx context.Context, filter NPCAvatarFilter, first *int, after *string) (pagination.Connection[NPCAvatar], error) {
	invalid := input.Errors{}
	page, err := pagination.NewCondition[NPCAvatarListCursor](first, after)
	if errors.Is(err, pagination.ErrFirstOutOfRange) {
		invalid = append(invalid, &input.OutOfRange{Field: "first", Min: pagination.MinFirst, Max: pagination.MaxFirst})
	}
	if errors.Is(err, pagination.ErrInvalidCursor) {
		invalid = append(invalid, &input.InvalidFormat{Field: "after"})
	}
	if err != nil && len(invalid) == 0 {
		return pagination.Connection[NPCAvatar]{}, err
	}
	nameContains, nameErr := normalizeNameContains(filter.NameContains)
	if nameErr != nil {
		invalid = append(invalid, nameErr)
	}
	if filter.Statuses != nil && len(filter.Statuses) == 0 {
		invalid = append(invalid, &input.Required{Field: "filter.statuses"})
	}
	if len(invalid) > 0 {
		return pagination.Connection[NPCAvatar]{}, invalid
	}
	rows, err := q.listNPCAvatars(ctx, ListNPCAvatarsCondition{
		NameContains: nameContains, Statuses: filter.Statuses, After: page.After, Limit: page.FetchLimit(),
	})
	if err != nil {
		return pagination.Connection[NPCAvatar]{}, err
	}
	rowConnection, err := pagination.BuildConnection(rows, page, func(row NPCAvatarListRow) NPCAvatarListCursor {
		return row.Cursor
	})
	if err != nil {
		return pagination.Connection[NPCAvatar]{}, err
	}
	connection := pagination.Connection[NPCAvatar]{
		Edges:    make([]pagination.Edge[NPCAvatar], 0, len(rowConnection.Edges)),
		PageInfo: rowConnection.PageInfo,
	}
	for _, edge := range rowConnection.Edges {
		connection.Edges = append(connection.Edges, pagination.Edge[NPCAvatar]{Node: edge.Node.NPCAvatar, Cursor: edge.Cursor})
	}
	return connection, nil
}

// 無ければ ErrNPCAvatarNotFound
func (q *NPCAvatarHandler) Get(ctx context.Context, id uuid.UUID) (NPCAvatar, error) {
	return q.getNPCAvatar(ctx, id)
}
