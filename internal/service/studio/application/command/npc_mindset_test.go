package command_test

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/constraint/constrainttest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/matchaction"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/npcavatar"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/npcmindset"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domaindef"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/identifier"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/studio/application/command"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/studio/application/input"
)

func restoredNPCMindset(t *testing.T, ctx context.Context, publicID uuid.UUID) npcmindset.NPCMindset {
	t.Helper()
	registry, err := domaindef.Load(ctx)
	require.NoError(t, err)
	id, err := identifier.New(registry, npcmindset.NewID).Resolve(ctx, publicID)
	require.NoError(t, err)
	state, err := npcMindsetStore(registry).Load(ctx, id)
	require.NoError(t, err)
	return state.(npcmindset.NPCMindset)
}

func registerNPCMindset(t *testing.T, ctx context.Context, commands *command.NPCMindsetHandler, name string) command.ChangedNPCMindset {
	t.Helper()
	registered, err := commands.Register(ctx, name)
	require.NoError(t, err)
	return registered
}

// 公開中の NPC の思考パターン
func publishedNPCMindset(t *testing.T, ctx context.Context, name string) command.ChangedNPCMindset {
	t.Helper()
	commands := newNPCMindsetHandler(t, ctx)
	registered := registerNPCMindset(t, ctx, commands, name)
	published, err := commands.Publish(ctx, registered.ID, registered.Revision)
	require.NoError(t, err)
	return published
}

// 種類のすべてを 1 回ずつ並べた重み。指定した種類だけ値を変える
func actionWeightInputs(changed map[matchaction.Action]int) []command.ActionWeightInput {
	inputs := make([]command.ActionWeightInput, 0, len(matchaction.Actions()))
	for _, action := range matchaction.Actions() {
		weight, ok := changed[action]
		if !ok {
			weight = 1
		}
		inputs = append(inputs, command.ActionWeightInput{Action: string(action), Weight: weight})
	}
	return inputs
}

func criterionWeightInputs(changed map[npcmindset.Criterion]int) []command.CriterionWeightInput {
	inputs := make([]command.CriterionWeightInput, 0, len(npcmindset.Criteria()))
	for _, criterion := range npcmindset.Criteria() {
		weight, ok := changed[criterion]
		if !ok {
			weight = 1
		}
		inputs = append(inputs, command.CriterionWeightInput{Criterion: string(criterion), Weight: weight})
	}
	return inputs
}

func TestRegisterNPCMindset_ReturnsPublicIDAndMakesDraftWithDefaultWeights(t *testing.T) {
	ctx := signedIn(t, isolatedDatabase(t), registrant(t))

	registered, err := newNPCMindsetHandler(t, ctx).Register(ctx, "  バランス ")

	require.NoError(t, err)
	require.Equal(t, int64(1), registered.Revision)
	current := restoredNPCMindset(t, ctx, registered.ID)
	require.Equal(t, "バランス", current.Name.String())
	require.Equal(t, npcmindset.StatusDraft, current.Status)
	require.Equal(t, npcmindset.DefaultActionWeights(), current.ActionWeights)
	require.Equal(t, npcmindset.DefaultCriterionWeights(), current.CriterionWeights)
}

func TestRegisterNPCMindset_RejectsNameAlreadyInUse(t *testing.T) {
	ctx := signedIn(t, isolatedDatabase(t), registrant(t))
	commands := newNPCMindsetHandler(t, ctx)
	registerNPCMindset(t, ctx, commands, "ＡＢＣ")

	_, err := commands.Register(ctx, " abc")

	require.ErrorIs(t, err, command.ErrNPCMindsetNameAlreadyInUse)
}

func TestRegisterNPCMindset_AllowsNameUsedByNPCAvatar(t *testing.T) {
	ctx := signedIn(t, isolatedDatabase(t), registrant(t))
	_, err := newNPCAvatarHandler(t, ctx).Register(ctx, "ゴリガン")
	require.NoError(t, err)

	_, err = newNPCMindsetHandler(t, ctx).Register(ctx, "ゴリガン")

	require.NoError(t, err)
}

