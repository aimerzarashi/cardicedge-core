package constraint_test

import (
	"fmt"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/constraint"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/operator"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/operator/operatortest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/identifier"
)

var emailSuffixes sync.Map

// 同時に動くほかのテストのオペレーターとぶつからないよう、テストごとに別のメールアドレスにする（同じテストの中では同じ値）
func email(t *testing.T, local string) string {
	t.Helper()
	suffix, _ := emailSuffixes.LoadOrStore(t.Name(), uuid.NewString())
	return fmt.Sprintf("%s-%s@example.com", local, suffix)
}

func (f fixture) register(t *testing.T, email string) operator.ID {
	t.Helper()
	id, _, err := identifier.New(f.registry, operator.NewID).Issue(f.ctx)
	require.NoError(t, err)
	require.NoError(t, constraint.UpdateOperatorEmail(f.ctx, id, operatortest.Email(t, email)))
	return id
}

func (f fixture) updateSubject(t *testing.T, id operator.ID, issuer, subject string) error {
	t.Helper()
	return constraint.UpdateOperatorSubject(f.ctx, id, operatortest.Issuer(t, issuer), operatortest.Subject(t, subject))
}

func TestUpdateOperatorEmail_RejectsEmailUsedByOtherOperator(t *testing.T) {
	f := setup(t)
	f.register(t, email(t, "operator"))
	id, _, err := identifier.New(f.registry, operator.NewID).Issue(f.ctx)
	require.NoError(t, err)

	err = constraint.UpdateOperatorEmail(f.ctx, id, operatortest.Email(t, email(t, "operator")))

	require.ErrorIs(t, err, operator.ErrAlreadyRegistered)
}

func TestUpdateOperatorEmail_ChangesOwnEmail(t *testing.T) {
	f := setup(t)
	id := f.register(t, email(t, "before"))

	require.NoError(t, constraint.UpdateOperatorEmail(f.ctx, id, operatortest.Email(t, email(t, "after"))))
	// 同じ値にし直しても、ほかのオペレーターが使っているとは扱わない
	require.NoError(t, constraint.UpdateOperatorEmail(f.ctx, id, operatortest.Email(t, email(t, "after"))))

	_, found, err := constraint.FindUnlinkedOperatorByEmail(f.ctx, operatortest.Email(t, email(t, "before")))
	require.NoError(t, err)
	require.False(t, found)
	got, found, err := constraint.FindUnlinkedOperatorByEmail(f.ctx, operatortest.Email(t, email(t, "after")))
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, id, got)
}

func TestUpdateOperatorSubject_RejectsSubjectLinkedToOtherOperator(t *testing.T) {
	f := setup(t)
	require.NoError(t, f.updateSubject(t, f.register(t, email(t, "other")), "diecardomain-studio", "user-1"))

	err := f.updateSubject(t, f.register(t, email(t, "operator")), "diecardomain-studio", "user-1")

	require.ErrorIs(t, err, operator.ErrAlreadyLinked)
}

func TestUpdateOperatorSubject_AllowsSameSubjectFromOtherIssuer(t *testing.T) {
	f := setup(t)
	require.NoError(t, f.updateSubject(t, f.register(t, email(t, "other")), "diecardomain-studio", "user-1"))

	err := f.updateSubject(t, f.register(t, email(t, "operator")), "other-issuer", "user-1")

	require.NoError(t, err)
}

func TestFindBySubject_ReturnsLinkedOperator(t *testing.T) {
	f := setup(t)
	id := f.register(t, email(t, "operator"))
	require.NoError(t, f.updateSubject(t, id, "diecardomain-studio", "user-1"))

	got, found, err := constraint.FindOperatorBySubject(f.ctx, operatortest.Issuer(t, "diecardomain-studio"), operatortest.Subject(t, "user-1"))
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, id, got)

	_, found, err = constraint.FindOperatorBySubject(f.ctx, operatortest.Issuer(t, "diecardomain-studio"), operatortest.Subject(t, "user-2"))
	require.NoError(t, err)
	require.False(t, found)
}

func TestFindUnlinkedByEmail_ReturnsOnlyUnlinkedOperator(t *testing.T) {
	f := setup(t)
	unlinked := f.register(t, email(t, "operator"))
	require.NoError(t, f.updateSubject(t, f.register(t, email(t, "linked")), "diecardomain-studio", "user-1"))

	got, found, err := constraint.FindUnlinkedOperatorByEmail(f.ctx, operatortest.Email(t, email(t, "operator")))
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, unlinked, got)

	_, found, err = constraint.FindUnlinkedOperatorByEmail(f.ctx, operatortest.Email(t, email(t, "linked")))
	require.NoError(t, err)
	require.False(t, found)
}

// 「1 人もいない」は、同時に動くほかのテストに左右されるため、ここでは確かめない
// （オペレーターがいないときの起動時の登録は、application 層のテストで ExistsFunc を差し替えて確かめる）
func TestOperatorExists_ReturnsTrueAfterRegister(t *testing.T) {
	f := setup(t)
	f.register(t, email(t, "operator"))

	exists, err := constraint.OperatorExists(f.ctx)

	require.NoError(t, err)
	require.True(t, exists)
}
