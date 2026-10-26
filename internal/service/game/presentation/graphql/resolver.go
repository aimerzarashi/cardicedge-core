package graphql

import (
	"context"

	"github.com/google/uuid"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/game/application/command"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/game/application/query"
)

// 実装は command.PlayerHandler
type Players interface {
	Register(ctx context.Context) (command.RegisteredPlayer, error)
	Me(ctx context.Context) (command.RegisteredPlayer, error)
}

// 実装は query.PlayerHandler
type PlayerQueries interface {
	StarterGrant(ctx context.Context, playerID uuid.UUID) (query.StarterGrantStatus, error)
}

// 実装は command.AvatarHandler
type Avatars interface {
	Register(ctx context.Context, name string) (command.RegisteredAvatar, error)
}

// 実装は query.AvatarHandler
type AvatarQueries interface {
	ListByPlayer(ctx context.Context, playerID uuid.UUID) ([]query.Avatar, error)
}

type Resolver struct {
	players     Players
	playerQuery PlayerQueries
	avatars     Avatars
	avatarQuery AvatarQueries
}

func NewResolver(players Players, playerQuery PlayerQueries, avatars Avatars, avatarQuery AvatarQueries) *Resolver {
	return &Resolver{players: players, playerQuery: playerQuery, avatars: avatars, avatarQuery: avatarQuery}
}
