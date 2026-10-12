package constraint_test

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/constraint"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/operator"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/operator/operatortest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/identifier"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/rdb/rdbtest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/typeregistry"
)

var operators = constraint.NewOperators()

var emailSuffixes sync.Map

// 開発用の DB には起動時に登録した初期オペレーターがいるため、テストごとに別のメールアドレスにする（同じテストの中では同じ値）
func email(t *testing.T, local string) string {
	t.Helper()
	suffix, _ := emailSuffixes.LoadOrStore(t.Name(), uuid.NewString())
	return fmt.Sprintf("%s-%s@example.com", local, suffix)
}

type fixture struct {
	ctx      context.Context
	registry *typeregistry.Registry
}

func setup(t *testing.T) fixture {
	t.Helper()
	ctx := rdbtest.Tx(t)
	registry, err := typeregistry.Load(ctx)
	require.NoError(t, err)
	return fixture{ctx: ctx, registry: registry}
}

func (f fixture) register(t *testing.T, email string) operator.ID {
	t.Helper()
	id, _, err := identifier.New(f.registry, operator.NewID).Issue(f.ctx)
	require.NoError(t, err)
	require.NoError(t, operators.ClaimEmail(f.ctx, id, operatortest.Email(t, email)))
	return id
}

func (f fixture) claimSubject(t *testing.T, id operator.ID, issuer, subject string) error {
	t.Helper()
	return operators.ClaimSubject(f.ctx, id, operatortest.Issuer(t, issuer), operatortest.Subject(t, subject))
}

func TestClaimEmail_RejectsEmailAlreadyClaimed(t *testing.T) {
	f := setup(t)
	f.register(t, email(t, "operator"))
	id, _, err := identifier.New(f.registry, operator.NewID).Issue(f.ctx)
	require.NoError(t, err)

	err = operators.ClaimEmail(f.ctx, id, operatortest.Email(t, email(t, "operator")))

	require.ErrorIs(t, err, operator.ErrAlreadyRegistered)
}

func TestClaimSubject_RejectsConflict(t *testing.T) {
	tests := []struct {
		name    string
		claimed func(t *testing.T, f fixture) operator.ID
	}{
		{name: "subject_claimed_by_other_operator", claimed: func(t *testing.T, f fixture) operator.ID {
			require.NoError(t, f.claimSubject(t, f.register(t, email(t, "other")), "diecardomain-cms", "user-1"))
			return f.register(t, email(t, "operator"))
		}},
		{name: "operator_already_linked", claimed: func(t *testing.T, f fixture) operator.ID {
			id := f.register(t, email(t, "operator"))
			require.NoError(t, f.claimSubject(t, id, "diecardomain-cms", "user-2"))
			return id
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := setup(t)
			id := tt.claimed(t, f)

			err := f.claimSubject(t, id, "diecardomain-cms", "user-1")

			require.ErrorIs(t, err, operator.ErrAlreadyLinked)
		})
	}
}

func TestClaimSubject_AllowsSameSubjectFromOtherIssuer(t *testing.T) {
	f := setup(t)
	require.NoError(t, f.claimSubject(t, f.register(t, email(t, "other")), "diecardomain-cms", "user-1"))

	err := f.claimSubject(t, f.register(t, email(t, "operator")), "other-issuer", "user-1")

	require.NoError(t, err)
}

func TestFindBySubject_ReturnsLinkedOperator(t *testing.T) {
	f := setup(t)
	id := f.register(t, email(t, "operator"))
	require.NoError(t, f.claimSubject(t, id, "diecardomain-cms", "user-1"))

	got, found, err := operators.FindBySubject(f.ctx, operatortest.Issuer(t, "diecardomain-cms"), operatortest.Subject(t, "user-1"))
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, id, got)

	_, found, err = operators.FindBySubject(f.ctx, operatortest.Issuer(t, "diecardomain-cms"), operatortest.Subject(t, "user-2"))
	require.NoError(t, err)
	require.False(t, found)
}

func TestFindUnlinkedByEmail_ReturnsOnlyUnlinkedOperator(t *testing.T) {
	f := setup(t)
	unlinked := f.register(t, email(t, "operator"))
	require.NoError(t, f.claimSubject(t, f.register(t, email(t, "linked")), "diecardomain-cms", "user-1"))

	got, found, err := operators.FindUnlinkedByEmail(f.ctx, operatortest.Email(t, email(t, "operator")))
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, unlinked, got)

	_, found, err = operators.FindUnlinkedByEmail(f.ctx, operatortest.Email(t, email(t, "linked")))
	require.NoError(t, err)
	require.False(t, found)
}
