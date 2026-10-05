package main

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCheckHealth_OK(t *testing.T) {
	srv := httptest.NewServer(newRouter())
	defer srv.Close()

	require.NoError(t, checkHealth(t.Context(), srv.URL+"/health"))
}

func TestCheckHealth_FailsOnNon200(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	require.Error(t, checkHealth(t.Context(), srv.URL+"/health"))
}

func TestCheckHealth_FailsWhenUnreachable(t *testing.T) {
	require.Error(t, checkHealth(t.Context(), "http://127.0.0.1:1/health"))
}
