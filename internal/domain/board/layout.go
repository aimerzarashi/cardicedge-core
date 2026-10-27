package board

import (
	"cmp"
	"encoding/json"
	"fmt"
	"maps"
	"slices"
)

// 幅・高さの範囲は、名前の長さと同じく値の形の決まりとして core に固定する。ゲーム全体のルール（gamerules）にすると、
// 運用で下げたときに既存のボードをどう扱うかを決める必要が出るため
const (
	MinSize     = 1
	MaxSize     = 32
	DefaultSize = 8
)

type Size struct {
	width  int
	height int
}

func NewSize(width, height int) (Size, error) {
	if width < MinSize || width > MaxSize || height < MinSize || height > MaxSize {
		return Size{}, fmt.Errorf("%w: %dx%d", ErrSizeOutOfRange, width, height)
	}
	return Size{width: width, height: height}, nil
}

func (s Size) Width() int {
	return s.width
}

func (s Size) Height() int {
	return s.height
}

type sizeJSON struct {
	Width  int `json:"width"`
	Height int `json:"height"`
}

func (s Size) MarshalJSON() ([]byte, error) {
	return json.Marshal(sizeJSON{Width: s.width, Height: s.height})
}

// 範囲は確かめない（読み出しをルールの変更に巻き込まないため。MinSize・MaxSize は書くときに NewSize で守る）
func (s *Size) UnmarshalJSON(data []byte) error {
	var v sizeJSON
	if err := json.Unmarshal(data, &v); err != nil {
		return err
	}
	*s = Size{width: v.Width, height: v.Height}
	return nil
}

// x が列、y が行（0 始まり）。並べ方は y → x
type Position struct {
	X int `json:"x"`
	Y int `json:"y"`
}

func NewPosition(x, y int) Position {
	return Position{X: x, Y: y}
}

func (p Position) Compare(other Position) int {
	return cmp.Or(cmp.Compare(p.Y, other.Y), cmp.Compare(p.X, other.X))
}

func (p Position) adjacentTo(other Position) bool {
	return abs(p.X-other.X)+abs(p.Y-other.Y) == 1
}

func abs(value int) int {
	if value < 0 {
		return -value
	}
	return value
}

type TileKind string

const (
	TileKindLand       TileKind = "land"
	TileKindBase       TileKind = "base"
	TileKindCheckpoint TileKind = "checkpoint"
)

func NewTileKind(value string) (TileKind, error) {
	kind := TileKind(value)
	switch kind {
	case TileKindLand, TileKindBase, TileKindCheckpoint:
		return kind, nil
	default:
		return "", fmt.Errorf("%w: %q", ErrTileKindUnknown, value)
	}
}

func (k TileKind) String() string {
	return string(k)
}

type Tile struct {
	Position Position
	Kind     TileKind
}

type tileJSON struct {
	X    int      `json:"x"`
	Y    int      `json:"y"`
	Kind TileKind `json:"kind"`
}

// 座標を入れ子にしない。イベントの payload を読みやすくするため
func (t Tile) MarshalJSON() ([]byte, error) {
	return json.Marshal(tileJSON{X: t.Position.X, Y: t.Position.Y, Kind: t.Kind})
}

func (t *Tile) UnmarshalJSON(data []byte) error {
	var v tileJSON
	if err := json.Unmarshal(data, &v); err != nil {
		return err
	}
	*t = Tile{Position: NewPosition(v.X, v.Y), Kind: v.Kind}
	return nil
}

// 向きが無いので、並べ方の小さい方を A にそろえて 1 通りにする（a,b と b,a を同じつながりにし、map の key にできるように）
type Connection struct {
	a Position
	b Position
}

func NewConnection(a, b Position) Connection {
	if a.Compare(b) > 0 {
		a, b = b, a
	}
	return Connection{a: a, b: b}
}

func (c Connection) A() Position {
	return c.a
}

func (c Connection) B() Position {
	return c.b
}

func (c Connection) Compare(other Connection) int {
	return cmp.Or(c.a.Compare(other.a), c.b.Compare(other.b))
}

