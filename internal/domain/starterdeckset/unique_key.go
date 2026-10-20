package starterdeckset

import (
	"context"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain"
)

// domain_unique_key_types の名前。スターターデッキセットをゲーム全体で 1 つにする（2 つ目を作らせない）
var singletonKeyType = domain.NewUniqueKeyType("starter_deck_set.singleton")

func UniqueKeyTypes() []domain.UniqueKeyType {
	return []domain.UniqueKeyType{singletonKeyType}
}

// 値は決まった文字列（1 つしか作らないため、何で区別するかは要らない）
func SingletonKey() domain.UniqueKey {
	return domain.NewUniqueKey(singletonKeyType, "singleton")
}

// 初めての設定で key を確保する。ほかのスターターデッキセットが確保していれば ErrAlreadyCreated
type CreateFunc func(ctx context.Context, id ID) error

// 1 つしかないスターターデッキセットの内部 ID を、key から引く。まだ無ければ false
type FindFunc func(ctx context.Context) (ID, bool, error)
