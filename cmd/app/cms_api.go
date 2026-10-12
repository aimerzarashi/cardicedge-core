package main

import (
	"context"
	"database/sql"
	"log/slog"
	"time"

	"github.com/go-chi/chi/v5"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/cms/app/command"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/cms/uif/graphql"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/constraint"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/creature"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/operator"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domaindef"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/eventstore"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/identifier"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/config"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/idp"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/rdb"
)

// cms 用 GraphQL の部品。作る・初期オペレーターを登録する・入口を付けるを分けるのは、
// 起動の手順（DB への書き込みを含む）を run だけで読めるようにし、入口を付けない起動の形（ワーカー等）でも登録できるようにするため
type cmsAPI struct {
	verifier  *idp.Verifier
	issuer    string
	operators *command.OperatorCommands
	creatures *command.CreatureCommands
}

func newCMSAPI(ctx context.Context, registry *domaindef.Registry, cfg config.Issuer) (cmsAPI, error) {
	verifier, err := idp.NewVerifier(ctx, cfg.JWKSURL, cfg.Issuer, cfg.Audience)
	if err != nil {
		return cmsAPI{}, err
	}
	now := func() time.Time { return time.Now().UTC() }
	return cmsAPI{
		verifier: verifier,
		issuer:   cfg.Issuer,
		operators: command.NewOperatorCommands(
			eventstore.New(registry, operator.NewID, operator.NewEventType, operator.InitialState, operator.Evolve),
			identifier.New(registry, operator.NewID),
			constraint.UpdateOperatorEmail,
			constraint.UpdateOperatorSubject,
			constraint.FindOperatorBySubject,
			constraint.FindUnlinkedOperatorByEmail,
			constraint.OperatorExists,
			now,
		),
		creatures: command.NewCreatureCommands(
			eventstore.New(registry, creature.NewID, creature.NewEventType, creature.InitialState, creature.Evolve),
			identifier.New(registry, creature.NewID),
			constraint.UpdateCreatureName,
			now,
		),
	}, nil
}

// 最初のオペレーターを作る権限を持つ人はまだいないため、GraphQL（認可）を通さずにコマンドを直接呼ぶ
func (a cmsAPI) registerInitialOperator(ctx context.Context, db *sql.DB, email string) error {
	registered, err := a.operators.RegisterInitial(rdb.WithDB(ctx, db), email)
	if err != nil {
		return err
	}
	if registered {
		slog.InfoContext(ctx, "registered initial operator", "email", email)
	}
	return nil
}

func (a cmsAPI) mount(mux chi.Router, db *sql.DB) {
	graphql.NewHandler(mux, graphql.NewResolver(version, a.creatures), db, a.verifier, a.issuer, a.operators)
}
