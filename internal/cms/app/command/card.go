package command

import (
	"errors"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/card"
)

// Creature・Item・Spell の登録で共有する（カード名は 3 種類をまたいで一意）。入力の誤りでなければ InputError は nil
func newCardName(field, value string) (card.Name, InputError) {
	name, err := card.NewName(value)
	switch {
	case errors.Is(err, card.ErrNameEmpty):
		return "", &RequiredInput{Field: field}
	case errors.Is(err, card.ErrNameTooLong):
		return "", &TooLongInput{Field: field, Max: card.MaxNameLength}
	case err != nil:
		return "", &InvalidFormatInput{Field: field}
	}
	return name, nil
}
