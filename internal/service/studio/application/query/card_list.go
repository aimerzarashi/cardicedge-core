// 問い合わせ（read model を読む操作）。読み出しの関数は infrastructure/repository の実装を internal/service/studio が渡す
package query

import (
	"cmp"
	"context"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/card"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/pagination"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/studio/application/input"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/studio/application/query/readmodel"
)

// presentation 層に read model の作りを見せず、表示に要る値だけを返す。presentation 層が Type で Creature・Item・Spell の型に振り分ける
type Card struct {
	ID           uuid.UUID
	Type         readmodel.CardType
	Name         string
	RegisteredAt time.Time
	UpdatedAt    time.Time
}

// Types が nil なら絞り込まない。空のリストは入力の誤り（何も選ばないのは、0 件を返すより選び忘れと考える）
type CardFilter struct {
	NameContains *string
	Types        []readmodel.CardType
}

// 並び順（種類 → 正規化した名前 → 公開 ID）の位置。項目名を 1 文字にするのは、URL に載ったときに短くするため
type CardListCursor struct {
	CardType readmodel.CardType `json:"t"`
	SortName string             `json:"n"`
	CardID   uuid.UUID          `json:"i"`
}

func (c CardListCursor) Validate() error {
	if c.CardType < readmodel.CardTypeCreature || c.CardType > readmodel.CardTypeSpell {
		return errors.New("card type is out of range")
	}
	if c.SortName == "" || c.CardID == uuid.Nil {
		return errors.New("sort name and card id are required")
	}
	return nil
}

// 読み出しの関数に渡す条件。値は application 層で確かめ・正規化した後のもの（LIKE の記号のエスケープは SQL の都合なので読み出しの関数がする）
type ListCardsCondition struct {
	Types        []readmodel.CardType
	NameContains string
	After        *CardListCursor
	Limit        int
}

type CardListRow struct {
	Card   Card
	Cursor CardListCursor
}

type ListCardsFunc func(ctx context.Context, condition ListCardsCondition) ([]CardListRow, error)

// 公開 ID で引く（プリセットデッキのカード名を引くデータローダーが、まとめて 1 回で呼ぶ）。無いカードは結果に含めない
type GetCardsFunc func(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID]Card, error)

type CardHandler struct {
	listCards ListCardsFunc
	getCards  GetCardsFunc
}

func NewCardHandler(listCards ListCardsFunc, getCards GetCardsFunc) *CardHandler {
	return &CardHandler{listCards: listCards, getCards: getCards}
}

// 無いカードは結果に含めない
func (q *CardHandler) Get(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID]Card, error) {
	return q.getCards(ctx, ids)
}

// カードの一覧と同じ並び（種類 → 正規化した名前 → 公開 ID）で比べる。read model を読まずに並べる場面
// （プリセットデッキのカード）でも、一覧と同じ順にするため（並べ方の決まりを presentation 層・studio に書き写さない）
func CompareCards(a, b Card) int {
	return cmp.Or(
		cmp.Compare(a.Type, b.Type),
		strings.Compare(card.NormalizeName(a.Name), card.NormalizeName(b.Name)),
		strings.Compare(a.ID.String(), b.ID.String()),
	)
}

// 入力の誤りは input.Errors。項目名は GraphQL の引数・入力の項目名（入れ子は filter.nameContains のように . でつなぐ）
func (q *CardHandler) List(ctx context.Context, filter CardFilter, first *int, after *string) (pagination.Connection[Card], error) {
	condition, err := q.condition(filter, first, after)
	if err != nil {
		return pagination.Connection[Card]{}, err
	}
	rows, err := q.listCards(ctx, condition.list)
	if err != nil {
		return pagination.Connection[Card]{}, err
	}
	rowConnection, err := pagination.BuildConnection(rows, condition.page, func(row CardListRow) CardListCursor {
		return row.Cursor
	})
	if err != nil {
		return pagination.Connection[Card]{}, err
	}
	connection := pagination.Connection[Card]{Edges: make([]pagination.Edge[Card], 0, len(rowConnection.Edges)), PageInfo: rowConnection.PageInfo}
	for _, edge := range rowConnection.Edges {
		connection.Edges = append(connection.Edges, pagination.Edge[Card]{Node: edge.Node.Card, Cursor: edge.Cursor})
	}
	return connection, nil
}

type cardListCondition struct {
	page pagination.Condition[CardListCursor]
	list ListCardsCondition
}

// 誤りは最初の 1 つで止めず、まとめて返す
func (q *CardHandler) condition(filter CardFilter, first *int, after *string) (cardListCondition, error) {
	invalid := input.Errors{}
	page, err := pagination.NewCondition[CardListCursor](first, after)
	if errors.Is(err, pagination.ErrFirstOutOfRange) {
		invalid = append(invalid, &input.OutOfRange{Field: "first", Min: pagination.MinFirst, Max: pagination.MaxFirst})
	}
	if errors.Is(err, pagination.ErrInvalidCursor) {
		invalid = append(invalid, &input.InvalidFormat{Field: "after"})
	}
	if err != nil && len(invalid) == 0 {
		return cardListCondition{}, err
	}
	nameContains, nameErr := normalizeNameContains(filter.NameContains)
	if nameErr != nil {
		invalid = append(invalid, nameErr)
	}
	if filter.Types != nil && len(filter.Types) == 0 {
		invalid = append(invalid, &input.Required{Field: "filter.types"})
	}
	if len(invalid) > 0 {
		return cardListCondition{}, invalid
	}
	list := ListCardsCondition{Types: filter.Types, NameContains: nameContains, After: page.After, Limit: page.FetchLimit()}
	return cardListCondition{page: page, list: list}, nil
}

// カード名と同じく、長さは正規化する前の値で数え、比べるのは重複の判定と同じ正規化をした値（card.NormalizeName）。
// 上限を超える値で探しても必ず 0 件になるため、入力の誤りにする
func normalizeNameContains(value *string) (string, input.Error) {
	if value == nil {
		return "", nil
	}
	trimmed := strings.TrimSpace(*value)
	if trimmed == "" {
		return "", &input.Required{Field: "filter.nameContains"}
	}
	if utf8.RuneCountInString(trimmed) > card.MaxNameLength {
		return "", &input.TooLong{Field: "filter.nameContains", Max: card.MaxNameLength}
	}
	return card.NormalizeName(trimmed), nil
}
