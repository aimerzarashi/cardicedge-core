package npcmindset

import "github.com/google/uuid"

type State interface {
	isNPCMindsetState()
}

type UnregisteredNPCMindset struct{}

func (UnregisteredNPCMindset) isNPCMindsetState() {}

// 登録した人・日時は持たない（Decide が使わず、画面に出す値は read model がイベントから作るため）
type NPCMindset struct {
	ID       ID
	Version  int64
	PublicID uuid.UUID
	Name     Name
	Status   Status
	// 種類のすべてを持つ（変えるか Decide が比べるために持つ）
	ActionWeights    ActionWeights
	CriterionWeights CriterionWeights
}

func (NPCMindset) isNPCMindsetState() {}

// 削除した後も、版を持つ（削除の後の操作を、版の衝突ではなく状態の誤りにするため）
type DeletedNPCMindset struct {
	ID      ID
	Version int64
}

func (DeletedNPCMindset) isNPCMindsetState() {}

func InitialState() State {
	return UnregisteredNPCMindset{}
}