func TestRegisterNPCMindset_RejectsInvalidName(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  input.Error
	}{
		{name: "blank", value: " ", want: &input.Required{Field: "name"}},
		{name: "too_long", value: strings.Repeat("あ", npcmindset.MaxNameLength+1), want: &input.TooLong{Field: "name", Max: npcmindset.MaxNameLength}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := signedIn(t, isolatedDatabase(t), registrant(t))

			_, err := newNPCMindsetHandler(t, ctx).Register(ctx, tt.value)

			invalid, ok := input.AsErrors(err)
			require.True(t, ok)
			require.Equal(t, input.Errors{tt.want}, invalid)
		})
	}
}

func TestNPCMindsetHandler_RejectsCallWithoutOperator(t *testing.T) {
	ctx := isolatedDatabase(t)
	commands := newNPCMindsetHandler(t, ctx)

	_, err := commands.Register(ctx, "バランス")
	require.ErrorIs(t, err, command.ErrNotOperator)
	_, err = commands.Rename(ctx, uuid.New(), 1, "侵略型")
	require.ErrorIs(t, err, command.ErrNotOperator)
	_, err = commands.SetActionWeights(ctx, uuid.New(), 1, nil)
	require.ErrorIs(t, err, command.ErrNotOperator)
	_, err = commands.SetCriterionWeights(ctx, uuid.New(), 1, nil)
	require.ErrorIs(t, err, command.ErrNotOperator)
	_, err = commands.Publish(ctx, uuid.New(), 1)
	require.ErrorIs(t, err, command.ErrNotOperator)
	_, err = commands.Retire(ctx, uuid.New(), 1)
	require.ErrorIs(t, err, command.ErrNotOperator)
	require.ErrorIs(t, commands.Delete(ctx, uuid.New(), 1), command.ErrNotOperator)
}

func TestRenameNPCMindset_ChangesNameAndFreesOldName(t *testing.T) {
	ctx := signedIn(t, isolatedDatabase(t), registrant(t))
	commands := newNPCMindsetHandler(t, ctx)
	registered := registerNPCMindset(t, ctx, commands, "バランス")

	renamed, err := commands.Rename(ctx, registered.ID, registered.Revision, "侵略型")

	require.NoError(t, err)
	require.Equal(t, registered.Revision+1, renamed.Revision)
	require.Equal(t, "侵略型", restoredNPCMindset(t, ctx, registered.ID).Name.String())
	registerNPCMindset(t, ctx, commands, "バランス")
}

func TestRenameNPCMindset_RejectsNameUsedByOtherNPCMindset(t *testing.T) {
	ctx := signedIn(t, isolatedDatabase(t), registrant(t))
	commands := newNPCMindsetHandler(t, ctx)
	registerNPCMindset(t, ctx, commands, "バランス")
	other := registerNPCMindset(t, ctx, commands, "侵略型")

	_, err := commands.Rename(ctx, other.ID, other.Revision, "バランス")

	require.ErrorIs(t, err, command.ErrNPCMindsetNameAlreadyInUse)
}

