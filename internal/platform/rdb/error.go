package rdb

import (
	"errors"

	"github.com/lib/pq"
)

var ErrNoDBConnection = errors.New("rdb: no db connection in context")

func IsUniqueViolation(err error) bool {
	var pgErr *pq.Error
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}
