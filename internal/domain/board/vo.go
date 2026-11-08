package board

import (
	"cmp"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"
	"unicode/utf8"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/card"
)

type ID int64

var entityType = domain.NewEntityType("board")

func (ID) EntityType() domain.EntityType {
	return entityType
}

func (id ID) Int64() int64 {
	return int64(id)
}

func NewID(value int64) (ID, error) {
	if value <= 0 {
		return 0, errors.New("board id must be positive")
	}
	return ID(value), nil
}

// カード名と同じ上限にそろえる（画面で同じ幅に収まり、決まりを 2 つ覚えずに済むため）
const MaxNameLength = card.MaxNameLength

type Name string

// 長さを正規化する前の値で数える理由は card.NewName と同じ
func NewName(value string) (Name, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "", ErrNameEmpty
	}
	if utf8.RuneCountInString(trimmed) > MaxNameLength {
		return "", ErrNameTooLong
	}
	return Name(trimmed), nil
}

func (n Name) String() string {
	return string(n)
}

// 配布デッキの状態（grantdeck.Status）と移り方は同じだが、型は共有しない。片方だけ移り方を変えたくなったときに
// 分けられなくなるため。準備中はレイアウトを自由に変えられ、公開の条件を満たさなくてよい。使用不可は新しい対戦に使わない。
// 公開中 → 準備中に戻せないのは、使われている間にレイアウトが変わるため
type Status string

const (
	StatusDraft     Status = "draft"
	StatusPublished Status = "published"
	StatusRetired   Status = "retired"
)

func NewStatus(value string) (Status, error) {
	status := Status(value)
	switch status {
	case StatusDraft, StatusPublished, StatusRetired:
		return status, nil
	default:
		return "", fmt.Errorf("%w: %q", ErrStatusUnknown, value)
	}
}

func (s Status) String() string {
	return string(s)
}

// 公開した後の状態と、状態が変わるか。準備中・使用不可からは公開中にでき、公開中にもう一度公開しても変わらない
func (s Status) Publish() (Status, bool, error) {
	switch s {
	case StatusDraft, StatusRetired:
		return StatusPublished, true, nil
	case StatusPublished:
		return StatusPublished, false, nil
	default:
		return s, false, ErrStatusUnknown
	}
}

// 使用不可にした後の状態と、状態が変わるか。一度も公開していない準備中は使用不可にできない。使用不可にもう一度しても変わらない
func (s Status) Retire() (Status, bool, error) {
	switch s {
	case StatusPublished:
		return StatusRetired, true, nil
	case StatusRetired:
		return StatusRetired, false, nil
	case StatusDraft:
		return s, false, ErrInvalidStatusTransition
	default:
		return s, false, ErrStatusUnknown
	}
}

// 登録の直後の既定のルール（対戦を始めるとき、ここから変えられる前提の値）。登録のイベントが値を明示して持つので、
// 変えても過去のボードの値は変わらない。ラウンドの上限（30）は仮の値
const (
	DefaultTargetMagic     = 8000
	DefaultStartingMagic   = 600
	DefaultLapBonus        = 300
	DefaultCheckpointBonus = 100
	DefaultRoundLimit      = 30
	DefaultDiceMin         = 1
	DefaultDiceMax         = 6
)

// 対戦の既定のルール。効き方は対戦の決まりなので、ここでは値の形だけを決める
type DefaultRules struct {
	// 届いて拠点に戻ると勝ちになる魔力
	TargetMagic   int `json:"target_magic"`
	StartingMagic int `json:"starting_magic"`
	// 拠点に戻ったときに増える魔力
	LapBonus int `json:"lap_bonus"`
	// チェックポイントを通ったときに増える魔力
	CheckpointBonus int `json:"checkpoint_bonus"`
	// 上限に達したら、その時点で魔力の多い人が勝つ
	RoundLimit int `json:"round_limit"`
	// サイコロの目の最小・最大
	DiceMin int `json:"dice_min"`
	DiceMax int `json:"dice_max"`
}