func TestNPCMindsetHandler_RejectsStaleRevisionAndMissingNPCMindset(t *testing.T) {
	ctx := signedIn(t, isolatedDatabase(t), registrant(t))
	commands := newNPCMindsetHandler(t, ctx)
	registered := registerNPCMindset(t, ctx, commands, "バランス")
	_, err := commands.Rename(ctx, registered.ID, registered.Revision, "侵略型")
	require.NoError(t, err)
	operations := []struct {
		name string
		call func(id uuid.UUID, revision int64) error
	}{
		{name: "rename", call: func(id uuid.UUID, revision int64) error {
			_, err := commands.Rename(ctx, id, revision, "育成型")
			return err
		}},
		{name: "set_action_weights", call: func(id uuid.UUID, revision int64) error {
			_, err := commands.SetActionWeights(ctx, id, revision, actionWeightInputs(nil))
			return err
		}},
		{name: "set_criterion_weights", call: func(id uuid.UUID, revision int64) error {
			_, err := commands.SetCriterionWeights(ctx, id, revision, criterionWeightInputs(nil))
			return err
		}},
		{name: "publish", call: func(id uuid.UUID, revision int64) error { _, err := commands.Publish(ctx, id, revision); return err }},
		{name: "retire", call: func(id uuid.UUID, revision int64) error { _, err := commands.Retire(ctx, id, revision); return err }},
		{name: "delete", call: func(id uuid.UUID, revision int64) error { return commands.Delete(ctx, id, revision) }},
	}
	for _, op := range operations {
		t.Run(op.name+"_stale_revision", func(t *testing.T) {
			require.ErrorIs(t, op.call(registered.ID, registered.Revision), command.ErrRevisionConflict)
		})
		t.Run(op.name+"_missing_npc_mindset", func(t *testing.T) {
			require.ErrorIs(t, op.call(uuid.New(), 1), command.ErrNPCMindsetNotFound)
		})
	}
}

func TestSetNPCMindsetActionWeights_ReplacesWeightsAndSameWeightsKeepRevision(t *testing.T) {
	ctx := signedIn(t, isolatedDatabase(t), registrant(t))
	commands := newNPCMindsetHandler(t, ctx)
	registered := registerNPCMindset(t, ctx, commands, "バランス")
	weights := actionWeightInputs(map[matchaction.Action]int{matchaction.ActionInvadeTerritory: 100, matchaction.ActionPayToll: 0})

	changed, err := commands.SetActionWeights(ctx, registered.ID, registered.Revision, weights)
	same, sameErr := commands.SetActionWeights(ctx, changed.ID, changed.Revision, weights)

	require.NoError(t, err)
	require.NoError(t, sameErr)
	require.Equal(t, command.ChangedNPCMindset{ID: registered.ID, Revision: registered.Revision + 1}, changed)
	require.Equal(t, changed, same)
	current := restoredNPCMindset(t, ctx, registered.ID)
	require.Equal(t, 100, current.ActionWeights[matchaction.ActionInvadeTerritory].Int())
	require.Equal(t, 0, current.ActionWeights[matchaction.ActionPayToll].Int())
	require.Equal(t, 1, current.ActionWeights[matchaction.ActionEndTurn].Int())
	require.Equal(t, npcmindset.DefaultCriterionWeights(), current.CriterionWeights)
}

func TestSetNPCMindsetCriterionWeights_ReplacesWeightsAndSameWeightsKeepRevision(t *testing.T) {
	ctx := signedIn(t, isolatedDatabase(t), registrant(t))
	commands := newNPCMindsetHandler(t, ctx)
	registered := registerNPCMindset(t, ctx, commands, "バランス")
	weights := criterionWeightInputs(map[npcmindset.Criterion]int{npcmindset.CriterionOwnTotalMagic: 3, npcmindset.CriterionTerritoryCount: 0})

	changed, err := commands.SetCriterionWeights(ctx, registered.ID, registered.Revision, weights)
	same, sameErr := commands.SetCriterionWeights(ctx, changed.ID, changed.Revision, weights)

	require.NoError(t, err)
	require.NoError(t, sameErr)
	require.Equal(t, registered.Revision+1, changed.Revision)
	require.Equal(t, changed, same)
	current := restoredNPCMindset(t, ctx, registered.ID)
	require.Equal(t, 3, current.CriterionWeights[npcmindset.CriterionOwnTotalMagic].Int())
	require.Equal(t, 0, current.CriterionWeights[npcmindset.CriterionTerritoryCount].Int())
	require.Equal(t, npcmindset.DefaultActionWeights(), current.ActionWeights)
}

