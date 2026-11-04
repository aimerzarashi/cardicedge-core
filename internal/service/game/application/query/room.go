package query

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/pagination"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/game/application/input"
)

// 公開 ID に当たる部屋が、そのプレイヤーの部屋として無い（無い・入っていない部屋。存在を明かさないため区別しない）
var ErrRoomNotFound = errors.New("query: room not found")

// 部屋の状態。公開しただけの状態は無く、公開は同時に準備中に進む。準備中の次の対戦中は、全員の準備完了で対戦が始まってから
type RoomStatus string

const (
	RoomStatusDraft     RoomStatus = "draft"
	RoomStatusPreparing RoomStatus = "preparing"
	RoomStatusPlaying   RoomStatus = "playing"
	RoomStatusClosed    RoomStatus = "closed"
)

type RoomSeatKind string

const (
	RoomSeatKindPlayer RoomSeatKind = "player"
	RoomSeatKindNPC    RoomSeatKind = "npc"
)

// 公開 ID と、その場で見せる名前（名前は読むときに avatars・decks・npc_avatars・npc_decks・boards から引く）
type RoomNamed struct {
	ID   uuid.UUID
	Name string
}

// 部屋に入っている人と、準備完了の申告
type RoomParticipant struct {
	PlayerID uuid.UUID
	Owner    bool
	Ready    bool
}

// 席 1 つ。座っている人はプレイヤーの席だけ。アバターとデッキは、まだ選んでいなければ nil（プレイヤーの席は自分のアバターとデッキ、
// NPC の席は NPC のアバターと、割り当てたデッキ）
type RoomSeat struct {
	Kind   RoomSeatKind
	Player *uuid.UUID
	Avatar *RoomNamed
	Deck   *RoomNamed
}

// 対戦ルール。ボードの名前と、7 つの値（ボードの既定のルールと同じ名前と決まり）
type RoomMatchRules struct {
	Board           RoomNamed
	TargetMagic     int
	StartingMagic   int
	LapBonus        int
	CheckpointBonus int
	RoundLimit      int
	DiceMin         int
	DiceMax         int
}

// プレイヤーの部屋。Revision は画面が見ていた版（変える操作に渡す）。席が 0 の間は Seats が空、対戦ルールをまだ決めていなければ MatchRules は nil
type Room struct {
	ID           uuid.UUID
	Name         string
	Revision     int64
	Status       RoomStatus
	Participants []RoomParticipant
	Seats        []RoomSeat
	MatchRules   *RoomMatchRules
	// 始まった対戦の公開用の ID。始まるまでは nil
	Match *uuid.UUID
	// 見ている人にできる操作。詳細（Get）だけが付ける（一覧は nil）
	Capabilities *RoomCapabilities
}

// 新しい順（作った日時 → 公開 ID の降順）の位置
type RoomListCursor struct {
	RegisteredAt time.Time `json:"t"`
	RoomID       uuid.UUID `json:"i"`
}

func (c RoomListCursor) Validate() error {
	if c.RegisteredAt.IsZero() || c.RoomID == uuid.Nil {
		return errors.New("registered at and room id are required")
	}
	return nil
}

type ListRoomsCondition struct {
	After *RoomListCursor
	Limit int
}

type RoomListRow struct {
	Room   Room
	Cursor RoomListCursor
}

// プレイヤーが入っている部屋のうち、閉じていないものを、新しい順に Limit 件まで返す
type ListRoomsByPlayerFunc func(ctx context.Context, playerID uuid.UUID, condition ListRoomsCondition) ([]RoomListRow, error)

// そのプレイヤーが入っている部屋 1 件（閉じた部屋も返す）。無い・入っていなければ ErrRoomNotFound
type GetRoomFunc func(ctx context.Context, playerID uuid.UUID, roomID uuid.UUID) (Room, error)

type RoomHandler struct {
	listByPlayer ListRoomsByPlayerFunc
	get          GetRoomFunc
	capabilities GetRoomCapabilitiesFunc
}

func NewRoomHandler(listByPlayer ListRoomsByPlayerFunc, get GetRoomFunc, capabilities GetRoomCapabilitiesFunc) *RoomHandler {
	return &RoomHandler{listByPlayer: listByPlayer, get: get, capabilities: capabilities}
}

// 入力の誤りは input.Errors（まとめて返す）。項目名は GraphQL の引数名。投影が遅れて 1 件も無いときは、空のページを返す
func (q *RoomHandler) ListByPlayer(ctx context.Context, playerID uuid.UUID, first *int, after *string) (pagination.Connection[Room], error) {
	page, err := pagination.NewCondition[RoomListCursor](first, after)
	if err != nil {
		invalid := input.Errors{}
		if errors.Is(err, pagination.ErrFirstOutOfRange) {
			invalid = append(invalid, &input.OutOfRange{Field: "first", Min: pagination.MinFirst, Max: pagination.MaxFirst})
		}
		if errors.Is(err, pagination.ErrInvalidCursor) {
			invalid = append(invalid, &input.InvalidFormat{Field: "after"})
		}
		if len(invalid) == 0 {
			return pagination.Connection[Room]{}, err
		}
		return pagination.Connection[Room]{}, invalid
	}
	rows, err := q.listByPlayer(ctx, playerID, ListRoomsCondition{After: page.After, Limit: page.FetchLimit()})
	if err != nil {
		return pagination.Connection[Room]{}, err
	}
	rowConnection, err := pagination.BuildConnection(rows, page, func(row RoomListRow) RoomListCursor {
		return row.Cursor
	})
	if err != nil {
		return pagination.Connection[Room]{}, err
	}
	connection := pagination.Connection[Room]{
		Edges:    make([]pagination.Edge[Room], 0, len(rowConnection.Edges)),
		PageInfo: rowConnection.PageInfo,
	}
	for _, edge := range rowConnection.Edges {
		connection.Edges = append(connection.Edges, pagination.Edge[Room]{Node: edge.Node.Room, Cursor: edge.Cursor})
	}
	return connection, nil
}

// 入っていない部屋も無いのと同じ ErrRoomNotFound にする。見ている人にできる操作（Capabilities）を付ける
func (q *RoomHandler) Get(ctx context.Context, playerID uuid.UUID, roomID uuid.UUID) (Room, error) {
	found, err := q.get(ctx, playerID, roomID)
	if err != nil {
		return Room{}, err
	}
	capabilities, err := q.capabilities(ctx, playerID, roomID)
	if err != nil {
		return Room{}, err
	}
	found.Capabilities = &capabilities
	return found, nil
}
