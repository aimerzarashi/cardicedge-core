package graphql

import (
	"context"
	"net/http"

	"github.com/google/uuid"
	"github.com/vikstrous/dataloadgen"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/studio/app/query"
)

// 実装は query.CardHandler
type CardGetter interface {
	Get(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID]query.Card, error)
}

type loadersKey struct{}

type loaders struct {
	cards *dataloadgen.Loader[uuid.UUID, query.Card]
}

// ローダーはリクエストごとに作る（キャッシュを別のリクエスト・オペレーターと共有しないため。gqlgen の文書と同じ形）。
// NewMappedLoader にするのは、読み出しの結果を鍵の順に並べ直さずに済み、無いカードが ErrNotFound になるため（core#24）
func withLoaders(cards CardGetter) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			l := &loaders{cards: dataloadgen.NewMappedLoader(cards.Get)}
			next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), loadersKey{}, l)))
		})
	}
}

func cardLoader(ctx context.Context) *dataloadgen.Loader[uuid.UUID, query.Card] {
	return ctx.Value(loadersKey{}).(*loaders).cards
}
