package command_test

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/constraint/constrainttest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/creature"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/grantdeck"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/presetdeck"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domaindef"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/identifier"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/studio/application/command"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/studio/application/input"
)

func restoredGrantDeck(t *testing.T, ctx context.Context, publicID uuid.UUID) grantdeck.GrantDeck {
	t.Helper()
	registry, err := domaindef.Load(ctx)
	require.NoError(t, err)
	id, err := identifier.New(registry, grantdeck.NewID).Resolve(ctx, publicID)
	require.NoError(t, err)
	state, err := grantDeckStore(registry).Load(ctx, id)
	require.NoError(t, err)
	return state.(grantdeck.GrantDeck)
}

func registerGrantDeck(t *testing.T, ctx context.Context, commands *command.GrantDeckHandler, name string) command.ChangedGrantDeck {
	t.Helper()
	registered, err := commands.Register(ctx, name)
	require.NoError(t, err)
	return registered
}

// smallRules で公開できる、公開中のカード 4 枚の準備中の配布デッキ
func fullGrantDeck(t *testing.T, ctx context.Context, commands *command.GrantDeckHandler) command.ChangedGrantDeck {
	t.Helper()
	registered := registerGrantDeck(t, ctx, commands, "デッキ-"+uuid.NewString())
	filled, err := commands.SetCardCounts(ctx, registered.ID, registered.Revision, []command.CardCountInput{{CardID: publishedCreature(t, ctx), Count: 4}})
	require.NoError(t, err)
	return filled
}

func TestRegisterGrantDeck_ReturnsPublicIDAndMakesDraftWithoutSource(t *testing.T) {
	ctx := signedIn(t, isolatedDatabase(t), registrant(t))

	registered, err := newGrantDeckHandler(t, ctx).Register(ctx, "  初心者デッキ ")

	require.NoError(t, err)
	require.Equal(t, int64(1), registered.Revision)
	deck := restoredGrantDeck(t, ctx, registered.ID)
	require.Equal(t, "初心者デッキ", deck.Name.String())
	require.Equal(t, grantdeck.StatusDraft, deck.Status)
	require.Nil(t, registeredSource(t, ctx, registered.ID))
	require.Empty(t, deck.Cards)
}

func TestRegisterGrantDeck_RejectsNameAlreadyInUse(t *testing.T) {
	ctx := signedIn(t, isolatedDatabase(t), registrant(t))
	commands := newGrantDeckHandler(t, ctx)
	registerGrantDeck(t, ctx, commands, "ＡＢＣ")

	_, err := commands.Register(ctx, " abc")

	require.ErrorIs(t, err, command.ErrGrantDeckNameAlreadyInUse)
}

func TestRegisterGrantDeck_AllowsNameUsedByPresetDeck(t *testing.T) {
	ctx := signedIn(t, isolatedDatabase(t), registrant(t))
	registerPresetDeck(t, ctx, "初心者デッキ")

	_, err := newGrantDeckHandler(t, ctx).Register(ctx, "初心者デッキ")

	require.NoError(t, err)
}

func TestRegisterGrantDeck_RejectsInvalidName(t *testing.T) {
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

			_, err := newGrantDeckHandler(t, ctx).Register(ctx, tt.value)

			invalid, ok := input.AsErrors(err)
			require.True(t, ok)
			require.Equal(t, input.Errors{tt.want}, invalid)
		})
	}
}

