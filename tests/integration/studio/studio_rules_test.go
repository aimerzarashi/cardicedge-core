package studio_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/studiorules"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/idp/idptest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/orm/sqlboiler/models"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/studio/application/query/readmodel"
	"gitlab.com/aimerzarashi/diecardomain/core/tests/testhelper"
)

const studioRulesQuery = `query {
	studioRules {
		__typename
		... on StudioRules { board { minSize maxSize } }
	}
}`

// 起動した後には、studio_rules が既定の値で 1 つあり、read model に投影されている
func TestStartup_CreatesStudioRulesAndProjectsReadModel(t *testing.T) {
	t.Parallel()

	testhelper.Eventually(t, func(c *assert.CollectT) {
		rows, err := models.StudioRules().All(t.Context(), studioDB)
		if !assert.NoError(c, err) || !assert.Len(c, rows, 1) {
			return
		}
		var metadata readmodel.StudioRulesMetadata
		if assert.NoError(c, json.Unmarshal(rows[0].Metadata, &metadata)) {
			assert.Equal(c, int64(1), metadata.Revision)
			assert.Equal(c, studiorules.Defaults(), metadata.Rules)
		}
	})
}

// 起動時に作るのは 1 つだけ（イベントが 1 つ）
func TestStartup_CreatesStudioRulesOnlyOnce(t *testing.T) {
	t.Parallel()

	var events int
	require.NoError(t, studioDB.QueryRowContext(t.Context(), `SELECT count(*) FROM domain_events WHERE entity_type_id = 2400`).Scan(&events))

	require.Equal(t, 1, events)
}

func TestStudioRules_ReturnsBoardRangeToOperator(t *testing.T) {
	t.Parallel()
	token := operators.Token(t)

	var rules map[string]any
	testhelper.Eventually(t, func(c *assert.CollectT) {
		var err error
		rules, err = client.TryData(t.Context(), token, studioRulesQuery, nil, "studioRules")
		if assert.NoError(c, err) {
			assert.Equal(c, "StudioRules", rules["__typename"])
		}
	})

	defaults := studiorules.Defaults().Board
	require.Equal(t, map[string]any{
		"__typename": "StudioRules",
		"board":      map[string]any{"minSize": float64(defaults.MinSize), "maxSize": float64(defaults.MaxSize)},
	}, rules)
}

func TestStudioRules_IsForbiddenForNonOperator(t *testing.T) {
	t.Parallel()
	notOperator := key.Sign(t, idptest.ValidClaims("not-operator-"+testhelper.UniqueID(t)))

	status, body := client.Post(t, notOperator, studioRulesQuery, nil)

	require.Equal(t, http.StatusOK, status)
	require.Nil(t, body["errors"])
	require.Equal(t, "ForbiddenError", body["data"].(map[string]any)["studioRules"].(map[string]any)["__typename"])
}
