package readmodel

import (
	"fmt"
	"maps"
	"slices"
	"time"

	"github.com/google/uuid"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/board"
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
)

func NewBoardTileKind(value string) (BoardTileKind, error) {
	kind := BoardTileKind(value)
	switch kind {
	case BoardTileKindLand, BoardTileKindBase, BoardTileKindCheckpoint:
		return kind, nil
	default:
		return "", fmt.Errorf("readmodel: unknown board tile kind: %q", value)
	}
}

// 土地の属性。domain の board.Attribute を使わない理由は BoardTileKind と同じ
type BoardTileAttribute string

const (
	BoardTileAttributeFire    BoardTileAttribute = "fire"
	BoardTileAttributeEarth   BoardTileAttribute = "earth"
	BoardTileAttributeWater   BoardTileAttribute = "water"
	BoardTileAttributeWind    BoardTileAttribute = "wind"
	BoardTileAttributeNeutral BoardTileAttribute = "neutral"
	BoardTileAttributeMulti   BoardTileAttribute = "multi"
)

func NewBoardTileAttribute(value string) (BoardTileAttribute, error) {
	attribute := BoardTileAttribute(value)
	switch attribute {
	case BoardTileAttributeFire, BoardTileAttributeEarth, BoardTileAttributeWater, BoardTileAttributeWind, BoardTileAttributeNeutral, BoardTileAttributeMulti:
		return attribute, nil
	default:
		return "", fmt.Errorf("readmodel: unknown board tile attribute: %q", value)
	}
}

// boards.metadata の default_rules。domain の board.DefaultRules を使わない理由は BoardTileKind と同じ
type BoardDefaultRules struct {
	TargetMagic     int `json:"target_magic"`
	StartingMagic   int `json:"starting_magic"`
	LapBonus        int `json:"lap_bonus"`
	CheckpointBonus int `json:"checkpoint_bonus"`
}

func NewBoardDefaultRules(targetMagic, startingMagic, lapBonus, checkpointBonus int) BoardDefaultRules {
	return BoardDefaultRules{TargetMagic: targetMagic, StartingMagic: startingMagic, LapBonus: lapBonus, CheckpointBonus: checkpointBonus}
}

