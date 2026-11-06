package starterset

import (
	"github.com/google/uuid"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/board"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/grantdeck"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/npcavatar"
)

type State interface {
	isStarterSetState()
}

type UnregisteredStarterSet struct{}

func (UnregisteredStarterSet) isStarterSetState() {}

// 登録した人・日時は持たない（Decide が使わず、画面に出す値は read model がイベントから作るため）
type StarterSet struct {
	ID       ID
	Version  int64
	PublicID uuid.UUID
	Name     Name
	Status   Status
	// 配るデッキ（変えるか Decide が比べるのと、公開の条件を確かめるために持つ。割り当てた順）
	GrantDecks []grantdeck.ID
	// 解放するボード（割り当てた順）
	UnlockBoards []board.ID
	// 解放する NPC のアバター（割り当てた順）
	UnlockNPCAvatars []npcavatar.ID
}

func (StarterSet) isStarterSetState() {}

// 削除した後も、版を持つ（削除の後の操作を、版の衝突ではなく状態の誤りにするため）
type DeletedStarterSet struct {
	ID      ID
	Version int64
}

func (DeletedStarterSet) isStarterSetState() {}

func InitialState() State {
	return UnregisteredStarterSet{}
}
