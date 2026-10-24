package query_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/game/application/query"
)

func TestPlayerHandler_StarterGrant(t *testing.T) {
	tests := []struct {
		name   string
		status query.StarterGrantStatus
		found  bool
		want   query.StarterGrantStatus
	}{
		{name: "no_row_or_null", found: false, want: query.StarterGrantInProgress},
		{name: "in_progress", status: query.StarterGrantInProgress, found: true, want: query.StarterGrantInProgress},
		{name: "completed", status: query.StarterGrantCompleted, found: true, want: query.StarterGrantCompleted},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			get := func(context.Context, uuid.UUID) (query.StarterGrantStatus, bool, error) {
				return tt.status, tt.found, nil
			}

			got, err := query.NewPlayerHandler(get).StarterGrant(t.Context(), uuid.New())

			require.NoError(t, err)
			require.Equal(t, tt.want, got)
		})
	}
}
