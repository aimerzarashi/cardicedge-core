package deck

import (
	"context"
	"strconv"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/startergrant"
)

// domain_unique_key_types の名前。配布の同じ番号のデッキを 1 つにする
var StarterKeyType = domain.NewUniqueKeyType("deck.starter")

func UniqueKeyTypes() []domain.UniqueKeyType {
	return []domain.UniqueKeyType{StarterKeyType}
}

// 値は「配布の ID:番号」
func StarterKey(grant startergrant.ID, number startergrant.DeckNumber) domain.UniqueKey {
	return domain.NewUniqueKey(StarterKeyType, strconv.FormatInt(grant.Int64(), 10)+":"+strconv.Itoa(number.Int()))
}

// 同じ配布の同じ番号のデッキがもうあれば ErrAlreadyCreated
type UpdateStarterFunc func(ctx context.Context, id ID, grant startergrant.ID, number startergrant.DeckNumber) error
