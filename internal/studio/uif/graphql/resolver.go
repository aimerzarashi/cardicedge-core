package graphql

import (
	"context"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/pagination"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/studio/app/command"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/studio/app/query"
)

// 実装は command.CreatureCommands
type CreatureRegistrar interface {
	Register(ctx context.Context, name string) (command.RegisteredCreature, error)
}

// 実装は command.ItemCommands
type ItemRegistrar interface {
	Register(ctx context.Context, name string) (command.RegisteredItem, error)
}

// 実装は query.CardList
type CardLister interface {
	List(ctx context.Context, filter query.CardFilter, first *int, after *string) (pagination.Connection[query.Card], error)
}

type Resolver struct {
	version   string
	creatures CreatureRegistrar
	items     ItemRegistrar
	cards     CardLister
}

func NewResolver(version string, creatures CreatureRegistrar, items ItemRegistrar, cards CardLister) *Resolver {
	return &Resolver{version: version, creatures: creatures, items: items, cards: cards}
}
