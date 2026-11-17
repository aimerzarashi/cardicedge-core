package query

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/pagination"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/game/application/input"
)

// 公開 ID に当たる公開中のボードが無い（無い・準備中・使用不可・まだ写されていない）
var ErrBoardNotFound = errors.New("query: board not found")

// 写した boards.metadata の tiles の kind の値。studio の read model の型を使わず、game が読む値だけを持つ
type BoardTileKind string

const (
	BoardTileKindLand       BoardTileKind = "land"
	BoardTileKindBase       BoardTileKind = "base"
	BoardTileKindCheckpoint BoardTileKind = "checkpoint"
	// 施設の名前は仮（世界観のデザインで見直す）
	BoardTileKindWarp         BoardTileKind = "warp"
	BoardTileKindBridge       BoardTileKind = "bridge"
	BoardTileKindSanctuary    BoardTileKind = "sanctuary"
	BoardTileKindShrine       BoardTileKind = "shrine"
	BoardTileKindProphecyShop BoardTileKind = "prophecy_shop"
	BoardTileKindCardShop     BoardTileKind = "card_shop"
)

func NewBoardTileKind(value string) (BoardTileKind, error) {
	kind := BoardTileKind(value)
	switch kind {
	case BoardTileKindLand, BoardTileKindBase, BoardTileKindCheckpoint,
		BoardTileKindWarp, BoardTileKindBridge, BoardTileKindSanctuary, BoardTileKindShrine, BoardTileKindProphecyShop, BoardTileKindCardShop:
		return kind, nil
	default:
		return "", fmt.Errorf("query: unknown board tile kind: %q", value)
	}
}

// 写した boards.metadata の tiles の element の値（土地のときだけある）
type BoardTileElement string

const (
	BoardTileElementFire    BoardTileElement = "fire"
	BoardTileElementEarth   BoardTileElement = "earth"
	BoardTileElementWater   BoardTileElement = "water"
	BoardTileElementWind    BoardTileElement = "wind"
	BoardTileElementNeutral BoardTileElement = "neutral"
	BoardTileElementMulti   BoardTileElement = "multi"
)

func NewBoardTileElement(value string) (BoardTileElement, error) {
	element := BoardTileElement(value)
	switch element {
	case BoardTileElementFire, BoardTileElementEarth, BoardTileElementWater, BoardTileElementWind, BoardTileElementNeutral, BoardTileElementMulti:
		return element, nil
	default:
		return "", fmt.Errorf("query: unknown board tile element: %q", value)
	}
}

type BoardPosition struct {
	X int
	Y int
}

func NewBoardPosition(x, y int) BoardPosition {
	return BoardPosition{X: x, Y: y}
}

// Element は土地のときだけ値があり、そうでなければ空
type BoardTile struct {
	X       int
	Y       int
	Kind    BoardTileKind
	Element BoardTileElement
}

type BoardConnection struct {
	A BoardPosition
	B BoardPosition
}

// 写した boards.metadata の pairs の 1 つ（転送円どうし・橋どうし）
type BoardPair struct {
	A BoardPosition
	B BoardPosition
}

// 写した boards.metadata の default_rules。対戦を始めるときの既定の値で、始めるときにここから変えられる前提
type BoardDefaultRules struct {
	TargetMagic          int
	StartingMagic        int
	LapBonus             int
	CheckpointBonus      int
	RoundLimit           int
	DiceMin              int
	DiceMax              int
	BaseLandValue        int
	LapBonusPerTerritory int
}

func NewBoardDefaultRules(targetMagic, startingMagic, lapBonus, checkpointBonus, roundLimit, diceMin, diceMax, baseLandValue, lapBonusPerTerritory int) BoardDefaultRules {
	return BoardDefaultRules{
		TargetMagic: targetMagic, StartingMagic: startingMagic, LapBonus: lapBonus, CheckpointBonus: checkpointBonus,
		RoundLimit: roundLimit, DiceMin: diceMin, DiceMax: diceMax, BaseLandValue: baseLandValue, LapBonusPerTerritory: lapBonusPerTerritory,
	}
}

// 写した boards.metadata の tile_counts。種類ごと・土地の属性ごとのマスの数
type BoardTileCounts struct {
	Land       int
	Base       int
	Checkpoint int
	Fire       int
	Earth      int
	Water      int
	Wind       int
	Neutral    int
	Multi      int
	// 属性を持たない特殊なマスの種類ごとの数（土地の合計には入らない）
	Warp         int
	Bridge       int
	Sanctuary    int
	Shrine       int
	ProphecyShop int
	CardShop     int
}

