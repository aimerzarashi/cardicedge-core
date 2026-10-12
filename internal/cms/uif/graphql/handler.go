package graphql

import (
	"context"
	"database/sql"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"

	"github.com/99designs/gqlgen/graphql/handler"
	"github.com/99designs/gqlgen/graphql/handler/transport"
	"github.com/go-chi/chi/v5"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/cms/app/authentication"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/operator"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/idp"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/rdb"
)

type TokenVerifier interface {
	Verify(token string) (idp.Claims, error)
}

// 実装は command.OperatorCommands
type OperatorIdentifier interface {
	Identify(ctx context.Context, issuer operator.Issuer, subject operator.Subject, verifiedEmail *operator.Email) (operator.ID, bool, error)
}

// issuer は verifier が確かめる iss と同じ値
func NewHandler(mux chi.Router, resolver *Resolver, db *sql.DB, verifier TokenVerifier, issuer operator.Issuer, operators OperatorIdentifier) {
	srv := handler.New(NewExecutableSchema(Config{Resolvers: resolver}))
	// GET（クエリ文字列）は受け付けない（ミューテーションを GET で実行させる CSRF を防ぐため。cms のサーバーは POST で呼ぶ）
	srv.AddTransport(transport.POST{})

	mux.Route("/cms/graphql", func(r chi.Router) {
		r.Use(rdb.Middleware(db))
		r.Use(authenticate(verifier, issuer, operators))
		r.Handle("/", srv)
	})
}

// ネットワークの位置（内部からの呼び出しか）を信じず、すべての呼び出しで JWT を確かめる（game からも core に直接届くため。core#7）。
// JWT が正しければ、オペレーターに紐づかなくても拒否しない（オペレーターかどうか・権限は、操作ごとに認可で判定し、結果の型で返すため）
func authenticate(verifier TokenVerifier, issuer operator.Issuer, operators OperatorIdentifier) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
			if !ok || token == "" {
				unauthorized(w)
				return
			}
			claims, err := verifier.Verify(token)
			if err != nil {
				unauthorized(w)
				return
			}
			subject, err := operator.NewSubject(claims.Subject)
			if err != nil {
				unauthorized(w)
				return
			}
			id, found, err := operators.Identify(r.Context(), issuer, subject, verifiedEmail(claims))
			if err != nil {
				slog.ErrorContext(r.Context(), "failed to identify operator", "error", err)
				writeError(w, http.StatusInternalServerError, "internal server error", "INTERNAL_SERVER_ERROR")
				return
			}
			ctx := r.Context()
			if found {
				ctx = authentication.WithOperator(ctx, id)
			}
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// 確認していないメールアドレスでは紐づけない（他人のメールアドレスでサインアップしても、オペレーターになれないように）
func verifiedEmail(claims idp.Claims) *operator.Email {
	if !claims.EmailVerified {
		return nil
	}
	email, err := operator.NewEmail(claims.Email)
	if err != nil {
		return nil
	}
	return &email
}

// 失敗の理由（署名・期限・発行元のどれか）は返さない（攻撃者に手がかりを与えないため）
func unauthorized(w http.ResponseWriter) {
	writeError(w, http.StatusUnauthorized, "unauthenticated", "UNAUTHENTICATED")
}

func writeError(w http.ResponseWriter, status int, message, code string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"errors": []map[string]any{{"message": message, "extensions": map[string]string{"code": code}}},
	})
}
