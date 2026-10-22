package readmodel_test

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/card/cardtest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/creature"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/creature/creaturetest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/operator/operatortest"
)

func registeredCreature(t *testing.T, publicID uuid.UUID, name string, at time.Time) []creature.Event {
	t.Helper()
	events, err := creature.Decide(creature.InitialState(), &creature.RegisterCommand{
		ID: creaturetest.ID(t, 1), PublicID: publicID, Name: cardtest.Name(t, name), RegisteredBy: operatortest.ID(t, 10),
	}, at)
	require.NoError(t, err)
	return events
}
