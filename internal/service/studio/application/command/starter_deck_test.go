package command_test

import (
	"context"
	"strings"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/constraint/constrainttest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/creature"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/gamerules"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/presetdeck"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/starterdeck"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domaindef"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/identifier"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/studio/application/command"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/studio/application/input"
)

// 公開までの流れを小さく確かめるルール（合計 4 枚で公開でき、公開中は 2 つまで）
func smallRules() gamerules.Rules {
	rules := gamerules.Defaults()
	rules.Deck = gamerules.Deck{TotalCardCount: 4, MaxCardDuplicates: 4}
	rules.StarterDeck = gamerules.StarterDeck{MaxPublished: 2}
	return rules
}

func restoredStarterDeck(t *testing.T, ctx context.Context, publicID uuid.UUID) starterdeck.StarterDeck {
	t.Helper()
	registry, err := domaindef.Load(ctx)
	require.NoError(t, err)
	id, err := identifier.New(registry, starterdeck.NewID).Resolve(ctx, publicID)
	require.NoError(t, err)
	state, err := starterDeckStore(registry).Load(ctx, id)
	require.NoError(t, err)
	return state.(starterdeck.StarterDeck)
}

func registerStarterDeck(t *testing.T, ctx context.Context, commands *command.StarterDeckHandler, name string) command.ChangedStarterDeck {
	t.Helper()
	registered, err := commands.Register(ctx, name)
	require.NoError(t, err)
	return registered
}

// smallRules で公開できる、公開中のカード 4 枚の準備中のスターターデッキ
func fullStarterDeck(t *testing.T, ctx context.Context, commands *command.StarterDeckHandler) command.ChangedStarterDeck {
	t.Helper()
	registered := registerStarterDeck(t, ctx, commands, "デッキ-"+uuid.NewString())
	filled, err := commands.SetCardCounts(ctx, registered.ID, registered.Revision, []command.CardCountInput{{CardID: publishedCreature(t, ctx), Count: 4}})
	require.NoError(t, err)
	return filled
}

func TestRegisterStarterDeck_ReturnsPublicIDAndMakesDraftWithoutSource(t *testing.T) {
	ctx := signedIn(t, isolatedDatabase(t), registrant(t))

	registered, err := newStarterDeckHandler(t, ctx).Register(ctx, "  初心者デッキ ")

	require.NoError(t, err)
	require.Equal(t, int64(1), registered.Revision)
	deck := restoredStarterDeck(t, ctx, registered.ID)
	require.Equal(t, "初心者デッキ", deck.Name.String())
	require.Equal(t, starterdeck.StatusDraft, deck.Status)
	require.Nil(t, deck.Source)
	require.Empty(t, deck.Cards)
}

func TestRegisterStarterDeck_RejectsNameAlreadyInUse(t *testing.T) {
	ctx := signedIn(t, isolatedDatabase(t), registrant(t))
	commands := newStarterDeckHandler(t, ctx)
	registerStarterDeck(t, ctx, commands, "ＡＢＣ")

	_, err := commands.Register(ctx, " abc")

	require.ErrorIs(t, err, command.ErrStarterDeckNameAlreadyInUse)
}

func TestRegisterStarterDeck_AllowsNameUsedByPresetDeck(t *testing.T) {
	ctx := signedIn(t, isolatedDatabase(t), registrant(t))
	registerPresetDeck(t, ctx, "初心者デッキ")

	_, err := newStarterDeckHandler(t, ctx).Register(ctx, "初心者デッキ")

	require.NoError(t, err)
}

func TestRegisterStarterDeck_RejectsInvalidName(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  input.Error
	}{
		{name: "blank", value: " ", want: &input.Required{Field: "name"}},
		{name: "too_long", value: strings.Repeat("あ", presetdeck.MaxNameLength+1), want: &input.TooLong{Field: "name", Max: presetdeck.MaxNameLength}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := signedIn(t, isolatedDatabase(t), registrant(t))

			_, err := newStarterDeckHandler(t, ctx).Register(ctx, tt.value)

			invalid, ok := input.AsErrors(err)
			require.True(t, ok)
			require.Equal(t, input.Errors{tt.want}, invalid)
		})
	}
}

