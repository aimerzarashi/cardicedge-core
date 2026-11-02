package command_test

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/constraint"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/constraint/constrainttest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/creature"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/gamerules"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/operator/operatortest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/presetdeck"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domaindef"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/identifier"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/studio/application/command"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/studio/application/input"
)

func referencedCards(t *testing.T, ctx context.Context, presetDeckID uuid.UUID) []int64 {
	t.Helper()
	return constrainttest.Referenced(t, ctx, presetdeck.CardReferenceType(), internalID(t, ctx, presetDeckID))
}

// コマンドの結果は公開 ID と revision だけなので、変えた中身はイベントストアから集約を読み直して確かめる
func restoredPresetDeck(t *testing.T, ctx context.Context, publicID uuid.UUID) presetdeck.PresetDeck {
	t.Helper()
	registry, err := domaindef.Load(ctx)
	require.NoError(t, err)
	id, err := identifier.New(registry, presetdeck.NewID).Resolve(ctx, publicID)
	require.NoError(t, err)
	state, err := presetDeckStore(registry).Load(ctx, id)
	require.NoError(t, err)
	return state.(presetdeck.PresetDeck)
}

// 入れているカードを、公開 ID と枚数で返す
func restoredCardCounts(t *testing.T, ctx context.Context, publicID uuid.UUID) map[uuid.UUID]int {
	t.Helper()
	registry, err := domaindef.Load(ctx)
	require.NoError(t, err)
	deck := restoredPresetDeck(t, ctx, publicID)
	refs := make([]domain.EntityRef, 0, len(deck.Cards))
	for card := range deck.Cards {
		refs = append(refs, domain.NewEntityRef(card.EntityType(), card.ID()))
	}
	publicIDs, err := identifier.NewEntityRefResolver(registry).PublicIDs(ctx, refs)
	require.NoError(t, err)
	counts := make(map[uuid.UUID]int, len(deck.Cards))
	for card, count := range deck.Cards {
		counts[publicIDs[domain.NewEntityRef(card.EntityType(), card.ID())]] = count.Int()
	}
	return counts
}

func TestRegisterPresetDeck_ReturnsPublicIDAndFirstRevision(t *testing.T) {
	ctx := signedIn(t, isolatedDatabase(t), registrant(t))
	registered, err := newPresetDeckHandler(t, ctx).Register(ctx, "  初心者デッキ ")

	require.NoError(t, err)
	require.NotEqual(t, uuid.Nil, registered.ID)
	require.Equal(t, int64(1), registered.Revision)
	deck := restoredPresetDeck(t, ctx, registered.ID)
	require.Equal(t, "初心者デッキ", deck.Name.String())
	require.Empty(t, deck.Cards)
}

func TestRegisterPresetDeck_RejectsNameAlreadyInUse(t *testing.T) {
	ctx := signedIn(t, isolatedDatabase(t), registrant(t))
	registerPresetDeck(t, ctx, "ＡＢＣ")

	_, err := newPresetDeckHandler(t, ctx).Register(ctx, " abc")

	require.ErrorIs(t, err, command.ErrPresetDeckNameAlreadyInUse)
}

func TestRegisterPresetDeck_RejectsInvalidName(t *testing.T) {
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

			_, err := newPresetDeckHandler(t, ctx).Register(ctx, tt.value)

			invalid, ok := input.AsErrors(err)
			require.True(t, ok)
			require.Equal(t, input.Errors{tt.want}, invalid)
		})
	}
}

