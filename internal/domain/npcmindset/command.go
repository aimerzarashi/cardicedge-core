package npcmindset

import (
	"github.com/google/uuid"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/operator"
)

type Command interface {
	isNPCMindsetCommand()
}

// 重みは渡さず、既定の値（すべて 1）で登録する
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

// 入力は「この重みにする」で、種類のすべてを持つ。足りない・余分・知らない種類は ErrActionWeightsMismatch。
// 値の範囲は Weight の生成関数が確かめる
type SetActionWeightsCommand struct {
	Weights   ActionWeights
	ChangedBy operator.ID
}

// 採点基準ごとの重み。基準のすべてを持つ。足りない・余分・知らない基準は ErrCriterionWeightsMismatch
type SetCriterionWeightsCommand struct {
	Weights   CriterionWeights
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

func (*RegisterCommand) isNPCMindsetCommand()            {}
func (*RenameCommand) isNPCMindsetCommand()              {}
func (*SetActionWeightsCommand) isNPCMindsetCommand()    {}
func (*SetCriterionWeightsCommand) isNPCMindsetCommand() {}
func (*PublishCommand) isNPCMindsetCommand()             {}
func (*RetireCommand) isNPCMindsetCommand()              {}
func (*DeleteCommand) isNPCMindsetCommand()              {}