type connectionJSON struct {
	A Position `json:"a"`
	B Position `json:"b"`
}

func (c Connection) MarshalJSON() ([]byte, error) {
	return json.Marshal(connectionJSON{A: c.a, B: c.b})
}

func (c *Connection) UnmarshalJSON(data []byte) error {
	var v connectionJSON
	if err := json.Unmarshal(data, &v); err != nil {
		return err
	}
	*c = NewConnection(v.A, v.B)
	return nil
}

// どの要素の、どの部分か
type Subject string

const (
	SubjectTile       Subject = "tile"
	SubjectConnection Subject = "connection"
)

// 違反した部分。マスの座標は x・y、つながりの端は a・b。要素の全体なら空
type Part string

const (
	PartWhole Part = ""
	PartX     Part = "x"
	PartY     Part = "y"
	PartA     Part = "a"
	PartB     Part = "b"
)

type Reason string

const (
	// マスの x または y が格子の外
	ReasonOutOfRange Reason = "out_of_range"
	// 同じ座標のマスの 2 回目
	ReasonDuplicatedTile Reason = "duplicated_tile"
	// つながりの端にマスが無い
	ReasonTileNotFound Reason = "tile_not_found"
	// つながりが隣どうしでない（a と b が同じ座標を含む）
	ReasonNotAdjacent Reason = "not_adjacent"
	// 同じつながり（a,b の順を入れ替えたものを含む）の 2 回目
	ReasonDuplicatedConnection Reason = "duplicated_connection"
)

// Index は渡された tiles / connections の何番目か
type LayoutViolation struct {
	Subject Subject
	Index   int
	Part    Part
	Reason  Reason
}

// レイアウトの決まりはここだけに書く。application 層はこの一覧を入力の誤りの項目のパスに写すだけにし、決まりを 2 か所に書かない。
// 最初の違反で止めないのは、画面が直すべき項目を一度に示せるようにするため。
// 同じ座標のマスは、格子の外のものでは調べない（範囲の違反と二重に出さないため）。同じつながりは、隣どうしのものだけ調べる
// （隣どうしでないものは、それだけで違反になっているため）
func ValidateLayout(size Size, tiles []Tile, connections []Connection) []LayoutViolation {
	var violations []LayoutViolation
	placed := make(map[Position]struct{}, len(tiles))
	for i, tile := range tiles {
		pos := tile.Position
		inRange := true
		if pos.X < 0 || pos.X >= size.width {
			violations = append(violations, LayoutViolation{Subject: SubjectTile, Index: i, Part: PartX, Reason: ReasonOutOfRange})
			inRange = false
		}
		if pos.Y < 0 || pos.Y >= size.height {
			violations = append(violations, LayoutViolation{Subject: SubjectTile, Index: i, Part: PartY, Reason: ReasonOutOfRange})
			inRange = false
		}
		if !inRange {
			continue
		}
		if _, ok := placed[pos]; ok {
			violations = append(violations, LayoutViolation{Subject: SubjectTile, Index: i, Part: PartWhole, Reason: ReasonDuplicatedTile})
			continue
		}
		placed[pos] = struct{}{}
	}

	seen := make(map[Connection]struct{}, len(connections))
	for i, connection := range connections {
		if _, ok := placed[connection.a]; !ok {
			violations = append(violations, LayoutViolation{Subject: SubjectConnection, Index: i, Part: PartA, Reason: ReasonTileNotFound})
		}
		if _, ok := placed[connection.b]; !ok {
			violations = append(violations, LayoutViolation{Subject: SubjectConnection, Index: i, Part: PartB, Reason: ReasonTileNotFound})
		}
		if !connection.a.adjacentTo(connection.b) {
			violations = append(violations, LayoutViolation{Subject: SubjectConnection, Index: i, Part: PartWhole, Reason: ReasonNotAdjacent})
			continue
		}
		if _, ok := seen[connection]; ok {
			violations = append(violations, LayoutViolation{Subject: SubjectConnection, Index: i, Part: PartWhole, Reason: ReasonDuplicatedConnection})
			continue
		}
		seen[connection] = struct{}{}
	}
	return violations
}