// 上限は持たない（要るなら studio_rules に入れる）。違反は errors.Join でまとめて返し、呼ぶ側が errors.Is で項目ごとに分ける
func NewDefaultRules(targetMagic, startingMagic, lapBonus, checkpointBonus, roundLimit, diceMin, diceMax int) (DefaultRules, error) {
	var errs []error
	if targetMagic < 1 {
		errs = append(errs, ErrTargetMagicTooSmall)
	}
	if startingMagic < 0 {
		errs = append(errs, ErrStartingMagicTooSmall)
	}
	if lapBonus < 0 {
		errs = append(errs, ErrLapBonusTooSmall)
	}
	if checkpointBonus < 0 {
		errs = append(errs, ErrCheckpointBonusTooSmall)
	}
	if roundLimit < 1 {
		errs = append(errs, ErrRoundLimitTooSmall)
	}
	if diceMin < 1 {
		errs = append(errs, ErrDiceMinTooSmall)
	}
	// 下限を割った値と比べても意味がないので、下限を満たすときだけ比べる
	if startingMagic >= 0 && targetMagic <= startingMagic {
		errs = append(errs, ErrTargetMagicNotGreaterThanStartingMagic)
	}
	// 等しいのも誤り。最大と最小が同じだと毎回同じ数だけ進み、サイコロを振る意味が無くなるため（core#71・core#100）
	if diceMin >= 1 && diceMax <= diceMin {
		errs = append(errs, ErrDiceMaxNotGreaterThanDiceMin)
	}
	if len(errs) > 0 {
		return DefaultRules{}, errors.Join(errs...)
	}
	return DefaultRules{
		TargetMagic: targetMagic, StartingMagic: startingMagic, LapBonus: lapBonus, CheckpointBonus: checkpointBonus,
		RoundLimit: roundLimit, DiceMin: diceMin, DiceMax: diceMax,
	}, nil
}

// 登録の直後の既定のルール。定数が決まりを満たすことはテストで確かめる
func InitialDefaultRules() DefaultRules {
	return DefaultRules{
		TargetMagic: DefaultTargetMagic, StartingMagic: DefaultStartingMagic,
		LapBonus: DefaultLapBonus, CheckpointBonus: DefaultCheckpointBonus,
		RoundLimit: DefaultRoundLimit, DiceMin: DefaultDiceMin, DiceMax: DefaultDiceMax,
	}
}

// 登録の直後の大きさ。範囲（SizeRange）の外なら、範囲の中に収める。範囲に入れないのは、
// 下げた後の既存のボードの大きさと関係なく、新しく作るときの初期値でしかないため
const DefaultSize = 8

// ボードの幅・高さの下限（studio_rules の board.min_size）。1 以上（0 以下の大きさの格子は作れない）
type MinSize int

func NewMinSize(value int) (MinSize, error) {
	if value < 1 {
		return 0, fmt.Errorf("%w: min size %d", ErrSizeRangeInvalid, value)
	}
	return MinSize(value), nil
}

func (m MinSize) Int() int {
	return int(m)
}

// ボードの幅・高さの上限（studio_rules の board.max_size）。1 以上。下限以上かは SizeRange が確かめる
type MaxSize int

func NewMaxSize(value int) (MaxSize, error) {
	if value < 1 {
		return 0, fmt.Errorf("%w: max size %d", ErrSizeRangeInvalid, value)
	}
	return MaxSize(value), nil
}

func (m MaxSize) Int() int {
	return int(m)
}

// 幅・高さの範囲（両端を含む）。幅と高さで範囲を分けないのは、縦長・横長のボードを同じ決まりで作れるようにするため。
// 範囲の意味の持ち主は board のため型をここに置く（studiorules が持つ値の型。studiorules から board へ向く向きにして、循環を避ける）。
// 上限が下限以上という項目をまたぐ決まりがあるので、生成関数を持つ
type SizeRange struct {
	MinSize MinSize `json:"min_size"`
	MaxSize MaxSize `json:"max_size"`
}

func NewSizeRange(minSize MinSize, maxSize MaxSize) (SizeRange, error) {
	if maxSize.Int() < minSize.Int() {
		return SizeRange{}, fmt.Errorf("%w: max size %d is below min size %d", ErrSizeRangeInvalid, maxSize, minSize)
	}
	return SizeRange{MinSize: minSize, MaxSize: maxSize}, nil
}

