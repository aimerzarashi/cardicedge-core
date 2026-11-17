package command

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/match"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/room"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/rdb"
)

// 対戦の公開 ID から、その対戦を始めた部屋の公開 ID を引く。どの部屋にも無ければ found が false。実装は部屋の read model（rooms）で、
// 対戦の集約は部屋を持たない（パッケージが循環するため）ので、部屋の側の「始めた対戦の一覧」から逆に引く
type FindRoomByMatchFunc func(ctx context.Context, matchID uuid.UUID) (roomID uuid.UUID, number int, found bool, err error)

// 部屋が見つからなかった。部屋の投影が遅れている間は、まだ一覧に対戦が無い。ポリシーが失敗として返し、待てば直る失敗としてやり直しに任せる
// （握りつぶすと、部屋が次を決める状態に進まないまま残る）
var ErrRoomOfMatchNotFound = errors.New("command: room of match not found")

// 対戦の AllRewardsReceivedEvent（全員が戦利品を受け取り終えた）を受けて、その対戦の部屋を次を決める状態に進めるポリシー。
// 対戦の操作の中で部屋を変えない（1 つのトランザクションで変える集約を 1 つにするため）
type RoomMatchFinishHandler struct {
	rooms      domain.EventStore[room.ID, room.Event, room.State]
	roomIDs    domain.Identifier[room.ID]
	entityRefs domain.EntityRefResolver
	findRoom   FindRoomByMatchFunc
	now        func() time.Time
}

func NewRoomMatchFinishHandler(
	rooms domain.EventStore[room.ID, room.Event, room.State],
	roomIDs domain.Identifier[room.ID],
	entityRefs domain.EntityRefResolver,
	findRoom FindRoomByMatchFunc,
	now func() time.Time,
) *RoomMatchFinishHandler {
	return &RoomMatchFinishHandler{rooms: rooms, roomIDs: roomIDs, entityRefs: entityRefs, findRoom: findRoom, now: now}
}

// 購読する対戦のほかのイベントは何もしない
func (c *RoomMatchFinishHandler) Handle(ctx context.Context, event any) error {
	if e, ok := event.(*match.AllRewardsReceivedEvent); ok {
		return c.Finish(ctx, e.GetMeta().EntityID)
	}
	return nil
}

// 対戦を始めた部屋を、次を決める状態に進める。部屋は read model で引く。もう進んでいれば、部屋の Decide が何もしない（ポリシーは二度動くことがある）
func (c *RoomMatchFinishHandler) Finish(ctx context.Context, matchID match.ID) error {
	ref := domain.NewEntityRef(matchID.EntityType(), matchID.Int64())
	publicIDs, err := c.entityRefs.PublicIDs(ctx, []domain.EntityRef{ref})
	if err != nil {
		return err
	}
	matchPublicID, ok := publicIDs[ref]
	if !ok {
		return fmt.Errorf("room match finish: match %d has no public id", matchID)
	}
	roomPublicID, _, found, err := c.findRoom(ctx, matchPublicID)
	if err != nil {
		return err
	}
	if !found {
		return fmt.Errorf("room match finish: match %s: %w", matchPublicID, ErrRoomOfMatchNotFound)
	}
	roomID, err := c.roomIDs.Resolve(ctx, roomPublicID)
	if err != nil {
		return err
	}
	_, err = rdb.Transact(ctx, func(ctx context.Context) (struct{}, error) {
		state, err := c.rooms.Load(ctx, roomID)
		if err != nil {
			return struct{}{}, err
		}
		events, err := room.Decide(state, &room.FinishMatchCommand{Match: matchID}, c.now())
		if err != nil {
			return struct{}{}, err
		}
		return struct{}{}, c.rooms.Save(ctx, events)
	})
	return err
}
