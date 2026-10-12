package constraint

import (
	"context"
	"database/sql"
	"errors"

	"github.com/aarondl/sqlboiler/v4/queries/qm"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/operator"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/orm/sqlboiler/models"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/rdb"
)

// オペレーターのメールアドレスと (iss, sub) の一意性を守る（operators・operator_subjects）
type Operators struct{}

func NewOperators() Operators { return Operators{} }

// 一意制約違反のエラーにするとトランザクションが中断され、続けて既存の行を引けなくなるため、ON CONFLICT DO NOTHING にする。
// sqlboiler の Upsert は DO NOTHING で何も返らない場合の扱いがわかりにくいため、SQL を直接書く
const claimOperatorEmailQuery = `
	INSERT INTO operators (id, email)
	VALUES ($1, $2)
	ON CONFLICT DO NOTHING
	RETURNING id
`

func (Operators) ClaimEmail(ctx context.Context, id operator.ID, email operator.Email) error {
	return claim(ctx, operator.ErrAlreadyRegistered, claimOperatorEmailQuery, id.Int64(), email.String())
}

// 衝突の対象を指定しないのは、(iss, sub) の重複と、同じオペレーターの紐づけ直し（主キーの重複）の両方を断るため
const claimOperatorSubjectQuery = `
	INSERT INTO operator_subjects (id, issuer, subject)
	VALUES ($1, $2, $3)
	ON CONFLICT DO NOTHING
	RETURNING id
`

func (Operators) ClaimSubject(ctx context.Context, id operator.ID, issuer operator.Issuer, subject operator.Subject) error {
	return claim(ctx, operator.ErrAlreadyLinked, claimOperatorSubjectQuery, id.Int64(), issuer.String(), subject.String())
}

func claim(ctx context.Context, conflict error, query string, args ...any) error {
	exec, err := rdb.GetExecutor(ctx)
	if err != nil {
		return err
	}
	var claimed int64
	err = exec.QueryRowContext(ctx, query, args...).Scan(&claimed)
	if errors.Is(err, sql.ErrNoRows) {
		return conflict
	}
	return err
}

func (Operators) FindBySubject(ctx context.Context, issuer operator.Issuer, subject operator.Subject) (operator.ID, bool, error) {
	exec, err := rdb.GetExecutor(ctx)
	if err != nil {
		return 0, false, err
	}
	row, err := models.OperatorSubjects(
		models.OperatorSubjectWhere.Issuer.EQ(issuer.String()),
		models.OperatorSubjectWhere.Subject.EQ(subject.String()),
	).One(ctx, exec)
	return found(row, err, func(r *models.OperatorSubject) int64 { return r.ID })
}

// 紐づけ済みのオペレーターは返さない（同じメールアドレスの別のアカウントに紐づけ直させないため）
func (Operators) FindUnlinkedByEmail(ctx context.Context, email operator.Email) (operator.ID, bool, error) {
	exec, err := rdb.GetExecutor(ctx)
	if err != nil {
		return 0, false, err
	}
	row, err := models.Operators(
		models.OperatorWhere.Email.EQ(email.String()),
		qm.Where("NOT EXISTS (SELECT 1 FROM operator_subjects s WHERE s.id = operators.id)"),
	).One(ctx, exec)
	return found(row, err, func(r *models.Operator) int64 { return r.ID })
}

func (Operators) Exists(ctx context.Context) (bool, error) {
	exec, err := rdb.GetExecutor(ctx)
	if err != nil {
		return false, err
	}
	return models.Operators().Exists(ctx, exec)
}

func found[R any](row R, err error, id func(R) int64) (operator.ID, bool, error) {
	if errors.Is(err, sql.ErrNoRows) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, err
	}
	operatorID, err := operator.NewID(id(row))
	return operatorID, err == nil, err
}