func TestGrantDeckCommands_RejectWithoutOperator(t *testing.T) {
	tests := []struct {
		name string
		call func(t *testing.T, ctx context.Context, commands *command.GrantDeckHandler, deck command.ChangedGrantDeck) error
	}{
		{name: "register", call: func(_ *testing.T, ctx context.Context, commands *command.GrantDeckHandler, _ command.ChangedGrantDeck) error {
			_, err := commands.Register(ctx, "中級者デッキ")
			return err
		}},
		{name: "register_from_preset_deck", call: func(_ *testing.T, ctx context.Context, commands *command.GrantDeckHandler, _ command.ChangedGrantDeck) error {
			_, err := commands.RegisterFromPresetDeck(ctx, uuid.New(), "中級者デッキ")
			return err
		}},
		{name: "rename", call: func(_ *testing.T, ctx context.Context, commands *command.GrantDeckHandler, deck command.ChangedGrantDeck) error {
			_, err := commands.Rename(ctx, deck.ID, deck.Revision, "中級者デッキ")
			return err
		}},
		{name: "set_card_counts", call: func(t *testing.T, ctx context.Context, commands *command.GrantDeckHandler, deck command.ChangedGrantDeck) error {
			_, err := commands.SetCardCounts(ctx, deck.ID, deck.Revision, []command.CardCountInput{{CardID: issueCard(t, ctx, creature.NewID), Count: 1}})
			return err
		}},
		{name: "delete", call: func(_ *testing.T, ctx context.Context, commands *command.GrantDeckHandler, deck command.ChangedGrantDeck) error {
			return commands.Delete(ctx, deck.ID, deck.Revision)
		}},
		{name: "publish", call: func(_ *testing.T, ctx context.Context, commands *command.GrantDeckHandler, deck command.ChangedGrantDeck) error {
			_, err := commands.Publish(ctx, deck.ID, deck.Revision)
			return err
		}},
		{name: "retire", call: func(_ *testing.T, ctx context.Context, commands *command.GrantDeckHandler, deck command.ChangedGrantDeck) error {
			_, err := commands.Retire(ctx, deck.ID, deck.Revision)
			return err
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := isolatedDatabase(t)
			signed := signedIn(t, ctx, registrant(t))
			deck := registerGrantDeck(t, signed, newGrantDeckHandler(t, signed), "初心者デッキ")

			err := tt.call(t, ctx, newGrantDeckHandler(t, ctx), deck)

			require.ErrorIs(t, err, command.ErrNotOperator)
		})
	}
}

// カード・枚数を写して指定した名前で作り、写した元の公開 ID と revision を記録する。写した後に元を直しても、配布デッキは変わらない
func TestRegisterGrantDeckFromPresetDeck_UsesGivenNameAndCopiesCardsAndSource(t *testing.T) {
	ctx := signedIn(t, isolatedDatabase(t), registrant(t))
	presets := newPresetDeckHandler(t, ctx)
	dragon, potion := publishedCreature(t, ctx), publishedItem(t, ctx)
	preset := registerPresetDeck(t, ctx, "火のデッキ")
	filled, err := presets.SetCardCounts(ctx, preset.ID, preset.Revision, []command.CardCountInput{{CardID: dragon, Count: 3}, {CardID: potion, Count: 2}})
	require.NoError(t, err)
	commands := newGrantDeckHandler(t, ctx)

	copied, err := commands.RegisterFromPresetDeck(ctx, preset.ID, "炎のデッキ")

	require.NoError(t, err)
	require.Equal(t, int64(1), copied.Revision)
	deck := restoredGrantDeck(t, ctx, copied.ID)
	require.Equal(t, "炎のデッキ", deck.Name.String())
	require.Equal(t, grantdeck.StatusDraft, deck.Status)
	require.Equal(t, 5, deck.TotalCount())
	source := registeredSource(t, ctx, copied.ID)
	require.NotNil(t, source)
	require.Equal(t, internalID(t, ctx, preset.ID), source.PresetDeckID)
	require.Equal(t, filled.Revision, source.Revision)
	require.ElementsMatch(t, []int64{internalID(t, ctx, dragon), internalID(t, ctx, potion)}, constrainttest.Referenced(t, ctx, grantdeck.CardReferenceType(), internalID(t, ctx, copied.ID)))
	_, err = presets.SetCardCounts(ctx, filled.ID, filled.Revision, []command.CardCountInput{{CardID: dragon, Count: 0}})
	require.NoError(t, err)
	require.Equal(t, 5, restoredGrantDeck(t, ctx, copied.ID).TotalCount())
}

func TestRegisterGrantDeckFromPresetDeck_RejectsUnknownPresetDeckAndNameInUse(t *testing.T) {
	ctx := signedIn(t, isolatedDatabase(t), registrant(t))
	commands := newGrantDeckHandler(t, ctx)
	preset := registerPresetDeck(t, ctx, "火のデッキ")
	deleted := registerPresetDeck(t, ctx, "水のデッキ")
	require.NoError(t, newPresetDeckHandler(t, ctx).Delete(ctx, deleted.ID, deleted.Revision))
	_, err := commands.RegisterFromPresetDeck(ctx, preset.ID, "火のデッキ")
	require.NoError(t, err)

	_, sameName := commands.RegisterFromPresetDeck(ctx, preset.ID, "火のデッキ")
	_, unknown := commands.RegisterFromPresetDeck(ctx, uuid.New(), "炎のデッキ")
	_, deletedPreset := commands.RegisterFromPresetDeck(ctx, deleted.ID, "炎のデッキ")

	require.ErrorIs(t, sameName, command.ErrGrantDeckNameAlreadyInUse)
	require.ErrorIs(t, unknown, command.ErrPresetDeckNotFound)
	require.ErrorIs(t, deletedPreset, command.ErrPresetDeckNotFound)
}