func TestStarterDeckCommands_RejectWithoutOperator(t *testing.T) {
	tests := []struct {
		name string
		call func(t *testing.T, ctx context.Context, commands *command.StarterDeckHandler, deck command.ChangedStarterDeck) error
	}{
		{name: "register", call: func(_ *testing.T, ctx context.Context, commands *command.StarterDeckHandler, _ command.ChangedStarterDeck) error {
			_, err := commands.Register(ctx, "中級者デッキ")
			return err
		}},
		{name: "register_from_preset_deck", call: func(_ *testing.T, ctx context.Context, commands *command.StarterDeckHandler, _ command.ChangedStarterDeck) error {
			_, err := commands.RegisterFromPresetDeck(ctx, uuid.New())
			return err
		}},
		{name: "rename", call: func(_ *testing.T, ctx context.Context, commands *command.StarterDeckHandler, deck command.ChangedStarterDeck) error {
			_, err := commands.Rename(ctx, deck.ID, deck.Revision, "中級者デッキ")
			return err
		}},
		{name: "set_card_counts", call: func(t *testing.T, ctx context.Context, commands *command.StarterDeckHandler, deck command.ChangedStarterDeck) error {
			_, err := commands.SetCardCounts(ctx, deck.ID, deck.Revision, []command.CardCountInput{{CardID: issueCard(t, ctx, creature.NewID), Count: 1}})
			return err
		}},
		{name: "delete", call: func(_ *testing.T, ctx context.Context, commands *command.StarterDeckHandler, deck command.ChangedStarterDeck) error {
			return commands.Delete(ctx, deck.ID, deck.Revision)
		}},
		{name: "publish", call: func(_ *testing.T, ctx context.Context, commands *command.StarterDeckHandler, deck command.ChangedStarterDeck) error {
			_, err := commands.Publish(ctx, deck.ID, deck.Revision)
			return err
		}},
		{name: "retire", call: func(_ *testing.T, ctx context.Context, commands *command.StarterDeckHandler, deck command.ChangedStarterDeck) error {
			_, err := commands.Retire(ctx, deck.ID, deck.Revision)
			return err
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := isolatedDatabase(t)
			signed := signedIn(t, ctx, registrant(t))
			deck := registerStarterDeck(t, signed, newStarterDeckHandler(t, signed), "初心者デッキ")

			err := tt.call(t, ctx, newStarterDeckHandler(t, ctx), deck)

			require.ErrorIs(t, err, command.ErrNotOperator)
		})
	}
}

// 名前・カード・枚数を写し、写した元の公開 ID と revision を記録する。写した後に元を直しても、スターターデッキは変わらない
func TestRegisterStarterDeckFromPresetDeck_CopiesNameCardsAndSource(t *testing.T) {
	ctx := signedIn(t, isolatedDatabase(t), registrant(t))
	presets := newPresetDeckHandler(t, ctx)
	dragon, potion := publishedCreature(t, ctx), publishedItem(t, ctx)
	preset := registerPresetDeck(t, ctx, "火のデッキ")
	filled, err := presets.SetCardCounts(ctx, preset.ID, preset.Revision, []command.CardCountInput{{CardID: dragon, Count: 3}, {CardID: potion, Count: 2}})
	require.NoError(t, err)
	commands := newStarterDeckHandler(t, ctx)

	copied, err := commands.RegisterFromPresetDeck(ctx, preset.ID)

	require.NoError(t, err)
	require.Equal(t, int64(1), copied.Revision)
	deck := restoredStarterDeck(t, ctx, copied.ID)
	require.Equal(t, "火のデッキ", deck.Name.String())
	require.Equal(t, starterdeck.StatusDraft, deck.Status)
	require.Equal(t, 5, deck.TotalCount())
	require.NotNil(t, deck.Source)
	require.Equal(t, preset.ID, deck.Source.PresetDeckID())
	require.Equal(t, filled.Revision, deck.Source.Revision())
	require.ElementsMatch(t, []int64{internalID(t, ctx, dragon), internalID(t, ctx, potion)}, constrainttest.Referenced(t, ctx, starterdeck.CardReferenceType(), internalID(t, ctx, copied.ID)))
	_, err = presets.SetCardCounts(ctx, filled.ID, filled.Revision, []command.CardCountInput{{CardID: dragon, Count: 0}})
	require.NoError(t, err)
	require.Equal(t, 5, restoredStarterDeck(t, ctx, copied.ID).TotalCount())
}

