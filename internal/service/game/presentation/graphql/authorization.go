package graphql

import (
	"context"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/game/application/authentication"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/game/presentation/graphql/model"
)

// 登録済みのプレイヤーが要る操作の最初で呼び、未登録なら GraphQL の errors ではなく、操作の結果の型（PlayerNotRegisteredError）で
// 返す（呼び出し側が型で分岐し、registerPlayer を呼んでから操作し直せるように）。操作が増えても同じ判定を書き写さないよう、ここにまとめる
// （studio の requireOperator と同じ形）
func requirePlayer(ctx context.Context) *model.PlayerNotRegisteredError {
	if _, ok := authentication.Player(ctx); !ok {
		return &model.PlayerNotRegisteredError{Message: "player not registered"}
	}
	return nil
}
