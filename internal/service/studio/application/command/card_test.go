package command_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/card"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/creature"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/item"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/spell"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domaindef"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/identifier"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/studio/application/command"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/studio/application/input"
)

// 状態の変更はどの種類のカードでも同じように動くため、種類を変えて同じ確かめを流す
type cardKind struct {
	name     string
	register func(t *testing.T, ctx context.Context) command.ChangedCard
}

func cardKinds() []cardKind {
	return []cardKind{
		{name: "creature", register: func(t *testing.T, ctx context.Context) command.ChangedCard {
			registered, err := newCreatureHandler(t, ctx).Register(ctx, "カード-"+uuid.NewString())
			require.NoError(t, err)
			return command.ChangedCard{ID: registered.ID, Revision: registered.Revision}
		}},
		{name: "item", register: func(t *testing.T, ctx context.Context) command.ChangedCard {
			registered, err := newItemHandler(t, ctx).Register(ctx, "カード-"+uuid.NewString())
			require.NoError(t, err)
			return command.ChangedCard{ID: registered.ID, Revision: registered.Revision}
		}},
		// 効果の無いスペルは公開できないので、効果を入れた準備中のスペルにする
		{name: "spell", register: func(t *testing.T, ctx context.Context) command.ChangedCard {
			spells := newSpellHandler(t, ctx)
			registered, err := spells.Register(ctx, "カード-"+uuid.NewString())
			require.NoError(t, err)
			changed, err := spells.ChangeEffect(ctx, registered.ID, registered.Revision, command.SpellEffect{DrawCards: &command.SpellDrawCards{Count: 1}})
			require.NoError(t, err)
			return changed
		}},
	}
}

// コマンドの結果は公開 ID と revision だけなので、状態はイベントストアから集約を読み直して確かめる
func restoredCardStatus(t *testing.T, ctx context.Context, publicID uuid.UUID) card.Status {
	t.Helper()
	registry, err := domaindef.Load(ctx)
	require.NoError(t, err)
	ref := cardRef(t, ctx, publicID)
	statuses, err := newCardStatusReader(registry).Statuses(ctx, []card.Ref{ref})
	require.NoError(t, err)
	status, found := statuses[ref]
	require.True(t, found)
	return status
}

func TestPublishCard_PublishesDraftOfAnyKind(t *testing.T) {
	for _, kind := range cardKinds() {
		t.Run(kind.name, func(t *testing.T) {
			ctx := signedIn(t, isolatedDatabase(t), registrant(t))
			registered := kind.register(t, ctx)

			changed, err := newCardHandler(t, ctx).Publish(ctx, registered.ID, registered.Revision)

			require.NoError(t, err)
			require.Equal(t, command.ChangedCard{ID: registered.ID, Revision: registered.Revision + 1}, changed)
			require.Equal(t, card.StatusPublished, restoredCardStatus(t, ctx, registered.ID))
		})
	}
}

func TestRetireCard_RetiresPublishedOfAnyKind(t *testing.T) {
	for _, kind := range cardKinds() {
		t.Run(kind.name, func(t *testing.T) {
			ctx := signedIn(t, isolatedDatabase(t), registrant(t))
			registered := kind.register(t, ctx)
			commands := newCardHandler(t, ctx)
			published, err := commands.Publish(ctx, registered.ID, registered.Revision)
			require.NoError(t, err)

			retired, err := commands.Retire(ctx, published.ID, published.Revision)

			require.NoError(t, err)
			require.Equal(t, published.Revision+1, retired.Revision)
			require.Equal(t, card.StatusRetired, restoredCardStatus(t, ctx, registered.ID))
		})
	}
}

func TestPublishCard_PublishesRetiredAgain(t *testing.T) {
	ctx := signedIn(t, isolatedDatabase(t), registrant(t))
	registered := cardKinds()[0].register(t, ctx)
	commands := newCardHandler(t, ctx)
	published, err := commands.Publish(ctx, registered.ID, registered.Revision)
	require.NoError(t, err)
	retired, err := commands.Retire(ctx, published.ID, published.Revision)
	require.NoError(t, err)

	republished, err := commands.Publish(ctx, retired.ID, retired.Revision)

	require.NoError(t, err)
	require.Equal(t, retired.Revision+1, republished.Revision)
	require.Equal(t, card.StatusPublished, restoredCardStatus(t, ctx, registered.ID))
}