func toBoardDefaultRules(rules board.DefaultRules) BoardDefaultRules {
	return BoardDefaultRules{
		TargetMagic: rules.TargetMagic(), StartingMagic: rules.StartingMagic(), LapBonus: rules.LapBonus(), CheckpointBonus: rules.CheckpointBonus(),
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
}

func NewBoardTileCounts(land, base, checkpoint, fire, earth, water, wind, neutral, multi int) BoardTileCounts {
	return BoardTileCounts{Land: land, Base: base, Checkpoint: checkpoint, Fire: fire, Earth: earth, Water: water, Wind: wind, Neutral: neutral, Multi: multi}
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
		}
		switch tile.Attribute {
		case BoardTileAttributeFire:
			counts.Fire++
		case BoardTileAttributeEarth:
			counts.Earth++
		case BoardTileAttributeWater:
			counts.Water++
		case BoardTileAttributeWind:
			counts.Wind++
		case BoardTileAttributeNeutral:
			counts.Neutral++
		case BoardTileAttributeMulti:
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
)

func NewBoardPublishProblem(kind BoardPublishProblemKind, positions ...BoardPosition) BoardPublishProblem {
	if positions == nil {
		positions = []BoardPosition{}
	}
	return BoardPublishProblem{Kind: kind, Positions: positions}
}

// 置いてあるマスとつながりから、公開できない理由を調べる（判定は domain の board.CheckReadiness の 1 か所）。
// 理由の順は domain の Problems のまま。空（公開できる）でも nil にせず、JSON を [] にする
func publishProblems(tiles map[board.Position]BoardTile, connections map[board.Connection]struct{}) []BoardPublishProblem {
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
	problems := board.CheckReadiness(domainTiles, domainConnections).Problems()
	result := make([]BoardPublishProblem, 0, len(problems))
	for _, problem := range problems {
		positions := make([]BoardPosition, 0, len(problem.Positions))
		for _, pos := range problem.Positions {
			positions = append(positions, NewBoardPosition(pos.X, pos.Y))
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

// Attribute は土地のときだけ値があり、そうでなければ省く（空）
type BoardTile struct {
	X         int                `json:"x"`
	Y         int                `json:"y"`
	Kind      BoardTileKind      `json:"kind"`
	Attribute BoardTileAttribute `json:"attribute,omitempty"`
}

type BoardConnection struct {
	A BoardPosition `json:"a"`
	B BoardPosition `json:"b"`
}

// boards の 1 行。個別の列（SortName・Status）は並べ替え・絞り込み用、Metadata は表示用
type Board struct {
	BoardID  uuid.UUID
	SortName string
	Status   BoardStatus
	Metadata BoardMetadata
	// マスとつながりの今の集合。イベントの差分を当てるための作業用で、DB には持たない（Metadata.Tiles・Connections に並べて持つ）
	tiles       map[board.Position]BoardTile
	connections map[board.Connection]struct{}
	// 集約が削除された。DB には持たず、投影が行を消すかを決めるのに使う
	IsDeleted bool
}

// 日時はイベントの発生時刻から作る（DB の現在時刻は使わない）。マスは y → x、つながりは a → b の順に並べる
type BoardMetadata struct {
	ID           uuid.UUID         `json:"id"`
	Name         string            `json:"name"`
	Status       BoardStatus       `json:"status"`
	Revision     int64             `json:"revision"`
	Width        int               `json:"width"`
	Height       int               `json:"height"`
	Tiles        []BoardTile       `json:"tiles"`
	Connections  []BoardConnection `json:"connections"`
	DefaultRules BoardDefaultRules `json:"default_rules"`
	TileCounts   BoardTileCounts   `json:"tile_counts"`
	// 公開できない理由（空なら公開できる）。レイアウトが変わるたびに投影が計算し直す。
	// 状態（公開中等）は見ない。公開中のボードは条件を満たしているので、空になる
	PublishProblems []BoardPublishProblem `json:"publish_problems"`
	RegisteredAt    time.Time             `json:"registered_at"`
	UpdatedAt       time.Time             `json:"updated_at"`
}

// ボードのイベントだけを扱い、ほかは状態をそのまま返す。状態を書き換えないよう、マスとつながりは写してから変える
func (state Board) Evolve(event any) Board {
	switch e := event.(type) {
	case *board.RegisteredEvent:
		occurredAt := e.GetMeta().OccurredAt
		state.BoardID = e.PublicID
		state.SortName = domain.NormalizeName(e.Name.String())
		state.Status = BoardStatusDraft
		state.tiles = map[board.Position]BoardTile{}
		state.connections = map[board.Connection]struct{}{}
		state.Metadata = BoardMetadata{
			ID: e.PublicID, Name: e.Name.String(), Status: BoardStatusDraft, Revision: e.GetMeta().AggregateVersion,
			Width: e.Width, Height: e.Height,
			Tiles: []BoardTile{}, Connections: []BoardConnection{}, DefaultRules: toBoardDefaultRules(e.DefaultRules),
			PublishProblems: publishProblems(state.tiles, state.connections),
			RegisteredAt:    occurredAt, UpdatedAt: occurredAt,
		}
	case *board.RenamedEvent:
		state.SortName = domain.NormalizeName(e.Name.String())
		state.Metadata.Name = e.Name.String()
		state = state.touched(e.GetMeta().AggregateVersion, e.GetMeta().OccurredAt)
	case *board.LayoutChangedEvent:
		state = state.changeLayout(e)
		state = state.touched(e.GetMeta().AggregateVersion, e.GetMeta().OccurredAt)
	case *board.DefaultRulesChangedEvent:
		state.Metadata.DefaultRules = toBoardDefaultRules(e.DefaultRules)
		state = state.touched(e.GetMeta().AggregateVersion, e.GetMeta().OccurredAt)
	case *board.PublishedEvent:
		state = state.changeStatus(BoardStatusPublished, e.GetMeta().AggregateVersion, e.GetMeta().OccurredAt)
	case *board.RetiredEvent:
		state = state.changeStatus(BoardStatusRetired, e.GetMeta().AggregateVersion, e.GetMeta().OccurredAt)
	case *board.DeletedEvent:
		state.IsDeleted = true
	}
	return state
}

func (state Board) changeLayout(e *board.LayoutChangedEvent) Board {
	tiles := maps.Clone(state.tiles)
	connections := maps.Clone(state.connections)
	if e.Size != nil {
		state.Metadata.Width = e.Size.Width()
		state.Metadata.Height = e.Size.Height()
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
	state.tiles, state.connections = tiles, connections
	state.Metadata.TileCounts = countBoardTiles(tiles)
	state.Metadata.PublishProblems = publishProblems(tiles, connections)

	positions := slices.SortedFunc(maps.Keys(tiles), board.Position.Compare)
	state.Metadata.Tiles = make([]BoardTile, 0, len(positions))
	for _, pos := range positions {
		state.Metadata.Tiles = append(state.Metadata.Tiles, tiles[pos])
	}
	sorted := slices.SortedFunc(maps.Keys(connections), board.Connection.Compare)
	state.Metadata.Connections = make([]BoardConnection, 0, len(sorted))
	for _, connection := range sorted {
		state.Metadata.Connections = append(state.Metadata.Connections, BoardConnection{
			A: NewBoardPosition(connection.A().X, connection.A().Y),
			B: NewBoardPosition(connection.B().X, connection.B().Y),
		})
	}
	return state
}

func toBoardTile(tile board.Tile) BoardTile {
	return BoardTile{X: tile.Position.X, Y: tile.Position.Y, Kind: BoardTileKind(tile.Kind), Attribute: BoardTileAttribute(tile.Attribute)}
}

func (state Board) changeStatus(status BoardStatus, revision int64, at time.Time) Board {
	state.Status = status
	state.Metadata.Status = status
	return state.touched(revision, at)
}

func (state Board) touched(revision int64, at time.Time) Board {
	state.Metadata.Revision = revision
	state.Metadata.UpdatedAt = at
	return state
}

func (state Board) IsTerminal() bool {
	return state.IsDeleted
}
