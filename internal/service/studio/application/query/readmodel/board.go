package readmodel

import (
	"fmt"
	"maps"
	"slices"
	"time"

	"github.com/google/uuid"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/board"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/operator"
)

// boards.status の値（準備中・公開中・使用不可）。domain の board.Status を使わない理由は CardStatus と同じ
type BoardStatus string

const (
	BoardStatusDraft     BoardStatus = "draft"
	BoardStatusPublished BoardStatus = "published"
	BoardStatusRetired   BoardStatus = "retired"
)

// DB の boards.status など、外から来た値を状態にする
func NewBoardStatus(value string) (BoardStatus, error) {
	status := BoardStatus(value)
	switch status {
	case BoardStatusDraft, BoardStatusPublished, BoardStatusRetired:
		return status, nil
	default:
		return "", fmt.Errorf("readmodel: unknown board status: %q", value)
	}
}

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
		return "", fmt.Errorf("readmodel: unknown board tile kind: %q", value)
	}
}

// 土地の属性。domain の board.Element を使わない理由は BoardTileKind と同じ
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
		return "", fmt.Errorf("readmodel: unknown board tile element: %q", value)
	}
}

// boards.metadata の default_rules。domain の board.DefaultRules を使わない理由は BoardTileKind と同じ
type BoardDefaultRules struct {
	TargetMagic     int `json:"target_magic"`
	StartingMagic   int `json:"starting_magic"`
	LapBonus        int `json:"lap_bonus"`
	CheckpointBonus int `json:"checkpoint_bonus"`
	RoundLimit      int `json:"round_limit"`
	DiceMin         int `json:"dice_min"`
	DiceMax         int `json:"dice_max"`
	BaseLandValue   int `json:"base_land_value"`
}

func NewBoardDefaultRules(targetMagic, startingMagic, lapBonus, checkpointBonus, roundLimit, diceMin, diceMax, baseLandValue int) BoardDefaultRules {
	return BoardDefaultRules{
		TargetMagic: targetMagic, StartingMagic: startingMagic, LapBonus: lapBonus, CheckpointBonus: checkpointBonus,
		RoundLimit: roundLimit, DiceMin: diceMin, DiceMax: diceMax, BaseLandValue: baseLandValue,
	}
}

func toBoardDefaultRules(rules board.DefaultRules) BoardDefaultRules {
	return BoardDefaultRules{
		TargetMagic: rules.TargetMagic, StartingMagic: rules.StartingMagic, LapBonus: rules.LapBonus, CheckpointBonus: rules.CheckpointBonus,
		RoundLimit: rules.RoundLimit, DiceMin: rules.DiceMin, DiceMax: rules.DiceMax, BaseLandValue: rules.BaseLandValue,
	}
}

// boards.metadata の tile_counts。土地の合計と、属性ごとの数（土地だけが属性を持つので、属性の数の和が土地の合計になる）。
// studio は core が返した一覧を画面で絞り込まない決まりなので、画面が使う数は投影のときに数えて持つ
type BoardTileCounts struct {
	Land       int `json:"land"`
	Base       int `json:"base"`
	Checkpoint int `json:"checkpoint"`
	Fire       int `json:"fire"`
	Earth      int `json:"earth"`
	Water      int `json:"water"`
	Wind       int `json:"wind"`
	Neutral    int `json:"neutral"`
	Multi      int `json:"multi"`
	// 属性を持たない特殊なマスの種類ごとの数（土地の合計には入らない）
	Warp         int `json:"warp"`
	Bridge       int `json:"bridge"`
	Sanctuary    int `json:"sanctuary"`
	Shrine       int `json:"shrine"`
	ProphecyShop int `json:"prophecy_shop"`
	CardShop     int `json:"card_shop"`
}

func NewBoardTileCounts(land, base, checkpoint, fire, earth, water, wind, neutral, multi, warp, bridge, sanctuary, shrine, prophecyShop, cardShop int) BoardTileCounts {
	return BoardTileCounts{
		Land: land, Base: base, Checkpoint: checkpoint, Fire: fire, Earth: earth, Water: water, Wind: wind, Neutral: neutral, Multi: multi,
		Warp: warp, Bridge: bridge, Sanctuary: sanctuary, Shrine: shrine, ProphecyShop: prophecyShop, CardShop: cardShop,
	}
}

