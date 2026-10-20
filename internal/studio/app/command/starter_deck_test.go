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
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/item"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/operator/operatortest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/starterdeck"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domaindef"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/eventstore"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/identifier"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/studio/app/command"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/studio/app/input"
)

func newStarterDeckCommands(t *testing.T, ctx context.Context) *command.StarterDeckCommands {
	t.Helper()
	registry, err := domaindef.Load(ctx)
	require.NoError(t, err)
	return command.NewStarterDeckCommands(
		starterDeckStore(registry),
		identifier.New(registry, starterdeck.NewID),
		identifier.NewEntityRefResolver(registry),
		constraint.UpdateStarterDeckName,
		constraint.UpdateStarterDeckCardReferences,
		// wiring と同じく、DB に残る精度（マイクロ秒）にそろえる（保存の後に読み直した日時と、結果の日時を同じにするため）
		func() time.Time { return time.Now().UTC().Truncate(time.Microsecond) },
	)
}

func starterDeckStore(registry *domaindef.Registry) domain.EventStore[starterdeck.ID, starterdeck.Event, starterdeck.State] {
	return eventstore.New(registry, starterdeck.NewID, starterdeck.NewEventType, starterdeck.InitialState, starterdeck.Evolve)
}

// カードがあるとは、識別子が発行されていること（カードの集約の中身は、スターターデッキのコマンドは見ない）
func issueCard[I domain.ID](t *testing.T, ctx context.Context, newID domain.NewIDFunc[I]) uuid.UUID {
	t.Helper()
	registry, err := domaindef.Load(ctx)
	require.NoError(t, err)
	_, publicID, err := identifier.New(registry, newID).Issue(ctx)
	require.NoError(t, err)
	return publicID
}

// app 層は uif に内部 ID を返さないため、識別子から引く
func internalID(t *testing.T, ctx context.Context, publicID uuid.UUID) int64 {
	t.Helper()
	registry, err := domaindef.Load(ctx)
	require.NoError(t, err)
	ref, err := identifier.NewEntityRefResolver(registry).Resolve(ctx, publicID)
	require.NoError(t, err)
	return ref.ID()
}

func referencedCards(t *testing.T, ctx context.Context, starterDeckID uuid.UUID) []int64 {
	t.Helper()
	return constrainttest.Referenced(t, ctx, starterdeck.CardReferenceType(), internalID(t, ctx, starterDeckID))
}

// コマンドの結果は公開 ID と revision だけなので、変えた中身はイベントストアから集約を読み直して確かめる
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

// 入れているカードを、公開 ID と枚数で返す
func restoredCardCounts(t *testing.T, ctx context.Context, publicID uuid.UUID) map[uuid.UUID]int {
	t.Helper()
	registry, err := domaindef.Load(ctx)
	require.NoError(t, err)
	deck := restoredStarterDeck(t, ctx, publicID)
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

func registerStarterDeck(t *testing.T, ctx context.Context, name string) command.ChangedStarterDeck {
	t.Helper()
	registered, err := newStarterDeckCommands(t, ctx).Register(ctx, name)
	require.NoError(t, err)
	return registered
}

func TestRegisterStarterDeck_ReturnsPublicIDAndFirstRevision(t *testing.T) {
	ctx := signedIn(t, isolatedDatabase(t), registrant(t))
	before := time.Now().UTC().Truncate(time.Microsecond)

	registered, err := newStarterDeckCommands(t, ctx).Register(ctx, "  初心者デッキ ")

	require.NoError(t, err)
	require.NotEqual(t, uuid.Nil, registered.ID)
	require.Equal(t, int64(1), registered.Revision)
	deck := restoredStarterDeck(t, ctx, registered.ID)
	require.Equal(t, "初心者デッキ", deck.Name.String())
	require.Empty(t, deck.Cards)
	require.WithinRange(t, deck.RegisteredAt, before, time.Now().UTC())
	require.Equal(t, deck.RegisteredAt, deck.UpdatedAt)
}

func TestRegisterStarterDeck_RejectsNameAlreadyInUse(t *testing.T) {
	ctx := signedIn(t, isolatedDatabase(t), registrant(t))
	registerStarterDeck(t, ctx, "ＡＢＣ")

	_, err := newStarterDeckCommands(t, ctx).Register(ctx, " abc")

	require.ErrorIs(t, err, command.ErrStarterDeckNameAlreadyInUse)
}

func TestRegisterStarterDeck_RejectsInvalidName(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  input.Error
	}{
		{name: "blank", value: " ", want: &input.Required{Field: "name"}},
		{name: "too_long", value: strings.Repeat("あ", starterdeck.MaxNameLength+1), want: &input.TooLong{Field: "name", Max: starterdeck.MaxNameLength}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := signedIn(t, isolatedDatabase(t), registrant(t))

			_, err := newStarterDeckCommands(t, ctx).Register(ctx, tt.value)

			invalid, ok := input.AsErrors(err)
			require.True(t, ok)
			require.Equal(t, input.Errors{tt.want}, invalid)
		})
	}
}

