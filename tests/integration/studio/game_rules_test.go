package studio_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/gamerules"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/orm/sqlboiler/models"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/studio/application/query/readmodel"
	"gitlab.com/aimerzarashi/diecardomain/core/tests/testhelper"
)

// 起動した後には、ゲーム全体のルールが既定の値で 1 つあり、read model に投影されている（make publish-master が写す元）
func TestStartup_CreatesGameRulesAndProjectsReadModel(t *testing.T) {
	t.Parallel()

	testhelper.Eventually(t, func(c *assert.CollectT) {
		rows, err := models.GameRules().All(t.Context(), studioDB)
		if !assert.NoError(c, err) || !assert.Len(c, rows, 1) {
			return
		}
		var metadata readmodel.GameRulesMetadata
		if assert.NoError(c, json.Unmarshal(rows[0].Metadata, &metadata)) {
			assert.Equal(c, int64(1), metadata.Revision)
			assert.Equal(c, gamerules.Defaults(), metadata.Rules)
		}
	})
}

// 起動時に作るのは 1 つだけ（イベントが 1 つ）
func TestStartup_CreatesGameRulesOnlyOnce(t *testing.T) {
	t.Parallel()

	var events int
	require.NoError(t, studioDB.QueryRowContext(t.Context(), `SELECT count(*) FROM domain_events WHERE entity_type_id = 2000`).Scan(&events))

	require.Equal(t, 1, events)
}
