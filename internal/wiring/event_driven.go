package wiring

import (
	"context"
	"database/sql"
	"fmt"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/creature"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domaindef"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/eventdriven"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/eventstore"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/rdb"
)

// 止めるのは呼び出し側が Stop を呼んで行う
func startEventDriven(ctx context.Context, db *sql.DB, dsn string, registry *domaindef.Registry, options eventdriven.Options, register ...func(*eventdriven.Hub) error) (*eventdriven.Hub, error) {
	ctx = rdb.WithDB(ctx, db)
	subscribers, err := eventdriven.LoadSubscribers(ctx)
	if err != nil {
		return nil, err
	}
	// 購読される集約のイベントの読み方。購読する集約を足したらここにも足す（足し忘れると投影の登録で起動が止まる）
	hub, err := eventdriven.NewHub(subscribers, registry,
		options,
		eventstore.NewEventLoader(registry, creature.NewID, creature.NewEventType),
	)
	if err != nil {
		return nil, err
	}
	for _, r := range register {
		if err := r(hub); err != nil {
			return nil, err
		}
	}
	if err := hub.Start(ctx, eventdriven.NewListener(dsn)); err != nil {
		return nil, fmt.Errorf("failed to start event-driven hub: %w", err)
	}
	return hub, nil
}