func TestPresetDeckCommands_RejectWithoutOperator(t *testing.T) {
	tests := []struct {
		name string
		call func(t *testing.T, ctx context.Context, commands *command.PresetDeckHandler, deck command.ChangedPresetDeck) error
	}{
		{name: "register", call: func(_ *testing.T, ctx context.Context, commands *command.PresetDeckHandler, _ command.ChangedPresetDeck) error {
			_, err := commands.Register(ctx, "中級者デッキ")
			return err
		}},
		{name: "rename", call: func(_ *testing.T, ctx context.Context, commands *command.PresetDeckHandler, deck command.ChangedPresetDeck) error {
			_, err := commands.Rename(ctx, deck.ID, deck.Revision, "中級者デッキ")
			return err
		}},
		{name: "set_card_counts", call: func(t *testing.T, ctx context.Context, commands *command.PresetDeckHandler, deck command.ChangedPresetDeck) error {
			_, err := commands.SetCardCounts(ctx, deck.ID, deck.Revision, []command.CardCountInput{{CardID: issueCard(t, ctx, creature.NewID), Count: 1}})
			return err
		}},
		{name: "delete", call: func(_ *testing.T, ctx context.Context, commands *command.PresetDeckHandler, deck command.ChangedPresetDeck) error {
			return commands.Delete(ctx, deck.ID, deck.Revision)
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := isolatedDatabase(t)
			deck := registerPresetDeck(t, signedIn(t, ctx, registrant(t)), "初心者デッキ")

			err := tt.call(t, ctx, newPresetDeckHandler(t, ctx), deck)

			require.ErrorIs(t, err, command.ErrNotOperator)
		})
	}
}

func TestRenamePresetDeck_RenamesAndReturnsNewRevision(t *testing.T) {
	ctx := signedIn(t, isolatedDatabase(t), registrant(t))
	registered := registerPresetDeck(t, ctx, "初心者デッキ")

	renamed, err := newPresetDeckHandler(t, ctx).Rename(ctx, registered.ID, registered.Revision, " 中級者デッキ ")

	require.NoError(t, err)
	require.Equal(t, command.ChangedPresetDeck{ID: registered.ID, Revision: 2}, renamed)
	require.Equal(t, "中級者デッキ", restoredPresetDeck(t, ctx, registered.ID).Name.String())
}

// 同じ名前にし直しても revision を進めない（ほかの画面で見ている revision を古くしないため）
func TestRenamePresetDeck_KeepsRevisionWhenNameIsUnchanged(t *testing.T) {
	ctx := signedIn(t, isolatedDatabase(t), registrant(t))
	registered := registerPresetDeck(t, ctx, "初心者デッキ")

	renamed, err := newPresetDeckHandler(t, ctx).Rename(ctx, registered.ID, registered.Revision, "初心者デッキ")

	require.NoError(t, err)
	require.Equal(t, registered.Revision, renamed.Revision)
}

func TestRenamePresetDeck_RejectsNameAlreadyInUse(t *testing.T) {
	ctx := signedIn(t, isolatedDatabase(t), registrant(t))
	registerPresetDeck(t, ctx, "初心者デッキ")
	other := registerPresetDeck(t, ctx, "中級者デッキ")

	_, err := newPresetDeckHandler(t, ctx).Rename(ctx, other.ID, other.Revision, "初心者デッキ")

	require.ErrorIs(t, err, command.ErrPresetDeckNameAlreadyInUse)
}

func TestRenamePresetDeck_ReleasesPreviousName(t *testing.T) {
	ctx := signedIn(t, isolatedDatabase(t), registrant(t))
	registered := registerPresetDeck(t, ctx, "初心者デッキ")
	_, err := newPresetDeckHandler(t, ctx).Rename(ctx, registered.ID, registered.Revision, "中級者デッキ")
	require.NoError(t, err)

	_, err = newPresetDeckHandler(t, ctx).Register(ctx, "初心者デッキ")

	require.NoError(t, err)
}

func TestRenamePresetDeck_RejectsInvalidName(t *testing.T) {
	ctx := signedIn(t, isolatedDatabase(t), registrant(t))
	registered := registerPresetDeck(t, ctx, "初心者デッキ")

	_, err := newPresetDeckHandler(t, ctx).Rename(ctx, registered.ID, registered.Revision, " ")

	invalid, ok := input.AsErrors(err)
	require.True(t, ok)
	require.Equal(t, input.Errors{&input.Required{Field: "name"}}, invalid)
}

// 一覧の全部のカードを 1 回の操作で変え、revision は 1 つだけ進む
func TestSetPresetDeckCardCounts_ChangesCardsWithOneRevision(t *testing.T) {
	ctx := signedIn(t, isolatedDatabase(t), registrant(t))
	registered := registerPresetDeck(t, ctx, "初心者デッキ")
	creatureID := publishedCreature(t, ctx)
	itemID := publishedItem(t, ctx)

	changed, err := newPresetDeckHandler(t, ctx).SetCardCounts(ctx, registered.ID, registered.Revision, []command.CardCountInput{
		{CardID: creatureID, Count: 4},
		{CardID: itemID, Count: 3},
	})

	require.NoError(t, err)
	require.Equal(t, map[uuid.UUID]int{creatureID: 4, itemID: 3}, restoredCardCounts(t, ctx, registered.ID))
	require.Equal(t, int64(2), changed.Revision)
}

