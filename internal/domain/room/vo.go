package room

import (
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/avatar"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/board"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/deck"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/gamerules"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/npcavatar"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/npcdeck"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/player"
)

type ID int64

var entityType = domain.NewEntityType("room")

func (ID) EntityType() domain.EntityType {
	return entityType
}

func (id ID) Int64() int64 {
	return int64(id)
}

func NewID(value int64) (ID, error) {
	if value <= 0 {
		return 0, errors.New("room id must be positive")
	}
	return ID(value), nil
}

// 部屋の名前の長さの上限。ほかの集約の名前（avatar.MaxNameLength 等）と今は同じ長さだが、参照せずに値を持つ。
// どれかだけ変えたくなったときに分けられなくなるため
const MaxNameLength = 20

// 部屋の名前。ほかの部屋と同じ名前でもよい（一意な key を持たない。部屋はプレイヤーごとに作るもので、マスタのように 1 つの一覧で見分ける必要がないため）
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

// 部屋の状態。マスタの draft の訳語が「準備中」なので、部屋の DRAFT は日本語で「公開前」と書いて分ける。
// 募集中（RECRUITING）・対戦中（PLAYING）は、使う操作（参加・対戦の開始）を作るときに足す（使わない値を先に置かない。
// 公開は同時に準備中へ進むので、公開しただけの状態は持たない）
type Status string

const (
	StatusDraft     Status = "draft"
	StatusPreparing Status = "preparing"
	StatusClosed    Status = "closed"
)

func (s Status) String() string {
	return string(s)
}

var ErrSeatKindUnknown = errors.New("room: unknown seat kind")

// 席に座るのがプレイヤーか NPC か。形式（個人戦／チーム戦）とチームは持たない
type SeatKind string

const (
	SeatKindPlayer SeatKind = "player"
	SeatKindNPC    SeatKind = "npc"
)

func NewSeatKind(value string) (SeatKind, error) {
	kind := SeatKind(value)
	switch kind {
	case SeatKindPlayer, SeatKindNPC:
		return kind, nil
	default:
		return "", fmt.Errorf("%w: %q", ErrSeatKindUnknown, value)
	}
}

func (k SeatKind) String() string {
	return string(k)
}

var ErrSeatIndexNegative = errors.New("room: seat index must not be negative")

// 席の並びの何番目か（0 始まり）。範囲は席の並びを持つ部屋の側が確かめる
type SeatIndex int

func NewSeatIndex(value int) (SeatIndex, error) {
	if value < 0 {
		return 0, ErrSeatIndexNegative
	}
	return SeatIndex(value), nil
}

func (i SeatIndex) Int() int {
	return int(i)
}

var (
	ErrTooFewSeats  = errors.New("room: too few seats")
	ErrTooManySeats = errors.New("room: too many seats")
	// プレイヤーの席の数が決まりと違う
	ErrPlayerSeatCount = errors.New("room: player seat count mismatch")
)

// このマイルストーンのプレイヤーの席の数。作った人だけが座る。ほかの人が参加できるようになるときに、範囲に変える
const PlayerSeatCount = 1

// 席の種類の並び。席の数は全体のルール（game_rules.match）の人数の範囲で、範囲は型で受け取る（値を確かめずに作れないように）。
// 段階ごとに確かめ、席の数が範囲の外ならプレイヤーの席の数は確かめない
type SeatKinds []SeatKind

func NewSeatKinds(kinds []SeatKind, minPlayers gamerules.MinPlayers, maxPlayers gamerules.MaxPlayers) (SeatKinds, error) {
	if len(kinds) < minPlayers.Int() {
		return nil, ErrTooFewSeats
	}
	if len(kinds) > maxPlayers.Int() {
		return nil, ErrTooManySeats
	}
	playerSeats := 0
	for _, kind := range kinds {
		if kind == SeatKindPlayer {
			playerSeats++
		}
	}
	if playerSeats != PlayerSeatCount {
		return nil, ErrPlayerSeatCount
	}
	return SeatKinds(kinds), nil
}

