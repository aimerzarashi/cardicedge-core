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

// 公開 ID に当たる NPC の思考パターンが read model に無い（無い・削除した・まだ投影されていない）
var ErrNPCMindsetNotFound = errors.New("query: npc mindset not found")

// presentation 層に read model の作りを見せず、表示に要る値だけを返す
type NPCMindset struct {
	ID     uuid.UUID
	Name   string
	Status readmodel.NPCMindsetStatus
	// 集約の版（画面が変える操作に渡す）。投影が遅れているあいだは古いことがある（internal/eventstore/README.md）
	Revision     int64
	RegisteredAt time.Time
	// 操作したオペレーターの公開 ID。GraphQL にはまだ出さない（オペレーターの read model と、何を見せるかが決まっていないため）
	RegisteredBy uuid.UUID
	UpdatedAt    time.Time
	UpdatedBy    uuid.UUID
	// 行動の種類ごとの重みと採点基準ごとの重み（種類・基準のすべてを持つ）
	ActionWeights    []NPCMindsetActionWeight
	CriterionWeights []NPCMindsetCriterionWeight
}

// 行動の種類とその重み。種類は値の文字列（matchaction.Action の値）で、presentation 層にドメインの型を見せない
type NPCMindsetActionWeight struct {
	Action string
	Weight int
}

type NPCMindsetCriterionWeight struct {
	Criterion string
	Weight    int
}

// Statuses が nil なら絞り込まない。空のリストは入力の誤り（CardFilter と同じ）
type NPCMindsetFilter struct {
	NameContains *string
	Statuses     []readmodel.NPCMindsetStatus
}

// 並び順（正規化した名前 → 公開 ID）の位置。項目名を 1 文字にする理由は CardListCursor と同じ
type NPCMindsetListCursor struct {
	SortName     string    `json:"n"`
	NPCMindsetID uuid.UUID `json:"i"`
}

func (c NPCMindsetListCursor) Validate() error {
	if c.SortName == "" || c.NPCMindsetID == uuid.Nil {
		return errors.New("sort name and npc mindset id are required")
	}
	return nil
}

// 読み出しの関数に渡す条件。値は application 層で確かめ・正規化した後のもの
type ListNPCMindsetsCondition struct {
	NameContains string
	Statuses     []readmodel.NPCMindsetStatus
	After        *NPCMindsetListCursor
	Limit        int
}

type NPCMindsetListRow struct {
	NPCMindset NPCMindset
	Cursor     NPCMindsetListCursor
}

type ListNPCMindsetsFunc func(ctx context.Context, condition ListNPCMindsetsCondition) ([]NPCMindsetListRow, error)

// 無ければ ErrNPCMindsetNotFound
type GetNPCMindsetFunc func(ctx context.Context, id uuid.UUID) (NPCMindset, error)

type NPCMindsetHandler struct {
	listNPCMindsets ListNPCMindsetsFunc
	getNPCMindset   GetNPCMindsetFunc
}

func NewNPCMindsetHandler(listNPCMindsets ListNPCMindsetsFunc, getNPCMindset GetNPCMindsetFunc) *NPCMindsetHandler {
	return &NPCMindsetHandler{listNPCMindsets: listNPCMindsets, getNPCMindset: getNPCMindset}
}

// 入力の誤りは input.Errors（まとめて返す）。項目名は GraphQL の引数・入力の項目名
func (q *NPCMindsetHandler) List(ctx context.Context, filter NPCMindsetFilter, first *int, after *string) (pagination.Connection[NPCMindset], error) {
	invalid := input.Errors{}
	page, err := pagination.NewCondition[NPCMindsetListCursor](first, after)
	if errors.Is(err, pagination.ErrFirstOutOfRange) {
		invalid = append(invalid, &input.OutOfRange{Field: "first", Min: pagination.MinFirst, Max: pagination.MaxFirst})
	}
	if errors.Is(err, pagination.ErrInvalidCursor) {
		invalid = append(invalid, &input.InvalidFormat{Field: "after"})
	}
	if err != nil && len(invalid) == 0 {
		return pagination.Connection[NPCMindset]{}, err
	}
	nameContains, nameErr := normalizeNameContains(filter.NameContains)
	if nameErr != nil {
		invalid = append(invalid, nameErr)
	}
	if filter.Statuses != nil && len(filter.Statuses) == 0 {
		invalid = append(invalid, &input.Required{Field: "filter.statuses"})
	}
	if len(invalid) > 0 {
		return pagination.Connection[NPCMindset]{}, invalid
	}
	rows, err := q.listNPCMindsets(ctx, ListNPCMindsetsCondition{
		NameContains: nameContains, Statuses: filter.Statuses, After: page.After, Limit: page.FetchLimit(),
	})
	if err != nil {
		return pagination.Connection[NPCMindset]{}, err
	}
	rowConnection, err := pagination.BuildConnection(rows, page, func(row NPCMindsetListRow) NPCMindsetListCursor {
		return row.Cursor
	})
	if err != nil {
		return pagination.Connection[NPCMindset]{}, err
	}
	connection := pagination.Connection[NPCMindset]{
		Edges:    make([]pagination.Edge[NPCMindset], 0, len(rowConnection.Edges)),
		PageInfo: rowConnection.PageInfo,
	}
	for _, edge := range rowConnection.Edges {
		connection.Edges = append(connection.Edges, pagination.Edge[NPCMindset]{Node: edge.Node.NPCMindset, Cursor: edge.Cursor})
	}
	return connection, nil
}

// 無ければ ErrNPCMindsetNotFound
func (q *NPCMindsetHandler) Get(ctx context.Context, id uuid.UUID) (NPCMindset, error) {
	return q.getNPCMindset(ctx, id)
}
