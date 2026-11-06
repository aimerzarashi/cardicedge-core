package query

import (
	"context"

	"github.com/google/uuid"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/avatar"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/deck"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/player"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/room"
)

// 部屋を作るのに先に持っていなければならないもの
type RoomRequirement string

const (
	RoomRequirementAvatar RoomRequirement = "avatar"
	RoomRequirementDeck   RoomRequirement = "deck"
)

// 部屋を作るのに足りないもの（Player.roomRequirements）を、部屋を作る操作と同じく書く側の数の記録と room.MissingRequirements で答える。
// read model（avatars・decks の行の数）から数えないのは、作った直後のものが投影の遅れで数に入らず、操作では作れるのに画面が
// 「先に作る」を出し続けることがあるため（できる操作 RoomCapabilitiesLoader と同じく、答えの出どころを操作の判断とそろえる）
type RoomRequirementsLoader struct {
	countQuota domain.CountQuotaFunc
	players    domain.Identifier[player.ID]
}

func NewRoomRequirementsLoader(countQuota domain.CountQuotaFunc, players domain.Identifier[player.ID]) *RoomRequirementsLoader {
	return &RoomRequirementsLoader{countQuota: countQuota, players: players}
}

func (l *RoomRequirementsLoader) Get(ctx context.Context, playerID uuid.UUID) ([]RoomRequirement, error) {
	owner, err := l.players.Resolve(ctx, playerID)
	if err != nil {
		return nil, err
	}
	avatars, err := l.countQuota(ctx, avatar.PlayerQuota(owner))
	if err != nil {
		return nil, err
	}
	decks, err := l.countQuota(ctx, deck.PlayerQuota(owner))
	if err != nil {
		return nil, err
	}
	missing := room.MissingRequirements(avatars, decks)
	requirements := make([]RoomRequirement, len(missing))
	for i, requirement := range missing {
		requirements[i] = RoomRequirement(requirement)
	}
	return requirements, nil
}
