package graphql

import (
	"context"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/game/application/command"
)

// 実装は command.PlayerHandler
type Players interface {
	Register(ctx context.Context) (command.RegisteredPlayer, error)
	Me(ctx context.Context) (command.RegisteredPlayer, error)
}

type Resolver struct {
	players Players
}

func NewResolver(players Players) *Resolver {
	return &Resolver{players: players}
}
