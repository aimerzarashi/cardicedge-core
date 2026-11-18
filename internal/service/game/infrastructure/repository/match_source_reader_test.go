package repository_test

import (
	"encoding/json"
	"testing"

	"github.com/aarondl/sqlboiler/v4/boil"
	"github.com/stretchr/testify/require"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/matchaction"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/npcmindset"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/npcmindset/npcmindsettest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/identifier"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/orm/sqlboiler/models"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/rdb"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/game/infrastructure/repository"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/studio/application/query/readmodel"
)

// 写した行の重みを、種類・基準の値の文字列のまま返す。公開中でなくても読む（選んだ後に使用不可になっても、対戦は始めた時点の写しで進むため）
func TestMatchSourceReader_ReadsWeightsOfCopiedNPCMindsetEvenWhenRetired(t *testing.T) {
	f := newPlayerFixture(t)
	id, publicID := issueEntity(t, f, npcmindset.NewID)
	metadata := readmodel.NPCMindsetMetadata{
		ID: publicID, Name: "慎重", Status: readmodel.NPCMindsetStatusRetired,
		ActionWeights:    map[matchaction.Action]int{matchaction.ActionDrawCard: 5, matchaction.ActionEndTurn: 0},
		CriterionWeights: map[npcmindset.Criterion]int{npcmindset.CriterionMagicReserve: 3},
	}
	raw, err := json.Marshal(metadata)
	require.NoError(t, err)
	exec, err := rdb.GetExecutor(f.ctx)
	require.NoError(t, err)
	row := models.NPCMindset{NPCMindsetID: publicID.String(), SortName: "慎重", Status: "retired", Metadata: raw}
	require.NoError(t, row.Insert(f.ctx, exec, boil.Infer()))

	got, err := repository.NewMatchSourceReader(identifier.NewEntityRefResolver(f.registry)).NPCMindset(f.ctx, id)

	require.NoError(t, err)
	require.Equal(t, map[string]int{"draw_card": 5, "end_turn": 0}, got.ActionWeights)
	require.Equal(t, map[string]int{"magic_reserve": 3}, got.CriterionWeights)
}

func TestMatchSourceReader_FailsForNPCMindsetThatWasNotCopiedOrHasNoWeights(t *testing.T) {
	f := newPlayerFixture(t)
	notCopied, _ := issueEntity(t, f, npcmindset.NewID)
	withoutWeights, withoutWeightsPublicID := issueEntity(t, f, npcmindset.NewID)
	exec, err := rdb.GetExecutor(f.ctx)
	require.NoError(t, err)
	row := models.NPCMindset{NPCMindsetID: withoutWeightsPublicID.String(), SortName: "x", Status: "published", Metadata: []byte(`{"name":"x"}`)}
	require.NoError(t, row.Insert(f.ctx, exec, boil.Infer()))
	reader := repository.NewMatchSourceReader(identifier.NewEntityRefResolver(f.registry))

	_, notCopiedErr := reader.NPCMindset(f.ctx, notCopied)
	_, withoutWeightsErr := reader.NPCMindset(f.ctx, withoutWeights)
	_, unknownErr := reader.NPCMindset(f.ctx, npcmindsettest.ID(t, 999999))

	require.ErrorIs(t, notCopiedErr, domain.ErrEntityNotFound)
	require.ErrorContains(t, withoutWeightsErr, "no action_weights or criterion_weights")
	require.ErrorIs(t, unknownErr, domain.ErrEntityNotFound)
}
