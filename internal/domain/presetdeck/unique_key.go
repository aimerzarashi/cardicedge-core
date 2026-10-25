package presetdeck

import (
	"context"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain"
)

// domain_unique_key_types の名前。プリセットデッキどうしで名前を一意にする（カード名とは別の決まり）
var nameKeyType = domain.NewUniqueKeyType("preset_deck.name")

func UniqueKeyTypes() []domain.UniqueKeyType {
	return []domain.UniqueKeyType{nameKeyType}
}

// 比べ方はカード名と同じ（前後の空白を除き、NFKC・小文字）。決まりを 2 つ持たないよう domain.NormalizeName を使う
func NameKey(name Name) domain.UniqueKey {
	return domain.NewUniqueKey(nameKeyType, domain.NormalizeName(name.String()))
}

// 登録にも名前の変更にも使う。ほかのプリセットデッキが同じ名前（正規化後）を使っていれば ErrNameAlreadyInUse
type UpdateNameFunc func(ctx context.Context, id ID, name Name) error
