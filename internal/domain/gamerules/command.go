package gamerules

import "github.com/google/uuid"

type Command interface {
	isGameRulesCommand()
}

// 値の変更のコマンドは作らない（core#23）
type CreateCommand struct {
	ID       ID
	PublicID uuid.UUID
	Rules    Rules
}

func (*CreateCommand) isGameRulesCommand() {}