func TestRegisterStarterDeckFromPresetDeck_RejectsUnknownPresetDeckAndNameInUse(t *testing.T) {
	ctx := signedIn(t, isolatedDatabase(t), registrant(t))
	commands := newStarterDeckHandler(t, ctx)
	preset := registerPresetDeck(t, ctx, "火のデッキ")
	deleted := registerPresetDeck(t, ctx, "水のデッキ")
	require.NoError(t, newPresetDeckHandler(t, ctx).Delete(ctx, deleted.ID, deleted.Revision))
	_, err := commands.RegisterFromPresetDeck(ctx, preset.ID)
	require.NoError(t, err)

	_, sameName := commands.RegisterFromPresetDeck(ctx, preset.ID)
	_, unknown := commands.RegisterFromPresetDeck(ctx, uuid.New())
	_, deletedPreset := commands.RegisterFromPresetDeck(ctx, deleted.ID)

	require.ErrorIs(t, sameName, command.ErrStarterDeckNameAlreadyInUse)
	require.ErrorIs(t, unknown, command.ErrPresetDeckNotFound)
	require.ErrorIs(t, deletedPreset, command.ErrPresetDeckNotFound)
}

// 準備中は決まり（合計の枚数）を満たさなくてよいので、40 枚でないプリセットデッキも写せる
func TestRegisterStarterDeckFromPresetDeck_AllowsPresetDeckThatDoesNotMeetRules(t *testing.T) {
	ctx := signedIn(t, isolatedDatabase(t), registrant(t))
	preset := registerPresetDeck(t, ctx, "空のデッキ")

	copied, err := newStarterDeckHandler(t, ctx).RegisterFromPresetDeck(ctx, preset.ID)

	require.NoError(t, err)
	require.Equal(t, 0, restoredStarterDeck(t, ctx, copied.ID).TotalCount())
}

func TestRenameStarterDeck_RenamesAndRejectsNameInUse(t *testing.T) {
	ctx := signedIn(t, isolatedDatabase(t), registrant(t))
	commands := newStarterDeckHandler(t, ctx)
	registered := registerStarterDeck(t, ctx, commands, "初心者デッキ")
	registerStarterDeck(t, ctx, commands, "上級者デッキ")

	inUse, inUseErr := commands.Rename(ctx, registered.ID, registered.Revision, " 上級者デッキ")
	renamed, err := commands.Rename(ctx, registered.ID, registered.Revision, " 中級者デッキ ")

	require.ErrorIs(t, inUseErr, command.ErrStarterDeckNameAlreadyInUse)
	require.Zero(t, inUse)
	require.NoError(t, err)
	require.Equal(t, command.ChangedStarterDeck{ID: registered.ID, Revision: 2}, renamed)
	require.Equal(t, "中級者デッキ", restoredStarterDeck(t, ctx, registered.ID).Name.String())
}