func (r SizeRange) Contains(size int) bool {
	return size >= r.MinSize.Int() && size <= r.MaxSize.Int()
}

func (r SizeRange) Clamp(size int) int {
	return min(max(size, r.MinSize.Int()), r.MaxSize.Int())
}

// 範囲を引数に取るのは、範囲の外の幅・高さを作れないようにするため。確かめるのは書くとき（コマンドを作るとき）だけで、
// 保存済みのイベントを読み直すときは確かめない（JSON をそのまま読む）。範囲を下げても、すでにあるボードを読めるようにするため
type Width int

func NewWidth(value int, within SizeRange) (Width, error) {
	if !within.Contains(value) {
		return 0, fmt.Errorf("%w: width %d", ErrSizeOutOfRange, value)
	}
	return Width(value), nil
}

func (w Width) Int() int {
	return int(w)
}

type Height int

func NewHeight(value int, within SizeRange) (Height, error) {
	if !within.Contains(value) {
		return 0, fmt.Errorf("%w: height %d", ErrSizeOutOfRange, value)
	}
	return Height(value), nil
}

func (h Height) Int() int {
	return int(h)
}

type Size struct {
	Width  Width  `json:"width"`
	Height Height `json:"height"`
}

// 列（0 始まり）。格子の中かは大きさとの組なので ValidateLayout が確かめる
type X int

func NewX(value int) (X, error) {
	if value < 0 {
		return 0, fmt.Errorf("%w: x %d", ErrPositionNegative, value)
	}
	return X(value), nil
}

func (x X) Int() int {
	return int(x)
}

// 行（0 始まり）
type Y int

func NewY(value int) (Y, error) {
	if value < 0 {
		return 0, fmt.Errorf("%w: y %d", ErrPositionNegative, value)
	}
	return Y(value), nil
}

func (y Y) Int() int {
	return int(y)
}

// x が列、y が行（0 始まり）。並べ方は y → x
type Position struct {
	X X `json:"x"`
	Y Y `json:"y"`
}

func (p Position) Compare(other Position) int {
	return cmp.Or(cmp.Compare(p.Y.Int(), other.Y.Int()), cmp.Compare(p.X.Int(), other.X.Int()))
}

func (p Position) adjacentTo(other Position) bool {
	return abs(p.X.Int()-other.X.Int())+abs(p.Y.Int()-other.Y.Int()) == 1
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
	// 転送円と橋は対の相手を持つ（Pair）。施設 4 つは x・y だけ。どれも属性は持たず、動きは対戦を作るときに作る。
	// 施設の名前は仮（世界観のデザインで見直す）
	TileKindWarp         TileKind = "warp"
	TileKindBridge       TileKind = "bridge"
	TileKindSanctuary    TileKind = "sanctuary"
	TileKindShrine       TileKind = "shrine"
	TileKindProphecyShop TileKind = "prophecy_shop"
	TileKindCardShop     TileKind = "card_shop"
)

// 対の相手を持つ種類（転送円・橋）
func (k TileKind) pairable() bool {
	return k == TileKindWarp || k == TileKindBridge
}

func NewTileKind(value string) (TileKind, error) {
	kind := TileKind(value)
	switch kind {
	case TileKindLand, TileKindBase, TileKindCheckpoint,
		TileKindWarp, TileKindBridge, TileKindSanctuary, TileKindShrine, TileKindProphecyShop, TileKindCardShop:
		return kind, nil
	default:
		return "", fmt.Errorf("%w: %q", ErrTileKindUnknown, value)
	}
}

func (k TileKind) String() string {
	return string(k)
}

// 土地の属性（element）。火・地・水・風・無・複の 6 つで、複は火・地・水・風の 4 色のマークが描かれた特殊な土地を表す値の 1 つ
// （組み合わせは持たない）。名前はクリーチャーの element にそろえるが、型は分ける（クリーチャーの element は無が単独・複数持てるなど、
// このボードの element と決まりが別で、共通にすると片方を変えたくなったときに分けられなくなるため）
type Element string

