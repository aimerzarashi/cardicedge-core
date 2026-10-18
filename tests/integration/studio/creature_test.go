package studio_test

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/idp/idptest"
	"gitlab.com/aimerzarashi/diecardomain/core/tests/testhelper"
)

const registerCreatureMutation = `mutation ($input: RegisterCreatureInput!) {
	registerCreature(input: $input) {
		__typename
		... on RegisterCreatureSuccess { creature { id name registeredAt updatedAt } }
		... on ValidationErrors { errors { __typename field ... on FieldTooLong { max } } }
	}
}`

func registerCreature(t *testing.T, token, name string) map[string]any {
	t.Helper()
	return client.Data(t, token, registerCreatureMutation, map[string]any{"input": map[string]any{"name": name}}, "registerCreature")
}

func TestRegisterCreature_RegistersCreatureForOperator(t *testing.T) {
	t.Parallel()
	name := "ドラゴン-" + testhelper.UniqueID(t)

	result := registerCreature(t, operators.Token(t), "  "+name+" ")

	require.Equal(t, "RegisterCreatureSuccess", result["__typename"])
	registered := result["creature"].(map[string]any)
	require.Equal(t, name, registered["name"])
	_, err := uuid.Parse(registered["id"].(string))
	require.NoError(t, err)
	_, err = time.Parse(time.RFC3339, registered["registeredAt"].(string))
	require.NoError(t, err)
	require.Equal(t, registered["registeredAt"], registered["updatedAt"])
}

func TestRegisterCreature_ReturnsResultTypeWhenNotRegistered(t *testing.T) {
	t.Parallel()
	taken := "ゴブリン-" + testhelper.UniqueID(t)
	require.Equal(t, "RegisterCreatureSuccess", registerCreature(t, operators.Token(t), taken)["__typename"])
	tests := []struct {
		name     string
		token    string
		cardName string
		want     string
	}{
		{name: "not_operator", token: key.Sign(t, idptest.ValidClaims("not-operator")), cardName: "スライム-" + testhelper.UniqueID(t), want: "ForbiddenError"},
		{name: "blank_name", token: operators.Token(t), cardName: " ", want: "ValidationErrors"},
		{name: "name_already_in_use", token: operators.Token(t), cardName: taken, want: "CardNameAlreadyInUseError"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status, body := client.Post(t, tt.token, registerCreatureMutation, map[string]any{"input": map[string]any{"name": tt.cardName}})

			require.Equal(t, http.StatusOK, status)
			require.Nil(t, body["errors"])
			require.Equal(t, tt.want, body["data"].(map[string]any)["registerCreature"].(map[string]any)["__typename"])
		})
	}
}

// 文言は返さず、理由ごとの型と、その理由に要る値（上限）を返す（studio が文言を作る）
func TestRegisterCreature_ReturnsFieldErrorPerReason(t *testing.T) {
	t.Parallel()
	token := operators.Token(t)
	tests := []struct {
		name     string
		cardName string
		want     map[string]any
	}{
		{name: "blank", cardName: " ", want: map[string]any{"__typename": "FieldRequired", "field": "name"}},
		{name: "too_long", cardName: strings.Repeat("あ", 51), want: map[string]any{"__typename": "FieldTooLong", "field": "name", "max": float64(50)}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := registerCreature(t, token, tt.cardName)

			require.Equal(t, "ValidationErrors", result["__typename"])
			require.Equal(t, []any{tt.want}, result["errors"])
		})
	}
}
