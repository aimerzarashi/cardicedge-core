package board

import (
	"context"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain"
)

// domain_unique_key_types の名前。ボードどうしで名前を一意にする（状態によらない。配布デッキの名前と同じ扱い）
var nameKeyType = domain.NewUniqueKeyType("board.name")

func UniqueKeyTypes() []domain.UniqueKeyType {
	return []domain.UniqueKeyType{nameKeyType}
}

// 比べ方はカード名・配布デッキと同じ（domain.NormalizeName）
func NameKey(name Name) domain.UniqueKey {
	return domain.NewUniqueKey(nameKeyType, domain.NormalizeName(name.String()))
}

// 登録にも名前の変更にも使う。ほかのボードが同じ名前（正規化後）を使っていれば ErrNameAlreadyInUse。
// 公開・使用不可・レイアウトの変更では触らず、削除の識別子の削除と一緒に外れる
type UpdateNameFunc func(ctx context.Context, id ID, name Name) error