func countBoardTiles(tiles map[board.Position]BoardTile) BoardTileCounts {
	var counts BoardTileCounts
	for _, tile := range tiles {
		switch tile.Kind {
		case BoardTileKindLand:
			counts.Land++
		case BoardTileKindBase:
			counts.Base++
		case BoardTileKindCheckpoint:
			counts.Checkpoint++
		case BoardTileKindWarp:
			counts.Warp++
		case BoardTileKindBridge:
			counts.Bridge++
		case BoardTileKindSanctuary:
			counts.Sanctuary++
		case BoardTileKindShrine:
			counts.Shrine++
		case BoardTileKindProphecyShop:
			counts.ProphecyShop++
		case BoardTileKindCardShop:
			counts.CardShop++
		}
		switch tile.Element {
		case BoardTileElementFire:
			counts.Fire++
		case BoardTileElementEarth:
			counts.Earth++
		case BoardTileElementWater:
			counts.Water++
		case BoardTileElementWind:
			counts.Wind++
		case BoardTileElementNeutral:
			counts.Neutral++
		case BoardTileElementMulti:
			counts.Multi++
		}
	}
	return counts
}

// boards.metadata の publish_problems の 1 つ。公開できない理由の種類と、関わるマスの座標（無ければ空）。
// 種類は domain の board.PublishProblemKind の値と同じ。domain の型を使わない理由は BoardTileKind と同じ
type BoardPublishProblem struct {
	Kind      BoardPublishProblemKind `json:"kind"`
	Positions []BoardPosition         `json:"positions"`
}

type BoardPublishProblemKind string

const (
	BoardPublishProblemBaseCount        BoardPublishProblemKind = "base_count"
	BoardPublishProblemNoCheckpoint     BoardPublishProblemKind = "no_checkpoint"
	BoardPublishProblemUnreachableTiles BoardPublishProblemKind = "unreachable_tiles"
	BoardPublishProblemUnpairedTiles    BoardPublishProblemKind = "unpaired_tiles"
)

func NewBoardPublishProblem(kind BoardPublishProblemKind, positions ...BoardPosition) BoardPublishProblem {
	if positions == nil {
		positions = []BoardPosition{}
	}
	return BoardPublishProblem{Kind: kind, Positions: positions}
}

// 置いてあるマスとつながりと対から、公開できない理由を調べる（判定は domain の board.CheckReadiness の 1 か所）。
// 理由の順は domain の Problems のまま。空（公開できる）でも nil にせず、JSON を [] にする
func publishProblems(tiles map[board.Position]BoardTile, connections map[board.Connection]struct{}, pairs map[board.Pair]struct{}) []BoardPublishProblem {
	domainTiles := make([]board.Tile, 0, len(tiles))
	for pos, tile := range tiles {
		kind, err := board.NewTileKind(string(tile.Kind))
		if err != nil {
			// マスの種類は検査済みのイベントから作った値なので、ここに来るのはコードの誤り。黙って数えずに止める
			panic(fmt.Sprintf("readmodel: board tile at %v: %v", pos, err))
		}
		domainTiles = append(domainTiles, board.Tile{Position: pos, Kind: kind})
	}
	domainConnections := slices.Collect(maps.Keys(connections))
	problems := board.CheckReadiness(domainTiles, domainConnections, slices.Collect(maps.Keys(pairs))).Problems()
	result := make([]BoardPublishProblem, 0, len(problems))
	for _, problem := range problems {
		positions := make([]BoardPosition, 0, len(problem.Positions))
		for _, pos := range problem.Positions {
			positions = append(positions, NewBoardPosition(pos.X.Int(), pos.Y.Int()))
		}
		result = append(result, BoardPublishProblem{Kind: BoardPublishProblemKind(problem.Kind), Positions: positions})
	}
	return result
}

type BoardPosition struct {
	X int `json:"x"`
	Y int `json:"y"`
}

func NewBoardPosition(x, y int) BoardPosition {
	return BoardPosition{X: x, Y: y}
}

// Element は土地のときだけ値があり、そうでなければ省く（空）
type BoardTile struct {
	X       int              `json:"x"`
	Y       int              `json:"y"`
	Kind    BoardTileKind    `json:"kind"`
	Element BoardTileElement `json:"element,omitempty"`
}

