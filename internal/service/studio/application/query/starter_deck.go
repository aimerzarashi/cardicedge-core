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

// 公開 ID に当たるスターターデッキが read model に無い（無い・削除した・まだ投影されていない）
var ErrStarterDeckNotFound = errors.New("query: starter deck not found")

// presentation 層に read model の作りを見せず、表示に要る値だけを返す
type StarterDeck struct {
	ID     uuid.UUID
	Name   string
	Status readmodel.StarterDeckStatus
	// カード名・種類は持たない（presentation 層が CardHandler.Get で引く）
	Cards      []PresetDeckCard
	TotalCount int
	// プリセットデッキから写して作ったときだけ。空から作ったときは nil
	Source *StarterDeckSource
	// 集約の版（画面が変える操作に渡す）。投影が遅れているあいだは古いことがある（internal/eventstore/README.md）
	Revision     int64
	RegisteredAt time.Time
	UpdatedAt    time.Time
}

type StarterDeckSource struct {
	PresetDeckID uuid.UUID
	// 写したときのプリセットデッキの revision
	Revision int64
	// 写した後に、元のプリセットデッキが変わっている。元が消されていれば false（比べる相手が無い）
	ChangedSinceCopied bool
	// 元のプリセットデッキが消されている（消されたことを「変わっていない」と見分けられるように。core#32）
	PresetDeckDeleted bool
}

// Statuses が nil なら絞り込まない。空のリストは入力の誤り（CardFilter と同じ）
type StarterDeckFilter struct {
	NameContains *string
	Statuses     []readmodel.StarterDeckStatus
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

// 読み出しの関数に渡す条件。値は application 層で確かめ・正規化した後のもの
type ListStarterDecksCondition struct {
	NameContains string
	Statuses     []readmodel.StarterDeckStatus
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

type StarterDeckHandler struct {
	listStarterDecks       ListStarterDecksFunc
	getStarterDeck         GetStarterDeckFunc
	getPresetDeckRevisions GetPresetDeckRevisionsFunc
}

func NewStarterDeckHandler(listStarterDecks ListStarterDecksFunc, getStarterDeck GetStarterDeckFunc, getPresetDeckRevisions GetPresetDeckRevisionsFunc) *StarterDeckHandler {
	return &StarterDeckHandler{listStarterDecks: listStarterDecks, getStarterDeck: getStarterDeck, getPresetDeckRevisions: getPresetDeckRevisions}
}

// 入力の誤りは input.Errors（まとめて返す）。項目名は GraphQL の引数・入力の項目名
func (q *StarterDeckHandler) List(ctx context.Context, filter StarterDeckFilter, first *int, after *string) (pagination.Connection[StarterDeck], error) {
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
	nameContains, nameErr := normalizeNameContains(filter.NameContains)
	if nameErr != nil {
		invalid = append(invalid, nameErr)
	}
	if filter.Statuses != nil && len(filter.Statuses) == 0 {
		invalid = append(invalid, &input.Required{Field: "filter.statuses"})
	}
	if len(invalid) > 0 {
		return pagination.Connection[StarterDeck]{}, invalid
	}
	rows, err := q.listStarterDecks(ctx, ListStarterDecksCondition{
		NameContains: nameContains, Statuses: filter.Statuses, After: page.After, Limit: page.FetchLimit(),
	})
	if err != nil {
		return pagination.Connection[StarterDeck]{}, err
	}
	rowConnection, err := pagination.BuildConnection(rows, page, func(row StarterDeckListRow) StarterDeckListCursor {
		return row.Cursor
	})
	if err != nil {
		return pagination.Connection[StarterDeck]{}, err
	}
	decks := make([]StarterDeck, 0, len(rowConnection.Edges))
	for _, edge := range rowConnection.Edges {
		decks = append(decks, edge.Node.StarterDeck)
	}
	if err := q.compareSources(ctx, decks); err != nil {
		return pagination.Connection[StarterDeck]{}, err
	}
	connection := pagination.Connection[StarterDeck]{
		Edges:    make([]pagination.Edge[StarterDeck], 0, len(rowConnection.Edges)),
		PageInfo: rowConnection.PageInfo,
	}
	for i, edge := range rowConnection.Edges {
		connection.Edges = append(connection.Edges, pagination.Edge[StarterDeck]{Node: decks[i], Cursor: edge.Cursor})
	}
	return connection, nil
}

// 無ければ ErrStarterDeckNotFound
func (q *StarterDeckHandler) Get(ctx context.Context, id uuid.UUID) (StarterDeck, error) {
	deck, err := q.getStarterDeck(ctx, id)
	if err != nil {
		return StarterDeck{}, err
	}
	decks := []StarterDeck{deck}
	if err := q.compareSources(ctx, decks); err != nil {
		return StarterDeck{}, err
	}
	return decks[0], nil
}

// 「写した後に変わったか」は保存せず、ここで 2 つの read model を突き合わせて出す（どちらか 1 つの集約からは投影できないため。
// 投影の遅れ方が違うので、一瞬だけ古い組み合わせが見えることがあるが、表示のための値なので受け入れる）。
// decks の要素を直接書き換える
func (q *StarterDeckHandler) compareSources(ctx context.Context, decks []StarterDeck) error {
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
