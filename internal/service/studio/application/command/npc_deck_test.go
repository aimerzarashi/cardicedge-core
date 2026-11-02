package command_test

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/constraint/constrainttest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/creature"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/npcdeck"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/presetdeck"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domaindef"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/identifier"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/studio/application/command"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/studio/application/input"
)

func restoredNPCDeck(t *testing.T, ctx context.Context, publicID uuid.UUID) npcdeck.NPCDeck {
	t.Helper()
	registry, err := domaindef.Load(ctx)
	require.NoError(t, err)
	id, err := identifier.New(registry, npcdeck.NewID).Resolve(ctx, publicID)
	require.NoError(t, err)
	state, err := npcDeckStore(registry).Load(ctx, id)
	require.NoError(t, err)
	return state.(npcdeck.NPCDeck)
}

func registerNPCDeck(t *testing.T, ctx context.Context, commands *command.NPCDeckHandler, name string) command.ChangedNPCDeck {
	t.Helper()
	registered, err := commands.Register(ctx, name)
	require.NoError(t, err)
	return registered
}

// smallRules で公開できる、公開中のカード 4 枚の準備中の NPC のデッキ
func fullNPCDeck(t *testing.T, ctx context.Context, commands *command.NPCDeckHandler) command.ChangedNPCDeck {
	t.Helper()
	registered := registerNPCDeck(t, ctx, commands, "デッキ-"+uuid.NewString())
	filled, err := commands.SetCardCounts(ctx, registered.ID, registered.Revision, []command.CardCountInput{{CardID: publishedCreature(t, ctx), Count: 4}})
	require.NoError(t, err)
	return filled
}

func TestRegisterNPCDeck_ReturnsPublicIDAndMakesDraftWithoutSource(t *testing.T) {
	ctx := signedIn(t, isolatedDatabase(t), registrant(t))

	registered, err := newNPCDeckHandler(t, ctx).Register(ctx, "  初心者デッキ ")

	require.NoError(t, err)
	require.Equal(t, int64(1), registered.Revision)
	npcDeck := restoredNPCDeck(t, ctx, registered.ID)
	require.Equal(t, "初心者デッキ", npcDeck.Name.String())
	require.Equal(t, npcdeck.StatusDraft, npcDeck.Status)
	require.Nil(t, npcDeck.Source)
	require.Empty(t, npcDeck.Cards)
}

func TestRegisterNPCDeck_RejectsNameAlreadyInUse(t *testing.T) {
	ctx := signedIn(t, isolatedDatabase(t), registrant(t))
	commands := newNPCDeckHandler(t, ctx)
	registerNPCDeck(t, ctx, commands, "ＡＢＣ")

	_, err := commands.Register(ctx, " abc")

	require.ErrorIs(t, err, command.ErrNPCDeckNameAlreadyInUse)
}

func TestRegisterNPCDeck_AllowsNameUsedByPresetDeck(t *testing.T) {
	ctx := signedIn(t, isolatedDatabase(t), registrant(t))
	registerPresetDeck(t, ctx, "初心者デッキ")

	_, err := newNPCDeckHandler(t, ctx).Register(ctx, "初心者デッキ")

	require.NoError(t, err)
}

func TestRegisterNPCDeck_AllowsNameUsedByStarterDeck(t *testing.T) {
	ctx := signedIn(t, isolatedDatabase(t), registrant(t))
	_, err := newStarterDeckHandler(t, ctx).Register(ctx, "初心者デッキ")
	require.NoError(t, err)

	_, err = newNPCDeckHandler(t, ctx).Register(ctx, "初心者デッキ")

	require.NoError(t, err)
}

func TestRegisterNPCDeck_RejectsInvalidName(t *testing.T) {
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

			_, err := newNPCDeckHandler(t, ctx).Register(ctx, tt.value)

			invalid, ok := input.AsErrors(err)
			require.True(t, ok)
			require.Equal(t, input.Errors{tt.want}, invalid)
		})
	}
}