type BoardConnection struct {
	A BoardPosition `json:"a"`
	B BoardPosition `json:"b"`
}

// boards.metadata の pairs の 1 つ。a → b の順（a が並べ方の小さい方）
type BoardPair struct {
	A BoardPosition `json:"a"`
	B BoardPosition `json:"b"`
}

// boards の 1 行。個別の列（SortName・Status）は並べ替え・絞り込み用、Metadata は表示用
type Board struct {
	BoardID  uuid.UUID
	SortName string
	Status   BoardStatus
	Metadata BoardMetadata
	// イベントはオペレーターを内部 ID で持つため、内部 ID で持ち、書くときに公開 ID に引き直す（NPCAvatar と同じ）
	RegisteredBy operator.ID
	UpdatedBy    operator.ID
	// マスとつながりと対の今の集合。イベントの差分を当てるための作業用で、DB には持たない（Metadata.Tiles・Connections・Pairs に並べて持つ）
	tiles       map[board.Position]BoardTile
	connections map[board.Connection]struct{}
	pairs       map[board.Pair]struct{}
	// 集約が削除された。DB には持たず、投影が行を消すかを決めるのに使う
	IsDeleted bool
}

// 日時はイベントの発生時刻から作る（DB の現在時刻は使わない）。マスは y → x、つながりと対は a → b の順に並べる
type BoardMetadata struct {
	ID           uuid.UUID         `json:"id"`
	Name         string            `json:"name"`
	Status       BoardStatus       `json:"status"`
	Revision     int64             `json:"revision"`
	Width        int               `json:"width"`
	Height       int               `json:"height"`
	Tiles        []BoardTile       `json:"tiles"`
	Connections  []BoardConnection `json:"connections"`
	Pairs        []BoardPair       `json:"pairs"`
	DefaultRules BoardDefaultRules `json:"default_rules"`
	TileCounts   BoardTileCounts   `json:"tile_counts"`
	// 公開できない理由（空なら公開できる）。レイアウトが変わるたびに投影が計算し直す。
	// 状態（公開中等）は見ない。公開中のボードは条件を満たしているので、空になる
	PublishProblems []BoardPublishProblem `json:"publish_problems"`
	RegisteredAt    time.Time             `json:"registered_at"`
	UpdatedAt       time.Time             `json:"updated_at"`
	// 投影では空のまま。書くときに repository が RegisteredBy・UpdatedBy から公開 ID を入れる
	RegisteredBy uuid.UUID `json:"registered_by"`
	UpdatedBy    uuid.UUID `json:"updated_by"`
}

// ボードのイベントだけを扱い、ほかは状態をそのまま返す。状態を書き換えないよう、マスとつながりと対は写してから変える
func (state Board) Evolve(event any) Board {
	switch e := event.(type) {
	case *board.RegisteredEvent:
		occurredAt := e.GetMeta().OccurredAt
		state.BoardID = e.PublicID
		state.SortName = domain.NormalizeName(e.Name.String())
		state.Status = BoardStatusDraft
		state.tiles = map[board.Position]BoardTile{}
		state.connections = map[board.Connection]struct{}{}
		state.pairs = map[board.Pair]struct{}{}
		state.Metadata = BoardMetadata{
			ID: e.PublicID, Name: e.Name.String(), Status: BoardStatusDraft, Revision: e.GetMeta().AggregateVersion,
			Width: e.Width.Int(), Height: e.Height.Int(),
			Tiles: []BoardTile{}, Connections: []BoardConnection{}, Pairs: []BoardPair{}, DefaultRules: toBoardDefaultRules(e.DefaultRules),
			PublishProblems: publishProblems(state.tiles, state.connections, state.pairs),
			RegisteredAt:    occurredAt, UpdatedAt: occurredAt,
		}
		state.RegisteredBy = e.RegisteredBy
		state.UpdatedBy = e.RegisteredBy
	case *board.RenamedEvent:
		state.SortName = domain.NormalizeName(e.Name.String())
		state.Metadata.Name = e.Name.String()
		state = state.touched(e.GetMeta().AggregateVersion, e.GetMeta().OccurredAt, e.RenamedBy)
	case *board.LayoutChangedEvent:
		state = state.changeLayout(e)
		state = state.touched(e.GetMeta().AggregateVersion, e.GetMeta().OccurredAt, e.ChangedBy)
	case *board.DefaultRulesChangedEvent:
		state.Metadata.DefaultRules = toBoardDefaultRules(e.DefaultRules)
		state = state.touched(e.GetMeta().AggregateVersion, e.GetMeta().OccurredAt, e.ChangedBy)
	case *board.PublishedEvent:
		state = state.changeStatus(BoardStatusPublished, e.GetMeta().AggregateVersion, e.GetMeta().OccurredAt, e.PublishedBy)
	case *board.RetiredEvent:
		state = state.changeStatus(BoardStatusRetired, e.GetMeta().AggregateVersion, e.GetMeta().OccurredAt, e.RetiredBy)
	case *board.DeletedEvent:
		state.IsDeleted = true
	}
	return state
}

