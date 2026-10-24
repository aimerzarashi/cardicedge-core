package game_test

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestRegisterPlayer_RegistersPlayerOnFirstCall(t *testing.T) {
	t.Parallel()

	id := registeredPlayerID(t, registerPlayer(t, newPlayerToken(t)))

	_, err := uuid.Parse(id)
	require.NoError(t, err)
}

func TestRegisterPlayer_ReturnsSamePlayerWhenCalledAgain(t *testing.T) {
	t.Parallel()
	token := newPlayerToken(t)
	first := registeredPlayerID(t, registerPlayer(t, token))

	second := registeredPlayerID(t, registerPlayer(t, token))

	require.Equal(t, first, second)
}

func TestRegisterPlayer_RegistersOtherPlayerForOtherSubject(t *testing.T) {
	t.Parallel()

	first := registeredPlayerID(t, registerPlayer(t, newPlayerToken(t)))
	second := registeredPlayerID(t, registerPlayer(t, newPlayerToken(t)))

	require.NotEqual(t, first, second)
}

func TestMe_ReturnsErrorTypeWhenPlayerNotRegistered(t *testing.T) {
	t.Parallel()

	result := me(t, newPlayerToken(t))

	require.Equal(t, "PlayerNotRegisteredError", result["__typename"])
	require.NotEmpty(t, result["message"])
}

func TestMe_ReturnsRegisteredPlayer(t *testing.T) {
	t.Parallel()
	token := newPlayerToken(t)
	id := registeredPlayerID(t, registerPlayer(t, token))

	result := me(t, token)

	require.Equal(t, "Player", result["__typename"])
	require.Equal(t, id, result["id"])
}