func TestNPCDeckCommands_RejectWithoutOperator(t *testing.T) {
	tests := []struct {
		name string
		call func(t *testing.T, ctx context.Context, commands *command.NPCDeckHandler, npcDeck command.ChangedNPCDeck) error
	}{
		{name: "register", call: func(_ *testing.T, ctx context.Context, commands *command.NPCDeckHandler, _ command.ChangedNPCDeck) error {
			_, err := commands.Register(ctx, "中級者デッキ")
			return err
		}},
		{name: "register_from_preset_deck", call: func(_ *testing.T, ctx context.Context, commands *command.NPCDeckHandler, _ command.ChangedNPCDeck) error {
			_, err := commands.RegisterFromPresetDeck(ctx, uuid.New(), "中級者デッキ")
			return err
		}},
		{name: "rename", call: func(_ *testing.T, ctx context.Context, commands *command.NPCDeckHandler, npcDeck command.ChangedNPCDeck) error {
			_, err := commands.Rename(ctx, npcDeck.ID, npcDeck.Revision, "中級者デッキ")
			return err
		}},
		{name: "set_card_counts", call: func(t *testing.T, ctx context.Context, commands *command.NPCDeckHandler, npcDeck command.ChangedNPCDeck) error {
			_, err := commands.SetCardCounts(ctx, npcDeck.ID, npcDeck.Revision, []command.CardCountInput{{CardID: issueCard(t, ctx, creature.NewID), Count: 1}})
			return err
		}},
		{name: "delete", call: func(_ *testing.T, ctx context.Context, commands *command.NPCDeckHandler, npcDeck command.ChangedNPCDeck) error {
			return commands.Delete(ctx, npcDeck.ID, npcDeck.Revision)
		}},
		{name: "publish", call: func(_ *testing.T, ctx context.Context, commands *command.NPCDeckHandler, npcDeck command.ChangedNPCDeck) error {
			_, err := commands.Publish(ctx, npcDeck.ID, npcDeck.Revision)
			return err
		}},
		{name: "retire", call: func(_ *testing.T, ctx context.Context, commands *command.NPCDeckHandler, npcDeck command.ChangedNPCDeck) error {
			_, err := commands.Retire(ctx, npcDeck.ID, npcDeck.Revision)
			return err
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := isolatedDatabase(t)
			signed := signedIn(t, ctx, registrant(t))
			npcDeck := registerNPCDeck(t, signed, newNPCDeckHandler(t, signed), "初心者デッキ")

			err := tt.call(t, ctx, newNPCDeckHandler(t, ctx), npcDeck)

			require.ErrorIs(t, err, command.ErrNotOperator)
		})
	}
}

// カード・枚数を写して指定した名前で作り、写した元の公開 ID と revision を記録する。写した後に元を直しても、NPC のデッキは変わらない
func TestRegisterNPCDeckFromPresetDeck_UsesGivenNameAndCopiesCardsAndSource(t *testing.T) {
	ctx := signedIn(t, isolatedDatabase(t), registrant(t))
	presets := newPresetDeckHandler(t, ctx)
	dragon, potion := publishedCreature(t, ctx), publishedItem(t, ctx)
	preset := registerPresetDeck(t, ctx, "火のデッキ")
	filled, err := presets.SetCardCounts(ctx, preset.ID, preset.Revision, []command.CardCountInput{{CardID: dragon, Count: 3}, {CardID: potion, Count: 2}})
	require.NoError(t, err)
	commands := newNPCDeckHandler(t, ctx)

	copied, err := commands.RegisterFromPresetDeck(ctx, preset.ID, "炎のデッキ")

	require.NoError(t, err)
	require.Equal(t, int64(1), copied.Revision)
	npcDeck := restoredNPCDeck(t, ctx, copied.ID)
	require.Equal(t, "炎のデッキ", npcDeck.Name.String())
	require.Equal(t, npcdeck.StatusDraft, npcDeck.Status)
	require.Equal(t, 5, npcDeck.TotalCount())
	require.NotNil(t, npcDeck.Source)
	require.Equal(t, preset.ID, npcDeck.Source.PresetDeckID())
	require.Equal(t, filled.Revision, npcDeck.Source.Revision())
	require.ElementsMatch(t, []int64{internalID(t, ctx, dragon), internalID(t, ctx, potion)}, constrainttest.Referenced(t, ctx, npcdeck.CardReferenceType(), internalID(t, ctx, copied.ID)))
	_, err = presets.SetCardCounts(ctx, filled.ID, filled.Revision, []command.CardCountInput{{CardID: dragon, Count: 0}})
	require.NoError(t, err)
	require.Equal(t, 5, restoredNPCDeck(t, ctx, copied.ID).TotalCount())
}