// 席 1 つ。プレイヤーの席に座っている人は、Decide が席の並びを決めるときに入れる（部品なので生成関数は無い）。NPC の席は 0
type Seat struct {
	Kind   SeatKind  `json:"kind"`
	Player player.ID `json:"player,omitempty"`
}

// プレイヤーの席の選択。アバターもデッキもそのプレイヤーのもの。ほかの集約は同じ境界（game）の内部 ID で指す
type PlayerSeatChoice struct {
	Avatar avatar.ID `json:"avatar"`
	Deck   deck.ID   `json:"deck"`
}

// NPC の席の選択。デッキは、そのアバターに割り当てた NPC のデッキ
type NPCSeatChoice struct {
	Avatar npcavatar.ID `json:"avatar"`
	Deck   npcdeck.ID   `json:"deck"`
}

// 席のアバターとデッキの選択。席の種類に合う側 1 つだけを持つ（項目をまたぐ決まりがあるので生成関数でだけ作る）。
// 保存済みのイベントを読み直すときは、タグで読むだけで確かめない
type SeatChoice struct {
	Player *PlayerSeatChoice `json:"player,omitempty"`
	NPC    *NPCSeatChoice    `json:"npc,omitempty"`
}

func NewPlayerSeatChoice(avatarID avatar.ID, deckID deck.ID) SeatChoice {
	return SeatChoice{Player: &PlayerSeatChoice{Avatar: avatarID, Deck: deckID}}
}

func NewNPCSeatChoice(npcAvatarID npcavatar.ID, npcDeckID npcdeck.ID) SeatChoice {
	return SeatChoice{NPC: &NPCSeatChoice{Avatar: npcAvatarID, Deck: npcDeckID}}
}

// どの席の種類の選択か。どちらも持たない値は読み直しでしか起きないので、プレイヤーの席の選択とみなさない（空にする）
func (c SeatChoice) Kind() SeatKind {
	switch {
	case c.Player != nil:
		return SeatKindPlayer
	case c.NPC != nil:
		return SeatKindNPC
	default:
		return ""
	}
}

// ポインターの先の値で比べる（== はポインターの比べになるため）
func (c SeatChoice) equal(other SeatChoice) bool {
	switch {
	case c.Player != nil && other.Player != nil:
		return *c.Player == *other.Player
	case c.NPC != nil && other.NPC != nil:
		return *c.NPC == *other.NPC
	default:
		return c.Player == nil && other.Player == nil && c.NPC == nil && other.NPC == nil
	}
}

// 部屋の対戦ルール。ボードと、ボードの既定のルール（board.DefaultRules）と同じ名前と決まりの 7 つの値。
// 部屋では既定ではないので入れ子にせず並べる。ボードとルールを 1 つにするのは、ルールはボードの既定を写して変えるもので、
// ボードを変えると前のルールが意味を失うため。決まりは board.NewDefaultRules に 1 つだけ置く
type MatchRules struct {
	Board           board.ID `json:"board"`
	TargetMagic     int      `json:"target_magic"`
	StartingMagic   int      `json:"starting_magic"`
	LapBonus        int      `json:"lap_bonus"`
	CheckpointBonus int      `json:"checkpoint_bonus"`
	RoundLimit      int      `json:"round_limit"`
	DiceMin         int      `json:"dice_min"`
	DiceMax         int      `json:"dice_max"`
}

// 違反は board.NewDefaultRules のエラー（errors.Join）のまま返し、呼ぶ側が errors.Is で項目ごとに分ける
func NewMatchRules(boardID board.ID, targetMagic, startingMagic, lapBonus, checkpointBonus, roundLimit, diceMin, diceMax int) (MatchRules, error) {
	rules, err := board.NewDefaultRules(targetMagic, startingMagic, lapBonus, checkpointBonus, roundLimit, diceMin, diceMax)
	if err != nil {
		return MatchRules{}, err
	}
	return MatchRules{
		Board: boardID, TargetMagic: rules.TargetMagic, StartingMagic: rules.StartingMagic, LapBonus: rules.LapBonus,
		CheckpointBonus: rules.CheckpointBonus, RoundLimit: rules.RoundLimit, DiceMin: rules.DiceMin, DiceMax: rules.DiceMax,
	}, nil
}
