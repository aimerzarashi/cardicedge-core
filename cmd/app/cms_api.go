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
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/operator"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/eventstore"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/identifier"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/config"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/idp"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/rdb"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/typeregistry"
)

func mountCMSAPI(ctx context.Context, mux chi.Router, db *sql.DB, registry *typeregistry.Registry, cfg config.Config) error {
	issuer, err := operator.NewIssuer(cfg.CMS.Issuer)
	if err != nil {
		return err
	}
	initialEmail, err := operator.NewEmail(cfg.InitialOperatorEmail)
	if err != nil {
		return err
	}
	verifier, err := idp.NewVerifier(ctx, cfg.CMS.JWKSURL, cfg.CMS.Issuer, cfg.CMS.Audience)
	if err != nil {
		return err
	}
	operators := command.NewOperatorCommands(
		eventstore.New(registry, operator.NewID, operator.NewEventType, operator.InitialState, operator.Evolve),
		identifier.New(registry, operator.NewID),
		constraint.NewOperators(),
		func() time.Time { return time.Now().UTC() },
	)
	// 最初のオペレーターを作る権限を持つ人はまだいないため、GraphQL（認可）を通さずにコマンドを直接呼ぶ
	registered, err := operators.RegisterInitial(rdb.WithDB(ctx, db), initialEmail)
	if err != nil {
		return err
	}
	if registered {
		slog.InfoContext(ctx, "registered initial operator", "email", initialEmail.String())
	}
	graphql.NewHandler(mux, graphql.NewResolver(version), db, verifier, issuer, operators)
	return nil
}
