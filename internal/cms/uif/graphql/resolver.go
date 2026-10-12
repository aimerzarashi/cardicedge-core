package graphql

import (
	"context"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/cms/app/command"
)

// 実装は command.CreatureCommands
type CreatureRegistrar interface {
	Register(ctx context.Context, name string) (command.RegisteredCreature, error)
}

type Resolver struct {
	version   string
	creatures CreatureRegistrar
}

func NewResolver(version string, creatures CreatureRegistrar) *Resolver {
	return &Resolver{version: version, creatures: creatures}
}
