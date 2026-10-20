package presetdeck

import (
	"github.com/google/uuid"

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
type SetCardCountsCommand struct {
	Counts    map[CardRef]CardCount
	ChangedBy operator.ID
}

type DeleteCommand struct {
	DeletedBy operator.ID
}

func (*RegisterCommand) isPresetDeckCommand()      {}
func (*RenameCommand) isPresetDeckCommand()        {}
func (*SetCardCountsCommand) isPresetDeckCommand() {}
func (*DeleteCommand) isPresetDeckCommand()        {}