// 変わらない操作は、イベントを作らず revision を進めない
func TestCardStatusCommands_KeepRevisionWhenStatusIsUnchanged(t *testing.T) {
	ctx := signedIn(t, isolatedDatabase(t), registrant(t))
	registered := cardKinds()[0].register(t, ctx)
	commands := newCardHandler(t, ctx)
	published, err := commands.Publish(ctx, registered.ID, registered.Revision)
	require.NoError(t, err)

	publishedAgain, err := commands.Publish(ctx, published.ID, published.Revision)
	require.NoError(t, err)
	require.Equal(t, published, publishedAgain)
	retired, err := commands.Retire(ctx, published.ID, published.Revision)
	require.NoError(t, err)
	retiredAgain, err := commands.Retire(ctx, retired.ID, retired.Revision)

	require.NoError(t, err)
	require.Equal(t, retired, retiredAgain)
}

func TestRetireCard_RejectsDraft(t *testing.T) {
	ctx := signedIn(t, isolatedDatabase(t), registrant(t))
	registered := cardKinds()[0].register(t, ctx)

	_, err := newCardHandler(t, ctx).Retire(ctx, registered.ID, registered.Revision)

	require.ErrorIs(t, err, command.ErrInvalidCardStatusTransition)
	require.Equal(t, card.StatusDraft, restoredCardStatus(t, ctx, registered.ID))
}

// revision が古ければ、変わらない操作（公開中にもう一度公開する）でも「先に変えられた」にする
func TestCardStatusCommands_RejectStaleRevision(t *testing.T) {
	tests := []struct {
		name string
		call func(ctx context.Context, commands *command.CardHandler, stale command.ChangedCard) error
	}{
		{name: "publish", call: func(ctx context.Context, commands *command.CardHandler, stale command.ChangedCard) error {
			_, err := commands.Publish(ctx, stale.ID, stale.Revision)
			return err
		}},
		{name: "retire", call: func(ctx context.Context, commands *command.CardHandler, stale command.ChangedCard) error {
			_, err := commands.Retire(ctx, stale.ID, stale.Revision)
			return err
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := signedIn(t, isolatedDatabase(t), registrant(t))
			stale := cardKinds()[0].register(t, ctx)
			commands := newCardHandler(t, ctx)
			_, err := commands.Publish(ctx, stale.ID, stale.Revision)
			require.NoError(t, err)

			err = tt.call(ctx, commands, stale)

			require.ErrorIs(t, err, command.ErrRevisionConflict)
		})
	}
}

func TestCardStatusCommands_RejectUnknownCard(t *testing.T) {
	tests := []struct {
		name string
		id   func(t *testing.T, ctx context.Context) uuid.UUID
	}{
		{name: "unknown_id", id: func(*testing.T, context.Context) uuid.UUID { return uuid.New() }},
		// 識別子はあるが、カードの種類でない（プリセットデッキ）
		{name: "not_a_card", id: func(t *testing.T, ctx context.Context) uuid.UUID {
			return registerPresetDeck(t, ctx, "初心者デッキ").ID
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := signedIn(t, isolatedDatabase(t), registrant(t))
			commands := newCardHandler(t, ctx)
			id := tt.id(t, ctx)

			_, publishErr := commands.Publish(ctx, id, 1)
			_, retireErr := commands.Retire(ctx, id, 1)

			require.ErrorIs(t, publishErr, command.ErrCardNotFound)
			require.ErrorIs(t, retireErr, command.ErrCardNotFound)
		})
	}
}

func TestCardStatusCommands_RejectWithoutOperator(t *testing.T) {
	ctx := isolatedDatabase(t)
	registered := cardKinds()[0].register(t, signedIn(t, ctx, registrant(t)))
	commands := newCardHandler(t, ctx)

	_, publishErr := commands.Publish(ctx, registered.ID, registered.Revision)
	_, retireErr := commands.Retire(ctx, registered.ID, registered.Revision)

	require.ErrorIs(t, publishErr, command.ErrNotOperator)
	require.ErrorIs(t, retireErr, command.ErrNotOperator)
}

func TestCardStatusReader_ReturnsStatusOfEachKind(t *testing.T) {
	ctx := signedIn(t, isolatedDatabase(t), registrant(t))
	registry, err := domaindef.Load(ctx)
	require.NoError(t, err)
	creatureID := publishedCreature(t, ctx)
	itemID := publishedItem(t, ctx)
	spellRegistered, err := newSpellHandler(t, ctx).Register(ctx, "スペル-"+uuid.NewString())
	require.NoError(t, err)

	creatureRef := cardRef(t, ctx, creatureID)
	itemRef := cardRef(t, ctx, itemID)
	spellRef := cardRef(t, ctx, spellRegistered.ID)
	statuses, err := newCardStatusReader(registry).Statuses(ctx, []card.Ref{creatureRef, itemRef, spellRef})

	require.NoError(t, err)
	require.Equal(t, map[card.Ref]card.Status{
		creatureRef: card.StatusPublished,
		itemRef:     card.StatusPublished,
		spellRef:    card.StatusDraft,
	}, statuses)
	require.Equal(t, creature.Types().EntityType, creatureRef.EntityType)
	require.Equal(t, item.Types().EntityType, itemRef.EntityType)
	require.Equal(t, spell.Types().EntityType, spellRef.EntityType)
}