func TestSetNPCMindsetActionWeights_ReturnsFieldErrors(t *testing.T) {
	ctx := signedIn(t, isolatedDatabase(t), registrant(t))
	commands := newNPCMindsetHandler(t, ctx)
	registered := registerNPCMindset(t, ctx, commands, "バランス")
	full := actionWeightInputs(nil)
	withWeight := func(index, weight int) []command.ActionWeightInput {
		inputs := append([]command.ActionWeightInput(nil), full...)
		inputs[index].Weight = weight
		return inputs
	}
	tests := []struct {
		name    string
		weights []command.ActionWeightInput
		want    []input.Error
	}{
		{name: "negative_weight", weights: withWeight(2, -1), want: []input.Error{&input.OutOfRange{Field: "weights.2.weight", Min: 0, Max: 100}}},
		{name: "over_upper_limit", weights: withWeight(0, 101), want: []input.Error{&input.OutOfRange{Field: "weights.0.weight", Min: 0, Max: 100}}},
		{name: "missing_action", weights: full[1:], want: []input.Error{&input.Required{Field: "weights"}}},
		{name: "empty", weights: nil, want: []input.Error{&input.Required{Field: "weights"}}},
		{name: "duplicated_action", weights: append(append([]command.ActionWeightInput(nil), full...), full[0]), want: []input.Error{&input.Duplicated{Field: "weights.19"}}},
		{name: "unknown_action", weights: append(append([]command.ActionWeightInput(nil), full[1:]...), command.ActionWeightInput{Action: "unknown", Weight: 1}), want: []input.Error{&input.InvalidFormat{Field: "weights.18"}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := commands.SetActionWeights(ctx, registered.ID, registered.Revision, tt.weights)

			invalid, ok := input.AsErrors(err)
			require.True(t, ok)
			require.Equal(t, tt.want, []input.Error(invalid))
			require.Equal(t, npcmindset.DefaultActionWeights(), restoredNPCMindset(t, ctx, registered.ID).ActionWeights)
		})
	}
}

func TestSetNPCMindsetCriterionWeights_ReturnsFieldErrors(t *testing.T) {
	ctx := signedIn(t, isolatedDatabase(t), registrant(t))
	commands := newNPCMindsetHandler(t, ctx)
	registered := registerNPCMindset(t, ctx, commands, "バランス")
	full := criterionWeightInputs(nil)
	withWeight := func(index, weight int) []command.CriterionWeightInput {
		inputs := append([]command.CriterionWeightInput(nil), full...)
		inputs[index].Weight = weight
		return inputs
	}
	tests := []struct {
		name    string
		weights []command.CriterionWeightInput
		want    []input.Error
	}{
		{name: "negative_weight", weights: withWeight(1, -1), want: []input.Error{&input.OutOfRange{Field: "weights.1.weight", Min: 0, Max: 100}}},
		{name: "over_upper_limit", weights: withWeight(2, 101), want: []input.Error{&input.OutOfRange{Field: "weights.2.weight", Min: 0, Max: 100}}},
		{name: "missing_criterion", weights: full[1:], want: []input.Error{&input.Required{Field: "weights"}}},
		{name: "duplicated_criterion", weights: append(append([]command.CriterionWeightInput(nil), full...), full[0]), want: []input.Error{&input.Duplicated{Field: "weights.3"}}},
		{name: "unknown_criterion", weights: append(append([]command.CriterionWeightInput(nil), full[1:]...), command.CriterionWeightInput{Criterion: "unknown", Weight: 1}), want: []input.Error{&input.InvalidFormat{Field: "weights.2"}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := commands.SetCriterionWeights(ctx, registered.ID, registered.Revision, tt.weights)

			invalid, ok := input.AsErrors(err)
			require.True(t, ok)
			require.Equal(t, tt.want, []input.Error(invalid))
			require.Equal(t, npcmindset.DefaultCriterionWeights(), restoredNPCMindset(t, ctx, registered.ID).CriterionWeights)
		})
	}
}

