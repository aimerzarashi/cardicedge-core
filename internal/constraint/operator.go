package constraint

import (
	"context"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/operator"
)

var (
	_ operator.UpdateEmailFunc         = UpdateOperatorEmail
	_ operator.UpdateSubjectFunc       = UpdateOperatorSubject
	_ operator.FindBySubjectFunc       = FindOperatorBySubject
	_ operator.FindByEmailFunc         = FindOperatorByEmail
	_ operator.FindUnlinkedByEmailFunc = FindUnlinkedOperatorByEmail
	_ operator.ExistsFunc              = OperatorExists
)

func UpdateOperatorEmail(ctx context.Context, id operator.ID, email operator.Email) error {
	return updateUniqueKey(ctx, id.Int64(), operator.EmailKey(email), operator.ErrAlreadyRegistered)
}

func UpdateOperatorSubject(ctx context.Context, id operator.ID, issuer operator.Issuer, subject operator.Subject) error {
	return updateUniqueKey(ctx, id.Int64(), operator.SubjectKey(issuer, subject), operator.ErrAlreadyLinked)
}

func FindOperatorBySubject(ctx context.Context, issuer operator.Issuer, subject operator.Subject) (operator.ID, bool, error) {
	return operatorID(findUniqueKeyOwner(ctx, operator.SubjectKey(issuer, subject)))
}

func FindOperatorByEmail(ctx context.Context, email operator.Email) (operator.ID, bool, error) {
	return operatorID(findUniqueKeyOwner(ctx, operator.EmailKey(email)))
}

// 紐づけ済み（(iss, sub) の key を持つ）オペレーターは返さない（同じメールアドレスの別のアカウントに紐づけ直させないため）
func FindUnlinkedOperatorByEmail(ctx context.Context, email operator.Email) (operator.ID, bool, error) {
	return operatorID(findUniqueKeyOwnerWithout(ctx, operator.EmailKey(email), operator.SubjectKeyType))
}

// オペレーターは登録のときに必ずメールアドレスの key を確保するため、その有無で数える
func OperatorExists(ctx context.Context) (bool, error) {
	return existsOfKeyType(ctx, operator.EmailKeyType)
}

func operatorID(id int64, found bool, err error) (operator.ID, bool, error) {
	if err != nil || !found {
		return 0, false, err
	}
	operatorID, err := operator.NewID(id)
	return operatorID, err == nil, err
}
