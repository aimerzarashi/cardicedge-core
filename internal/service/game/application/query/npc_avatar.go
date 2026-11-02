package query

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/pagination"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/game/application/input"
)

// 公開 ID に当たる公開中の NPC のアバターが無い（無い・準備中・使用不可・まだ写されていない）
var ErrNPCAvatarNotFound = errors.New("query: npc avatar not found")

// カードの種類。写した cards.card_type の値（1: Creature、2: Item、3: Spell）の並びが、そのまま表示の並びになる。
// studio の read model の型を使わず、game が読む値だけを持つ
type CardType int16

const (
	CardTypeCreature CardType = 1
	CardTypeItem     CardType = 2
	CardTypeSpell    CardType = 3
)

func NewCardType(value int16) (CardType, error) {
	cardType := CardType(value)
	if cardType < CardTypeCreature || cardType > CardTypeSpell {
		return 0, fmt.Errorf("query: unknown card type: %d", value)
	}
	return cardType, nil
}

// NPC のデッキに入っているカード 1 種類。カードは NPC のデッキを見せるのに要る名前と種類だけを持つ
// （カードの一覧・詳細を game に出す問い合わせは作らない）
type NPCDeckCard struct {
	CardID uuid.UUID
	Name   string
	Type   CardType
	Count  int
}

// 公開中の NPC のデッキ。Cards は種類（Creature → Item → Spell）→ 名前の順（studio のデッキのカードと同じ並び）
type NPCDeck struct {
	ID    uuid.UUID
	Name  string
	Cards []NPCDeckCard
}

func (d NPCDeck) TotalCount() int {
	total := 0
	for _, card := range d.Cards {
		total += card.Count
	}
	return total
}

// 公開中の NPC のアバターと、使うデッキ。Decks は割り当てた順で、公開中のものだけ（準備中・使用不可のデッキは出さない）。
// NPC のデッキは単独では使わず、アバターを通してだけ使う（core#63）
type NPCAvatar struct {
	ID    uuid.UUID
	Name  string
	Decks []NPCDeck
}

// 読み出した NPC のアバター 1 件。DeckIDs は割り当てた順（写した npc_avatars.metadata の decks）で、状態は見ていない
type NPCAvatarRecord struct {
	ID      uuid.UUID
	Name    string
	DeckIDs []uuid.UUID
}

// 並び順（正規化した名前 → 公開 ID）の位置。項目名を 1 文字にする理由は studio のカーソルと同じ
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

type ListPublishedNPCAvatarsCondition struct {
	After *NPCAvatarListCursor
	Limit int
}

type NPCAvatarListRow struct {
	Avatar NPCAvatarRecord
	Cursor NPCAvatarListCursor
}

// 公開中の NPC のアバターだけを、名前の正規化 → 公開 ID の順に Limit 件まで返す
type ListPublishedNPCAvatarsFunc func(ctx context.Context, condition ListPublishedNPCAvatarsCondition) ([]NPCAvatarListRow, error)

// 公開中でなければ（使用不可を含む）ErrNPCAvatarNotFound
type GetPublishedNPCAvatarFunc func(ctx context.Context, id uuid.UUID) (NPCAvatarRecord, error)

// 渡した公開 ID のうち公開中の NPC のデッキだけを、順不同で返す（無い・公開中でないものは含めない）。
// 1 ページ分のアバターのデッキを 1 回の読み出しにまとめるため、ID の一覧で受ける
type ListPublishedNPCDecksFunc func(ctx context.Context, ids []uuid.UUID) ([]NPCDeck, error)

type NPCAvatarHandler struct {
	listPublished ListPublishedNPCAvatarsFunc
	getPublished  GetPublishedNPCAvatarFunc
	listDecks     ListPublishedNPCDecksFunc
}

func NewNPCAvatarHandler(listPublished ListPublishedNPCAvatarsFunc, getPublished GetPublishedNPCAvatarFunc, listDecks ListPublishedNPCDecksFunc) *NPCAvatarHandler {
	return &NPCAvatarHandler{listPublished: listPublished, getPublished: getPublished, listDecks: listDecks}
}