// 名前を元から写さないので、同じプリセットデッキから名前を変えて何度でも作れる
func TestRegisterGrantDeckFromPresetDeck_AllowsSamePresetDeckWithDifferentNames(t *testing.T) {
	ctx := signedIn(t, isolatedDatabase(t), registrant(t))
	commands := newGrantDeckHandler(t, ctx)
	preset := registerPresetDeck(t, ctx, "火のデッキ")
	_, err := commands.RegisterFromPresetDeck(ctx, preset.ID, "火のデッキ")
	require.NoError(t, err)

	_, err = commands.RegisterFromPresetDeck(ctx, preset.ID, "火のデッキ 2")

	require.NoError(t, err)
}

func TestRegisterGrantDeckFromPresetDeck_RejectsInvalidName(t *testing.T) {
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
			preset := registerPresetDeck(t, ctx, "火のデッキ")

			_, err := newGrantDeckHandler(t, ctx).RegisterFromPresetDeck(ctx, preset.ID, tt.value)

			invalid, ok := input.AsErrors(err)
			require.True(t, ok)
			require.Equal(t, input.Errors{tt.want}, invalid)
		})
	}
}

// 準備中は決まり（合計の枚数）を満たさなくてよいので、40 枚でないプリセットデッキも写せる
func TestRegisterGrantDeckFromPresetDeck_AllowsPresetDeckThatDoesNotMeetRules(t *testing.T) {
	ctx := signedIn(t, isolatedDatabase(t), registrant(t))
	preset := registerPresetDeck(t, ctx, "空のデッキ")

	copied, err := newGrantDeckHandler(t, ctx).RegisterFromPresetDeck(ctx, preset.ID, "空のデッキ")

	require.NoError(t, err)
	require.Equal(t, 0, restoredGrantDeck(t, ctx, copied.ID).TotalCount())
}

func TestRenameGrantDeck_RenamesAndRejectsNameInUse(t *testing.T) {
	ctx := signedIn(t, isolatedDatabase(t), registrant(t))
	commands := newGrantDeckHandler(t, ctx)
	registered := registerGrantDeck(t, ctx, commands, "初心者デッキ")
	registerGrantDeck(t, ctx, commands, "上級者デッキ")

	inUse, inUseErr := commands.Rename(ctx, registered.ID, registered.Revision, " 上級者デッキ")
	renamed, err := commands.Rename(ctx, registered.ID, registered.Revision, " 中級者デッキ ")

	require.ErrorIs(t, inUseErr, command.ErrGrantDeckNameAlreadyInUse)
	require.Zero(t, inUse)
	require.NoError(t, err)
	require.Equal(t, command.ChangedGrantDeck{ID: registered.ID, Revision: 2}, renamed)
	require.Equal(t, "中級者デッキ", restoredGrantDeck(t, ctx, registered.ID).Name.String())
}

// 名前は公開・使用不可でも手放さない（状態によらず、すべての配布デッキの中で一意）
func TestGrantDeckName_StaysReservedWhilePublishedAndRetired(t *testing.T) {
	ctx := signedIn(t, isolatedDatabase(t), registrant(t))
	commands := newGrantDeckHandlerWithRules(t, ctx, smallRules())
	deck := fullGrantDeck(t, ctx, commands)
	name := restoredGrantDeck(t, ctx, deck.ID).Name.String()
	published, err := commands.Publish(ctx, deck.ID, deck.Revision)
	require.NoError(t, err)

	_, whilePublished := commands.Register(ctx, name)
	_, err = commands.Retire(ctx, published.ID, published.Revision)
	require.NoError(t, err)
	_, whileRetired := commands.Register(ctx, name)

	require.ErrorIs(t, whilePublished, command.ErrGrantDeckNameAlreadyInUse)
	require.ErrorIs(t, whileRetired, command.ErrGrantDeckNameAlreadyInUse)
}