func TestPublishNPCMindset_MakesPublishedAndSecondCallKeepsRevision(t *testing.T) {
	ctx := signedIn(t, isolatedDatabase(t), registrant(t))
	commands := newNPCMindsetHandler(t, ctx)
	registered := registerNPCMindset(t, ctx, commands, "バランス")

	published, err := commands.Publish(ctx, registered.ID, registered.Revision)
	require.NoError(t, err)
	again, err := commands.Publish(ctx, published.ID, published.Revision)

	require.NoError(t, err)
	require.Equal(t, registered.Revision+1, published.Revision)
	require.Equal(t, published.Revision, again.Revision)
	require.Equal(t, npcmindset.StatusPublished, restoredNPCMindset(t, ctx, registered.ID).Status)
}

func TestRetireNPCMindset_MakesRetiredOnlyFromPublishedAndCanBePublishedAgain(t *testing.T) {
	ctx := signedIn(t, isolatedDatabase(t), registrant(t))
	commands := newNPCMindsetHandler(t, ctx)
	registered := registerNPCMindset(t, ctx, commands, "バランス")

	_, err := commands.Retire(ctx, registered.ID, registered.Revision)
	require.ErrorIs(t, err, command.ErrInvalidNPCMindsetStatusTransition)

	published, err := commands.Publish(ctx, registered.ID, registered.Revision)
	require.NoError(t, err)
	retired, err := commands.Retire(ctx, published.ID, published.Revision)
	require.NoError(t, err)
	require.Equal(t, npcmindset.StatusRetired, restoredNPCMindset(t, ctx, registered.ID).Status)
	again, err := commands.Publish(ctx, retired.ID, retired.Revision)
	require.NoError(t, err)
	require.Equal(t, npcmindset.StatusPublished, restoredNPCMindset(t, ctx, again.ID).Status)
}

func TestNPCMindsetHandler_RejectsEditingUnlessDraft(t *testing.T) {
	ctx := signedIn(t, isolatedDatabase(t), registrant(t))
	commands := newNPCMindsetHandler(t, ctx)
	published := publishedNPCMindset(t, ctx, "バランス")

	_, err := commands.Rename(ctx, published.ID, published.Revision, "侵略型")
	require.ErrorIs(t, err, command.ErrNPCMindsetNotDraft)
	_, err = commands.SetActionWeights(ctx, published.ID, published.Revision, actionWeightInputs(nil))
	require.ErrorIs(t, err, command.ErrNPCMindsetNotDraft)
	_, err = commands.SetCriterionWeights(ctx, published.ID, published.Revision, criterionWeightInputs(nil))
	require.ErrorIs(t, err, command.ErrNPCMindsetNotDraft)
	require.ErrorIs(t, commands.Delete(ctx, published.ID, published.Revision), command.ErrNPCMindsetNotDraft)
}

func TestDeleteNPCMindset_RemovesNPCMindsetAndFreesName(t *testing.T) {
	ctx := signedIn(t, isolatedDatabase(t), registrant(t))
	commands := newNPCMindsetHandler(t, ctx)
	registered := registerNPCMindset(t, ctx, commands, "バランス")

	require.NoError(t, commands.Delete(ctx, registered.ID, registered.Revision))

	_, err := commands.Rename(ctx, registered.ID, registered.Revision, "侵略型")
	require.ErrorIs(t, err, command.ErrNPCMindsetNotFound)
	registerNPCMindset(t, ctx, commands, "バランス")
}

func TestSetNPCAvatarMindsets_AssignsPublishedMindsetsAndReferences(t *testing.T) {
	ctx := signedIn(t, isolatedDatabase(t), registrant(t))
	commands := newNPCAvatarHandler(t, ctx)
	registered := registerNPCAvatar(t, ctx, commands, "森の番人")
	first, second := publishedNPCMindset(t, ctx, "バランス"), publishedNPCMindset(t, ctx, "侵略型")

	changed, err := commands.SetMindsets(ctx, registered.ID, registered.Revision, []uuid.UUID{second.ID, first.ID})

	require.NoError(t, err)
	require.Equal(t, command.ChangedNPCAvatar{ID: registered.ID, Revision: registered.Revision + 1}, changed)
	current := restoredNPCAvatar(t, ctx, registered.ID)
	require.Equal(t, []int64{internalID(t, ctx, second.ID), internalID(t, ctx, first.ID)}, internalMindsetIDs(current))
	require.ElementsMatch(t, []int64{internalID(t, ctx, first.ID), internalID(t, ctx, second.ID)},
		constrainttest.Referenced(t, ctx, npcavatar.MindsetReferenceType(), current.ID.Int64()))
}