func (state Board) changeLayout(e *board.LayoutChangedEvent) Board {
	tiles := maps.Clone(state.tiles)
	connections := maps.Clone(state.connections)
	pairs := maps.Clone(state.pairs)
	if e.Size != nil {
		state.Metadata.Width = e.Size.Width.Int()
		state.Metadata.Height = e.Size.Height.Int()
	}
	for _, pos := range e.RemovedTiles {
		delete(tiles, pos)
	}
	for _, tile := range e.PlacedTiles {
		tiles[tile.Position] = toBoardTile(tile)
	}
	for _, tile := range e.ChangedTiles {
		tiles[tile.Position] = toBoardTile(tile)
	}
	for _, connection := range e.Disconnected {
		delete(connections, connection)
	}
	for _, connection := range e.Connected {
		connections[connection] = struct{}{}
	}
	for _, pair := range e.Unpaired {
		delete(pairs, pair)
	}
	for _, pair := range e.Paired {
		pairs[pair] = struct{}{}
	}
	state.tiles, state.connections, state.pairs = tiles, connections, pairs
	state.Metadata.TileCounts = countBoardTiles(tiles)
	state.Metadata.PublishProblems = publishProblems(tiles, connections, pairs)

	positions := slices.SortedFunc(maps.Keys(tiles), board.Position.Compare)
	state.Metadata.Tiles = make([]BoardTile, 0, len(positions))
	for _, pos := range positions {
		state.Metadata.Tiles = append(state.Metadata.Tiles, tiles[pos])
	}
	sorted := slices.SortedFunc(maps.Keys(connections), board.Connection.Compare)
	state.Metadata.Connections = make([]BoardConnection, 0, len(sorted))
	for _, connection := range sorted {
		state.Metadata.Connections = append(state.Metadata.Connections, BoardConnection{
			A: NewBoardPosition(connection.A.X.Int(), connection.A.Y.Int()),
			B: NewBoardPosition(connection.B.X.Int(), connection.B.Y.Int()),
		})
	}
	sortedPairs := slices.SortedFunc(maps.Keys(pairs), board.Pair.Compare)
	state.Metadata.Pairs = make([]BoardPair, 0, len(sortedPairs))
	for _, pair := range sortedPairs {
		state.Metadata.Pairs = append(state.Metadata.Pairs, BoardPair{
			A: NewBoardPosition(pair.A.X.Int(), pair.A.Y.Int()),
			B: NewBoardPosition(pair.B.X.Int(), pair.B.Y.Int()),
		})
	}
	return state
}

func toBoardTile(tile board.Tile) BoardTile {
	return BoardTile{X: tile.Position.X.Int(), Y: tile.Position.Y.Int(), Kind: BoardTileKind(tile.Kind), Element: BoardTileElement(tile.Element)}
}

func (state Board) changeStatus(status BoardStatus, revision int64, at time.Time, by operator.ID) Board {
	state.Status = status
	state.Metadata.Status = status
	return state.touched(revision, at, by)
}

func (state Board) touched(revision int64, at time.Time, by operator.ID) Board {
	state.Metadata.Revision = revision
	state.Metadata.UpdatedAt = at
	state.UpdatedBy = by
	return state
}

func (state Board) IsTerminal() bool {
	return state.IsDeleted
}
