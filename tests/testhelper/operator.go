package testhelper

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/constraint"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/operator"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/operator/operatortest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domaindef"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/eventstore"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/identifier"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/idp/idptest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/rdb"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/studio/app/command"
)

// テストの前提として、オペレーターを用意する。オペレーターを登録する GraphQL の操作がまだ無いため、
// app 層のコマンドを直接呼ぶ（操作ができたら、それを呼ぶように変える）
type Operators struct {
	db       *sql.DB
	key      idptest.Key
	commands *command.OperatorCommands
}

func NewOperators(ctx context.Context, db *sql.DB, key idptest.Key) (*Operators, error) {
	registry, err := domaindef.Load(rdb.WithDB(ctx, db))
	if err != nil {
		return nil, err
	}
	commands := command.NewOperatorCommands(
		eventstore.New(registry, operator.NewID, operator.NewEventType, operator.InitialState, operator.Evolve),
		identifier.New(registry, operator.NewID),
		constraint.UpdateOperatorEmail,
		constraint.UpdateOperatorSubject,
		constraint.FindOperatorBySubject,
		constraint.FindUnlinkedOperatorByEmail,
		constraint.OperatorExists,
		func() time.Time { return time.Now().UTC() },
	)
	return &Operators{db: db, key: key, commands: commands}, nil
}

// まだ紐づけていないオペレーターを登録し、その ID を返す
func (o *Operators) Register(t *testing.T, email string) operator.ID {
	t.Helper()
	ctx := rdb.WithDB(t.Context(), o.db)
	require.NoError(t, o.commands.Register(ctx, email))
	id, found, err := constraint.FindUnlinkedOperatorByEmail(ctx, operatortest.Email(t, email))
	require.NoError(t, err)
	require.True(t, found)
	return id
}

// オペレーターを登録し、そのメールアドレスを確認済みにした JWT を返す（最初のリクエストで紐づく）
func (o *Operators) Token(t *testing.T) string {
	t.Helper()
	id := UniqueID(t)
	email := "operator-" + id + "@example.com"
	o.Register(t, email)
	return o.key.Sign(t, idptest.VerifiedClaims("operator-"+id, email))
}

// subject（studio の JWT の発行元）に紐づいたオペレーター
func (o *Operators) Linked(t *testing.T, subject string) (operator.ID, bool) {
	t.Helper()
	id, found, err := constraint.FindOperatorBySubject(rdb.WithDB(t.Context(), o.db), operatortest.Issuer(t, idptest.Issuer), operatortest.Subject(t, subject))
	require.NoError(t, err)
	return id, found
}
