// game 用 GraphQL の部品（リゾルバー・入口）
package game

import (
	"database/sql"
	"io"
	"time"

	"github.com/go-chi/chi/v5"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/constraint"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/player"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domaindef"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/eventstore"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/identifier"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/idp"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/game/application/command"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/game/presentation/graphql"
)

// JWT の検証は外の世界（発行元の JWKS）に触れるため、作ったものを受け取る（テストでは idptest の鍵で作る）
type Config struct {
	Verifier *idp.Verifier
	Issuer   string
}

type Game struct {
	config  Config
	players *command.PlayerHandler
}

func New(registry *domaindef.Registry, config Config) *Game {
	// DB（timestamptz）はマイクロ秒までしか持たないため、そろえておく（studio と同じ）
	now := func() time.Time {
		return time.Now().UTC().Truncate(time.Microsecond)
	}
	return &Game{
		config: config,
		players: command.NewPlayerHandler(
			eventstore.New(registry, player.NewID, player.NewEventType, player.InitialState, player.Evolve),
			identifier.New(registry, player.NewID),
			constraint.UpdatePlayerSubject,
			constraint.FindPlayerBySubject,
			now,
		),
	}
}

// db は game 用の DB（core_game）。game の入口の操作は、この DB だけを読み書きする（core#33）
func (g *Game) Mount(mux chi.Router, db *sql.DB) {
	graphql.NewHandler(mux, graphql.NewResolver(g.players), db, g.config.Verifier, g.config.Issuer, g.players)
}

// `app schema game` が書く（入口の presentation の graphql パッケージは studio と名前が重なり、cmd/app で並べて import できないため、
// サービスのパッケージから出す）
func WriteSchema(w io.Writer) {
	graphql.WriteSchema(w)
}
