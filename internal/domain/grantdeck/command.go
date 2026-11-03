package grantdeck

import (
	"github.com/google/uuid"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/gamerules"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/operator"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/presetdeck"
)

type Command interface {
	isGrantDeckCommand()
}

// 空から作るときは Source も Cards も空。プリセットデッキから写して作るときは、名前・カード・写した元を渡す。
// 準備中は決まり（合計の枚数等）を満たさなくてよいため、写すカードにルールの確かめはしない
type RegisterCommand struct {
	ID           ID
	PublicID     uuid.UUID
	Name         presetdeck.Name
	Source       *Source
	Cards        map[presetdeck.CardRef]presetdeck.CardCount
	RegisteredBy operator.ID
}

type RenameCommand struct {
	Name      presetdeck.Name
	RenamedBy operator.ID
}

// 入力は「その枚数にする」で、イベントの差分は Decide が計算する（presetdeck.SetCardCountsCommand と同じ）。
// ルールは、ドメインが既定の値を読まず、コマンドを動かす側が game_rules を読んで渡す
type SetCardCountsCommand struct {
	Counts    map[presetdeck.CardRef]presetdeck.CardCount
	Rules     gamerules.Deck
	ChangedBy operator.ID
}

// 公開するとき確かめる合計の枚数は Rules から。入っているカードが公開中か・公開中の数・名前は、ほかの集約や
// 他の行にまたがるため Decide では見ず、コマンドを動かす側が確かめる
type PublishCommand struct {
	Rules       gamerules.Deck
	PublishedBy operator.ID
}

type RetireCommand struct {
	RetiredBy operator.ID
}

type DeleteCommand struct {
	DeletedBy operator.ID
}

func (*RegisterCommand) isGrantDeckCommand()      {}
func (*RenameCommand) isGrantDeckCommand()        {}
func (*SetCardCountsCommand) isGrantDeckCommand() {}
func (*PublishCommand) isGrantDeckCommand()       {}
func (*RetireCommand) isGrantDeckCommand()        {}
func (*DeleteCommand) isGrantDeckCommand()        {}
