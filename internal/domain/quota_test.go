package domain_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain"
)

func TestNewQuotaLimit_RequiresAtLeastOne(t *testing.T) {
	tests := []struct {
		name  string
		value int
		err   error
	}{
		{name: "negative", value: -1, err: domain.ErrInvalidQuotaLimit},
		{name: "zero", value: 0, err: domain.ErrInvalidQuotaLimit},
		{name: "one", value: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			limit, err := domain.NewQuotaLimit(tt.value)

			require.ErrorIs(t, err, tt.err)
			if tt.err == nil {
				require.Equal(t, tt.value, limit.Int())
			}
		})
	}
}
