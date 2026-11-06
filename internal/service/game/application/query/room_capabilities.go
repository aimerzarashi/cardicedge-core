package query

import (
	"context"
	"errors"

	"github.com/google/uuid"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/player"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/room"
)

// 見ている人にできる操作。ChangeSeatChoice は席の並びの順（Room.Seats と同じ順）。
// 画面は担当・公開前に決めたものの固定・準備完了の条件を自分で判断せず、この値で出し分けるだけにする
type RoomCapabilities struct {
	ChangeSeats      bool
	ChangeMatchRules bool
	ChangeSeatChoice []bool
	Publish          bool
	DeclareReady     bool
	DeclareNext      bool
	Resume           bool
	Close            bool
	Rename           bool
}

// 見ている人の、部屋の詳細に付ける「できる操作」。見ている人は Room を引いた後（入っている部屋と分かった後）に呼ぶ
type GetRoomCapabilitiesFunc func(ctx context.Context, playerID uuid.UUID, roomID uuid.UUID) (RoomCapabilities, error)

// できる操作を、read model ではなくイベントストアから読み直した集約の状態に room.CapabilitiesOf（Decide が通る check と同じ関数）を
// 当てて答える。read model から作り直さないのは、公開前に決めた印（ChoiceLocked 等）が read model に無く、読み替える決まりを
// 持つとドメインの決まりが 2 か所になるため（Decide が読む State を持つ集約が、答えの唯一の出どころ）。
// 投影が遅れていると、同じ詳細の席の並び・revision（read model）とできる操作（集約）が一瞬ずれうるが、
// 画面は revision を付けて操作し、古ければ RevisionConflictError で読み直すので、操作が誤って通ることはない
type RoomCapabilitiesLoader struct {
	eventStore domain.EventStore[room.ID, room.Event, room.State]
	rooms      domain.Identifier[room.ID]
	players    domain.Identifier[player.ID]
}

func NewRoomCapabilitiesLoader(
	eventStore domain.EventStore[room.ID, room.Event, room.State],
	rooms domain.Identifier[room.ID],
	players domain.Identifier[player.ID],
) *RoomCapabilitiesLoader {
	return &RoomCapabilitiesLoader{eventStore: eventStore, rooms: rooms, players: players}
}

func (l *RoomCapabilitiesLoader) Get(ctx context.Context, playerID uuid.UUID, roomID uuid.UUID) (RoomCapabilities, error) {
	id, err := l.rooms.Resolve(ctx, roomID)
	if errors.Is(err, domain.ErrEntityNotFound) {
		return RoomCapabilities{}, ErrRoomNotFound
	}
	if err != nil {
		return RoomCapabilities{}, err
	}
	viewer, err := l.players.Resolve(ctx, playerID)
	if err != nil {
		return RoomCapabilities{}, err
	}
	state, err := l.eventStore.Load(ctx, id)
	if err != nil {
		return RoomCapabilities{}, err
	}
	capabilities := room.CapabilitiesOf(state, viewer)
	return RoomCapabilities(capabilities), nil
}