// 一覧に含めなかったカードは変えない。0 にしたカードはデッキから外れる
func TestSetPresetDeckCardCounts_ChangesOnlyGivenCards(t *testing.T) {
	ctx := signedIn(t, isolatedDatabase(t), registrant(t))
	registered := registerPresetDeck(t, ctx, "初心者デッキ")
	creatureID := publishedCreature(t, ctx)
	itemID := publishedItem(t, ctx)
	commands := newPresetDeckHandler(t, ctx)
	changed, err := commands.SetCardCounts(ctx, registered.ID, registered.Revision, []command.CardCountInput{
		{CardID: creatureID, Count: 2},
		{CardID: itemID, Count: 3},
	})
	require.NoError(t, err)

	changed, err = commands.SetCardCounts(ctx, changed.ID, changed.Revision, []command.CardCountInput{{CardID: creatureID, Count: 0}})

	require.NoError(t, err)
	require.Equal(t, map[uuid.UUID]int{itemID: 3}, restoredCardCounts(t, ctx, registered.ID))
}

// 空の一覧・今と同じ枚数は、変わらない操作として revision を進めない
func TestSetPresetDeckCardCounts_KeepsRevisionWhenNothingChanges(t *testing.T) {
	ctx := signedIn(t, isolatedDatabase(t), registrant(t))
	registered := registerPresetDeck(t, ctx, "初心者デッキ")
	cardID := publishedCreature(t, ctx)
	commands := newPresetDeckHandler(t, ctx)
	changed, err := commands.SetCardCounts(ctx, registered.ID, registered.Revision, []command.CardCountInput{{CardID: cardID, Count: 2}})
	require.NoError(t, err)
	tests := []struct {
		name  string
		cards []command.CardCountInput
	}{
		{name: "empty", cards: []command.CardCountInput{}},
		{name: "same_count", cards: []command.CardCountInput{{CardID: cardID, Count: 2}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			unchanged, err := commands.SetCardCounts(ctx, changed.ID, changed.Revision, tt.cards)

			require.NoError(t, err)
			require.Equal(t, changed, unchanged)
		})
	}
}

// 入れているカードを参照し（参照されている間はカードを消せない）、0 にしたカードは参照から外す
func TestSetPresetDeckCardCounts_ReferencesCardsInPresetDeck(t *testing.T) {
	ctx := signedIn(t, isolatedDatabase(t), registrant(t))
	registered := registerPresetDeck(t, ctx, "初心者デッキ")
	creatureID := publishedCreature(t, ctx)
	itemID := publishedItem(t, ctx)
	commands := newPresetDeckHandler(t, ctx)
	changed, err := commands.SetCardCounts(ctx, registered.ID, registered.Revision, []command.CardCountInput{
		{CardID: creatureID, Count: 2},
		{CardID: itemID, Count: 1},
	})
	require.NoError(t, err)

	_, err = commands.SetCardCounts(ctx, changed.ID, changed.Revision, []command.CardCountInput{{CardID: creatureID, Count: 0}})

	require.NoError(t, err)
	require.Equal(t, []int64{internalID(t, ctx, itemID)}, referencedCards(t, ctx, registered.ID))
}