// 名前は公開・使用不可でも手放さない（状態によらず、すべてのスターターデッキの中で一意）
func TestStarterDeckName_StaysReservedWhilePublishedAndRetired(t *testing.T) {
	ctx := signedIn(t, isolatedDatabase(t), registrant(t))
	commands := newStarterDeckHandlerWithRules(t, ctx, smallRules())
	deck := fullStarterDeck(t, ctx, commands)
	name := restoredStarterDeck(t, ctx, deck.ID).Name.String()
	published, err := commands.Publish(ctx, deck.ID, deck.Revision)
	require.NoError(t, err)

	_, whilePublished := commands.Register(ctx, name)
	_, err = commands.Retire(ctx, published.ID, published.Revision)
	require.NoError(t, err)
	_, whileRetired := commands.Register(ctx, name)

	require.ErrorIs(t, whilePublished, command.ErrStarterDeckNameAlreadyInUse)
	require.ErrorIs(t, whileRetired, command.ErrStarterDeckNameAlreadyInUse)
}

func TestSetStarterDeckCardCounts_ChangesCardsAndReferences(t *testing.T) {
	ctx := signedIn(t, isolatedDatabase(t), registrant(t))
	commands := newStarterDeckHandler(t, ctx)
	registered := registerStarterDeck(t, ctx, commands, "初心者デッキ")
	dragon, potion := publishedCreature(t, ctx), publishedItem(t, ctx)

	changed, err := commands.SetCardCounts(ctx, registered.ID, registered.Revision, []command.CardCountInput{{CardID: dragon, Count: 3}, {CardID: potion, Count: 1}})
	require.NoError(t, err)
	next, err := commands.SetCardCounts(ctx, changed.ID, changed.Revision, []command.CardCountInput{{CardID: potion, Count: 0}})

	require.NoError(t, err)
	require.Equal(t, int64(3), next.Revision)
	require.Equal(t, 3, restoredStarterDeck(t, ctx, registered.ID).TotalCount())
	require.Equal(t, []int64{internalID(t, ctx, dragon)}, constrainttest.Referenced(t, ctx, starterdeck.CardReferenceType(), internalID(t, ctx, registered.ID)))
}

// 新しく入れるカードは公開中だけ。すでに入っているカードは、使用不可になっても残せる（減らせる）
func TestSetStarterDeckCardCounts_RejectsCardNotPublishedButKeepsRetiredCardInDeck(t *testing.T) {
	ctx := signedIn(t, isolatedDatabase(t), registrant(t))
	commands := newStarterDeckHandler(t, ctx)
	registered := registerStarterDeck(t, ctx, commands, "初心者デッキ")
	draftCard, err := newCreatureHandler(t, ctx).Register(ctx, "下書き-"+uuid.NewString())
	require.NoError(t, err)
	inDeck := publishedCreature(t, ctx)
	filled, err := commands.SetCardCounts(ctx, registered.ID, registered.Revision, []command.CardCountInput{{CardID: inDeck, Count: 2}})
	require.NoError(t, err)
	_, err = newCardHandler(t, ctx).Retire(ctx, inDeck, 2)
	require.NoError(t, err)

	_, unavailable := commands.SetCardCounts(ctx, filled.ID, filled.Revision, []command.CardCountInput{{CardID: draftCard.ID, Count: 1}})
	reduced, reducedErr := commands.SetCardCounts(ctx, filled.ID, filled.Revision, []command.CardCountInput{{CardID: inDeck, Count: 1}})

	invalid, ok := input.AsErrors(unavailable)
	require.True(t, ok)
	require.Equal(t, input.Errors{&input.Unavailable{Field: "cards.0.cardId"}}, invalid)
	require.NoError(t, reducedErr)
	require.Equal(t, int64(3), reduced.Revision)
}

