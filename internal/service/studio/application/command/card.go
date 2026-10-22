package command

import (
	"errors"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/card"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/studio/application/input"
)

// Creature・Item・Spell の登録で共有する（カード名は 3 種類をまたいで一意）。入力の誤りでなければ input.Error は nil
func newCardName(field, value string) (card.Name, input.Error) {
	name, err := card.NewName(value)
	switch {
	case errors.Is(err, card.ErrNameEmpty):
		return "", &input.Required{Field: field}
	case errors.Is(err, card.ErrNameTooLong):
		return "", &input.TooLong{Field: field, Max: card.MaxNameLength}
	case err != nil:
		return "", &input.InvalidFormat{Field: field}
	}
	return name, nil
}
