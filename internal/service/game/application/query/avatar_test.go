package query_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/game/application/query"
)

func TestAvatarHandler_ListByPlayer(t *testing.T) {
	first := query.Avatar{ID: uuid.New(), Name: "アリス"}
	second := query.Avatar{ID: uuid.New(), Name: "ボブ"}
	tests := []struct {
		name string
		rows []query.Avatar
		want []query.Avatar
	}{
		{name: "in_given_order", rows: []query.Avatar{first, second}, want: []query.Avatar{first, second}},
		{name: "none_is_empty_list", rows: nil, want: []query.Avatar{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			list := func(context.Context, uuid.UUID) ([]query.Avatar, error) { return tt.rows, nil }

			got, err := query.NewAvatarHandler(list).ListByPlayer(t.Context(), uuid.New())

			require.NoError(t, err)
			require.Equal(t, tt.want, got)
		})
	}
}