func TestSetStarterDeckCardCounts_RejectsTotalOverRule(t *testing.T) {
	ctx := signedIn(t, isolatedDatabase(t), registrant(t))
	commands := newStarterDeckHandlerWithRules(t, ctx, smallRules())
	registered := registerStarterDeck(t, ctx, commands, "初心者デッキ")
	filled, err := commands.SetCardCounts(ctx, registered.ID, registered.Revision, []command.CardCountInput{{CardID: publishedCreature(t, ctx), Count: 4}})
	require.NoError(t, err)

	_, err = commands.SetCardCounts(ctx, filled.ID, filled.Revision, []command.CardCountInput{{CardID: publishedItem(t, ctx), Count: 1}})

	invalid, ok := input.AsErrors(err)
	require.True(t, ok)
	require.Equal(t, input.Errors{&input.TotalTooLarge{Field: "cards", Max: 4}}, invalid)
}

func TestPublishStarterDeck_PublishesAndKeepsRevisionWhenAlreadyPublished(t *testing.T) {
	ctx := signedIn(t, isolatedDatabase(t), registrant(t))
	commands := newStarterDeckHandlerWithRules(t, ctx, smallRules())
	deck := fullStarterDeck(t, ctx, commands)

	published, err := commands.Publish(ctx, deck.ID, deck.Revision)
	again, againErr := commands.Publish(ctx, published.ID, published.Revision)
	_, stale := commands.Publish(ctx, deck.ID, deck.Revision)

	require.NoError(t, err)
	require.Equal(t, command.ChangedStarterDeck{ID: deck.ID, Revision: deck.Revision + 1}, published)
	require.Equal(t, starterdeck.StatusPublished, restoredStarterDeck(t, ctx, deck.ID).Status)
	require.NoError(t, againErr)
	require.Equal(t, published, again)
	require.ErrorIs(t, stale, command.ErrRevisionConflict)
}

func TestPublishStarterDeck_ReturnsFieldErrorsWhenTotalOrCardsAreInvalid(t *testing.T) {
	ctx := signedIn(t, isolatedDatabase(t), registrant(t))
	commands := newStarterDeckHandlerWithRules(t, ctx, smallRules())
	short := registerStarterDeck(t, ctx, commands, "少ない")
	shortFilled, err := commands.SetCardCounts(ctx, short.ID, short.Revision, []command.CardCountInput{{CardID: publishedCreature(t, ctx), Count: 3}})
	require.NoError(t, err)
	retiredCard := publishedCreature(t, ctx)
	withRetired := registerStarterDeck(t, ctx, commands, "使用不可入り")
	withRetiredFilled, err := commands.SetCardCounts(ctx, withRetired.ID, withRetired.Revision, []command.CardCountInput{{CardID: retiredCard, Count: 4}})
	require.NoError(t, err)
	_, err = newCardHandler(t, ctx).Retire(ctx, retiredCard, 2)
	require.NoError(t, err)

	_, totalErr := commands.Publish(ctx, shortFilled.ID, shortFilled.Revision)
	_, cardErr := commands.Publish(ctx, withRetiredFilled.ID, withRetiredFilled.Revision)

	totalInvalid, ok := input.AsErrors(totalErr)
	require.True(t, ok)
	require.Equal(t, input.Errors{&input.TotalMismatch{Field: "cards", Required: 4}}, totalInvalid)
	cardInvalid, ok := input.AsErrors(cardErr)
	require.True(t, ok)
	require.Equal(t, input.Errors{&input.Unavailable{Field: "cards"}}, cardInvalid)
	require.Equal(t, starterdeck.StatusDraft, restoredStarterDeck(t, ctx, short.ID).Status)
}

