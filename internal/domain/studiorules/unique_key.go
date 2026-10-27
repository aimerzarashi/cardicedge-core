package studiorules

import (
	"context"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain"
)

// domain_unique_key_types の名前。studio_rules を 1 つにする（2 つ目を作らせない）
var singletonKeyType = domain.NewUniqueKeyType("studio_rules.singleton")

func UniqueKeyTypes() []domain.UniqueKeyType {
	return []domain.UniqueKeyType{singletonKeyType}
}

// 値は決まった文字列（1 つしか作らないため、何で区別するかは要らない）
func SingletonKey() domain.UniqueKey {
	return domain.NewUniqueKey(singletonKeyType, "singleton")
}

// 作るときに key を確保する。ほかが確保していれば ErrAlreadyCreated
type CreateFunc func(ctx context.Context, id ID) error

// 1 つしかない studio_rules の内部 ID を、key から引く。まだ無ければ false
type FindFunc func(ctx context.Context) (ID, bool, error)