const (
	ElementFire    Element = "fire"
	ElementEarth   Element = "earth"
	ElementWater   Element = "water"
	ElementWind    Element = "wind"
	ElementNeutral Element = "neutral"
	ElementMulti   Element = "multi"
)

func NewElement(value string) (Element, error) {
	element := Element(value)
	switch element {
	case ElementFire, ElementEarth, ElementWater, ElementWind, ElementNeutral, ElementMulti:
		return element, nil
	default:
		return "", fmt.Errorf("%w: %q", ErrElementUnknown, value)
	}
}

func (e Element) String() string {
	return string(e)
}

// Element は土地のときだけ値を持ち、そうでなければ空。属性の有無は種類と組で ValidateLayout が確かめる
type Tile struct {
	Position Position `json:"position"`
	Kind     TileKind `json:"kind"`
	Element  Element  `json:"element,omitempty"`
}

// 向きが無いので、並べ方の小さい方を A にそろえて 1 通りにする（a,b と b,a を同じつながりにし、map の key にできるように）
type Connection struct {
	A Position `json:"a"`
	B Position `json:"b"`
}

func NewConnection(a, b Position) Connection {
	if a.Compare(b) > 0 {
		a, b = b, a
	}
	return Connection{A: a, B: b}
}

func (c Connection) Compare(other Connection) int {
	return cmp.Or(c.A.Compare(other.A), c.B.Compare(other.B))
}

// 転送円どうし・橋どうしの対。向きが無いので Connection と同じく並べ方の小さい方を A にそろえる。
// Connection と型を分けるのは、隣どうしの道（歩数を使う）と、離れていてよい対（転送円・橋）で決まりも意味も違い、取り違えないため
type Pair struct {
	A Position `json:"a"`
	B Position `json:"b"`
}

func NewPair(a, b Position) Pair {
	if a.Compare(b) > 0 {
		a, b = b, a
	}
	return Pair{A: a, B: b}
}

func (p Pair) Compare(other Pair) int {
	return cmp.Or(p.A.Compare(other.A), p.B.Compare(other.B))
}

// どの要素の、どの部分か
type Subject string

const (
	SubjectTile       Subject = "tile"
	SubjectConnection Subject = "connection"
	SubjectPair       Subject = "pair"
)

// 違反した部分。マスの座標は x・y、つながりの端は a・b。要素の全体なら空
type Part string

const (
	PartWhole Part = ""
	PartX     Part = "x"
	PartY     Part = "y"
	PartA     Part = "a"
	PartB     Part = "b"
	// マスの属性
	PartElement Part = "element"
)

type Reason string

const (
	// マスの x または y が格子の外
	ReasonOutOfRange Reason = "out_of_range"
	// 同じ座標のマスの 2 回目
	ReasonDuplicatedTile Reason = "duplicated_tile"
	// つながり・対の端にマスが無い
	ReasonTileNotFound Reason = "tile_not_found"
	// つながりが隣どうしでない（a と b が同じ座標を含む）
	ReasonNotAdjacent Reason = "not_adjacent"
	// 同じつながり（a,b の順を入れ替えたものを含む）の 2 回目
	ReasonDuplicatedConnection Reason = "duplicated_connection"
	// 土地に属性が無い
	ReasonElementRequired Reason = "element_required"
	// 拠点・チェックポイントに属性がある
	ReasonElementNotAllowed Reason = "element_not_allowed"
	// 対の端が転送円でも橋でもない
	ReasonNotPairableTile Reason = "not_pairable_tile"
	// 対の両端が転送円と橋（どちらの対かは端のマスの種類で決まるので、組めるのは同じ種類どうしだけ）
	ReasonMixedPairKinds Reason = "mixed_pair_kinds"
	// 対の両端が同じ座標
	ReasonPairedWithSelf Reason = "paired_with_self"
	// すでに別の対に入っているマス（2 つ目の対の端）
	ReasonTileAlreadyPaired Reason = "tile_already_paired"
	// 同じ対（a,b の順を入れ替えたものを含む）の 2 回目
	ReasonDuplicatedPair Reason = "duplicated_pair"
)

