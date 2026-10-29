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

// 実装は command.CardHandler
type CardStatusHandler interface {
	Publish(ctx context.Context, id uuid.UUID, revision int64) (command.ChangedCard, error)
	Retire(ctx context.Context, id uuid.UUID, revision int64) (command.ChangedCard, error)
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

// 実装は command.StarterDeckHandler
type StarterDeckHandler interface {
	Register(ctx context.Context, name string) (command.ChangedStarterDeck, error)
	RegisterFromPresetDeck(ctx context.Context, presetDeckID uuid.UUID) (command.ChangedStarterDeck, error)
	Rename(ctx context.Context, id uuid.UUID, revision int64, name string) (command.ChangedStarterDeck, error)
	SetCardCounts(ctx context.Context, id uuid.UUID, revision int64, cards []command.CardCountInput) (command.ChangedStarterDeck, error)
	Delete(ctx context.Context, id uuid.UUID, revision int64) error
	Publish(ctx context.Context, id uuid.UUID, revision int64) (command.ChangedStarterDeck, error)
	Retire(ctx context.Context, id uuid.UUID, revision int64) (command.ChangedStarterDeck, error)
}

// 実装は query.StarterDeckHandler
type StarterDeckLister interface {
	List(ctx context.Context, filter query.StarterDeckFilter, first *int, after *string) (pagination.Connection[query.StarterDeck], error)
	Get(ctx context.Context, id uuid.UUID) (query.StarterDeck, error)
}

// 実装は command.BoardHandler
type BoardHandler interface {
	Register(ctx context.Context, name string) (command.ChangedBoard, error)
	Rename(ctx context.Context, id uuid.UUID, revision int64, name string) (command.ChangedBoard, error)
	ChangeLayout(ctx context.Context, id uuid.UUID, revision int64, layout command.BoardLayoutInput) (command.ChangedBoard, error)
	ChangeDefaultRules(ctx context.Context, id uuid.UUID, revision int64, rules command.BoardDefaultRulesInput) (command.ChangedBoard, error)
	Delete(ctx context.Context, id uuid.UUID, revision int64) error
	Publish(ctx context.Context, id uuid.UUID, revision int64) (command.ChangedBoard, error)
	Retire(ctx context.Context, id uuid.UUID, revision int64) (command.ChangedBoard, error)
}

// 実装は query.BoardHandler
type BoardLister interface {
	List(ctx context.Context, filter query.BoardFilter, first *int, after *string) (pagination.Connection[query.Board], error)
	Get(ctx context.Context, id uuid.UUID) (query.Board, error)
}

// 実装は query.StudioRulesHandler
type StudioRulesGetter interface {
	Get(ctx context.Context) (query.StudioRules, error)
}

type Resolver struct {
	creatures       CreatureRegistrar
	items           ItemRegistrar
	spells          SpellRegistrar
	cardStatuses    CardStatusHandler
	presetDecks     PresetDeckHandler
	cards           CardLister
	presetDeckList  PresetDeckLister
	starterDecks    StarterDeckHandler
	starterDeckList StarterDeckLister
	boards          BoardHandler
	boardList       BoardLister
	studioRules     StudioRulesGetter
}

func NewResolver(
	creatures CreatureRegistrar,
	items ItemRegistrar,
	spells SpellRegistrar,
	cardStatuses CardStatusHandler,
	presetDecks PresetDeckHandler,
	cards CardLister,
	presetDeckList PresetDeckLister,
	starterDecks StarterDeckHandler,
	starterDeckList StarterDeckLister,
	boards BoardHandler,
	boardList BoardLister,
	studioRules StudioRulesGetter,
) *Resolver {
	return &Resolver{
		creatures: creatures, items: items, spells: spells, cardStatuses: cardStatuses, presetDecks: presetDecks,
		cards: cards, presetDeckList: presetDeckList, starterDecks: starterDecks, starterDeckList: starterDeckList,
		boards: boards, boardList: boardList, studioRules: studioRules,
	}
}
