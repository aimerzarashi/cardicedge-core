package board

import (
	"github.com/google/uuid"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/operator"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/studiorules"
)

type Command interface {
	isBoardCommand()
}

// 名前だけで作る。形は四角形、大きさは DefaultSize を Rules の範囲に収めた値、マスもつながりも無い準備中になる。
// 範囲は、ドメインが既定の値を読まず、コマンドを動かす側が studio_rules を読んで渡す（starterdeck.RegisterCommand の Rules と同じ）
type RegisterCommand struct {
	ID           ID
	PublicID     uuid.UUID
	Name         Name
	Rules        studiorules.Board
	RegisteredBy operator.ID
}

type RenameCommand struct {
	Name      Name
	RenamedBy operator.ID
}

// 入力は変えた後の全体で、イベントの差分は Decide が前の状態と比べて作る（presetdeck.SetCardCountsCommand と同じ）。
// Layout は作る時点でレイアウトの決まりを守っているが、大きさの範囲は Rules で Decide が確かめる
// （範囲を下げた後に、準備中のボードを保存し直すときも新しい範囲で確かめるため）
type ChangeLayoutCommand struct {
	Layout    Layout
	Rules     studiorules.Board
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