// 入力の誤りは input.Errors（まとめて返す）。項目名は GraphQL の引数名
func (q *NPCAvatarHandler) List(ctx context.Context, first *int, after *string) (pagination.Connection[NPCAvatar], error) {
	page, err := pagination.NewCondition[NPCAvatarListCursor](first, after)
	if err != nil {
		invalid := input.Errors{}
		if errors.Is(err, pagination.ErrFirstOutOfRange) {
			invalid = append(invalid, &input.OutOfRange{Field: "first", Min: pagination.MinFirst, Max: pagination.MaxFirst})
		}
		if errors.Is(err, pagination.ErrInvalidCursor) {
			invalid = append(invalid, &input.InvalidFormat{Field: "after"})
		}
		if len(invalid) == 0 {
			return pagination.Connection[NPCAvatar]{}, err
		}
		return pagination.Connection[NPCAvatar]{}, invalid
	}
	rows, err := q.listPublished(ctx, ListPublishedNPCAvatarsCondition{After: page.After, Limit: page.FetchLimit()})
	if err != nil {
		return pagination.Connection[NPCAvatar]{}, err
	}
	rowConnection, err := pagination.BuildConnection(rows, page, func(row NPCAvatarListRow) NPCAvatarListCursor {
		return row.Cursor
	})
	if err != nil {
		return pagination.Connection[NPCAvatar]{}, err
	}
	records := make([]NPCAvatarRecord, 0, len(rowConnection.Edges))
	for _, edge := range rowConnection.Edges {
		records = append(records, edge.Node.Avatar)
	}
	avatars, err := q.withDecks(ctx, records)
	if err != nil {
		return pagination.Connection[NPCAvatar]{}, err
	}
	connection := pagination.Connection[NPCAvatar]{
		Edges:    make([]pagination.Edge[NPCAvatar], 0, len(rowConnection.Edges)),
		PageInfo: rowConnection.PageInfo,
	}
	for i, edge := range rowConnection.Edges {
		connection.Edges = append(connection.Edges, pagination.Edge[NPCAvatar]{Node: avatars[i], Cursor: edge.Cursor})
	}
	return connection, nil
}

// 公開中でなければ ErrNPCAvatarNotFound
func (q *NPCAvatarHandler) Get(ctx context.Context, id uuid.UUID) (NPCAvatar, error) {
	record, err := q.getPublished(ctx, id)
	if err != nil {
		return NPCAvatar{}, err
	}
	avatars, err := q.withDecks(ctx, []NPCAvatarRecord{record})
	if err != nil {
		return NPCAvatar{}, err
	}
	return avatars[0], nil
}

// ページ全体のデッキを 1 回で読み、アバターごとに割り当てた順に並べ直す。公開中でないデッキは読み出しの結果に無いので飛ばす。
// 同じデッキを複数のアバターが使ってよい（複製せず、同じ値を渡す）
func (q *NPCAvatarHandler) withDecks(ctx context.Context, records []NPCAvatarRecord) ([]NPCAvatar, error) {
	seen := map[uuid.UUID]struct{}{}
	ids := []uuid.UUID{}
	for _, record := range records {
		for _, id := range record.DeckIDs {
			if _, ok := seen[id]; !ok {
				seen[id] = struct{}{}
				ids = append(ids, id)
			}
		}
	}
	published := map[uuid.UUID]NPCDeck{}
	if len(ids) > 0 {
		decks, err := q.listDecks(ctx, ids)
		if err != nil {
			return nil, err
		}
		for _, deck := range decks {
			published[deck.ID] = deck
		}
	}
	avatars := make([]NPCAvatar, 0, len(records))
	for _, record := range records {
		decks := make([]NPCDeck, 0, len(record.DeckIDs))
		for _, id := range record.DeckIDs {
			if deck, ok := published[id]; ok {
				decks = append(decks, deck)
			}
		}
		avatars = append(avatars, NPCAvatar{ID: record.ID, Name: record.Name, Decks: decks})
	}
	return avatars, nil
}
