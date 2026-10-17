package eventdriven

import (
	"context"
	"fmt"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/eventstore"
)

// 登録した投影に、イベントの並びを当てて書かせる。Hub が知らせを受けて投影する流れはまだ無いため、テストから直接呼ぶ
func Project(ctx context.Context, hub *Hub, name string, events []eventstore.LoadedEvent) error {
	for _, registered := range hub.projections {
		if registered.name == name {
			return registered.projection.project(ctx, events)
		}
	}
	return fmt.Errorf("projection %q is not registered", name)
}