func NewBoardTileCounts(land, base, checkpoint, fire, earth, water, wind, neutral, multi, warp, bridge, sanctuary, shrine, prophecyShop, cardShop int) BoardTileCounts {
	return BoardTileCounts{
		Land: land, Base: base, Checkpoint: checkpoint, Fire: fire, Earth: earth, Water: water, Wind: wind, Neutral: neutral, Multi: multi,
		Warp: warp, Bridge: bridge, Sanctuary: sanctuary, Shrine: shrine, ProphecyShop: prophecyShop, CardShop: cardShop,
	}
}

// 公開中のボードの、盤面に要る項目だけ。状態・revision・日時は game で使わないので持たない。マスは y → x、つながりと対は a → b の順
// （写した metadata の並びのまま）。一覧の要素にもマスとつながりを持たせるのは、別に引き直す仕組みを作らなくても、
// 1 ページ 100 件・最大 32×32 に収まるため（studio の BoardHandler と同じ）
type Board struct {
	ID           uuid.UUID
	Name         string
	Width        int
	Height       int
	Tiles        []BoardTile
	Connections  []BoardConnection
	Pairs        []BoardPair
	DefaultRules BoardDefaultRules
	TileCounts   BoardTileCounts
}

// 並び順（正規化した名前 → 公開 ID）の位置。項目名を 1 文字にする理由は studio のカーソルと同じ
type BoardListCursor struct {
	SortName string    `json:"n"`
	BoardID  uuid.UUID `json:"i"`
}

func (c BoardListCursor) Validate() error {
	if c.SortName == "" || c.BoardID == uuid.Nil {
		return errors.New("sort name and board id are required")
	}
	return nil
}

type ListPublishedBoardsCondition struct {
	After *BoardListCursor
	Limit int
}

type BoardListRow struct {
	Board  Board
	Cursor BoardListCursor
}

// 公開中のボードだけを、名前の正規化 → 公開 ID の順に Limit 件まで返す
type ListPublishedBoardsFunc func(ctx context.Context, condition ListPublishedBoardsCondition) ([]BoardListRow, error)

// 公開中でなければ（使用不可を含む）ErrBoardNotFound
type GetPublishedBoardFunc func(ctx context.Context, id uuid.UUID) (Board, error)

type BoardHandler struct {
	listPublished ListPublishedBoardsFunc
	getPublished  GetPublishedBoardFunc
}

func NewBoardHandler(listPublished ListPublishedBoardsFunc, getPublished GetPublishedBoardFunc) *BoardHandler {
	return &BoardHandler{listPublished: listPublished, getPublished: getPublished}
}

// 入力の誤りは input.Errors（まとめて返す）。項目名は GraphQL の引数名
func (q *BoardHandler) List(ctx context.Context, first *int, after *string) (pagination.Connection[Board], error) {
	page, err := pagination.NewCondition[BoardListCursor](first, after)
	if err != nil {
		invalid := input.Errors{}
		if errors.Is(err, pagination.ErrFirstOutOfRange) {
			invalid = append(invalid, &input.OutOfRange{Field: "first", Min: pagination.MinFirst, Max: pagination.MaxFirst})
		}
		if errors.Is(err, pagination.ErrInvalidCursor) {
			invalid = append(invalid, &input.InvalidFormat{Field: "after"})
		}
		if len(invalid) == 0 {
			return pagination.Connection[Board]{}, err
		}
		return pagination.Connection[Board]{}, invalid
	}
	rows, err := q.listPublished(ctx, ListPublishedBoardsCondition{After: page.After, Limit: page.FetchLimit()})
	if err != nil {
		return pagination.Connection[Board]{}, err
	}
	rowConnection, err := pagination.BuildConnection(rows, page, func(row BoardListRow) BoardListCursor {
		return row.Cursor
	})
	if err != nil {
		return pagination.Connection[Board]{}, err
	}
	connection := pagination.Connection[Board]{
		Edges:    make([]pagination.Edge[Board], 0, len(rowConnection.Edges)),
		PageInfo: rowConnection.PageInfo,
	}
	for _, edge := range rowConnection.Edges {
		connection.Edges = append(connection.Edges, pagination.Edge[Board]{Node: edge.Node.Board, Cursor: edge.Cursor})
	}
	return connection, nil
}

// 公開中でなければ ErrBoardNotFound
func (q *BoardHandler) Get(ctx context.Context, id uuid.UUID) (Board, error) {
	return q.getPublished(ctx, id)
}