func TestSetPresetDeckCardCounts_RejectsInvalidInput(t *testing.T) {
	tests := []struct {
		name  string
		cards func(t *testing.T, ctx context.Context, deck command.ChangedPresetDeck) []command.CardCountInput
		want  []input.Error
	}{
		{
			name: "unknown_card",
			cards: func(*testing.T, context.Context, command.ChangedPresetDeck) []command.CardCountInput {
				return []command.CardCountInput{{CardID: uuid.New(), Count: 1}}
			},
			want: []input.Error{&input.NotFound{Field: "cards.0.cardId"}},
		},
		{
			// 識別子はあるが、カードの種類でない（プリセットデッキ自身）
			name: "not_a_card",
			cards: func(_ *testing.T, _ context.Context, deck command.ChangedPresetDeck) []command.CardCountInput {
				return []command.CardCountInput{{CardID: deck.ID, Count: 1}}
			},
			want: []input.Error{&input.NotFound{Field: "cards.0.cardId"}},
		},
		{
			name: "count_above_max",
			cards: func(t *testing.T, ctx context.Context, _ command.ChangedPresetDeck) []command.CardCountInput {
				return []command.CardCountInput{{CardID: publishedCreature(t, ctx), Count: defaultRules.MaxCardDuplicates + 1}}
			},
			want: []input.Error{&input.OutOfRange{Field: "cards.0.count", Min: 0, Max: defaultRules.MaxCardDuplicates}},
		},
		{
			name: "negative_count",
			cards: func(t *testing.T, ctx context.Context, _ command.ChangedPresetDeck) []command.CardCountInput {
				return []command.CardCountInput{{CardID: publishedCreature(t, ctx), Count: -1}}
			},
			want: []input.Error{&input.OutOfRange{Field: "cards.0.count", Min: 0, Max: defaultRules.MaxCardDuplicates}},
		},
		{
			// 同じ枚数でも、2 回目を誤りにする
			name: "duplicated_card",
			cards: func(t *testing.T, ctx context.Context, _ command.ChangedPresetDeck) []command.CardCountInput {
				cardID := publishedCreature(t, ctx)
				return []command.CardCountInput{{CardID: cardID, Count: 1}, {CardID: publishedItem(t, ctx), Count: 1}, {CardID: cardID, Count: 1}}
			},
			want: []input.Error{&input.Duplicated{Field: "cards.2.cardId"}},
		},
		{
			name: "errors_in_several_cards",
			cards: func(t *testing.T, ctx context.Context, _ command.ChangedPresetDeck) []command.CardCountInput {
				return []command.CardCountInput{
					{CardID: uuid.New(), Count: defaultRules.MaxCardDuplicates + 1},
					{CardID: publishedCreature(t, ctx), Count: 1},
					{CardID: publishedItem(t, ctx), Count: -1},
				}
			},
			want: []input.Error{
				&input.NotFound{Field: "cards.0.cardId"},
				&input.OutOfRange{Field: "cards.0.count", Min: 0, Max: defaultRules.MaxCardDuplicates},
				&input.OutOfRange{Field: "cards.2.count", Min: 0, Max: defaultRules.MaxCardDuplicates},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := signedIn(t, isolatedDatabase(t), registrant(t))
			deck := registerPresetDeck(t, ctx, "初心者デッキ")

			_, err := newPresetDeckHandler(t, ctx).SetCardCounts(ctx, deck.ID, deck.Revision, tt.cards(t, ctx, deck))

			invalid, ok := input.AsErrors(err)
			require.True(t, ok)
			require.Equal(t, tt.want, []input.Error(invalid))
		})
	}
}

// 一覧が長すぎるときは、1 件ずつの誤りを返さない（存在しない ID を大量に送られても、その数だけ処理しないため）
func TestSetPresetDeckCardCounts_RejectsTooManyCards(t *testing.T) {
	ctx := signedIn(t, isolatedDatabase(t), registrant(t))
	deck := registerPresetDeck(t, ctx, "初心者デッキ")
	cards := make([]command.CardCountInput, 0, defaultRules.TotalCardCount+1)
	for range defaultRules.TotalCardCount + 1 {
		cards = append(cards, command.CardCountInput{CardID: uuid.New(), Count: 1})
	}

	_, err := newPresetDeckHandler(t, ctx).SetCardCounts(ctx, deck.ID, deck.Revision, cards)

	invalid, ok := input.AsErrors(err)
	require.True(t, ok)
	require.Equal(t, []input.Error{&input.OutOfRange{Field: "cards", Min: 0, Max: defaultRules.TotalCardCount}}, []input.Error(invalid))
}

