package starterdeckset

import (
	"github.com/google/uuid"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/operator"
)

type Command interface {
	isStarterDeckSetCommand()
}

// 初めての設定も、選び直しも、同じプリセットデッキからの写し直し（反映）も、この 1 つで表す（「この組にする」）。
// PublicID は初めての設定のときだけ使う（発行した公開 ID）
type SetCommand struct {
	ID       ID
	PublicID uuid.UUID
	Decks    []Deck
	SetBy    operator.ID
}

func (*SetCommand) isStarterDeckSetCommand() {}