// 大きさ・マス・つながりをまとめた値オブジェクト。保存の時点で ValidateLayout の決まりを必ず守る（準備中でも）。
// 作った後は変えない（中の map を外に出さず、読むときは並べて写す）
type Layout struct {
	size        Size
	tiles       map[Position]TileKind
	connections map[Connection]struct{}
}

// 違反があれば *InvalidLayoutError（errors.Is で ErrInvalidLayout）
func NewLayout(size Size, tiles []Tile, connections []Connection) (Layout, error) {
	if violations := ValidateLayout(size, tiles, connections); len(violations) > 0 {
		return Layout{}, &InvalidLayoutError{Violations: violations}
	}
	layout := Layout{
		size:        size,
		tiles:       make(map[Position]TileKind, len(tiles)),
		connections: make(map[Connection]struct{}, len(connections)),
	}
	for _, tile := range tiles {
		layout.tiles[tile.Position] = tile.Kind
	}
	for _, connection := range connections {
		layout.connections[connection] = struct{}{}
	}
	return layout, nil
}

// 登録の直後のレイアウト。マスもつながりも無い
func EmptyLayout(size Size) Layout {
	return Layout{size: size, tiles: map[Position]TileKind{}, connections: map[Connection]struct{}{}}
}

func (l Layout) Size() Size {
	return l.size
}

// y → x の順
func (l Layout) Tiles() []Tile {
	tiles := make([]Tile, 0, len(l.tiles))
	for pos, kind := range l.tiles {
		tiles = append(tiles, Tile{Position: pos, Kind: kind})
	}
	slices.SortFunc(tiles, func(a, b Tile) int { return a.Position.Compare(b.Position) })
	return tiles
}

// a → b の順
func (l Layout) Connections() []Connection {
	connections := slices.Collect(maps.Keys(l.connections))
	slices.SortFunc(connections, Connection.Compare)
	return connections
}

func (l Layout) KindAt(pos Position) (TileKind, bool) {
	kind, ok := l.tiles[pos]
	return kind, ok
}

// 公開の条件を数えた結果。条件を満たすかの判定は Publishable
type Readiness struct {
	BaseCount       int
	CheckpointCount int
	// 拠点からつながりをたどって行けないマス（y → x の順）。拠点がちょうど 1 つのときだけ計算し、そうでなければ空
	// （どこから行けるかが決まらないため）
	UnreachableTiles []Position
}

// 拠点がちょうど 1 つ、チェックポイントが 1 つ以上、すべてのマスに拠点から行ける（行き止まりは認める）
func (r Readiness) Publishable() bool {
	return r.BaseCount == 1 && r.CheckpointCount >= 1 && len(r.UnreachableTiles) == 0
}

func (l Layout) Readiness() Readiness {
	var readiness Readiness
	var base Position
	for pos, kind := range l.tiles {
		switch kind {
		case TileKindBase:
			readiness.BaseCount++
			base = pos
		case TileKindCheckpoint:
			readiness.CheckpointCount++
		}
	}
	if readiness.BaseCount != 1 {
		return readiness
	}

	neighbors := make(map[Position][]Position, len(l.tiles))
	for connection := range l.connections {
		neighbors[connection.a] = append(neighbors[connection.a], connection.b)
		neighbors[connection.b] = append(neighbors[connection.b], connection.a)
	}
	reached := map[Position]struct{}{base: {}}
	queue := []Position{base}
	for len(queue) > 0 {
		pos := queue[0]
		queue = queue[1:]
		for _, next := range neighbors[pos] {
			if _, ok := reached[next]; !ok {
				reached[next] = struct{}{}
				queue = append(queue, next)
			}
		}
	}
	for pos := range l.tiles {
		if _, ok := reached[pos]; !ok {
			readiness.UnreachableTiles = append(readiness.UnreachableTiles, pos)
		}
	}
	slices.SortFunc(readiness.UnreachableTiles, Position.Compare)
	return readiness
}