// 合計は今のデッキと合わせて確かめ、超えれば何も変えない
func TestSetPresetDeckCardCounts_RejectsTotalCountOverMax(t *testing.T) {
	ctx := signedIn(t, isolatedDatabase(t), registrant(t))
	deck := registerPresetDeck(t, ctx, "初心者デッキ")
	full := make([]command.CardCountInput, 0, defaultRules.TotalCardCount/defaultRules.MaxCardDuplicates)
	for range defaultRules.TotalCardCount / defaultRules.MaxCardDuplicates {
		full = append(full, command.CardCountInput{CardID: publishedCreature(t, ctx), Count: defaultRules.MaxCardDuplicates})
	}
	commands := newPresetDeckHandler(t, ctx)
	filled, err := commands.SetCardCounts(ctx, deck.ID, deck.Revision, full)
	require.NoError(t, err)
	require.Equal(t, defaultRules.TotalCardCount, restoredPresetDeck(t, ctx, deck.ID).TotalCount())

	_, err = commands.SetCardCounts(ctx, filled.ID, filled.Revision, []command.CardCountInput{{CardID: publishedItem(t, ctx), Count: 1}})

	invalid, ok := input.AsErrors(err)
	require.True(t, ok)
	require.Equal(t, []input.Error{&input.TotalTooLarge{Field: "cards", Max: defaultRules.TotalCardCount}}, []input.Error(invalid))
	require.Equal(t, filled.Revision, restoredPresetDeck(t, ctx, deck.ID).Version)
}

// 上限はルールの値で決まる（既定の値でなくても、その値で入力の誤りと合計の超過を返す）
func TestSetPresetDeckCardCounts_UsesLimitsOfRules(t *testing.T) {
	ctx := signedIn(t, isolatedDatabase(t), registrant(t))
	rules := gamerules.Defaults()
	rules.Deck.TotalCardCount = 3
	rules.Deck.MaxCardDuplicates = 2
	commands := newPresetDeckHandlerWithRules(t, ctx, rules)
	deck := registerPresetDeck(t, ctx, "初心者デッキ")
	tests := []struct {
		name  string
		cards []command.CardCountInput
		want  []input.Error
	}{
		{
			name:  "count_above_rule",
			cards: []command.CardCountInput{{CardID: publishedCreature(t, ctx), Count: 3}},
			want:  []input.Error{&input.OutOfRange{Field: "cards.0.count", Min: 0, Max: 2}},
		},
		{
			name: "too_many_cards_for_rule",
			cards: []command.CardCountInput{
				{CardID: uuid.New(), Count: 1}, {CardID: uuid.New(), Count: 1}, {CardID: uuid.New(), Count: 1}, {CardID: uuid.New(), Count: 1},
			},
			want: []input.Error{&input.OutOfRange{Field: "cards", Min: 0, Max: 3}},
		},
		{
			name: "total_above_rule",
			cards: []command.CardCountInput{
				{CardID: publishedCreature(t, ctx), Count: 2}, {CardID: publishedCreature(t, ctx), Count: 2},
			},
			want: []input.Error{&input.TotalTooLarge{Field: "cards", Max: 3}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := commands.SetCardCounts(ctx, deck.ID, deck.Revision, tt.cards)

			invalid, ok := input.AsErrors(err)
			require.True(t, ok)
			require.Equal(t, tt.want, []input.Error(invalid))
		})
	}
}

// ルールがまだ作られていなければ、既定の値で確かめずにエラーにする
func TestSetPresetDeckCardCounts_FailsWhenRulesNotCreated(t *testing.T) {
	ctx := signedIn(t, isolatedDatabase(t), registrant(t))
	deck := registerPresetDeck(t, ctx, "初心者デッキ")
	registry, err := domaindef.Load(ctx)
	require.NoError(t, err)
	handler := command.NewPresetDeckHandler(
		presetDeckStore(registry),
		identifier.New(registry, presetdeck.NewID),
		identifier.NewEntityRefResolver(registry),
		constraint.UpdatePresetDeckName,
		constraint.UpdatePresetDeckCardReferences,
		newCardStatusReader(registry).Statuses,
		newGameRulesHandler(t, ctx).Get,
		func() time.Time { return time.Now().UTC() },
	)

	_, err = handler.SetCardCounts(ctx, deck.ID, deck.Revision, nil)

	require.ErrorIs(t, err, command.ErrGameRulesNotCreated)
}

