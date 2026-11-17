package element_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/element"
)

func TestNewCreature_AcceptsTheFiveElements(t *testing.T) {
	for _, value := range []string{"fire", "earth", "water", "wind", "neutral"} {
		creature, err := element.NewCreature(value)

		require.NoError(t, err)
		require.Equal(t, value, creature.String())
	}
}

func TestNewCreature_RejectsMulti(t *testing.T) {
	_, err := element.NewCreature("multi")

	require.ErrorIs(t, err, element.ErrCreatureUnknown)
}

func TestNewLand_AcceptsTheSixElements(t *testing.T) {
	for _, value := range []string{"fire", "earth", "water", "wind", "neutral", "multi"} {
		land, err := element.NewLand(value)

		require.NoError(t, err)
		require.Equal(t, value, land.String())
	}
}

func TestNewLand_RejectsUnknown(t *testing.T) {
	_, err := element.NewLand("light")

	require.ErrorIs(t, err, element.ErrLandUnknown)
}