// Index は渡された tiles / connections / pairs の何番目か
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
func ValidateLayout(size Size, tiles []Tile, connections []Connection, pairs []Pair) []LayoutViolation {
	var violations []LayoutViolation
	placed := make(map[Position]TileKind, len(tiles))
	for i, tile := range tiles {
		pos := tile.Position
		inRange := true
		if pos.X.Int() >= size.Width.Int() {
			violations = append(violations, LayoutViolation{Subject: SubjectTile, Index: i, Part: PartX, Reason: ReasonOutOfRange})
			inRange = false
		}
		if pos.Y.Int() >= size.Height.Int() {
			violations = append(violations, LayoutViolation{Subject: SubjectTile, Index: i, Part: PartY, Reason: ReasonOutOfRange})
			inRange = false
		}
		// 座標の誤りとは別に示せるよう、格子の外のマスでも調べる
		switch {
		case tile.Kind == TileKindLand && tile.Element == "":
			violations = append(violations, LayoutViolation{Subject: SubjectTile, Index: i, Part: PartElement, Reason: ReasonElementRequired})
		case tile.Kind != TileKindLand && tile.Element != "":
			violations = append(violations, LayoutViolation{Subject: SubjectTile, Index: i, Part: PartElement, Reason: ReasonElementNotAllowed})
		}
		if !inRange {
			continue
		}
		if _, ok := placed[pos]; ok {
			violations = append(violations, LayoutViolation{Subject: SubjectTile, Index: i, Part: PartWhole, Reason: ReasonDuplicatedTile})
			continue
		}
		placed[pos] = tile.Kind
	}

	seen := make(map[Connection]struct{}, len(connections))
	for i, connection := range connections {
		if _, ok := placed[connection.A]; !ok {
			violations = append(violations, LayoutViolation{Subject: SubjectConnection, Index: i, Part: PartA, Reason: ReasonTileNotFound})
		}
		if _, ok := placed[connection.B]; !ok {
			violations = append(violations, LayoutViolation{Subject: SubjectConnection, Index: i, Part: PartB, Reason: ReasonTileNotFound})
		}
		if !connection.A.adjacentTo(connection.B) {
			violations = append(violations, LayoutViolation{Subject: SubjectConnection, Index: i, Part: PartWhole, Reason: ReasonNotAdjacent})
			continue
		}
		if _, ok := seen[connection]; ok {
			violations = append(violations, LayoutViolation{Subject: SubjectConnection, Index: i, Part: PartWhole, Reason: ReasonDuplicatedConnection})
			continue
		}
		seen[connection] = struct{}{}
	}

	// 端の誤りがある対は、別の対との重なりの検査に使わない（誤りを重ねて出さないため）。離れていてよいので隣どうしは調べない
	seenPairs := make(map[Pair]struct{}, len(pairs))
	paired := make(map[Position]struct{}, len(pairs)*2)
	for i, pair := range pairs {
		before := len(violations)
		kindA, okA := placed[pair.A]
		kindB, okB := placed[pair.B]
		if !okA {
			violations = append(violations, LayoutViolation{Subject: SubjectPair, Index: i, Part: PartA, Reason: ReasonTileNotFound})
		}
		if !okB {
			violations = append(violations, LayoutViolation{Subject: SubjectPair, Index: i, Part: PartB, Reason: ReasonTileNotFound})
		}
		if pair.A == pair.B {
			violations = append(violations, LayoutViolation{Subject: SubjectPair, Index: i, Part: PartWhole, Reason: ReasonPairedWithSelf})
			continue
		}
		if okA && !kindA.pairable() {
			violations = append(violations, LayoutViolation{Subject: SubjectPair, Index: i, Part: PartA, Reason: ReasonNotPairableTile})
		}
		if okB && !kindB.pairable() {
			violations = append(violations, LayoutViolation{Subject: SubjectPair, Index: i, Part: PartB, Reason: ReasonNotPairableTile})
		}
		if okA && okB && kindA.pairable() && kindB.pairable() && kindA != kindB {
			violations = append(violations, LayoutViolation{Subject: SubjectPair, Index: i, Part: PartWhole, Reason: ReasonMixedPairKinds})
		}
		if len(violations) > before {
			continue
		}
		if _, ok := seenPairs[pair]; ok {
			violations = append(violations, LayoutViolation{Subject: SubjectPair, Index: i, Part: PartWhole, Reason: ReasonDuplicatedPair})
			continue
		}
		seenPairs[pair] = struct{}{}
		if _, ok := paired[pair.A]; ok {
			violations = append(violations, LayoutViolation{Subject: SubjectPair, Index: i, Part: PartA, Reason: ReasonTileAlreadyPaired})
		}
		if _, ok := paired[pair.B]; ok {
			violations = append(violations, LayoutViolation{Subject: SubjectPair, Index: i, Part: PartB, Reason: ReasonTileAlreadyPaired})
		}
		paired[pair.A] = struct{}{}
		paired[pair.B] = struct{}{}
	}
	return violations
}