// 上限は game_rules の値。使用不可にすると数が戻り、使用不可から公開し直すときも上限を確かめる
func TestPublishStarterDeck_StopsAtRuleLimitAndRetireMakesRoom(t *testing.T) {
	ctx := signedIn(t, isolatedDatabase(t), registrant(t))
	commands := newStarterDeckHandlerWithRules(t, ctx, smallRules())
	first, second, third := fullStarterDeck(t, ctx, commands), fullStarterDeck(t, ctx, commands), fullStarterDeck(t, ctx, commands)
	firstPublished, err := commands.Publish(ctx, first.ID, first.Revision)
	require.NoError(t, err)
	_, err = commands.Publish(ctx, second.ID, second.Revision)
	require.NoError(t, err)

	_, atLimit := commands.Publish(ctx, third.ID, third.Revision)
	retired, err := commands.Retire(ctx, firstPublished.ID, firstPublished.Revision)
	require.NoError(t, err)
	_, afterRetire := commands.Publish(ctx, third.ID, third.Revision)
	_, republishAtLimit := commands.Publish(ctx, retired.ID, retired.Revision)

	require.ErrorIs(t, atLimit, command.ErrStarterDeckLimitReached)
	require.NoError(t, afterRetire)
	require.ErrorIs(t, republishAtLimit, command.ErrStarterDeckLimitReached)
	require.Equal(t, starterdeck.StatusPublished, restoredStarterDeck(t, ctx, third.ID).Status)
}

// 同時に公開しても、上限を超えない（確保が 1 つの文で、同じ行を順番に更新するため）
func TestPublishStarterDeck_NeverExceedsLimitWhenPublishedConcurrently(t *testing.T) {
	ctx := signedIn(t, isolatedDatabase(t), registrant(t))
	commands := newStarterDeckHandlerWithRules(t, ctx, smallRules())
	const decks = 6
	targets := make([]command.ChangedStarterDeck, 0, decks)
	for range decks {
		targets = append(targets, fullStarterDeck(t, ctx, commands))
	}
	errs := make([]error, decks)
	var wg sync.WaitGroup
	for i, target := range targets {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, errs[i] = commands.Publish(ctx, target.ID, target.Revision)
		}()
	}
	wg.Wait()

	succeeded, limited := 0, 0
	for _, err := range errs {
		if err == nil {
			succeeded++
			continue
		}
		require.ErrorIs(t, err, command.ErrStarterDeckLimitReached)
		limited++
	}
	require.Equal(t, 2, succeeded)
	require.Equal(t, decks-2, limited)
}

func TestRetireStarterDeck_RetiresOnlyPublishedAndKeepsRevisionWhenAlreadyRetired(t *testing.T) {
	ctx := signedIn(t, isolatedDatabase(t), registrant(t))
	commands := newStarterDeckHandlerWithRules(t, ctx, smallRules())
	deck := fullStarterDeck(t, ctx, commands)

	_, fromDraft := commands.Retire(ctx, deck.ID, deck.Revision)
	published, err := commands.Publish(ctx, deck.ID, deck.Revision)
	require.NoError(t, err)
	retired, err := commands.Retire(ctx, published.ID, published.Revision)
	require.NoError(t, err)
	again, againErr := commands.Retire(ctx, retired.ID, retired.Revision)

	require.ErrorIs(t, fromDraft, command.ErrInvalidStarterDeckStatusTransition)
	require.Equal(t, starterdeck.StatusRetired, restoredStarterDeck(t, ctx, deck.ID).Status)
	require.NoError(t, againErr)
	require.Equal(t, retired, again)
}

func TestStarterDeckEditing_IsRejectedUnlessDraft(t *testing.T) {
	ctx := signedIn(t, isolatedDatabase(t), registrant(t))
	commands := newStarterDeckHandlerWithRules(t, ctx, smallRules())
	deck := fullStarterDeck(t, ctx, commands)
	published, err := commands.Publish(ctx, deck.ID, deck.Revision)
	require.NoError(t, err)
	card := publishedCreature(t, ctx)

	_, rename := commands.Rename(ctx, published.ID, published.Revision, "別名")
	_, setCounts := commands.SetCardCounts(ctx, published.ID, published.Revision, []command.CardCountInput{{CardID: card, Count: 1}})
	deleted := commands.Delete(ctx, published.ID, published.Revision)

	require.ErrorIs(t, rename, command.ErrStarterDeckNotDraft)
	require.ErrorIs(t, setCounts, command.ErrStarterDeckNotDraft)
	require.ErrorIs(t, deleted, command.ErrStarterDeckNotDraft)
}

