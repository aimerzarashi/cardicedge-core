package wiring_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/wiring"
)

func TestRouter_Version(t *testing.T) {
	rec := httptest.NewRecorder()
	wiring.NewRouter("v1.2.3").ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/version", nil))

	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "application/json", rec.Header().Get("Content-Type"))

	var body map[string]string
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Equal(t, "v1.2.3", body["version"])
	require.NotEmpty(t, body["timestamp"])
}

func TestRouter_Health(t *testing.T) {
	rec := httptest.NewRecorder()
	wiring.NewRouter("v1.2.3").ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/health", nil))

	require.Equal(t, http.StatusOK, rec.Code)
	require.JSONEq(t, `{"status":"ok"}`, rec.Body.String())
}
