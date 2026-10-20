package graphql

import (
	"context"

	"github.com/google/uuid"

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

// 実装は command.SpellCommands
type SpellRegistrar interface {
	Register(ctx context.Context, name string) (command.RegisteredSpell, error)
}

// 実装は command.StarterDeckCommands
type StarterDeckCommands interface {
	Register(ctx context.Context, name string) (command.ChangedStarterDeck, error)
	Rename(ctx context.Context, id uuid.UUID, revision int64, name string) (command.ChangedStarterDeck, error)
	SetCardCounts(ctx context.Context, id uuid.UUID, revision int64, cards []command.CardCountInput) (command.ChangedStarterDeck, error)
	Delete(ctx context.Context, id uuid.UUID, revision int64) error
}

// 実装は query.CardList
type CardLister interface {
	CardGetter
	List(ctx context.Context, filter query.CardFilter, first *int, after *string) (pagination.Connection[query.Card], error)
}

// 実装は query.StarterDeckList
type StarterDeckLister interface {
	List(ctx context.Context, filter query.StarterDeckFilter, first *int, after *string) (pagination.Connection[query.StarterDeck], error)
	Get(ctx context.Context, id uuid.UUID) (query.StarterDeck, error)
}

type Resolver struct {
	version         string
	creatures       CreatureRegistrar
	items           ItemRegistrar
	spells          SpellRegistrar
	starterDecks    StarterDeckCommands
	cards           CardLister
	starterDeckList StarterDeckLister
}

func NewResolver(
	version string,
	creatures CreatureRegistrar,
	items ItemRegistrar,
	spells SpellRegistrar,
	starterDecks StarterDeckCommands,
	cards CardLister,
	starterDeckList StarterDeckLister,
) *Resolver {
	return &Resolver{
		version: version, creatures: creatures, items: items, spells: spells, starterDecks: starterDecks,
		cards: cards, starterDeckList: starterDeckList,
	}
}
