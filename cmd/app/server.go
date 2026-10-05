package main

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

// newRouter は /version・/health を提供するルーターを作る。
func newRouter() chi.Router {
	mux := chi.NewRouter()
	mux.Use(middleware.RequestID)
	mux.Use(middleware.Logger)

	mux.Get("/version", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]string{"version": version, "timestamp": time.Now().Format(time.RFC3339)})
	})

	// プロセスが応答できることだけを示す（依存先の状態は見ない）
	mux.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]string{"status": "ok"})
	})

	return mux
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}
