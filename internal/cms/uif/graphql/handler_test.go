package graphql_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/cms/app/command"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/cms/uif/graphql"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/constraint"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/operator"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/operator/operatortest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/eventstore"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/identifier"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/idp"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/idp/idptest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/rdb"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/rdb/rdbtest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/typeregistry"
)

const versionQuery = `{"query":"{ version }"}`

type fixture struct {
	server    *httptest.Server
	db        *sql.DB
	operators *command.OperatorCommands
}

func newFixture(t *testing.T, key idptest.Key) fixture {
	t.Helper()
	db := rdbtest.Open(t)
	registry, err := typeregistry.Load(rdb.WithDB(t.Context(), db))
	require.NoError(t, err)
	verifier, err := idp.NewVerifier(t.Context(), idptest.NewJWKSServer(t, key).URL, idptest.Issuer, idptest.Audience)
	require.NoError(t, err)
	operators := command.NewOperatorCommands(
		eventstore.New(registry, operator.NewID, operator.NewEventType, operator.InitialState, operator.Evolve),
		identifier.New(registry, operator.NewID),
		constraint.NewOperators(),
		func() time.Time { return time.Now().UTC() },
	)
	mux := chi.NewRouter()
	graphql.NewHandler(mux, graphql.NewResolver("test"), db, verifier, operatortest.Issuer(t, idptest.Issuer), operators)
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	return fixture{server: server, db: db, operators: operators}
}

func (f fixture) post(t *testing.T, token, body string) (int, map[string]any) {
	t.Helper()
	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, f.server.URL+"/cms/graphql", strings.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	res, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer res.Body.Close()
	var decoded map[string]any
	require.NoError(t, json.NewDecoder(res.Body).Decode(&decoded))
	return res.StatusCode, decoded
}

// HTTP のリクエストで作った行は commit されるため、テストの終わりに消す。登録した ID を返す
func (f fixture) register(t *testing.T, email string) operator.ID {
	t.Helper()
	t.Cleanup(func() {
		ctx := context.WithoutCancel(t.Context())
		_, _ = f.db.ExecContext(ctx, `DELETE FROM domain_events WHERE entity_type_id = 1000 AND entity_id IN (SELECT id FROM operators WHERE email = $1)`, email)
		_, _ = f.db.ExecContext(ctx, `DELETE FROM domain_identifiers WHERE id IN (SELECT id FROM operators WHERE email = $1)`, email)
	})
	id, err := f.operators.Register(rdb.WithDB(t.Context(), f.db), operatortest.Email(t, email))
	require.NoError(t, err)
	return id
}

func (f fixture) linkedOperator(t *testing.T, subject string) (operator.ID, bool) {
	t.Helper()
	id, found, err := constraint.NewOperators().FindBySubject(rdb.WithDB(t.Context(), f.db), operatortest.Issuer(t, idptest.Issuer), operatortest.Subject(t, subject))
	require.NoError(t, err)
	return id, found
}

func TestHandler_LinksOperatorByVerifiedEmailOnFirstRequest(t *testing.T) {
	key := idptest.NewKey(t, "k1")
	f := newFixture(t, key)
	suffix := uuid.NewString()
	email, subject := fmt.Sprintf("handler-%s@example.com", suffix), "handler-"+suffix
	registered := f.register(t, email)

	status, body := f.post(t, key.Sign(t, idptest.VerifiedClaims(subject, email)), versionQuery)

	require.Equal(t, http.StatusOK, status)
	require.Nil(t, body["errors"])
	linked, found := f.linkedOperator(t, subject)
	require.True(t, found)
	require.Equal(t, registered, linked)
}

// オペレーターかどうか・権限は操作ごとに認可で判定し、結果の型で返すため、入口では拒否しない
func TestHandler_AcceptsValidTokenOfNonOperator(t *testing.T) {
	key := idptest.NewKey(t, "k1")
	f := newFixture(t, key)
	suffix := uuid.NewString()
	email, subject := fmt.Sprintf("handler-%s@example.com", suffix), "handler-"+suffix
	f.register(t, email)
	unverified := idptest.ValidClaims(subject)
	unverified.Email = email
	tests := []struct {
		name   string
		claims idptest.Claims
	}{
		{name: "without_email", claims: idptest.ValidClaims(subject)},
		{name: "unverified_email", claims: unverified},
		{name: "unregistered_email", claims: idptest.VerifiedClaims(subject, "unregistered-"+email)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status, body := f.post(t, key.Sign(t, tt.claims), versionQuery)

			require.Equal(t, http.StatusOK, status)
			require.Equal(t, "test", body["data"].(map[string]any)["version"])
			_, found := f.linkedOperator(t, subject)
			require.False(t, found)
		})
	}
}

func TestHandler_RejectsRequestWithoutValidToken(t *testing.T) {
	key := idptest.NewKey(t, "k1")
	f := newFixture(t, key)
	otherIssuer := idptest.ValidClaims("user-1")
	otherIssuer.Issuer = "diecardomain-game"
	tests := []struct {
		name  string
		token string
	}{
		{name: "without_token", token: ""},
		{name: "malformed_token", token: "not-a-jwt"},
		{name: "signed_by_other_key", token: idptest.NewKey(t, "k1").Sign(t, idptest.ValidClaims("user-1"))},
		{name: "issued_by_other_issuer", token: key.Sign(t, otherIssuer)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status, body := f.post(t, tt.token, versionQuery)

			require.Equal(t, http.StatusUnauthorized, status)
			require.Equal(t, "UNAUTHENTICATED", body["errors"].([]any)[0].(map[string]any)["extensions"].(map[string]any)["code"])
		})
	}
}

func TestHandler_RejectsGetRequest(t *testing.T) {
	key := idptest.NewKey(t, "k1")
	f := newFixture(t, key)
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, f.server.URL+"/cms/graphql?query=%7Bversion%7D", nil)
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+key.Sign(t, idptest.ValidClaims("user-1")))

	res, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer res.Body.Close()

	require.NotEqual(t, http.StatusOK, res.StatusCode)
}