func TestRegisterNPCDeckFromPresetDeck_RejectsUnknownPresetDeckAndNameInUse(t *testing.T) {
	ctx := signedIn(t, isolatedDatabase(t), registrant(t))
	commands := newNPCDeckHandler(t, ctx)
	preset := registerPresetDeck(t, ctx, "火のデッキ")
	deleted := registerPresetDeck(t, ctx, "水のデッキ")
	require.NoError(t, newPresetDeckHandler(t, ctx).Delete(ctx, deleted.ID, deleted.Revision))
	_, err := commands.RegisterFromPresetDeck(ctx, preset.ID, "火のデッキ")
	require.NoError(t, err)

	_, sameName := commands.RegisterFromPresetDeck(ctx, preset.ID, "火のデッキ")
	_, unknown := commands.RegisterFromPresetDeck(ctx, uuid.New(), "炎のデッキ")
	_, deletedPreset := commands.RegisterFromPresetDeck(ctx, deleted.ID, "炎のデッキ")

	require.ErrorIs(t, sameName, command.ErrNPCDeckNameAlreadyInUse)
	require.ErrorIs(t, unknown, command.ErrPresetDeckNotFound)
	require.ErrorIs(t, deletedPreset, command.ErrPresetDeckNotFound)
}

// 名前を元から写さないので、同じプリセットデッキから名前を変えて何度でも作れる
func TestRegisterNPCDeckFromPresetDeck_AllowsSamePresetDeckWithDifferentNames(t *testing.T) {
	ctx := signedIn(t, isolatedDatabase(t), registrant(t))
	commands := newNPCDeckHandler(t, ctx)
	preset := registerPresetDeck(t, ctx, "火のデッキ")
	_, err := commands.RegisterFromPresetDeck(ctx, preset.ID, "火のデッキ")
	require.NoError(t, err)

	_, err = commands.RegisterFromPresetDeck(ctx, preset.ID, "火のデッキ 2")

	require.NoError(t, err)
}

func TestRegisterNPCDeckFromPresetDeck_RejectsInvalidName(t *testing.T) {
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

			_, err := newNPCDeckHandler(t, ctx).RegisterFromPresetDeck(ctx, preset.ID, tt.value)

			invalid, ok := input.AsErrors(err)
			require.True(t, ok)
			require.Equal(t, input.Errors{tt.want}, invalid)
		})
	}
}

// 準備中は決まり（合計の枚数）を満たさなくてよいので、40 枚でないプリセットデッキも写せる
func TestRegisterNPCDeckFromPresetDeck_AllowsPresetDeckThatDoesNotMeetRules(t *testing.T) {
	ctx := signedIn(t, isolatedDatabase(t), registrant(t))
	preset := registerPresetDeck(t, ctx, "空のデッキ")

	copied, err := newNPCDeckHandler(t, ctx).RegisterFromPresetDeck(ctx, preset.ID, "空のデッキ")

	require.NoError(t, err)
	require.Equal(t, 0, restoredNPCDeck(t, ctx, copied.ID).TotalCount())
}

func TestRenameNPCDeck_RenamesAndRejectsNameInUse(t *testing.T) {
	ctx := signedIn(t, isolatedDatabase(t), registrant(t))
	commands := newNPCDeckHandler(t, ctx)
	registered := registerNPCDeck(t, ctx, commands, "初心者デッキ")
	registerNPCDeck(t, ctx, commands, "上級者デッキ")

	inUse, inUseErr := commands.Rename(ctx, registered.ID, registered.Revision, " 上級者デッキ")
	renamed, err := commands.Rename(ctx, registered.ID, registered.Revision, " 中級者デッキ ")

	require.ErrorIs(t, inUseErr, command.ErrNPCDeckNameAlreadyInUse)
	require.Zero(t, inUse)
	require.NoError(t, err)
	require.Equal(t, command.ChangedNPCDeck{ID: registered.ID, Revision: 2}, renamed)
	require.Equal(t, "中級者デッキ", restoredNPCDeck(t, ctx, registered.ID).Name.String())
}

