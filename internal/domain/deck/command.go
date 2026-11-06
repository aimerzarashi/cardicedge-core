package deck

import (
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/card"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/player"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/startergrant"
)

type Command interface {
	isDeckCommand()
}

type CreateCommand struct {
	ID     ID
	Player player.ID
	Name   Name
	Cards  []Card
	// 決まりの枚数（game_rules の deck.total_card_count）。合計がこれと違うデッキは作れない
	Total        card.DeckTotalCount
	StarterGrant startergrant.ID
	Number       startergrant.DeckNumber
}

func (*CreateCommand) isDeckCommand() {}
