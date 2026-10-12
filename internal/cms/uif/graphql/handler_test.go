package graphql_test

import (
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
	"gitlab.com/aimerzarashi/diecardomain/core/internal/constraint/constrainttest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/card"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/card/cardtest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/creature"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/operator"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/operator/operatortest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domaindef"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/eventstore"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/identifier"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/idp"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/idp/idptest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/rdb"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/rdb/rdbtest"
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
	registry, err := domaindef.Load(rdb.WithDB(t.Context(), db))
	require.NoError(t, err)
	verifier, err := idp.NewVerifier(t.Context(), idptest.NewJWKSServer(t, key).URL, idptest.Issuer, idptest.Audience)
	require.NoError(t, err)
	now := func() time.Time { return time.Now().UTC() }
	operators := command.NewOperatorCommands(
		eventstore.New(registry, operator.NewID, operator.NewEventType, operator.InitialState, operator.Evolve),
		identifier.New(registry, operator.NewID),
		constraint.UpdateOperatorEmail,
		constraint.UpdateOperatorSubject,
		constraint.FindOperatorBySubject,
		constraint.FindUnlinkedOperatorByEmail,
		constraint.OperatorExists,
		now,
	)
	creatures := command.NewCreatureCommands(
		eventstore.New(registry, creature.NewID, creature.NewEventType, creature.InitialState, creature.Evolve),
		identifier.New(registry, creature.NewID),
		constraint.UpdateCreatureName,
		now,
	)
	mux := chi.NewRouter()
	graphql.NewHandler(mux, graphql.NewResolver("test", creatures), db, verifier, idptest.Issuer, operators)
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
	constrainttest.Cleanup(t, f.db, operator.EmailKey(operatortest.Email(t, email)))
	ctx := rdb.WithDB(t.Context(), f.db)
	require.NoError(t, f.operators.Register(ctx, email))
	id, found, err := constraint.FindUnlinkedOperatorByEmail(ctx, operatortest.Email(t, email))
	require.NoError(t, err)
	require.True(t, found)
	return id
}

func (f fixture) linkedOperator(t *testing.T, subject string) (operator.ID, bool) {
	t.Helper()
	id, found, err := constraint.FindOperatorBySubject(rdb.WithDB(t.Context(), f.db), operatortest.Issuer(t, idptest.Issuer), operatortest.Subject(t, subject))
	require.NoError(t, err)
	return id, found
}

// 紐づいたオペレーターの JWT を返す（登録したオペレーターのメールアドレスを確認済みにした JWT で、最初の呼び出しで紐づく）
func (f fixture) operatorToken(t *testing.T, key idptest.Key) string {
	t.Helper()
	suffix := uuid.NewString()
	f.register(t, fmt.Sprintf("handler-%s@example.com", suffix))
	return key.Sign(t, idptest.VerifiedClaims("handler-"+suffix, fmt.Sprintf("handler-%s@example.com", suffix)))
}

// HTTP のリクエストで登録したカードは commit されるため、テストの終わりに消す
func (f fixture) cleanupCardName(t *testing.T, name string) {
	t.Helper()
	constrainttest.Cleanup(t, f.db, card.NameKey(cardtest.Name(t, name)))
}

func registerCreatureQuery(t *testing.T, name string) string {
	t.Helper()
	body, err := json.Marshal(map[string]any{
		"query": `mutation ($input: RegisterCreatureInput!) {
			registerCreature(input: $input) {
				__typename
				... on RegisterCreatureSuccess { creature { id name } }
				... on ValidationErrors { errors { __typename field ... on FieldTooLong { max } } }
			}
		}`,
		"variables": map[string]any{"input": map[string]any{"name": name}},
	})
	require.NoError(t, err)
	return string(body)
}

func registerCreatureResult(t *testing.T, body map[string]any) map[string]any {
	t.Helper()
	require.Nil(t, body["errors"])
	return body["data"].(map[string]any)["registerCreature"].(map[string]any)
}

func TestHandler_RegistersCreatureForOperator(t *testing.T) {
	key := idptest.NewKey(t, "k1")
	f := newFixture(t, key)
	name := "ドラゴン-" + uuid.NewString()
	f.cleanupCardName(t, name)

	status, body := f.post(t, f.operatorToken(t, key), registerCreatureQuery(t, "  "+name+" "))

	require.Equal(t, http.StatusOK, status)
	result := registerCreatureResult(t, body)
	require.Equal(t, "RegisterCreatureSuccess", result["__typename"])
	registered := result["creature"].(map[string]any)
	require.Equal(t, name, registered["name"])
	_, err := uuid.Parse(registered["id"].(string))
	require.NoError(t, err)
}

func TestHandler_ReturnsResultTypeWhenCreatureIsNotRegistered(t *testing.T) {
	key := idptest.NewKey(t, "k1")
	f := newFixture(t, key)
	taken := "ゴブリン-" + uuid.NewString()
	f.cleanupCardName(t, taken)
	_, body := f.post(t, f.operatorToken(t, key), registerCreatureQuery(t, taken))
	require.Equal(t, "RegisterCreatureSuccess", registerCreatureResult(t, body)["__typename"])
	tests := []struct {
		name     string
		token    string
		cardName string
		want     string
	}{
		{name: "not_operator", token: key.Sign(t, idptest.ValidClaims("handler-"+uuid.NewString())), cardName: "スライム-" + uuid.NewString(), want: "ForbiddenError"},
		{name: "blank_name", token: f.operatorToken(t, key), cardName: " ", want: "ValidationErrors"},
		{name: "name_already_in_use", token: f.operatorToken(t, key), cardName: taken, want: "CardNameAlreadyInUseError"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status, body := f.post(t, tt.token, registerCreatureQuery(t, tt.cardName))

			require.Equal(t, http.StatusOK, status)
			require.Equal(t, tt.want, registerCreatureResult(t, body)["__typename"])
		})
	}
}

// 文言は返さず、理由ごとの型と、その理由に要る値（上限）を返す（cms が文言を作る）
func TestHandler_ReturnsFieldErrorPerReason(t *testing.T) {
	key := idptest.NewKey(t, "k1")
	f := newFixture(t, key)
	token := f.operatorToken(t, key)
	tests := []struct {
		name     string
		cardName string
		want     map[string]any
	}{
		{name: "blank", cardName: " ", want: map[string]any{"__typename": "FieldRequired", "field": "name"}},
		{name: "too_long", cardName: strings.Repeat("あ", 51), want: map[string]any{"__typename": "FieldTooLong", "field": "name", "max": float64(50)}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, body := f.post(t, token, registerCreatureQuery(t, tt.cardName))

			result := registerCreatureResult(t, body)
			require.Equal(t, "ValidationErrors", result["__typename"])
			require.Equal(t, []any{tt.want}, result["errors"])
		})
	}
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