// 大きさ・マス・つながり・対をまとめた値オブジェクト。保存の時点で ValidateLayout の決まりを必ず守る（準備中でも）。
// 作った後は変えない（中の map を外に出さず、読むときは並べて写す）
type Layout struct {
	size        Size
	tiles       map[Position]Tile
	connections map[Connection]struct{}
	pairs       map[Pair]struct{}
}

// 違反があれば *InvalidLayoutError（errors.Is で ErrInvalidLayout）
func NewLayout(size Size, tiles []Tile, connections []Connection, pairs []Pair) (Layout, error) {
	if violations := ValidateLayout(size, tiles, connections, pairs); len(violations) > 0 {
		return Layout{}, &InvalidLayoutError{Violations: violations}
	}
	layout := Layout{
		size:        size,
		tiles:       make(map[Position]Tile, len(tiles)),
		connections: make(map[Connection]struct{}, len(connections)),
		pairs:       make(map[Pair]struct{}, len(pairs)),
	}
	for _, tile := range tiles {
		layout.tiles[tile.Position] = tile
	}
	for _, connection := range connections {
		layout.connections[connection] = struct{}{}
	}
	for _, pair := range pairs {
		layout.pairs[pair] = struct{}{}
	}
	return layout, nil
}

// 登録の直後のレイアウト。マスもつながりも対も無い
func EmptyLayout(size Size) Layout {
	return Layout{size: size, tiles: map[Position]Tile{}, connections: map[Connection]struct{}{}, pairs: map[Pair]struct{}{}}
}

func (l Layout) Size() Size {
	return l.size
}

