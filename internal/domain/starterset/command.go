package starterset

import (
	"github.com/google/uuid"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/board"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/grantdeck"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/npcavatar"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/operator"
)

type Command interface {
	isStarterSetCommand()
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

// 入力は「この一覧にする」で、割り当て・割り当て解除の差分は Decide が計算する（npcavatar.SetDecksCommand と同じ）。
// 同じものを 2 回入れないことは application が入力として確かめる（Decide は重複がないものとして扱う）
type SetGrantDecksCommand struct {
	GrantDecks []grantdeck.ID
	ChangedBy  operator.ID
}

type SetUnlockBoardsCommand struct {
	UnlockBoards []board.ID
	ChangedBy    operator.ID
}

type SetUnlockNPCAvatarsCommand struct {
	UnlockNPCAvatars []npcavatar.ID
	ChangedBy        operator.ID
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

func (*RegisterCommand) isStarterSetCommand()            {}
func (*RenameCommand) isStarterSetCommand()              {}
func (*SetGrantDecksCommand) isStarterSetCommand()       {}
func (*SetUnlockBoardsCommand) isStarterSetCommand()     {}
func (*SetUnlockNPCAvatarsCommand) isStarterSetCommand() {}
func (*PublishCommand) isStarterSetCommand()             {}
func (*RetireCommand) isStarterSetCommand()              {}
func (*DeleteCommand) isStarterSetCommand()              {}