// 1 つでも誤りがあれば、ほかの正しいカードも変えない
func TestSetPresetDeckCardCounts_ChangesNothingWhenAnyCardIsInvalid(t *testing.T) {
	ctx := signedIn(t, isolatedDatabase(t), registrant(t))
	registered := registerPresetDeck(t, ctx, "初心者デッキ")

	_, err := newPresetDeckHandler(t, ctx).SetCardCounts(ctx, registered.ID, registered.Revision, []command.CardCountInput{
		{CardID: publishedCreature(t, ctx), Count: 2},
		{CardID: uuid.New(), Count: 1},
	})

	require.Error(t, err)
	deck := restoredPresetDeck(t, ctx, registered.ID)
	require.Empty(t, deck.Cards)
	require.Equal(t, registered.Revision, deck.Version)
}

func TestDeletePresetDeck_MakesPresetDeckNotFound(t *testing.T) {
	ctx := signedIn(t, isolatedDatabase(t), registrant(t))
	registered := registerPresetDeck(t, ctx, "初心者デッキ")
	commands := newPresetDeckHandler(t, ctx)

	require.NoError(t, commands.Delete(ctx, registered.ID, registered.Revision))

	_, err := commands.Rename(ctx, registered.ID, registered.Revision+1, "中級者デッキ")
	require.ErrorIs(t, err, command.ErrPresetDeckNotFound)
}

func TestDeletePresetDeck_ReleasesName(t *testing.T) {
	ctx := signedIn(t, isolatedDatabase(t), registrant(t))
	registered := registerPresetDeck(t, ctx, "初心者デッキ")
	commands := newPresetDeckHandler(t, ctx)
	require.NoError(t, commands.Delete(ctx, registered.ID, registered.Revision))

	_, err := commands.Register(ctx, "初心者デッキ")

	require.NoError(t, err)
}

func TestDeletePresetDeck_RecordsOperatorInEvent(t *testing.T) {
	by := operatortest.ID(t, 42)
	ctx := signedIn(t, isolatedDatabase(t), by)
	registered := registerPresetDeck(t, ctx, "初心者デッキ")
	registry, err := domaindef.Load(ctx)
	require.NoError(t, err)
	id, err := identifier.New(registry, presetdeck.NewID).Resolve(ctx, registered.ID)
	require.NoError(t, err)

	require.NoError(t, newPresetDeckHandler(t, ctx).Delete(ctx, registered.ID, registered.Revision))

	// 識別子を消してもイベントは残る
	restored, err := presetDeckStore(registry).Load(ctx, id)
	require.NoError(t, err)
	require.Equal(t, presetdeck.DeletedPresetDeck{ID: id, Version: 2}, restored)
}

func TestPresetDeckCommands_RejectUnknownPresetDeck(t *testing.T) {
	tests := []struct {
		name string
		call func(t *testing.T, ctx context.Context, commands *command.PresetDeckHandler, id uuid.UUID) error
	}{
		{name: "rename", call: func(_ *testing.T, ctx context.Context, commands *command.PresetDeckHandler, id uuid.UUID) error {
			_, err := commands.Rename(ctx, id, 1, "中級者デッキ")
			return err
		}},
		{name: "set_card_counts", call: func(t *testing.T, ctx context.Context, commands *command.PresetDeckHandler, id uuid.UUID) error {
			_, err := commands.SetCardCounts(ctx, id, 1, []command.CardCountInput{{CardID: publishedCreature(t, ctx), Count: 1}})
			return err
		}},
		{name: "delete", call: func(_ *testing.T, ctx context.Context, commands *command.PresetDeckHandler, id uuid.UUID) error {
			return commands.Delete(ctx, id, 1)
		}},
		{
			// 識別子はあるが、プリセットデッキの種類でない
			name: "card_id",
			call: func(t *testing.T, ctx context.Context, commands *command.PresetDeckHandler, _ uuid.UUID) error {
				_, err := commands.Rename(ctx, publishedCreature(t, ctx), 1, "中級者デッキ")
				return err
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := signedIn(t, isolatedDatabase(t), registrant(t))

			err := tt.call(t, ctx, newPresetDeckHandler(t, ctx), uuid.New())

			require.ErrorIs(t, err, command.ErrPresetDeckNotFound)
		})
	}
}

