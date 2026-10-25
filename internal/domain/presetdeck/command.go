package presetdeck

import (
	"github.com/google/uuid"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/gamerules"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/operator"
)

type Command interface {
	isPresetDeckCommand()
}

type RegisterCommand struct {
	ID           ID
	PublicID     uuid.UUID
	Name         Name
	RegisteredBy operator.ID
}

type RenameCommand struct {
	Name      Name
	RenamedBy operator.ID
}

// 入力は「その枚数にする」（画面が選んだ値をそのまま送れ、今の枚数を知らずに済むため）。イベントの差分は Decide が計算する
//
// ルールは、ドメインが既定の値を読まず、コマンドを動かす側が game_rules を読んで渡す（値が変わっても、テストで小さくしても、Decide は変えない）
type SetCardCountsCommand struct {
	Counts    map[CardRef]CardCount
	Rules     gamerules.Deck
	ChangedBy operator.ID
}

type DeleteCommand struct {
	DeletedBy operator.ID
}

func (*RegisterCommand) isPresetDeckCommand()      {}
func (*RenameCommand) isPresetDeckCommand()        {}
func (*SetCardCountsCommand) isPresetDeckCommand() {}
func (*DeleteCommand) isPresetDeckCommand()        {}