func TestStarterDeckCommands_RejectWithoutOperator(t *testing.T) {
	tests := []struct {
		name string
		call func(t *testing.T, ctx context.Context, commands *command.StarterDeckCommands, deck command.ChangedStarterDeck) error
	}{
		{name: "register", call: func(_ *testing.T, ctx context.Context, commands *command.StarterDeckCommands, _ command.ChangedStarterDeck) error {
			_, err := commands.Register(ctx, "中級者デッキ")
			return err
		}},
		{name: "rename", call: func(_ *testing.T, ctx context.Context, commands *command.StarterDeckCommands, deck command.ChangedStarterDeck) error {
			_, err := commands.Rename(ctx, deck.ID, deck.Revision, "中級者デッキ")
			return err
		}},
		{name: "set_card_counts", call: func(t *testing.T, ctx context.Context, commands *command.StarterDeckCommands, deck command.ChangedStarterDeck) error {
			_, err := commands.SetCardCounts(ctx, deck.ID, deck.Revision, []command.CardCountInput{{CardID: issueCard(t, ctx, creature.NewID), Count: 1}})
			return err
		}},
		{name: "delete", call: func(_ *testing.T, ctx context.Context, commands *command.StarterDeckCommands, deck command.ChangedStarterDeck) error {
			return commands.Delete(ctx, deck.ID, deck.Revision)
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := isolatedDatabase(t)
			deck := registerStarterDeck(t, signedIn(t, ctx, registrant(t)), "初心者デッキ")

			err := tt.call(t, ctx, newStarterDeckCommands(t, ctx), deck)

			require.ErrorIs(t, err, command.ErrNotOperator)
		})
	}
}

func TestRenameStarterDeck_RenamesAndReturnsNewRevision(t *testing.T) {
	ctx := signedIn(t, isolatedDatabase(t), registrant(t))
	registered := registerStarterDeck(t, ctx, "初心者デッキ")

	renamed, err := newStarterDeckCommands(t, ctx).Rename(ctx, registered.ID, registered.Revision, " 中級者デッキ ")

	require.NoError(t, err)
	require.Equal(t, command.ChangedStarterDeck{ID: registered.ID, Revision: 2}, renamed)
	require.Equal(t, "中級者デッキ", restoredStarterDeck(t, ctx, registered.ID).Name.String())
}

// 同じ名前にし直しても revision を進めない（ほかの画面で見ている revision を古くしないため）
func TestRenameStarterDeck_KeepsRevisionWhenNameIsUnchanged(t *testing.T) {
	ctx := signedIn(t, isolatedDatabase(t), registrant(t))
	registered := registerStarterDeck(t, ctx, "初心者デッキ")

	renamed, err := newStarterDeckCommands(t, ctx).Rename(ctx, registered.ID, registered.Revision, "初心者デッキ")

	require.NoError(t, err)
	require.Equal(t, registered.Revision, renamed.Revision)
}

func TestRenameStarterDeck_RejectsNameAlreadyInUse(t *testing.T) {
	ctx := signedIn(t, isolatedDatabase(t), registrant(t))
	registerStarterDeck(t, ctx, "初心者デッキ")
	other := registerStarterDeck(t, ctx, "中級者デッキ")

	_, err := newStarterDeckCommands(t, ctx).Rename(ctx, other.ID, other.Revision, "初心者デッキ")

	require.ErrorIs(t, err, command.ErrStarterDeckNameAlreadyInUse)
}

