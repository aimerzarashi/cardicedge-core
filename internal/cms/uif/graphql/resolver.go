package graphql

import (
	"context"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/cms/app/command"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/cms/app/query"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/pagination"
)

// 実装は command.CreatureCommands
type CreatureRegistrar interface {
	Register(ctx context.Context, name string) (command.RegisteredCreature, error)
}

// 実装は query.CardList
type CardLister interface {
	List(ctx context.Context, filter query.CardFilter, first *int, after *string) (pagination.Connection[query.Card], error)
}

type Resolver struct {
	version   string
	creatures CreatureRegistrar
	cards     CardLister
}

func NewResolver(version string, creatures CreatureRegistrar, cards CardLister) *Resolver {
	return &Resolver{version: version, creatures: creatures, cards: cards}
}
