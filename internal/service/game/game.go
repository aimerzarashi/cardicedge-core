// game 用 GraphQL の部品（リゾルバー・入口）
package game

import (
	"database/sql"
	"io"

	"github.com/go-chi/chi/v5"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/idp"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/game/presentation/graphql"
)

// JWT の検証は外の世界（発行元の JWKS）に触れるため、作ったものを受け取る（テストでは idptest の鍵で作る）
type Config struct {
	Verifier *idp.Verifier
}

type Game struct {
	config Config
}

func New(config Config) *Game {
	return &Game{config: config}
}

// db は game 用の DB（core_game）。game の入口の操作は、この DB だけを読み書きする（core#33）
func (g *Game) Mount(mux chi.Router, db *sql.DB, version string) {
	graphql.NewHandler(mux, graphql.NewResolver(version), db, g.config.Verifier)
}

// `app schema game` が書く（入口の presentation の graphql パッケージは studio と名前が重なり、cmd/app で並べて import できないため、
// サービスのパッケージから出す）
func WriteSchema(w io.Writer) {
	graphql.WriteSchema(w)
}
