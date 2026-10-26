package deck

import (
	"context"
	"fmt"
	"strconv"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/player"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/startergrant"
)

// domain_unique_key_types の名前。配布の同じ番号のデッキを 1 つにする
var StarterKeyType = domain.NewUniqueKeyType("deck.starter")

// domain_unique_key_types の名前。同じ持ち主のデッキの名前を一意にする
var NameKeyType = domain.NewUniqueKeyType("deck.name")

func UniqueKeyTypes() []domain.UniqueKeyType {
	return []domain.UniqueKeyType{StarterKeyType, NameKeyType}
}

// 値は「配布の ID:番号」
func StarterKey(starterGrantID startergrant.ID, number startergrant.DeckNumber) domain.UniqueKey {
	return domain.NewUniqueKey(StarterKeyType, strconv.FormatInt(starterGrantID.Int64(), 10)+":"+strconv.Itoa(number.Int()))
}

// 同じ配布の同じ番号のデッキがもうあれば ErrAlreadyCreated
type UpdateStarterFunc func(ctx context.Context, id ID, starterGrantID startergrant.ID, number startergrant.DeckNumber) error

// 持ち主の ID の桁数を前に付けてつなぐのは、区切り文字でつなぐと、名前に区切り文字が入ったときに別の組と同じ key になりうるため
// （player.SubjectKey と同じ）。名前の比べ方は domain.NormalizeName（カード名・プリセットデッキと同じ決まり）
func NameKey(playerID player.ID, name Name) domain.UniqueKey {
	playerText := strconv.FormatInt(playerID.Int64(), 10)
	return domain.NewUniqueKey(NameKeyType, fmt.Sprintf("%d:%s%s", len(playerText), playerText, domain.NormalizeName(name.String())))
}

// 同じ持ち主の別のデッキが同じ名前（正規化後）を使っていれば ErrNameAlreadyInUse
type UpdateNameFunc func(ctx context.Context, id ID, playerID player.ID, name Name) error
