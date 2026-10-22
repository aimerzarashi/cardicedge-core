package graphql

import (
	"context"

	"github.com/google/uuid"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/pagination"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/studio/application/command"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/studio/application/query"
)

// 実装は command.CreatureHandler
type CreatureRegistrar interface {
	Register(ctx context.Context, name string) (command.RegisteredCreature, error)
}

// 実装は command.ItemHandler
type ItemRegistrar interface {
	Register(ctx context.Context, name string) (command.RegisteredItem, error)
}

// 実装は command.SpellHandler
type SpellRegistrar interface {
	Register(ctx context.Context, name string) (command.RegisteredSpell, error)
}

// 実装は command.PresetDeckHandler
type PresetDeckHandler interface {
	Register(ctx context.Context, name string) (command.ChangedPresetDeck, error)
	Rename(ctx context.Context, id uuid.UUID, revision int64, name string) (command.ChangedPresetDeck, error)
	SetCardCounts(ctx context.Context, id uuid.UUID, revision int64, cards []command.CardCountInput) (command.ChangedPresetDeck, error)
	Delete(ctx context.Context, id uuid.UUID, revision int64) error
}

// 実装は query.CardHandler
type CardLister interface {
	CardGetter
	List(ctx context.Context, filter query.CardFilter, first *int, after *string) (pagination.Connection[query.Card], error)
}

// 実装は query.PresetDeckHandler
type PresetDeckLister interface {
	List(ctx context.Context, filter query.PresetDeckFilter, first *int, after *string) (pagination.Connection[query.PresetDeck], error)
	Get(ctx context.Context, id uuid.UUID) (query.PresetDeck, error)
}

// 実装は command.StarterDeckSetHandler
type StarterDeckSetHandler interface {
	Set(ctx context.Context, revision int64, presetDeckIDs []uuid.UUID) (command.ChangedStarterDeckSet, error)
}

// 実装は query.StarterDeckSetHandler
type StarterDeckSetGetter interface {
	Get(ctx context.Context) (query.StarterDeckSet, error)
}

type Resolver struct {
	version         string
	creatures       CreatureRegistrar
	items           ItemRegistrar
	spells          SpellRegistrar
	presetDecks     PresetDeckHandler
	cards           CardLister
	presetDeckList  PresetDeckLister
	starterDeckSets StarterDeckSetHandler
	starterDeckSet  StarterDeckSetGetter
}

func NewResolver(
	version string,
	creatures CreatureRegistrar,
	items ItemRegistrar,
	spells SpellRegistrar,
	presetDecks PresetDeckHandler,
	cards CardLister,
	presetDeckList PresetDeckLister,
	starterDeckSets StarterDeckSetHandler,
	starterDeckSet StarterDeckSetGetter,
) *Resolver {
	return &Resolver{
		version: version, creatures: creatures, items: items, spells: spells, presetDecks: presetDecks,
		cards: cards, presetDeckList: presetDeckList, starterDeckSets: starterDeckSets, starterDeckSet: starterDeckSet,
	}
}
