package eventdriven_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domaindef"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/eventdriven"
)

func TestParseEventRef_ReadsNotificationPayload(t *testing.T) {
	ref, err := eventdriven.ParseEventRef("2000:2000:147:3:78")

	require.NoError(t, err)
	require.Equal(t, domaindef.NewEntityTypeID(2000), ref.EntityTypeID)
	require.Equal(t, domaindef.NewEventTypeID(2000), ref.EventTypeID)
	require.Equal(t, int64(147), ref.EntityID)
	require.Equal(t, int64(3), ref.AggregateVersion)
	require.Equal(t, int64(78), ref.EventID)
}

func TestParseEventRef_RejectsInvalidPayload(t *testing.T) {
	tests := []struct {
		name    string
		payload string
	}{
		{name: "empty", payload: ""},
		{name: "too_few_parts", payload: "2000:2000:147:3"},
		{name: "too_many_parts", payload: "2000:2000:147:3:78:1"},
		{name: "not_a_number", payload: "2000:x:147:3:78"},
		{name: "entity_type_out_of_range", payload: "40000:2000:147:3:78"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := eventdriven.ParseEventRef(tt.payload)

			require.ErrorIs(t, err, eventdriven.ErrInvalidPayload)
		})
	}
}
