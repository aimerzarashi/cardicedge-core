package matchaction_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/matchaction"
)

func TestNewAction_AcceptsEveryActionAndRejectsUnknownValue(t *testing.T) {
	for _, action := range matchaction.Actions() {
		t.Run(string(action), func(t *testing.T) {
			made, err := matchaction.NewAction(string(action))

			require.NoError(t, err)
			require.Equal(t, action, made)
		})
	}

	_, err := matchaction.NewAction("unknown")
	require.ErrorIs(t, err, matchaction.ErrUnknownAction)
}