// 画面を開いた後にほかの画面で変わっていたら、変わらない操作（同じ名前・同じ枚数）でも「先に変えられた」にする
func TestPresetDeckCommands_RejectStaleRevision(t *testing.T) {
	tests := []struct {
		name string
		call func(ctx context.Context, commands *command.PresetDeckHandler, stale command.ChangedPresetDeck, cardID uuid.UUID) error
	}{
		{name: "rename", call: func(ctx context.Context, commands *command.PresetDeckHandler, stale command.ChangedPresetDeck, _ uuid.UUID) error {
			_, err := commands.Rename(ctx, stale.ID, stale.Revision, "上級者デッキ")
			return err
		}},
		{name: "rename_to_same_name", call: func(ctx context.Context, commands *command.PresetDeckHandler, stale command.ChangedPresetDeck, _ uuid.UUID) error {
			_, err := commands.Rename(ctx, stale.ID, stale.Revision, "中級者デッキ")
			return err
		}},
		{name: "set_card_counts", call: func(ctx context.Context, commands *command.PresetDeckHandler, stale command.ChangedPresetDeck, cardID uuid.UUID) error {
			_, err := commands.SetCardCounts(ctx, stale.ID, stale.Revision, []command.CardCountInput{{CardID: cardID, Count: 2}})
			return err
		}},
		{name: "set_card_counts_to_same_count", call: func(ctx context.Context, commands *command.PresetDeckHandler, stale command.ChangedPresetDeck, cardID uuid.UUID) error {
			_, err := commands.SetCardCounts(ctx, stale.ID, stale.Revision, []command.CardCountInput{{CardID: cardID, Count: 0}})
			return err
		}},
		{name: "set_card_counts_with_no_cards", call: func(ctx context.Context, commands *command.PresetDeckHandler, stale command.ChangedPresetDeck, _ uuid.UUID) error {
			_, err := commands.SetCardCounts(ctx, stale.ID, stale.Revision, []command.CardCountInput{})
			return err
		}},
		{name: "delete", call: func(ctx context.Context, commands *command.PresetDeckHandler, stale command.ChangedPresetDeck, _ uuid.UUID) error {
			return commands.Delete(ctx, stale.ID, stale.Revision)
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := signedIn(t, isolatedDatabase(t), registrant(t))
			stale := registerPresetDeck(t, ctx, "初心者デッキ")
			commands := newPresetDeckHandler(t, ctx)
			_, err := commands.Rename(ctx, stale.ID, stale.Revision, "中級者デッキ")
			require.NoError(t, err)

			err = tt.call(ctx, commands, stale, publishedCreature(t, ctx))

			require.ErrorIs(t, err, command.ErrRevisionConflict)
		})
	}
}

// 同じ revision から同時に変えても、1 つだけ通り、ほかは「先に変えられた」になる
// （比べた後から保存までに先を越された場合も、eventstore の版の衝突として同じエラーになる）
func TestRenamePresetDeck_AcceptsOnlyOneOfConcurrentChanges(t *testing.T) {
	ctx := signedIn(t, isolatedDatabase(t), registrant(t))
	registered := registerPresetDeck(t, ctx, "初心者デッキ")
	commands := newPresetDeckHandler(t, ctx)

	const callers = 20
	errs := make([]error, callers)
	var wg sync.WaitGroup
	for i := range callers {
		wg.Go(func() {
			_, errs[i] = commands.Rename(ctx, registered.ID, registered.Revision, "デッキ-"+uuid.NewString())
		})
	}
	wg.Wait()

	succeeded := 0
	for _, err := range errs {
		if err == nil {
			succeeded++
			continue
		}
		require.ErrorIs(t, err, command.ErrRevisionConflict)
	}
	require.Equal(t, 1, succeeded)
}

// 新しく入れるカードは公開中だけ。準備中・使用不可のカードは入力の誤り（何も変えない）
func TestSetPresetDeckCardCounts_RejectsAddingUnavailableCards(t *testing.T) {
	tests := []struct {
		name string
		card func(t *testing.T, ctx context.Context) uuid.UUID
	}{
		{name: "draft", card: func(t *testing.T, ctx context.Context) uuid.UUID {
			return cardKinds()[0].register(t, ctx).ID
		}},
		{name: "retired", card: func(t *testing.T, ctx context.Context) uuid.UUID {
			retired := retireCard(t, ctx, publishCard(t, ctx, cardKinds()[1].register(t, ctx)))
			return retired.ID
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := signedIn(t, isolatedDatabase(t), registrant(t))
			deck := registerPresetDeck(t, ctx, "初心者デッキ")
			unavailable := tt.card(t, ctx)

			_, err := newPresetDeckHandler(t, ctx).SetCardCounts(ctx, deck.ID, deck.Revision, []command.CardCountInput{
				{CardID: publishedCreature(t, ctx), Count: 1},
				{CardID: unavailable, Count: 1},
			})

			invalid, ok := input.AsErrors(err)
			require.True(t, ok)
			require.Equal(t, []input.Error{&input.Unavailable{Field: "cards.1.cardId"}}, []input.Error(invalid))
			unchanged := restoredPresetDeck(t, ctx, deck.ID)
			require.Empty(t, unchanged.Cards)
			require.Equal(t, deck.Revision, unchanged.Version)
		})
	}
}