func internalMindsetIDs(current npcavatar.NPCAvatar) []int64 {
	ids := make([]int64, 0, len(current.Mindsets))
	for _, mindset := range current.Mindsets {
		ids = append(ids, mindset.Int64())
	}
	return ids
}

func TestSetNPCAvatarMindsets_ReplacesWholeListAndReferences(t *testing.T) {
	ctx := signedIn(t, isolatedDatabase(t), registrant(t))
	commands := newNPCAvatarHandler(t, ctx)
	registered := registerNPCAvatar(t, ctx, commands, "森の番人")
	first, second := publishedNPCMindset(t, ctx, "バランス"), publishedNPCMindset(t, ctx, "侵略型")
	assigned, err := commands.SetMindsets(ctx, registered.ID, registered.Revision, []uuid.UUID{first.ID})
	require.NoError(t, err)

	replaced, err := commands.SetMindsets(ctx, assigned.ID, assigned.Revision, []uuid.UUID{second.ID})
	require.NoError(t, err)
	cleared, err := commands.SetMindsets(ctx, replaced.ID, replaced.Revision, nil)

	require.NoError(t, err)
	// 割り当て（+1）→ 入れ替え（外す +1・割り当てる +1）→ すべて外す（+1）
	require.Equal(t, assigned.Revision+2, replaced.Revision)
	require.Equal(t, registered.Revision+4, cleared.Revision)
	current := restoredNPCAvatar(t, ctx, registered.ID)
	require.Empty(t, current.Mindsets)
	require.Empty(t, constrainttest.Referenced(t, ctx, npcavatar.MindsetReferenceType(), current.ID.Int64()))
}

// 同じ NPC の思考パターンを、複数の NPC のアバターが使える。デッキの割り当てとは別の一覧
func TestSetNPCAvatarMindsets_AllowsMindsetSharedByNPCAvatarsAndKeepsDecks(t *testing.T) {
	ctx := signedIn(t, isolatedDatabase(t), registrant(t))
	commands := newNPCAvatarHandler(t, ctx)
	mindset := publishedNPCMindset(t, ctx, "バランス")
	withDeck := publishableNPCAvatar(t, ctx, commands, "森の番人")

	for _, registered := range []command.ChangedNPCAvatar{withDeck, registerNPCAvatar(t, ctx, commands, "炎の剣士")} {
		_, err := commands.SetMindsets(ctx, registered.ID, registered.Revision, []uuid.UUID{mindset.ID})
		require.NoError(t, err)
	}

	require.Len(t, restoredNPCAvatar(t, ctx, withDeck.ID).Decks, 1)
}

func TestSetNPCAvatarMindsets_SameMindsetsKeepRevision(t *testing.T) {
	ctx := signedIn(t, isolatedDatabase(t), registrant(t))
	commands := newNPCAvatarHandler(t, ctx)
	registered := registerNPCAvatar(t, ctx, commands, "森の番人")
	first, second := publishedNPCMindset(t, ctx, "バランス"), publishedNPCMindset(t, ctx, "侵略型")
	assigned, err := commands.SetMindsets(ctx, registered.ID, registered.Revision, []uuid.UUID{first.ID, second.ID})
	require.NoError(t, err)

	same, err := commands.SetMindsets(ctx, assigned.ID, assigned.Revision, []uuid.UUID{second.ID, first.ID})

	require.NoError(t, err)
	require.Equal(t, assigned, same)
}