// y → x の順
func (l Layout) Tiles() []Tile {
	tiles := make([]Tile, 0, len(l.tiles))
	for _, tile := range l.tiles {
		tiles = append(tiles, tile)
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

// a → b の順
func (l Layout) Pairs() []Pair {
	pairs := slices.Collect(maps.Keys(l.pairs))
	slices.SortFunc(pairs, Pair.Compare)
	return pairs
}

func (l Layout) KindAt(pos Position) (TileKind, bool) {
	tile, ok := l.tiles[pos]
	return tile.Kind, ok
}

// 公開の条件を調べた結果。条件を満たすかの判定は Publishable、満たさない理由の一覧は Problems
type Readiness struct {
	// 拠点の座標（y → x の順）。ちょうど 1 つでなければ公開できない
	BasePositions   []Position
	CheckpointCount int
	// 拠点からつながりをたどって行けないマス（y → x の順）。拠点がちょうど 1 つのときだけ計算し、そうでなければ空
	// （どこから行けるかが決まらないため）
	UnreachableTiles []Position
	// 対の相手のいない転送円・橋（y → x の順）。拠点の数によらず計算する
	UnpairedTiles []Position
}

func (r Readiness) BaseCount() int {
	return len(r.BasePositions)
}

// 公開できない理由の種類。理由を足すときは、ここに種類を足して Problems に並べる
type PublishProblemKind string

const (
	// 拠点がちょうど 1 つでない
	PublishProblemBaseCount PublishProblemKind = "base_count"
	// チェックポイントが無い
	PublishProblemNoCheckpoint PublishProblemKind = "no_checkpoint"
	// 拠点から行けないマスがある
	PublishProblemUnreachableTiles PublishProblemKind = "unreachable_tiles"
	// 対の相手のいない転送円・橋がある
	PublishProblemUnpairedTiles PublishProblemKind = "unpaired_tiles"
)

// 公開できない理由 1 つ。Positions は理由に関わるマスの座標（関わるマスの無い理由では空）
type PublishProblem struct {
	Kind      PublishProblemKind
	Positions []Position
}

// 公開できない理由の一覧。順は 拠点の数 → チェックポイント → 行けないマス → 相手のいない転送円・橋 で固定。空なら公開できる
// （拠点がちょうど 1 つ、チェックポイントが 1 つ以上、すべてのマスに拠点から行ける、すべての転送円・橋に対の相手がいる。行き止まりは認める）
func (r Readiness) Problems() []PublishProblem {
	var problems []PublishProblem
	if r.BaseCount() != 1 {
		problems = append(problems, PublishProblem{Kind: PublishProblemBaseCount, Positions: r.BasePositions})
	}
	if r.CheckpointCount < 1 {
		problems = append(problems, PublishProblem{Kind: PublishProblemNoCheckpoint})
	}
	if len(r.UnreachableTiles) > 0 {
		problems = append(problems, PublishProblem{Kind: PublishProblemUnreachableTiles, Positions: r.UnreachableTiles})
	}
	if len(r.UnpairedTiles) > 0 {
		problems = append(problems, PublishProblem{Kind: PublishProblemUnpairedTiles, Positions: r.UnpairedTiles})
	}
	return problems
}

func (r Readiness) Publishable() bool {
	return len(r.Problems()) == 0
}

func (l Layout) Readiness() Readiness {
	return CheckReadiness(l.Tiles(), l.Connections(), l.Pairs())
}

// 公開の条件を調べる。Layout を作らず（大きさも決まりの検査もせず）置いてあるマスとつながりだけで調べるのは、
// 保存済みのイベントから作った read model の投影が、書くときの決まりを読み直さずに同じ判定を使えるようにするため
func CheckReadiness(tiles []Tile, connections []Connection, pairs []Pair) Readiness {
	var readiness Readiness
	tilePositions := make(map[Position]struct{}, len(tiles))
	pairedPositions := make(map[Position]struct{}, len(pairs)*2)
	for _, pair := range pairs {
		pairedPositions[pair.A] = struct{}{}
		pairedPositions[pair.B] = struct{}{}
	}
	for _, tile := range tiles {
		tilePositions[tile.Position] = struct{}{}
		switch tile.Kind {
		case TileKindBase:
			readiness.BasePositions = append(readiness.BasePositions, tile.Position)
		case TileKindCheckpoint:
			readiness.CheckpointCount++
		}
		if _, ok := pairedPositions[tile.Position]; !ok && tile.Kind.pairable() {
			readiness.UnpairedTiles = append(readiness.UnpairedTiles, tile.Position)
		}
	}
	slices.SortFunc(readiness.BasePositions, Position.Compare)
	slices.SortFunc(readiness.UnpairedTiles, Position.Compare)
	if readiness.BaseCount() != 1 {
		return readiness
	}
	base := readiness.BasePositions[0]

	neighbors := make(map[Position][]Position, len(tiles))
	for _, connection := range connections {
		neighbors[connection.A] = append(neighbors[connection.A], connection.B)
		neighbors[connection.B] = append(neighbors[connection.B], connection.A)
	}
	// 対の 2 マスも、隣どうしと同じに道として数える（転送円は止まれば、橋は通れば相手へ移るため、どちらも行き来がある）
	for _, pair := range pairs {
		neighbors[pair.A] = append(neighbors[pair.A], pair.B)
		neighbors[pair.B] = append(neighbors[pair.B], pair.A)
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
	for pos := range tilePositions {
		if _, ok := reached[pos]; !ok {
			readiness.UnreachableTiles = append(readiness.UnreachableTiles, pos)
		}
	}
	slices.SortFunc(readiness.UnreachableTiles, Position.Compare)
	return readiness
}