func TestDeleteStarterDeck_DeletesDraftAndReleasesNameAndReferences(t *testing.T) {
	ctx := signedIn(t, isolatedDatabase(t), registrant(t))
	commands := newStarterDeckHandler(t, ctx)
	registered := registerStarterDeck(t, ctx, commands, "初心者デッキ")
	card := publishedCreature(t, ctx)
	filled, err := commands.SetCardCounts(ctx, registered.ID, registered.Revision, []command.CardCountInput{{CardID: card, Count: 1}})
	require.NoError(t, err)

	err = commands.Delete(ctx, filled.ID, filled.Revision)

	require.NoError(t, err)
	_, renamed := commands.Rename(ctx, filled.ID, filled.Revision+1, "別名")
	require.ErrorIs(t, renamed, command.ErrStarterDeckNotFound)
	registerStarterDeck(t, ctx, commands, "初心者デッキ")
	registry, err := domaindef.Load(ctx)
	require.NoError(t, err)
	cardID, err := creature.NewID(internalID(t, ctx, card))
	require.NoError(t, err)
	require.NoError(t, identifier.New(registry, creature.NewID).Delete(ctx, cardID))
}

func TestStarterDeckCommands_RejectUnknownAndStaleRevision(t *testing.T) {
	ctx := signedIn(t, isolatedDatabase(t), registrant(t))
	commands := newStarterDeckHandlerWithRules(t, ctx, smallRules())
	deck := fullStarterDeck(t, ctx, commands)
	card := publishedCreature(t, ctx)
	tests := []struct {
		name string
		id   uuid.UUID
		call func(id uuid.UUID, revision int64) error
		want error
	}{
		{name: "rename_unknown", id: uuid.New(), want: command.ErrStarterDeckNotFound, call: func(id uuid.UUID, revision int64) error {
			_, err := commands.Rename(ctx, id, revision, "別名")
			return err
		}},
		{name: "publish_unknown", id: uuid.New(), want: command.ErrStarterDeckNotFound, call: func(id uuid.UUID, revision int64) error {
			_, err := commands.Publish(ctx, id, revision)
			return err
		}},
		{name: "retire_unknown", id: uuid.New(), want: command.ErrStarterDeckNotFound, call: func(id uuid.UUID, revision int64) error {
			_, err := commands.Retire(ctx, id, revision)
			return err
		}},
		{name: "delete_unknown", id: uuid.New(), want: command.ErrStarterDeckNotFound, call: func(id uuid.UUID, revision int64) error {
			return commands.Delete(ctx, id, revision)
		}},
		{name: "rename_stale", id: deck.ID, want: command.ErrRevisionConflict, call: func(id uuid.UUID, revision int64) error {
			_, err := commands.Rename(ctx, id, revision, "別名")
			return err
		}},
		{name: "set_card_counts_stale", id: deck.ID, want: command.ErrRevisionConflict, call: func(id uuid.UUID, revision int64) error {
			_, err := commands.SetCardCounts(ctx, id, revision, []command.CardCountInput{{CardID: card, Count: 1}})
			return err
		}},
		{name: "publish_stale", id: deck.ID, want: command.ErrRevisionConflict, call: func(id uuid.UUID, revision int64) error {
			_, err := commands.Publish(ctx, id, revision)
			return err
		}},
		{name: "delete_stale", id: deck.ID, want: command.ErrRevisionConflict, call: func(id uuid.UUID, revision int64) error {
			return commands.Delete(ctx, id, revision)
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.call(tt.id, deck.Revision-1)

			require.ErrorIs(t, err, tt.want)
		})
	}
}