func TestSetNPCAvatarMindsets_ReturnsFieldErrorsForDuplicateUnknownAndUnpublished(t *testing.T) {
	ctx := signedIn(t, isolatedDatabase(t), registrant(t))
	commands := newNPCAvatarHandler(t, ctx)
	registered := registerNPCAvatar(t, ctx, commands, "森の番人")
	mindsets := newNPCMindsetHandler(t, ctx)
	published := publishedNPCMindset(t, ctx, "バランス")
	draft := registerNPCMindset(t, ctx, mindsets, "準備中")
	retired := publishedNPCMindset(t, ctx, "使用不可")
	_, err := mindsets.Retire(ctx, retired.ID, retired.Revision)
	require.NoError(t, err)
	deleted := registerNPCMindset(t, ctx, mindsets, "消す")
	require.NoError(t, mindsets.Delete(ctx, deleted.ID, deleted.Revision))
	tests := []struct {
		name     string
		mindsets []uuid.UUID
		want     []input.Error
	}{
		{name: "duplicated", mindsets: []uuid.UUID{published.ID, published.ID}, want: []input.Error{&input.Duplicated{Field: "mindsetIds.1"}}},
		{name: "unknown", mindsets: []uuid.UUID{published.ID, uuid.New()}, want: []input.Error{&input.NotFound{Field: "mindsetIds.1"}}},
		{name: "deleted", mindsets: []uuid.UUID{deleted.ID}, want: []input.Error{&input.NotFound{Field: "mindsetIds.0"}}},
		{name: "not_npc_mindset", mindsets: []uuid.UUID{registered.ID}, want: []input.Error{&input.NotFound{Field: "mindsetIds.0"}}},
		{name: "draft_and_retired", mindsets: []uuid.UUID{draft.ID, published.ID, retired.ID}, want: []input.Error{&input.Unavailable{Field: "mindsetIds.0"}, &input.Unavailable{Field: "mindsetIds.2"}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := commands.SetMindsets(ctx, registered.ID, registered.Revision, tt.mindsets)

			invalid, ok := input.AsErrors(err)
			require.True(t, ok)
			require.Equal(t, tt.want, []input.Error(invalid))
			require.Empty(t, restoredNPCAvatar(t, ctx, registered.ID).Mindsets)
		})
	}
}

// 割り当てた後に使用不可にした思考パターンは、ほかのものだけを変えるときに外さなくてよい
func TestSetNPCAvatarMindsets_KeepsAssignedMindsetThatWasRetiredAfterwards(t *testing.T) {
	ctx := signedIn(t, isolatedDatabase(t), registrant(t))
	commands := newNPCAvatarHandler(t, ctx)
	registered := registerNPCAvatar(t, ctx, commands, "森の番人")
	retired, other := publishedNPCMindset(t, ctx, "バランス"), publishedNPCMindset(t, ctx, "侵略型")
	assigned, err := commands.SetMindsets(ctx, registered.ID, registered.Revision, []uuid.UUID{retired.ID})
	require.NoError(t, err)
	_, err = newNPCMindsetHandler(t, ctx).Retire(ctx, retired.ID, retired.Revision)
	require.NoError(t, err)

	_, err = commands.SetMindsets(ctx, assigned.ID, assigned.Revision, []uuid.UUID{retired.ID, other.ID})

	require.NoError(t, err)
	require.Len(t, restoredNPCAvatar(t, ctx, registered.ID).Mindsets, 2)
}

func TestSetNPCAvatarMindsets_RejectsWhenNotDraftOrWithoutOperator(t *testing.T) {
	ctx := signedIn(t, isolatedDatabase(t), registrant(t))
	commands := newNPCAvatarHandler(t, ctx)
	published, err := commands.Publish(ctx, publishableNPCAvatar(t, ctx, commands, "森の番人").ID, 2)
	require.NoError(t, err)

	_, notDraft := commands.SetMindsets(ctx, published.ID, published.Revision, nil)
	_, withoutOperator := commands.SetMindsets(isolatedDatabase(t), uuid.New(), 1, nil)

	require.ErrorIs(t, notDraft, command.ErrNPCAvatarNotDraft)
	require.ErrorIs(t, withoutOperator, command.ErrNotOperator)
}
