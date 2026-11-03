package query_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/game/application/query"
)

func TestUnlockHandler_ListBoardsByPlayer(t *testing.T) {
	first := query.Board{ID: uuid.New(), Name: "チュートリアル"}
	second := query.Board{ID: uuid.New(), Name: "周回"}
	tests := []struct {
		name string
		rows []query.Board
		want []query.Board
	}{
		{name: "in_given_order", rows: []query.Board{second, first}, want: []query.Board{second, first}},
		{name: "none_is_empty_list", rows: nil, want: []query.Board{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			list := func(context.Context, uuid.UUID) ([]query.Board, error) { return tt.rows, nil }

			got, err := query.NewUnlockHandler(list).ListBoardsByPlayer(t.Context(), uuid.New())

			require.NoError(t, err)
			require.Equal(t, tt.want, got)
		})
	}
}
