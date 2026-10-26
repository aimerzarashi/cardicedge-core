package game_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"gitlab.com/aimerzarashi/diecardomain/core/tests/testhelper"
)

const registerAvatarMutation = `mutation($name: String!) {
	registerAvatar(input: { name: $name }) {
		__typename
		... on RegisterAvatarSuccess { avatar { id name } }
		... on ValidationErrors {
			errors {
				__typename
				field
				... on FieldTooLong { max }
			}
		}
		... on AvatarNameAlreadyInUseError { message }
		... on AvatarLimitReachedError { message max }
		... on PlayerNotRegisteredError { message }
	}
}`

const meAvatarsQuery = `{
	me {
		__typename
		... on Player { avatars { id name } }
	}
}`

func registerAvatar(t *testing.T, token, name string) map[string]any {
	t.Helper()
	return client.Data(t, token, registerAvatarMutation, map[string]any{"name": name}, "registerAvatar")
}

func registeredAvatar(t *testing.T, result map[string]any) map[string]any {
	t.Helper()
	require.Equal(t, "RegisterAvatarSuccess", result["__typename"])
	return result["avatar"].(map[string]any)
}

// 投影が遅れて反映されるため、期待する一覧になるまで待つ
func waitMeAvatars(t *testing.T, token string, want []map[string]any) {
	t.Helper()
	testhelper.Eventually(t, func(c *assert.CollectT) {
		result, err := client.TryData(t.Context(), token, meAvatarsQuery, nil, "me")
		if !assert.NoError(c, err) {
			return
		}
		assert.Equal(c, "Player", result["__typename"])
		got := make([]map[string]any, 0)
		for _, avatar := range result["avatars"].([]any) {
			got = append(got, avatar.(map[string]any))
		}
		assert.Equal(c, want, got)
	})
}

func registeredPlayerToken(t *testing.T) string {
	t.Helper()
	token := newPlayerToken(t)
	registerPlayer(t, token)
	return token
}

func TestRegisterAvatar_CreatesAvatarListedInMeInRegistrationOrder(t *testing.T) {
	t.Parallel()
	token := registeredPlayerToken(t)

	first := registeredAvatar(t, registerAvatar(t, token, " アリス "))
	second := registeredAvatar(t, registerAvatar(t, token, "ボブ"))

	require.Equal(t, "アリス", first["name"])
	_, err := uuid.Parse(first["id"].(string))
	require.NoError(t, err)
	waitMeAvatars(t, token, []map[string]any{first, second})
}

func TestMe_ListsNoAvatarsBeforeCreating(t *testing.T) {
	t.Parallel()

	waitMeAvatars(t, registeredPlayerToken(t), []map[string]any{})
}

func TestRegisterAvatar_ReturnsValidationErrorsForInvalidName(t *testing.T) {
	t.Parallel()
	token := registeredPlayerToken(t)

	blank := registerAvatar(t, token, "  ")
	tooLong := registerAvatar(t, token, strings.Repeat("あ", 21))

	require.Equal(t, "ValidationErrors", blank["__typename"])
	require.Equal(t, []any{map[string]any{"__typename": "FieldRequired", "field": "name"}}, blank["errors"])
	require.Equal(t, "ValidationErrors", tooLong["__typename"])
	require.Equal(t, []any{map[string]any{"__typename": "FieldTooLong", "field": "name", "max": float64(20)}}, tooLong["errors"])
}

func TestRegisterAvatar_RejectsSameNameOfSamePlayerButNotOfOtherPlayer(t *testing.T) {
	t.Parallel()
	token := registeredPlayerToken(t)
	registeredAvatar(t, registerAvatar(t, token, "アリス"))

	same := registerAvatar(t, token, " アリス")
	other := registerAvatar(t, registeredPlayerToken(t), "アリス")

	require.Equal(t, "AvatarNameAlreadyInUseError", same["__typename"])
	require.NotEmpty(t, same["message"])
	registeredAvatar(t, other)
}

func TestRegisterAvatar_ReturnsLimitReachedWithMaxForSixthAvatar(t *testing.T) {
	t.Parallel()
	token := registeredPlayerToken(t)
	for i := range 5 {
		registeredAvatar(t, registerAvatar(t, token, fmt.Sprintf("アバター%d", i)))
	}

	sixth := registerAvatar(t, token, "アバター5")

	require.Equal(t, "AvatarLimitReachedError", sixth["__typename"])
	require.NotEmpty(t, sixth["message"])
	require.Equal(t, float64(5), sixth["max"])
}

func TestRegisterAvatar_ReturnsPlayerNotRegisteredForUnregisteredPlayer(t *testing.T) {
	t.Parallel()

	result := registerAvatar(t, newPlayerToken(t), "アリス")

	require.Equal(t, "PlayerNotRegisteredError", result["__typename"])
	require.NotEmpty(t, result["message"])
}