func TestSetGrantDeckCardCounts_ChangesCardsAndReferences(t *testing.T) {
	ctx := signedIn(t, isolatedDatabase(t), registrant(t))
	commands := newGrantDeckHandler(t, ctx)
	registered := registerGrantDeck(t, ctx, commands, "初心者デッキ")
	dragon, potion := publishedCreature(t, ctx), publishedItem(t, ctx)

	changed, err := commands.SetCardCounts(ctx, registered.ID, registered.Revision, []command.CardCountInput{{CardID: dragon, Count: 3}, {CardID: potion, Count: 1}})
	require.NoError(t, err)
	next, err := commands.SetCardCounts(ctx, changed.ID, changed.Revision, []command.CardCountInput{{CardID: potion, Count: 0}})

	require.NoError(t, err)
	require.Equal(t, int64(3), next.Revision)
	require.Equal(t, 3, restoredGrantDeck(t, ctx, registered.ID).TotalCount())
	require.Equal(t, []int64{internalID(t, ctx, dragon)}, constrainttest.Referenced(t, ctx, grantdeck.CardReferenceType(), internalID(t, ctx, registered.ID)))
}

// 新しく入れるカードは公開中だけ。すでに入っているカードは、使用不可になっても残せる（減らせる）
func TestSetGrantDeckCardCounts_RejectsCardNotPublishedButKeepsRetiredCardInDeck(t *testing.T) {
	ctx := signedIn(t, isolatedDatabase(t), registrant(t))
	commands := newGrantDeckHandler(t, ctx)
	registered := registerGrantDeck(t, ctx, commands, "初心者デッキ")
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

func TestSetGrantDeckCardCounts_RejectsTotalOverRule(t *testing.T) {
	ctx := signedIn(t, isolatedDatabase(t), registrant(t))
	commands := newGrantDeckHandlerWithRules(t, ctx, smallRules())
	registered := registerGrantDeck(t, ctx, commands, "初心者デッキ")
	filled, err := commands.SetCardCounts(ctx, registered.ID, registered.Revision, []command.CardCountInput{{CardID: publishedCreature(t, ctx), Count: 4}})
	require.NoError(t, err)

	_, err = commands.SetCardCounts(ctx, filled.ID, filled.Revision, []command.CardCountInput{{CardID: publishedItem(t, ctx), Count: 1}})

	invalid, ok := input.AsErrors(err)
	require.True(t, ok)
	require.Equal(t, input.Errors{&input.TotalTooLarge{Field: "cards", Max: 4}}, invalid)
}

func TestPublishGrantDeck_PublishesAndKeepsRevisionWhenAlreadyPublished(t *testing.T) {
	ctx := signedIn(t, isolatedDatabase(t), registrant(t))
	commands := newGrantDeckHandlerWithRules(t, ctx, smallRules())
	deck := fullGrantDeck(t, ctx, commands)

	published, err := commands.Publish(ctx, deck.ID, deck.Revision)
	again, againErr := commands.Publish(ctx, published.ID, published.Revision)
	_, stale := commands.Publish(ctx, deck.ID, deck.Revision)

	require.NoError(t, err)
	require.Equal(t, command.ChangedGrantDeck{ID: deck.ID, Revision: deck.Revision + 1}, published)
	require.Equal(t, grantdeck.StatusPublished, restoredGrantDeck(t, ctx, deck.ID).Status)
	require.NoError(t, againErr)
	require.Equal(t, published, again)
	require.ErrorIs(t, stale, command.ErrRevisionConflict)
}

func TestPublishGrantDeck_ReturnsFieldErrorsWhenTotalOrCardsAreInvalid(t *testing.T) {
	ctx := signedIn(t, isolatedDatabase(t), registrant(t))
	commands := newGrantDeckHandlerWithRules(t, ctx, smallRules())
	short := registerGrantDeck(t, ctx, commands, "少ない")
	shortFilled, err := commands.SetCardCounts(ctx, short.ID, short.Revision, []command.CardCountInput{{CardID: publishedCreature(t, ctx), Count: 3}})
	require.NoError(t, err)
	retiredCard := publishedCreature(t, ctx)
	withRetired := registerGrantDeck(t, ctx, commands, "使用不可入り")
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
	require.Equal(t, grantdeck.StatusDraft, restoredGrantDeck(t, ctx, short.ID).Status)
}

// 公開中の数に上限は持たない（どれを配るかはスターターセットが決めるため）
func TestPublishGrantDeck_HasNoLimitOnPublishedCount(t *testing.T) {
	ctx := signedIn(t, isolatedDatabase(t), registrant(t))
	commands := newGrantDeckHandlerWithRules(t, ctx, smallRules())

	for range 3 {
		deck := fullGrantDeck(t, ctx, commands)
		published, err := commands.Publish(ctx, deck.ID, deck.Revision)
		require.NoError(t, err)
		require.Equal(t, grantdeck.StatusPublished, restoredGrantDeck(t, ctx, published.ID).Status)
	}
}

func TestRetireGrantDeck_RetiresOnlyPublishedAndKeepsRevisionWhenAlreadyRetired(t *testing.T) {
	ctx := signedIn(t, isolatedDatabase(t), registrant(t))
	commands := newGrantDeckHandlerWithRules(t, ctx, smallRules())
	deck := fullGrantDeck(t, ctx, commands)

	_, fromDraft := commands.Retire(ctx, deck.ID, deck.Revision)
	published, err := commands.Publish(ctx, deck.ID, deck.Revision)
	require.NoError(t, err)
	retired, err := commands.Retire(ctx, published.ID, published.Revision)
	require.NoError(t, err)
	again, againErr := commands.Retire(ctx, retired.ID, retired.Revision)

	require.ErrorIs(t, fromDraft, command.ErrInvalidGrantDeckStatusTransition)
	require.Equal(t, grantdeck.StatusRetired, restoredGrantDeck(t, ctx, deck.ID).Status)
	require.NoError(t, againErr)
	require.Equal(t, retired, again)
}

func TestGrantDeckEditing_IsRejectedUnlessDraft(t *testing.T) {
	ctx := signedIn(t, isolatedDatabase(t), registrant(t))
	commands := newGrantDeckHandlerWithRules(t, ctx, smallRules())
	deck := fullGrantDeck(t, ctx, commands)
	published, err := commands.Publish(ctx, deck.ID, deck.Revision)
	require.NoError(t, err)
	card := publishedCreature(t, ctx)

	_, rename := commands.Rename(ctx, published.ID, published.Revision, "別名")
	_, setCounts := commands.SetCardCounts(ctx, published.ID, published.Revision, []command.CardCountInput{{CardID: card, Count: 1}})
	deleted := commands.Delete(ctx, published.ID, published.Revision)

	require.ErrorIs(t, rename, command.ErrGrantDeckNotDraft)
	require.ErrorIs(t, setCounts, command.ErrGrantDeckNotDraft)
	require.ErrorIs(t, deleted, command.ErrGrantDeckNotDraft)
}

func TestDeleteGrantDeck_DeletesDraftAndReleasesNameAndReferences(t *testing.T) {
	ctx := signedIn(t, isolatedDatabase(t), registrant(t))
	commands := newGrantDeckHandler(t, ctx)
	registered := registerGrantDeck(t, ctx, commands, "初心者デッキ")
	card := publishedCreature(t, ctx)
	filled, err := commands.SetCardCounts(ctx, registered.ID, registered.Revision, []command.CardCountInput{{CardID: card, Count: 1}})
	require.NoError(t, err)

	err = commands.Delete(ctx, filled.ID, filled.Revision)

	require.NoError(t, err)
	_, renamed := commands.Rename(ctx, filled.ID, filled.Revision+1, "別名")
	require.ErrorIs(t, renamed, command.ErrGrantDeckNotFound)
	registerGrantDeck(t, ctx, commands, "初心者デッキ")
	registry, err := domaindef.Load(ctx)
	require.NoError(t, err)
	cardID, err := creature.NewID(internalID(t, ctx, card))
	require.NoError(t, err)
	require.NoError(t, identifier.New(registry, creature.NewID).Delete(ctx, cardID))
}

func TestGrantDeckCommands_RejectUnknownAndStaleRevision(t *testing.T) {
	ctx := signedIn(t, isolatedDatabase(t), registrant(t))
	commands := newGrantDeckHandlerWithRules(t, ctx, smallRules())
	deck := fullGrantDeck(t, ctx, commands)
	card := publishedCreature(t, ctx)
	tests := []struct {
		name string
		id   uuid.UUID
		call func(id uuid.UUID, revision int64) error
		want error
	}{
		{name: "rename_unknown", id: uuid.New(), want: command.ErrGrantDeckNotFound, call: func(id uuid.UUID, revision int64) error {
			_, err := commands.Rename(ctx, id, revision, "別名")
			return err
		}},
		{name: "publish_unknown", id: uuid.New(), want: command.ErrGrantDeckNotFound, call: func(id uuid.UUID, revision int64) error {
			_, err := commands.Publish(ctx, id, revision)
			return err
		}},
		{name: "retire_unknown", id: uuid.New(), want: command.ErrGrantDeckNotFound, call: func(id uuid.UUID, revision int64) error {
			_, err := commands.Retire(ctx, id, revision)
			return err
		}},
		{name: "delete_unknown", id: uuid.New(), want: command.ErrGrantDeckNotFound, call: func(id uuid.UUID, revision int64) error {
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
