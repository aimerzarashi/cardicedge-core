package graphql

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/99designs/gqlgen/graphql/handler"
	"github.com/99designs/gqlgen/graphql/handler/transport"
	"github.com/go-chi/chi/v5"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/idp"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/rdb"
)

type TokenVerifier interface {
	Verify(token string) (idp.Claims, error)
}

func NewHandler(mux chi.Router, resolver *Resolver, db *sql.DB, verifier TokenVerifier) {
	srv := handler.New(NewExecutableSchema(Config{Resolvers: resolver}))
	// GET（クエリ文字列）は受け付けない（ミューテーションを GET で実行させる CSRF を防ぐため。game のサーバーは POST で呼ぶ）
	srv.AddTransport(transport.POST{})

	mux.Route("/game/graphql", func(r chi.Router) {
		r.Use(rdb.Middleware(db))
		r.Use(authenticate(verifier))
		r.Handle("/", srv)
	})
}

// ネットワークの位置（内部からの呼び出しか）を信じず、すべての呼び出しで JWT を確かめる（studio の入口と同じ。core#7）。
// JWT からプレイヤーを特定して context に載せるのは、プレイヤーの集約を足すときに足す（今は確かめるだけ。core#34）。
// studio の入口と書き方が重なるが、共通の部品にしない（サービスごとに縦に切った塊（core#29）で、認証の後の扱い
// （オペレーターの紐づけ・プレイヤーの特定）がサービスごとに違うため）
func authenticate(verifier TokenVerifier) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
			if !ok || token == "" {
				unauthorized(w)
				return
			}
			if _, err := verifier.Verify(token); err != nil {
				unauthorized(w)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// 失敗の理由（署名・期限・発行元のどれか）は返さない（攻撃者に手がかりを与えないため）
func unauthorized(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusUnauthorized)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"errors": []map[string]any{{"message": "unauthenticated", "extensions": map[string]string{"code": "UNAUTHENTICATED"}}},
	})
}