// すでに入っているカードは、使用不可になっても残せる（枚数を減らす・外す・変えないはできる）。増やすのは新しく入れるのと同じで誤り
func TestSetPresetDeckCardCounts_KeepsCardRetiredAfterBeingAdded(t *testing.T) {
	ctx := signedIn(t, isolatedDatabase(t), registrant(t))
	deck := registerPresetDeck(t, ctx, "初心者デッキ")
	commands := newPresetDeckHandler(t, ctx)
	registered, err := newCreatureHandler(t, ctx).Register(ctx, "クリーチャー-"+uuid.NewString())
	require.NoError(t, err)
	published := publishCard(t, ctx, command.ChangedCard{ID: registered.ID, Revision: registered.Revision})
	added, err := commands.SetCardCounts(ctx, deck.ID, deck.Revision, []command.CardCountInput{{CardID: published.ID, Count: 3}})
	require.NoError(t, err)
	retireCard(t, ctx, published)

	unchanged, err := commands.SetCardCounts(ctx, added.ID, added.Revision, []command.CardCountInput{{CardID: published.ID, Count: 3}})
	require.NoError(t, err)
	require.Equal(t, added, unchanged)
	_, err = commands.SetCardCounts(ctx, added.ID, added.Revision, []command.CardCountInput{{CardID: published.ID, Count: 4}})
	invalid, ok := input.AsErrors(err)
	require.True(t, ok)
	require.Equal(t, []input.Error{&input.Unavailable{Field: "cards.0.cardId"}}, []input.Error(invalid))
	decreased, err := commands.SetCardCounts(ctx, added.ID, added.Revision, []command.CardCountInput{{CardID: published.ID, Count: 1}})
	require.NoError(t, err)
	removed, err := commands.SetCardCounts(ctx, decreased.ID, decreased.Revision, []command.CardCountInput{{CardID: published.ID, Count: 0}})

	require.NoError(t, err)
	require.Equal(t, decreased.Revision+1, removed.Revision)
	require.Empty(t, restoredCardCounts(t, ctx, deck.ID))
}

// 入力の誤りは、入力の形の誤りと同じように、使えないカードもまとめて返す
func TestSetPresetDeckCardCounts_ReportsEveryUnavailableCard(t *testing.T) {
	ctx := signedIn(t, isolatedDatabase(t), registrant(t))
	deck := registerPresetDeck(t, ctx, "初心者デッキ")
	draftA := cardKinds()[0].register(t, ctx).ID
	draftB := cardKinds()[2].register(t, ctx).ID

	_, err := newPresetDeckHandler(t, ctx).SetCardCounts(ctx, deck.ID, deck.Revision, []command.CardCountInput{
		{CardID: draftA, Count: 1},
		{CardID: publishedItem(t, ctx), Count: 1},
		{CardID: draftB, Count: 2},
	})

	invalid, ok := input.AsErrors(err)
	require.True(t, ok)
	require.Equal(t, []input.Error{
		&input.Unavailable{Field: "cards.0.cardId"},
		&input.Unavailable{Field: "cards.2.cardId"},
	}, []input.Error(invalid))
}

func publishCard(t *testing.T, ctx context.Context, registered command.ChangedCard) command.ChangedCard {
	t.Helper()
	published, err := newCardHandler(t, ctx).Publish(ctx, registered.ID, registered.Revision)
	require.NoError(t, err)
	return published
}

func retireCard(t *testing.T, ctx context.Context, published command.ChangedCard) command.ChangedCard {
	t.Helper()
	retired, err := newCardHandler(t, ctx).Retire(ctx, published.ID, published.Revision)
	require.NoError(t, err)
	return retired
}
