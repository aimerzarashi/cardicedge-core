package rdb

import (
	"database/sql"
	"net/http"
)

func Middleware(db *sql.DB) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			next.ServeHTTP(w, r.WithContext(WithDB(r.Context(), db)))
		})
	}
}