func cardRef(t *testing.T, ctx context.Context, publicID uuid.UUID) card.Ref {
	t.Helper()
	registry, err := domaindef.Load(ctx)
	require.NoError(t, err)
	ref, err := identifier.NewEntityRefResolver(registry).Resolve(ctx, publicID)
	require.NoError(t, err)
	cardRef, err := card.NewRef(ref.EntityType(), ref.ID())
	require.NoError(t, err)
	return cardRef
}

func TestCardSpecCommands_ChangeCostAndRarityOfAnyKind(t *testing.T) {
	for _, kind := range cardKinds() {
		t.Run(kind.name, func(t *testing.T) {
			ctx := signedIn(t, isolatedDatabase(t), registrant(t))
			registered := kind.register(t, ctx)
			commands := newCardHandler(t, ctx)

			costChanged, err := commands.ChangeCost(ctx, registered.ID, registered.Revision, 30)
			require.NoError(t, err)
			rarityChanged, err := commands.ChangeRarity(ctx, costChanged.ID, costChanged.Revision, "rare")

			require.NoError(t, err)
			require.Equal(t, command.ChangedCard{ID: registered.ID, Revision: registered.Revision + 1}, costChanged)
			require.Equal(t, command.ChangedCard{ID: registered.ID, Revision: registered.Revision + 2}, rarityChanged)
		})
	}
}

// 同じ値への変更は、イベントを作らず revision を進めない
func TestCardSpecCommands_KeepRevisionWhenUnchanged(t *testing.T) {
	ctx := signedIn(t, isolatedDatabase(t), registrant(t))
	registered := cardKinds()[0].register(t, ctx)
	commands := newCardHandler(t, ctx)

	costChanged, costErr := commands.ChangeCost(ctx, registered.ID, registered.Revision, card.DefaultCost.Int())
	rarityChanged, rarityErr := commands.ChangeRarity(ctx, registered.ID, registered.Revision, card.DefaultRarity.String())

	require.NoError(t, costErr)
	require.NoError(t, rarityErr)
	require.Equal(t, registered, costChanged)
	require.Equal(t, registered, rarityChanged)
}

func TestCardSpecCommands_RejectNotDraft(t *testing.T) {
	ctx := signedIn(t, isolatedDatabase(t), registrant(t))
	registered := cardKinds()[0].register(t, ctx)
	commands := newCardHandler(t, ctx)
	published, err := commands.Publish(ctx, registered.ID, registered.Revision)
	require.NoError(t, err)

	_, costErr := commands.ChangeCost(ctx, published.ID, published.Revision, 30)
	_, rarityErr := commands.ChangeRarity(ctx, published.ID, published.Revision, "rare")

	require.ErrorIs(t, costErr, command.ErrCardNotDraft)
	require.ErrorIs(t, rarityErr, command.ErrCardNotDraft)
}

func TestCardSpecCommands_RejectInvalidValue(t *testing.T) {
	ctx := signedIn(t, isolatedDatabase(t), registrant(t))
	registered := cardKinds()[0].register(t, ctx)
	commands := newCardHandler(t, ctx)

	_, costErr := commands.ChangeCost(ctx, registered.ID, registered.Revision, -1)
	_, rarityErr := commands.ChangeRarity(ctx, registered.ID, registered.Revision, "N")

	require.Equal(t, input.Errors{&input.TooSmall{Field: "cost", Min: 0}}, costErr)
	require.Equal(t, input.Errors{&input.InvalidFormat{Field: "rarity"}}, rarityErr)
}

func TestCardSpecCommands_RejectStaleRevision(t *testing.T) {
	ctx := signedIn(t, isolatedDatabase(t), registrant(t))
	stale := cardKinds()[0].register(t, ctx)
	commands := newCardHandler(t, ctx)
	_, err := commands.ChangeCost(ctx, stale.ID, stale.Revision, 30)
	require.NoError(t, err)

	_, costErr := commands.ChangeCost(ctx, stale.ID, stale.Revision, 40)
	_, rarityErr := commands.ChangeRarity(ctx, stale.ID, stale.Revision, "rare")

	require.ErrorIs(t, costErr, command.ErrRevisionConflict)
	require.ErrorIs(t, rarityErr, command.ErrRevisionConflict)
}