func TestRenameStarterDeck_ReleasesPreviousName(t *testing.T) {
	ctx := signedIn(t, isolatedDatabase(t), registrant(t))
	registered := registerStarterDeck(t, ctx, "初心者デッキ")
	_, err := newStarterDeckCommands(t, ctx).Rename(ctx, registered.ID, registered.Revision, "中級者デッキ")
	require.NoError(t, err)

	_, err = newStarterDeckCommands(t, ctx).Register(ctx, "初心者デッキ")

	require.NoError(t, err)
}

func TestRenameStarterDeck_RejectsInvalidName(t *testing.T) {
	ctx := signedIn(t, isolatedDatabase(t), registrant(t))
	registered := registerStarterDeck(t, ctx, "初心者デッキ")

	_, err := newStarterDeckCommands(t, ctx).Rename(ctx, registered.ID, registered.Revision, " ")

	invalid, ok := input.AsErrors(err)
	require.True(t, ok)
	require.Equal(t, input.Errors{&input.Required{Field: "name"}}, invalid)
}

// 一覧の全部のカードを 1 回の操作で変え、revision は 1 つだけ進む
func TestSetStarterDeckCardCounts_ChangesCardsWithOneRevision(t *testing.T) {
	ctx := signedIn(t, isolatedDatabase(t), registrant(t))
	registered := registerStarterDeck(t, ctx, "初心者デッキ")
	creatureID := issueCard(t, ctx, creature.NewID)
	itemID := issueCard(t, ctx, item.NewID)

	changed, err := newStarterDeckCommands(t, ctx).SetCardCounts(ctx, registered.ID, registered.Revision, []command.CardCountInput{
		{CardID: creatureID, Count: 4},
		{CardID: itemID, Count: 3},
	})

	require.NoError(t, err)
	require.Equal(t, map[uuid.UUID]int{creatureID: 4, itemID: 3}, restoredCardCounts(t, ctx, registered.ID))
	require.Equal(t, int64(2), changed.Revision)
}

