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

// 公開 ID に当たる配布デッキが read model に無い（無い・削除した・まだ投影されていない）
var ErrGrantDeckNotFound = errors.New("query: grant deck not found")

// presentation 層に read model の作りを見せず、表示に要る値だけを返す
type GrantDeck struct {
	ID     uuid.UUID
	Name   string
	Status readmodel.GrantDeckStatus
	// カード名・種類は持たない（presentation 層が CardHandler.Get で引く）
	Cards      []PresetDeckCard
	TotalCount int
	// プリセットデッキから写して作ったときだけ。空から作ったときは nil
	Source *GrantDeckSource
	// 集約の版（画面が変える操作に渡す）。投影が遅れているあいだは古いことがある（internal/eventstore/README.md）
	Revision     int64
	RegisteredAt time.Time
	UpdatedAt    time.Time
	// 操作したオペレーターの公開 ID。GraphQL にはまだ出さない（オペレーターの read model と、何を見せるかが決まっていないため）。
	// この変更の前に投影した行は、次のイベントで投影し直すまでゼロ値
	RegisteredBy uuid.UUID
	UpdatedBy    uuid.UUID
}

type GrantDeckSource struct {
	PresetDeckID uuid.UUID
	// 写したときのプリセットデッキの revision
	Revision int64
	// 写した後に、元のプリセットデッキが変わっている。元が消されていれば false（比べる相手が無い）
	ChangedSinceCopied bool
	// 元のプリセットデッキが消されている（消されたことを「変わっていない」と見分けられるように。core#32）
	PresetDeckDeleted bool
}

// Statuses が nil なら絞り込まない。空のリストは入力の誤り（CardFilter と同じ）
type GrantDeckFilter struct {
	NameContains *string
	Statuses     []readmodel.GrantDeckStatus
}

// 並び順（正規化した名前 → 公開 ID）の位置。項目名を 1 文字にする理由は CardListCursor と同じ
type GrantDeckListCursor struct {
	SortName    string    `json:"n"`
	GrantDeckID uuid.UUID `json:"i"`
}

func (c GrantDeckListCursor) Validate() error {
	if c.SortName == "" || c.GrantDeckID == uuid.Nil {
		return errors.New("sort name and grant deck id are required")
	}
	return nil
}

// 読み出しの関数に渡す条件。値は application 層で確かめ・正規化した後のもの
type ListGrantDecksCondition struct {
	NameContains string
	Statuses     []readmodel.GrantDeckStatus
	After        *GrantDeckListCursor
	Limit        int
}

type GrantDeckListRow struct {
	GrantDeck GrantDeck
	Cursor    GrantDeckListCursor
}

type ListGrantDecksFunc func(ctx context.Context, condition ListGrantDecksCondition) ([]GrantDeckListRow, error)

// 無ければ ErrGrantDeckNotFound
type GetGrantDeckFunc func(ctx context.Context, id uuid.UUID) (GrantDeck, error)

type GrantDeckHandler struct {
	listGrantDecks         ListGrantDecksFunc
	getGrantDeck           GetGrantDeckFunc
	getPresetDeckRevisions GetPresetDeckRevisionsFunc
}

func NewGrantDeckHandler(listGrantDecks ListGrantDecksFunc, getGrantDeck GetGrantDeckFunc, getPresetDeckRevisions GetPresetDeckRevisionsFunc) *GrantDeckHandler {
	return &GrantDeckHandler{listGrantDecks: listGrantDecks, getGrantDeck: getGrantDeck, getPresetDeckRevisions: getPresetDeckRevisions}
}

// 入力の誤りは input.Errors（まとめて返す）。項目名は GraphQL の引数・入力の項目名
func (q *GrantDeckHandler) List(ctx context.Context, filter GrantDeckFilter, first *int, after *string) (pagination.Connection[GrantDeck], error) {
	invalid := input.Errors{}
	page, err := pagination.NewCondition[GrantDeckListCursor](first, after)
	if errors.Is(err, pagination.ErrFirstOutOfRange) {
		invalid = append(invalid, &input.OutOfRange{Field: "first", Min: pagination.MinFirst, Max: pagination.MaxFirst})
	}
	if errors.Is(err, pagination.ErrInvalidCursor) {
		invalid = append(invalid, &input.InvalidFormat{Field: "after"})
	}
	if err != nil && len(invalid) == 0 {
		return pagination.Connection[GrantDeck]{}, err
	}
	nameContains, nameErr := normalizeNameContains(filter.NameContains)
	if nameErr != nil {
		invalid = append(invalid, nameErr)
	}
	if filter.Statuses != nil && len(filter.Statuses) == 0 {
		invalid = append(invalid, &input.Required{Field: "filter.statuses"})
	}
	if len(invalid) > 0 {
		return pagination.Connection[GrantDeck]{}, invalid
	}
	rows, err := q.listGrantDecks(ctx, ListGrantDecksCondition{
		NameContains: nameContains, Statuses: filter.Statuses, After: page.After, Limit: page.FetchLimit(),
	})
	if err != nil {
		return pagination.Connection[GrantDeck]{}, err
	}
	rowConnection, err := pagination.BuildConnection(rows, page, func(row GrantDeckListRow) GrantDeckListCursor {
		return row.Cursor
	})
	if err != nil {
		return pagination.Connection[GrantDeck]{}, err
	}
	decks := make([]GrantDeck, 0, len(rowConnection.Edges))
	for _, edge := range rowConnection.Edges {
		decks = append(decks, edge.Node.GrantDeck)
	}
	if err := q.compareSources(ctx, decks); err != nil {
		return pagination.Connection[GrantDeck]{}, err
	}
	connection := pagination.Connection[GrantDeck]{
		Edges:    make([]pagination.Edge[GrantDeck], 0, len(rowConnection.Edges)),
		PageInfo: rowConnection.PageInfo,
	}
	for i, edge := range rowConnection.Edges {
		connection.Edges = append(connection.Edges, pagination.Edge[GrantDeck]{Node: decks[i], Cursor: edge.Cursor})
	}
	return connection, nil
}

// 無ければ ErrGrantDeckNotFound
func (q *GrantDeckHandler) Get(ctx context.Context, id uuid.UUID) (GrantDeck, error) {
	deck, err := q.getGrantDeck(ctx, id)
	if err != nil {
		return GrantDeck{}, err
	}
	decks := []GrantDeck{deck}
	if err := q.compareSources(ctx, decks); err != nil {
		return GrantDeck{}, err
	}
	return decks[0], nil
}

// 「写した後に変わったか」は保存せず、ここで 2 つの read model を突き合わせて出す（どちらか 1 つの集約からは投影できないため。
// 投影の遅れ方が違うので、一瞬だけ古い組み合わせが見えることがあるが、表示のための値なので受け入れる）。
// decks の要素を直接書き換える
func (q *GrantDeckHandler) compareSources(ctx context.Context, decks []GrantDeck) error {
	var ids []uuid.UUID
	for _, deck := range decks {
		if deck.Source != nil {
			ids = append(ids, deck.Source.PresetDeckID)
		}
	}
	if len(ids) == 0 {
		return nil
	}
	revisions, err := q.getPresetDeckRevisions(ctx, ids)
	if err != nil {
		return err
	}
	for i, deck := range decks {
		if deck.Source == nil {
			continue
		}
		source := *deck.Source
		current, ok := revisions[source.PresetDeckID]
		source.ChangedSinceCopied = ok && current != source.Revision
		source.PresetDeckDeleted = !ok
		decks[i].Source = &source
	}
	return nil
}
