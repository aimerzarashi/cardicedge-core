package grantdeck

import (
	"context"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/presetdeck"
)

// domain_unique_key_types の名前。配布デッキどうしで名前を一意にする（状態によらない。プリセットデッキの名前と同じ扱い）
var nameKeyType = domain.NewUniqueKeyType("grant_deck.name")

func UniqueKeyTypes() []domain.UniqueKeyType {
	return []domain.UniqueKeyType{nameKeyType}
}

// 比べ方はカード名・プリセットデッキと同じ（domain.NormalizeName）
func NameKey(name presetdeck.Name) domain.UniqueKey {
	return domain.NewUniqueKey(nameKeyType, domain.NormalizeName(name.String()))
}

// 登録にも名前の変更にも使う。ほかの配布デッキが同じ名前（正規化後）を使っていれば ErrNameAlreadyInUse。
// 公開・使用不可では触らず、削除の識別子の削除と一緒に外れる
type UpdateNameFunc func(ctx context.Context, id ID, name presetdeck.Name) error
