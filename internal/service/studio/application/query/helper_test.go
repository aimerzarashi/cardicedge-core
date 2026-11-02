package query_test

import (
	"context"

	"github.com/google/uuid"
)

func ptr[T any](value T) *T {
	return &value
}

func noRevisions(context.Context, []uuid.UUID) (map[uuid.UUID]int64, error) {
	return map[uuid.UUID]int64{}, nil
}
