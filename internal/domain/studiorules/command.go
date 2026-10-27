package studiorules

import "github.com/google/uuid"

type Command interface {
	isStudioRulesCommand()
}

// 値の変更のコマンドは作らない（core#23）
type CreateCommand struct {
	ID       ID
	PublicID uuid.UUID
	Rules    Rules
}

func (*CreateCommand) isStudioRulesCommand() {}
