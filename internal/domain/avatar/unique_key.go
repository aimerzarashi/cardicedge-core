package avatar

import (
	"context"
	"fmt"
	"strconv"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/player"
)

// domain_unique_key_types の名前。同じ持ち主のアバターの名前を一意にする
var NameKeyType = domain.NewUniqueKeyType("avatar.name")

func UniqueKeyTypes() []domain.UniqueKeyType {
	return []domain.UniqueKeyType{NameKeyType}
}

// 持ち主の ID の桁数を前に付けてつなぐ理由は deck.NameKey と同じ。名前の比べ方は domain.NormalizeName
func NameKey(owner player.ID, name Name) domain.UniqueKey {
	ownerText := strconv.FormatInt(owner.Int64(), 10)
	return domain.NewUniqueKey(NameKeyType, fmt.Sprintf("%d:%s%s", len(ownerText), ownerText, domain.NormalizeName(name.String())))
}

// 同じ持ち主の別のアバターが同じ名前（正規化後）を使っていれば ErrNameAlreadyInUse
type UpdateNameFunc func(ctx context.Context, id ID, owner player.ID, name Name) error
