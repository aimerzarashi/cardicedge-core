package npcavatar

import (
	"github.com/google/uuid"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/npcdeck"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/operator"
)

type Command interface {
	isNPCAvatarCommand()
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

// 入力は「この一覧にする」で、割り当て・割り当て解除の差分は Decide が計算する（SetCardCountsCommand と同じ。画面が一覧を持って
// 送るため、差分に直す処理を application に持たせない）。同じデッキを 2 回入れないことは application が入力として確かめる
// （Decide は重複がないものとして扱う）
type SetDecksCommand struct {
	Decks     []npcdeck.ID
	ChangedBy operator.ID
}

type PublishCommand struct {
	PublishedBy operator.ID
}

type RetireCommand struct {
	RetiredBy operator.ID
}

type DeleteCommand struct {
	DeletedBy operator.ID
}

func (*RegisterCommand) isNPCAvatarCommand() {}
func (*RenameCommand) isNPCAvatarCommand()   {}
func (*SetDecksCommand) isNPCAvatarCommand() {}
func (*PublishCommand) isNPCAvatarCommand()  {}
func (*RetireCommand) isNPCAvatarCommand()   {}
func (*DeleteCommand) isNPCAvatarCommand()   {}
