package graphql

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/99designs/gqlgen/graphql/handler"
	"github.com/99designs/gqlgen/graphql/handler/transport"
	"github.com/go-chi/chi/v5"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/idp"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/rdb"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/game/application/command"
)

type TokenVerifier interface {
	Verify(token string) (idp.Claims, error)
}

// 実装は command.PlayerHandler
type PlayerAuthenticator interface {
	Authenticate(ctx context.Context, issuer, subject string) (context.Context, error)
}

// issuer は verifier が確かめる iss と同じ値
func NewHandler(mux chi.Router, resolver *Resolver, db *sql.DB, verifier TokenVerifier, issuer string, players PlayerAuthenticator) {
	srv := handler.New(NewExecutableSchema(Config{Resolvers: resolver}))
	// GET（クエリ文字列）は受け付けない（ミューテーションを GET で実行させる CSRF を防ぐため。game のサーバーは POST で呼ぶ）
	srv.AddTransport(transport.POST{})

	mux.Route("/game/graphql", func(r chi.Router) {
		r.Use(rdb.Middleware(db))
		r.Use(authenticate(verifier, issuer, players))
		r.Handle("/", srv)
	})
}

// ネットワークの位置（内部からの呼び出しか）を信じず、すべての呼び出しで JWT を確かめる（studio の入口と同じ。core#7）。
// JWT が正しければ、プレイヤーに登録されていなくても拒否しない（初めての registerPlayer が登録するため。登録済みかは me が結果の型で返す）。
// studio の入口と書き方が重なるが、共通の部品にしない（サービスごとに縦に切った塊（core#29）で、認証の後の扱い
// （オペレーターの紐づけ・プレイヤーの特定）がサービスごとに違うため）
func authenticate(verifier TokenVerifier, issuer string, players PlayerAuthenticator) func(http.Handler) http.Handler {
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
			ctx, err := players.Authenticate(r.Context(), issuer, claims.Subject)
			if errors.Is(err, command.ErrInvalidCredentials) {
				unauthorized(w)
				return
			}
			if err != nil {
				slog.ErrorContext(r.Context(), "failed to authenticate player", "error", err)
				writeError(w, http.StatusInternalServerError, "internal server error", "INTERNAL_SERVER_ERROR")
				return
			}
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
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