// 一覧に含めなかったカードは変えない。0 にしたカードはデッキから外れる
func TestSetStarterDeckCardCounts_ChangesOnlyGivenCards(t *testing.T) {
	ctx := signedIn(t, isolatedDatabase(t), registrant(t))
	registered := registerStarterDeck(t, ctx, "初心者デッキ")
	creatureID := issueCard(t, ctx, creature.NewID)
	itemID := issueCard(t, ctx, item.NewID)
	commands := newStarterDeckCommands(t, ctx)
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
func TestSetStarterDeckCardCounts_KeepsRevisionWhenNothingChanges(t *testing.T) {
	ctx := signedIn(t, isolatedDatabase(t), registrant(t))
	registered := registerStarterDeck(t, ctx, "初心者デッキ")
	cardID := issueCard(t, ctx, creature.NewID)
	commands := newStarterDeckCommands(t, ctx)
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
func TestSetStarterDeckCardCounts_ReferencesCardsInStarterDeck(t *testing.T) {
	ctx := signedIn(t, isolatedDatabase(t), registrant(t))
	registered := registerStarterDeck(t, ctx, "初心者デッキ")
	creatureID := issueCard(t, ctx, creature.NewID)
	itemID := issueCard(t, ctx, item.NewID)
	commands := newStarterDeckCommands(t, ctx)
	changed, err := commands.SetCardCounts(ctx, registered.ID, registered.Revision, []command.CardCountInput{
		{CardID: creatureID, Count: 2},
		{CardID: itemID, Count: 1},
	})
	require.NoError(t, err)

	_, err = commands.SetCardCounts(ctx, changed.ID, changed.Revision, []command.CardCountInput{{CardID: creatureID, Count: 0}})

	require.NoError(t, err)
	require.Equal(t, []int64{internalID(t, ctx, itemID)}, referencedCards(t, ctx, registered.ID))
}

func TestSetStarterDeckCardCounts_RejectsInvalidInput(t *testing.T) {
	tests := []struct {
		name  string
		cards func(t *testing.T, ctx context.Context, deck command.ChangedStarterDeck) []command.CardCountInput
		want  []input.Error
	}{
		{
			name: "unknown_card",
			cards: func(*testing.T, context.Context, command.ChangedStarterDeck) []command.CardCountInput {
				return []command.CardCountInput{{CardID: uuid.New(), Count: 1}}
			},
			want: []input.Error{&input.NotFound{Field: "cards.0.cardId"}},
		},
		{
			// 識別子はあるが、カードの種類でない（スターターデッキ自身）
			name: "not_a_card",
			cards: func(_ *testing.T, _ context.Context, deck command.ChangedStarterDeck) []command.CardCountInput {
				return []command.CardCountInput{{CardID: deck.ID, Count: 1}}
			},
			want: []input.Error{&input.NotFound{Field: "cards.0.cardId"}},
		},
		{
			name: "count_above_max",
			cards: func(t *testing.T, ctx context.Context, _ command.ChangedStarterDeck) []command.CardCountInput {
				return []command.CardCountInput{{CardID: issueCard(t, ctx, creature.NewID), Count: starterdeck.MaxCardCount + 1}}
			},
			want: []input.Error{&input.OutOfRange{Field: "cards.0.count", Min: 0, Max: starterdeck.MaxCardCount}},
		},
		{
			name: "negative_count",
			cards: func(t *testing.T, ctx context.Context, _ command.ChangedStarterDeck) []command.CardCountInput {
				return []command.CardCountInput{{CardID: issueCard(t, ctx, creature.NewID), Count: -1}}
			},
			want: []input.Error{&input.OutOfRange{Field: "cards.0.count", Min: 0, Max: starterdeck.MaxCardCount}},
		},
		{
			// 同じ枚数でも、2 回目を誤りにする
			name: "duplicated_card",
			cards: func(t *testing.T, ctx context.Context, _ command.ChangedStarterDeck) []command.CardCountInput {
				cardID := issueCard(t, ctx, creature.NewID)
				return []command.CardCountInput{{CardID: cardID, Count: 1}, {CardID: issueCard(t, ctx, item.NewID), Count: 1}, {CardID: cardID, Count: 1}}
			},
			want: []input.Error{&input.Duplicated{Field: "cards.2.cardId"}},
		},
		{
			name: "errors_in_several_cards",
			cards: func(t *testing.T, ctx context.Context, _ command.ChangedStarterDeck) []command.CardCountInput {
				return []command.CardCountInput{
					{CardID: uuid.New(), Count: starterdeck.MaxCardCount + 1},
					{CardID: issueCard(t, ctx, creature.NewID), Count: 1},
					{CardID: issueCard(t, ctx, item.NewID), Count: -1},
				}
			},
			want: []input.Error{
				&input.NotFound{Field: "cards.0.cardId"},
				&input.OutOfRange{Field: "cards.0.count", Min: 0, Max: starterdeck.MaxCardCount},
				&input.OutOfRange{Field: "cards.2.count", Min: 0, Max: starterdeck.MaxCardCount},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := signedIn(t, isolatedDatabase(t), registrant(t))
			deck := registerStarterDeck(t, ctx, "初心者デッキ")

			_, err := newStarterDeckCommands(t, ctx).SetCardCounts(ctx, deck.ID, deck.Revision, tt.cards(t, ctx, deck))

			invalid, ok := input.AsErrors(err)
			require.True(t, ok)
			require.Equal(t, tt.want, []input.Error(invalid))
		})
	}
}

// 一覧が長すぎるときは、1 件ずつの誤りを返さない（存在しない ID を大量に送られても、その数だけ処理しないため）
func TestSetStarterDeckCardCounts_RejectsTooManyCards(t *testing.T) {
	ctx := signedIn(t, isolatedDatabase(t), registrant(t))
	deck := registerStarterDeck(t, ctx, "初心者デッキ")
	cards := make([]command.CardCountInput, 0, starterdeck.MaxTotalCount+1)
	for range starterdeck.MaxTotalCount + 1 {
		cards = append(cards, command.CardCountInput{CardID: uuid.New(), Count: 1})
	}

	_, err := newStarterDeckCommands(t, ctx).SetCardCounts(ctx, deck.ID, deck.Revision, cards)

	invalid, ok := input.AsErrors(err)
	require.True(t, ok)
	require.Equal(t, []input.Error{&input.OutOfRange{Field: "cards", Min: 0, Max: starterdeck.MaxTotalCount}}, []input.Error(invalid))
}

// 合計は今のデッキと合わせて確かめ、超えれば何も変えない
func TestSetStarterDeckCardCounts_RejectsTotalCountOverMax(t *testing.T) {
	ctx := signedIn(t, isolatedDatabase(t), registrant(t))
	deck := registerStarterDeck(t, ctx, "初心者デッキ")
	full := make([]command.CardCountInput, 0, starterdeck.MaxTotalCount/starterdeck.MaxCardCount)
	for range starterdeck.MaxTotalCount / starterdeck.MaxCardCount {
		full = append(full, command.CardCountInput{CardID: issueCard(t, ctx, creature.NewID), Count: starterdeck.MaxCardCount})
	}
	commands := newStarterDeckCommands(t, ctx)
	filled, err := commands.SetCardCounts(ctx, deck.ID, deck.Revision, full)
	require.NoError(t, err)
	require.Equal(t, starterdeck.MaxTotalCount, restoredStarterDeck(t, ctx, deck.ID).TotalCount())

	_, err = commands.SetCardCounts(ctx, filled.ID, filled.Revision, []command.CardCountInput{{CardID: issueCard(t, ctx, item.NewID), Count: 1}})

	invalid, ok := input.AsErrors(err)
	require.True(t, ok)
	require.Equal(t, []input.Error{&input.TotalTooLarge{Field: "cards", Max: starterdeck.MaxTotalCount}}, []input.Error(invalid))
	require.Equal(t, filled.Revision, restoredStarterDeck(t, ctx, deck.ID).Version)
}

// 1 つでも誤りがあれば、ほかの正しいカードも変えない
func TestSetStarterDeckCardCounts_ChangesNothingWhenAnyCardIsInvalid(t *testing.T) {
	ctx := signedIn(t, isolatedDatabase(t), registrant(t))
	registered := registerStarterDeck(t, ctx, "初心者デッキ")

	_, err := newStarterDeckCommands(t, ctx).SetCardCounts(ctx, registered.ID, registered.Revision, []command.CardCountInput{
		{CardID: issueCard(t, ctx, creature.NewID), Count: 2},
		{CardID: uuid.New(), Count: 1},
	})

	require.Error(t, err)
	deck := restoredStarterDeck(t, ctx, registered.ID)
	require.Empty(t, deck.Cards)
	require.Equal(t, registered.Revision, deck.Version)
}

func TestDeleteStarterDeck_MakesStarterDeckNotFound(t *testing.T) {
	ctx := signedIn(t, isolatedDatabase(t), registrant(t))
	registered := registerStarterDeck(t, ctx, "初心者デッキ")
	commands := newStarterDeckCommands(t, ctx)

	require.NoError(t, commands.Delete(ctx, registered.ID, registered.Revision))

	_, err := commands.Rename(ctx, registered.ID, registered.Revision+1, "中級者デッキ")
	require.ErrorIs(t, err, command.ErrStarterDeckNotFound)
}

func TestDeleteStarterDeck_ReleasesName(t *testing.T) {
	ctx := signedIn(t, isolatedDatabase(t), registrant(t))
	registered := registerStarterDeck(t, ctx, "初心者デッキ")
	commands := newStarterDeckCommands(t, ctx)
	require.NoError(t, commands.Delete(ctx, registered.ID, registered.Revision))

	_, err := commands.Register(ctx, "初心者デッキ")

	require.NoError(t, err)
}

func TestDeleteStarterDeck_RecordsOperatorInEvent(t *testing.T) {
	by := operatortest.ID(t, 42)
	ctx := signedIn(t, isolatedDatabase(t), by)
	registered := registerStarterDeck(t, ctx, "初心者デッキ")
	registry, err := domaindef.Load(ctx)
	require.NoError(t, err)
	id, err := identifier.New(registry, starterdeck.NewID).Resolve(ctx, registered.ID)
	require.NoError(t, err)

	require.NoError(t, newStarterDeckCommands(t, ctx).Delete(ctx, registered.ID, registered.Revision))

	// 識別子を消してもイベントは残る
	restored, err := starterDeckStore(registry).Load(ctx, id)
	require.NoError(t, err)
	require.Equal(t, starterdeck.DeletedStarterDeck{ID: id, Version: 2}, restored)
}

func TestStarterDeckCommands_RejectUnknownStarterDeck(t *testing.T) {
	tests := []struct {
		name string
		call func(t *testing.T, ctx context.Context, commands *command.StarterDeckCommands, id uuid.UUID) error
	}{
		{name: "rename", call: func(_ *testing.T, ctx context.Context, commands *command.StarterDeckCommands, id uuid.UUID) error {
			_, err := commands.Rename(ctx, id, 1, "中級者デッキ")
			return err
		}},
		{name: "set_card_counts", call: func(t *testing.T, ctx context.Context, commands *command.StarterDeckCommands, id uuid.UUID) error {
			_, err := commands.SetCardCounts(ctx, id, 1, []command.CardCountInput{{CardID: issueCard(t, ctx, creature.NewID), Count: 1}})
			return err
		}},
		{name: "delete", call: func(_ *testing.T, ctx context.Context, commands *command.StarterDeckCommands, id uuid.UUID) error {
			return commands.Delete(ctx, id, 1)
		}},
		{
			// 識別子はあるが、スターターデッキの種類でない
			name: "card_id",
			call: func(t *testing.T, ctx context.Context, commands *command.StarterDeckCommands, _ uuid.UUID) error {
				_, err := commands.Rename(ctx, issueCard(t, ctx, creature.NewID), 1, "中級者デッキ")
				return err
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := signedIn(t, isolatedDatabase(t), registrant(t))

			err := tt.call(t, ctx, newStarterDeckCommands(t, ctx), uuid.New())

			require.ErrorIs(t, err, command.ErrStarterDeckNotFound)
		})
	}
}

// 画面を開いた後にほかの画面で変わっていたら、変わらない操作（同じ名前・同じ枚数）でも「先に変えられた」にする
func TestStarterDeckCommands_RejectStaleRevision(t *testing.T) {
	tests := []struct {
		name string
		call func(ctx context.Context, commands *command.StarterDeckCommands, stale command.ChangedStarterDeck, cardID uuid.UUID) error
	}{
		{name: "rename", call: func(ctx context.Context, commands *command.StarterDeckCommands, stale command.ChangedStarterDeck, _ uuid.UUID) error {
			_, err := commands.Rename(ctx, stale.ID, stale.Revision, "上級者デッキ")
			return err
		}},
		{name: "rename_to_same_name", call: func(ctx context.Context, commands *command.StarterDeckCommands, stale command.ChangedStarterDeck, _ uuid.UUID) error {
			_, err := commands.Rename(ctx, stale.ID, stale.Revision, "中級者デッキ")
			return err
		}},
		{name: "set_card_counts", call: func(ctx context.Context, commands *command.StarterDeckCommands, stale command.ChangedStarterDeck, cardID uuid.UUID) error {
			_, err := commands.SetCardCounts(ctx, stale.ID, stale.Revision, []command.CardCountInput{{CardID: cardID, Count: 2}})
			return err
		}},
		{name: "set_card_counts_to_same_count", call: func(ctx context.Context, commands *command.StarterDeckCommands, stale command.ChangedStarterDeck, cardID uuid.UUID) error {
			_, err := commands.SetCardCounts(ctx, stale.ID, stale.Revision, []command.CardCountInput{{CardID: cardID, Count: 0}})
			return err
		}},
		{name: "set_card_counts_with_no_cards", call: func(ctx context.Context, commands *command.StarterDeckCommands, stale command.ChangedStarterDeck, _ uuid.UUID) error {
			_, err := commands.SetCardCounts(ctx, stale.ID, stale.Revision, []command.CardCountInput{})
			return err
		}},
		{name: "delete", call: func(ctx context.Context, commands *command.StarterDeckCommands, stale command.ChangedStarterDeck, _ uuid.UUID) error {
			return commands.Delete(ctx, stale.ID, stale.Revision)
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := signedIn(t, isolatedDatabase(t), registrant(t))
			stale := registerStarterDeck(t, ctx, "初心者デッキ")
			commands := newStarterDeckCommands(t, ctx)
			_, err := commands.Rename(ctx, stale.ID, stale.Revision, "中級者デッキ")
			require.NoError(t, err)

			err = tt.call(ctx, commands, stale, issueCard(t, ctx, creature.NewID))

			require.ErrorIs(t, err, command.ErrRevisionConflict)
		})
	}
}

// 同じ revision から同時に変えても、1 つだけ通り、ほかは「先に変えられた」になる
// （比べた後から保存までに先を越された場合も、eventstore の版の衝突として同じエラーになる）
func TestRenameStarterDeck_AcceptsOnlyOneOfConcurrentChanges(t *testing.T) {
	ctx := signedIn(t, isolatedDatabase(t), registrant(t))
	registered := registerStarterDeck(t, ctx, "初心者デッキ")
	commands := newStarterDeckCommands(t, ctx)

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
