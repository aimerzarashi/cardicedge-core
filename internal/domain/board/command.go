package board

import (
	"github.com/google/uuid"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/operator"
)

type Command interface {
	isBoardCommand()
}

// 名前だけで作る。形は四角形、大きさは DefaultSize、マスもつながりも無い準備中になる
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

// 入力は変えた後の全体で、イベントの差分は Decide が前の状態と比べて作る（presetdeck.SetCardCountsCommand と同じ）。
// Layout は作る時点で決まりを守っている
type ChangeLayoutCommand struct {
	Layout    Layout
	ChangedBy operator.ID
}

// 公開の条件（拠点・チェックポイント・行き着けるか）はレイアウトだけで決まるため、Decide で確かめる
type PublishCommand struct {
	PublishedBy operator.ID
}

type RetireCommand struct {
	RetiredBy operator.ID
}

type DeleteCommand struct {
	DeletedBy operator.ID
}

func (*RegisterCommand) isBoardCommand()     {}
func (*RenameCommand) isBoardCommand()       {}
func (*ChangeLayoutCommand) isBoardCommand() {}
func (*PublishCommand) isBoardCommand()      {}
func (*RetireCommand) isBoardCommand()       {}
func (*DeleteCommand) isBoardCommand()       {}