// 名前は公開・使用不可でも手放さない（状態によらず、すべての NPC のデッキの中で一意）
func TestNPCDeckName_StaysReservedWhilePublishedAndRetired(t *testing.T) {
	ctx := signedIn(t, isolatedDatabase(t), registrant(t))
	commands := newNPCDeckHandlerWithRules(t, ctx, smallRules())
	npcDeck := fullNPCDeck(t, ctx, commands)
	name := restoredNPCDeck(t, ctx, npcDeck.ID).Name.String()
	published, err := commands.Publish(ctx, npcDeck.ID, npcDeck.Revision)
	require.NoError(t, err)

	_, whilePublished := commands.Register(ctx, name)
	_, err = commands.Retire(ctx, published.ID, published.Revision)
	require.NoError(t, err)
	_, whileRetired := commands.Register(ctx, name)

	require.ErrorIs(t, whilePublished, command.ErrNPCDeckNameAlreadyInUse)
	require.ErrorIs(t, whileRetired, command.ErrNPCDeckNameAlreadyInUse)
}

func TestSetNPCDeckCardCounts_ChangesCardsAndReferences(t *testing.T) {
	ctx := signedIn(t, isolatedDatabase(t), registrant(t))
	commands := newNPCDeckHandler(t, ctx)
	registered := registerNPCDeck(t, ctx, commands, "初心者デッキ")
	dragon, potion := publishedCreature(t, ctx), publishedItem(t, ctx)

	changed, err := commands.SetCardCounts(ctx, registered.ID, registered.Revision, []command.CardCountInput{{CardID: dragon, Count: 3}, {CardID: potion, Count: 1}})
	require.NoError(t, err)
	next, err := commands.SetCardCounts(ctx, changed.ID, changed.Revision, []command.CardCountInput{{CardID: potion, Count: 0}})

	require.NoError(t, err)
	require.Equal(t, int64(3), next.Revision)
	require.Equal(t, 3, restoredNPCDeck(t, ctx, registered.ID).TotalCount())
	require.Equal(t, []int64{internalID(t, ctx, dragon)}, constrainttest.Referenced(t, ctx, npcdeck.CardReferenceType(), internalID(t, ctx, registered.ID)))
}

// 新しく入れるカードは公開中だけ。すでに入っているカードは、使用不可になっても残せる（減らせる）
func TestSetNPCDeckCardCounts_RejectsCardNotPublishedButKeepsRetiredCardInDeck(t *testing.T) {
	ctx := signedIn(t, isolatedDatabase(t), registrant(t))
	commands := newNPCDeckHandler(t, ctx)
	registered := registerNPCDeck(t, ctx, commands, "初心者デッキ")
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

func TestSetNPCDeckCardCounts_RejectsTotalOverRule(t *testing.T) {
	ctx := signedIn(t, isolatedDatabase(t), registrant(t))
	commands := newNPCDeckHandlerWithRules(t, ctx, smallRules())
	registered := registerNPCDeck(t, ctx, commands, "初心者デッキ")
	filled, err := commands.SetCardCounts(ctx, registered.ID, registered.Revision, []command.CardCountInput{{CardID: publishedCreature(t, ctx), Count: 4}})
	require.NoError(t, err)

	_, err = commands.SetCardCounts(ctx, filled.ID, filled.Revision, []command.CardCountInput{{CardID: publishedItem(t, ctx), Count: 1}})

	invalid, ok := input.AsErrors(err)
	require.True(t, ok)
	require.Equal(t, input.Errors{&input.TotalTooLarge{Field: "cards", Max: 4}}, invalid)
}

func TestPublishNPCDeck_PublishesAndKeepsRevisionWhenAlreadyPublished(t *testing.T) {
	ctx := signedIn(t, isolatedDatabase(t), registrant(t))
	commands := newNPCDeckHandlerWithRules(t, ctx, smallRules())
	npcDeck := fullNPCDeck(t, ctx, commands)

	published, err := commands.Publish(ctx, npcDeck.ID, npcDeck.Revision)
	again, againErr := commands.Publish(ctx, published.ID, published.Revision)
	_, stale := commands.Publish(ctx, npcDeck.ID, npcDeck.Revision)

	require.NoError(t, err)
	require.Equal(t, command.ChangedNPCDeck{ID: npcDeck.ID, Revision: npcDeck.Revision + 1}, published)
	require.Equal(t, npcdeck.StatusPublished, restoredNPCDeck(t, ctx, npcDeck.ID).Status)
	require.NoError(t, againErr)
	require.Equal(t, published, again)
	require.ErrorIs(t, stale, command.ErrRevisionConflict)
}

func TestPublishNPCDeck_ReturnsFieldErrorsWhenTotalOrCardsAreInvalid(t *testing.T) {
	ctx := signedIn(t, isolatedDatabase(t), registrant(t))
	commands := newNPCDeckHandlerWithRules(t, ctx, smallRules())
	short := registerNPCDeck(t, ctx, commands, "少ない")
	shortFilled, err := commands.SetCardCounts(ctx, short.ID, short.Revision, []command.CardCountInput{{CardID: publishedCreature(t, ctx), Count: 3}})
	require.NoError(t, err)
	retiredCard := publishedCreature(t, ctx)
	withRetired := registerNPCDeck(t, ctx, commands, "使用不可入り")
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
	require.Equal(t, npcdeck.StatusDraft, restoredNPCDeck(t, ctx, short.ID).Status)
}

// 公開中の数の上限は持たない
func TestPublishNPCDeck_IsNotLimitedByNumberOfPublishedDecks(t *testing.T) {
	ctx := signedIn(t, isolatedDatabase(t), registrant(t))
	commands := newNPCDeckHandlerWithRules(t, ctx, smallRules())

	for range 3 {
		npcDeck := fullNPCDeck(t, ctx, commands)
		_, err := commands.Publish(ctx, npcDeck.ID, npcDeck.Revision)
		require.NoError(t, err)
	}
}

func TestRetireNPCDeck_RetiresOnlyPublishedAndKeepsRevisionWhenAlreadyRetired(t *testing.T) {
	ctx := signedIn(t, isolatedDatabase(t), registrant(t))
	commands := newNPCDeckHandlerWithRules(t, ctx, smallRules())
	npcDeck := fullNPCDeck(t, ctx, commands)

	_, fromDraft := commands.Retire(ctx, npcDeck.ID, npcDeck.Revision)
	published, err := commands.Publish(ctx, npcDeck.ID, npcDeck.Revision)
	require.NoError(t, err)
	retired, err := commands.Retire(ctx, published.ID, published.Revision)
	require.NoError(t, err)
	again, againErr := commands.Retire(ctx, retired.ID, retired.Revision)

	require.ErrorIs(t, fromDraft, command.ErrInvalidNPCDeckStatusTransition)
	require.Equal(t, npcdeck.StatusRetired, restoredNPCDeck(t, ctx, npcDeck.ID).Status)
	require.NoError(t, againErr)
	require.Equal(t, retired, again)
}

func TestNPCDeckEditing_IsRejectedUnlessDraft(t *testing.T) {
	ctx := signedIn(t, isolatedDatabase(t), registrant(t))
	commands := newNPCDeckHandlerWithRules(t, ctx, smallRules())
	npcDeck := fullNPCDeck(t, ctx, commands)
	published, err := commands.Publish(ctx, npcDeck.ID, npcDeck.Revision)
	require.NoError(t, err)
	card := publishedCreature(t, ctx)

	_, rename := commands.Rename(ctx, published.ID, published.Revision, "別名")
	_, setCounts := commands.SetCardCounts(ctx, published.ID, published.Revision, []command.CardCountInput{{CardID: card, Count: 1}})
	deleted := commands.Delete(ctx, published.ID, published.Revision)

	require.ErrorIs(t, rename, command.ErrNPCDeckNotDraft)
	require.ErrorIs(t, setCounts, command.ErrNPCDeckNotDraft)
	require.ErrorIs(t, deleted, command.ErrNPCDeckNotDraft)
}

func TestDeleteNPCDeck_DeletesDraftAndReleasesNameAndReferences(t *testing.T) {
	ctx := signedIn(t, isolatedDatabase(t), registrant(t))
	commands := newNPCDeckHandler(t, ctx)
	registered := registerNPCDeck(t, ctx, commands, "初心者デッキ")
	card := publishedCreature(t, ctx)
	filled, err := commands.SetCardCounts(ctx, registered.ID, registered.Revision, []command.CardCountInput{{CardID: card, Count: 1}})
	require.NoError(t, err)

	err = commands.Delete(ctx, filled.ID, filled.Revision)

	require.NoError(t, err)
	_, renamed := commands.Rename(ctx, filled.ID, filled.Revision+1, "別名")
	require.ErrorIs(t, renamed, command.ErrNPCDeckNotFound)
	registerNPCDeck(t, ctx, commands, "初心者デッキ")
	registry, err := domaindef.Load(ctx)
	require.NoError(t, err)
	cardID, err := creature.NewID(internalID(t, ctx, card))
	require.NoError(t, err)
	require.NoError(t, identifier.New(registry, creature.NewID).Delete(ctx, cardID))
}

func TestNPCDeckCommands_RejectUnknownAndStaleRevision(t *testing.T) {
	ctx := signedIn(t, isolatedDatabase(t), registrant(t))
	commands := newNPCDeckHandlerWithRules(t, ctx, smallRules())
	npcDeck := fullNPCDeck(t, ctx, commands)
	card := publishedCreature(t, ctx)
	tests := []struct {
		name string
		id   uuid.UUID
		call func(id uuid.UUID, revision int64) error
		want error
	}{
		{name: "rename_unknown", id: uuid.New(), want: command.ErrNPCDeckNotFound, call: func(id uuid.UUID, revision int64) error {
			_, err := commands.Rename(ctx, id, revision, "別名")
			return err
		}},
		{name: "publish_unknown", id: uuid.New(), want: command.ErrNPCDeckNotFound, call: func(id uuid.UUID, revision int64) error {
			_, err := commands.Publish(ctx, id, revision)
			return err
		}},
		{name: "retire_unknown", id: uuid.New(), want: command.ErrNPCDeckNotFound, call: func(id uuid.UUID, revision int64) error {
			_, err := commands.Retire(ctx, id, revision)
			return err
		}},
		{name: "delete_unknown", id: uuid.New(), want: command.ErrNPCDeckNotFound, call: func(id uuid.UUID, revision int64) error {
			return commands.Delete(ctx, id, revision)
		}},
		{name: "rename_stale", id: npcDeck.ID, want: command.ErrRevisionConflict, call: func(id uuid.UUID, revision int64) error {
			_, err := commands.Rename(ctx, id, revision, "別名")
			return err
		}},
		{name: "set_card_counts_stale", id: npcDeck.ID, want: command.ErrRevisionConflict, call: func(id uuid.UUID, revision int64) error {
			_, err := commands.SetCardCounts(ctx, id, revision, []command.CardCountInput{{CardID: card, Count: 1}})
			return err
		}},
		{name: "publish_stale", id: npcDeck.ID, want: command.ErrRevisionConflict, call: func(id uuid.UUID, revision int64) error {
			_, err := commands.Publish(ctx, id, revision)
			return err
		}},
		{name: "delete_stale", id: npcDeck.ID, want: command.ErrRevisionConflict, call: func(id uuid.UUID, revision int64) error {
			return commands.Delete(ctx, id, revision)
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.call(tt.id, npcDeck.Revision-1)

			require.ErrorIs(t, err, tt.want)
		})
	}
}
