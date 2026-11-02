// 部品を組み立てて動かす（照合・初期オペレーター・イベント駆動・HTTP の入口）。cmd/app と結合テストの両方が使い、
// 本番と同じ組み立てをテストで通す。cmd/app に残すのはプロセスとしての起動（設定・シグナル・HTTP の Listen・Serve）だけ
package wiring

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"slices"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/avatar"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/board"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/card"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/collection"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/creature"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/deck"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/gamerules"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/item"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/npcavatar"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/npcdeck"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/operator"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/player"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/presetdeck"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/spell"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/starterdeck"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/startergrant"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/studiorules"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domaindef"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/eventdriven"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/rdb"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/telemetry"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/game"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/studio"
)

type Config struct {
	// studio 用・game 用の DB（core#33）。テーブルは共有せず、どちらにも同じマイグレーションを流す
	StudioDB *sql.DB
	// Listener が自分で接続するため、DB とは別に渡す
	StudioDSN            string
	GameDB               *sql.DB
	GameDSN              string
	Version              string
	Studio               studio.Config
	Game                 game.Config
	InitialOperatorEmail string
	EventDriven          eventdriven.Options
}

type App struct {
	Handler http.Handler
	hubs    []*eventdriven.Hub
}

// HTTP の入口を返す前に LISTEN を張る（コマンドが commit したイベントの知らせを受け取るため）。
// 止めるのは呼び出し側が HTTP を止めた後に Stop を呼んで行う
func Start(ctx context.Context, config Config) (*App, error) {
	studioPrepared, err := rdb.Transact(rdb.WithDB(ctx, config.StudioDB), func(ctx context.Context) (prepared, error) {
		return prepareStudio(ctx, config)
	})
	if err != nil {
		return nil, fmt.Errorf("studio: %w", err)
	}
	gamePrepared, err := rdb.Transact(rdb.WithDB(ctx, config.GameDB), func(ctx context.Context) (prepared, error) {
		return prepareGame(ctx, config)
	})
	if err != nil {
		return nil, fmt.Errorf("game: %w", err)
	}
	if err := studioPrepared.studio.RegisterInitialOperator(ctx, config.StudioDB, config.InitialOperatorEmail); err != nil {
		return nil, err
	}
	if err := studioPrepared.studio.CreateInitialGameRules(ctx, config.StudioDB); err != nil {
		return nil, err
	}
	if err := studioPrepared.studio.CreateInitialStudioRules(ctx, config.StudioDB); err != nil {
		return nil, err
	}
	// 取り消されない ctx を渡す（シグナルで取り消される ctx だと、HTTP より先に止まり、止める途中のリクエストが
	// commit したイベントの知らせを受け取れない）
	if err := studioPrepared.hub.Start(rdb.WithDB(context.WithoutCancel(ctx), config.StudioDB), eventdriven.NewListener(config.StudioDSN)); err != nil {
		return nil, fmt.Errorf("failed to start event-driven hub: %w", err)
	}

	if err := gamePrepared.hub.Start(rdb.WithDB(context.WithoutCancel(ctx), config.GameDB), eventdriven.NewListener(config.GameDSN)); err != nil {
		studioPrepared.hub.Stop()
		return nil, fmt.Errorf("failed to start game event-driven hub: %w", err)
	}

	mux := NewRouter(config.Version)
	studioPrepared.studio.Mount(mux, config.StudioDB)
	gamePrepared.game.Mount(mux, config.GameDB)
	return &App{Handler: telemetry.HTTP(mux), hubs: []*eventdriven.Hub{studioPrepared.hub, gamePrepared.hub}}, nil
}

type prepared struct {
	studio *studio.Studio
	game   *game.Game
	hub    *eventdriven.Hub
}

// DB の種別・購読者・購読の行を、コードの宣言に合わせてから組み立てる（行はマイグレーションでは入れない。core#33）。
// DB ごとに 1 つのトランザクションで行い、同じサービスを複数台で起動しても、錠で順番に動くようにする（後の台は何も足さない）
func prepareStudio(ctx context.Context, config Config) (prepared, error) {
	registry, err := syncTypes(ctx)
	if err != nil {
		return prepared{}, err
	}
	studioParts := studio.New(registry, config.Studio)
	hub, err := newHub(registry, config.EventDriven, subscribersOf(serviceStudio))
	if err != nil {
		return prepared{}, err
	}
	if err := studioParts.RegisterProjections(hub); err != nil {
		return prepared{}, err
	}
	if err := hub.Sync(ctx); err != nil {
		return prepared{}, fmt.Errorf("event subscribers do not match code: %w", err)
	}
	return prepared{studio: studioParts, hub: hub}, nil
}

func prepareGame(ctx context.Context, config Config) (prepared, error) {
	registry, err := syncTypes(ctx)
	if err != nil {
		return prepared{}, err
	}
	hub, err := newHub(registry, config.EventDriven, subscribersOf(serviceGame))
	if err != nil {
		return prepared{}, err
	}
	gameParts := game.New(registry, config.Game)
	if err := gameParts.RegisterPolicies(hub); err != nil {
		return prepared{}, err
	}
	if err := gameParts.RegisterProjections(hub); err != nil {
		return prepared{}, err
	}
	if err := hub.Sync(ctx); err != nil {
		return prepared{}, fmt.Errorf("event subscribers do not match code: %w", err)
	}
	return prepared{game: gameParts, hub: hub}, nil
}

// 錠を取り、種別の行をコードの一覧に合わせ、読み込んで、ドメインの宣言と照合する。種別は全サービスで共通なので、どの DB でも同じ
func syncTypes(ctx context.Context) (*domaindef.Registry, error) {
	exec, err := rdb.GetExecutor(ctx)
	if err != nil {
		return nil, err
	}
	if _, err := exec.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtext('core.sync'))`); err != nil {
		return nil, fmt.Errorf("failed to lock for sync: %w", err)
	}
	if err := domaindef.Sync(ctx, domaindef.Types); err != nil {
		return nil, fmt.Errorf("domain types do not match catalog: %w", err)
	}
	registry, err := domaindef.Load(ctx)
	if err != nil {
		return nil, err
	}
	// ドメインが宣言した種別名と DB（コードの一覧から入れた行）の食い違いを、使うときではなく起動時に検出する（両方向。core#20）
	aggregates := []domain.AggregateTypes{operator.Types(), creature.Types(), item.Types(), spell.Types(), presetdeck.Types(), starterdeck.Types(), player.Types(), startergrant.Types(), collection.Types(), deck.Types(), gamerules.Types(), avatar.Types(), board.Types(), studiorules.Types(), npcavatar.Types(), npcdeck.Types()}
	entityTypes := make([]domain.EntityType, 0, len(aggregates))
	for _, aggregate := range aggregates {
		entityTypes = append(entityTypes, aggregate.EntityType)
	}
	if err := registry.VerifyEntityTypes(entityTypes...); err != nil {
		return nil, fmt.Errorf("entity types do not match catalog: %w", err)
	}
	if err := registry.VerifyEventTypes(aggregates...); err != nil {
		return nil, fmt.Errorf("event types do not match catalog: %w", err)
	}
	if err := registry.VerifyUniqueKeyTypes(slices.Concat(operator.UniqueKeyTypes(), card.UniqueKeyTypes(), presetdeck.UniqueKeyTypes(), starterdeck.UniqueKeyTypes(), player.UniqueKeyTypes(), startergrant.UniqueKeyTypes(), collection.UniqueKeyTypes(), deck.UniqueKeyTypes(), gamerules.UniqueKeyTypes(), avatar.UniqueKeyTypes(), board.UniqueKeyTypes(), studiorules.UniqueKeyTypes(), npcavatar.UniqueKeyTypes(), npcdeck.UniqueKeyTypes())...); err != nil {
		return nil, fmt.Errorf("unique key types do not match catalog: %w", err)
	}
	if err := registry.VerifyReferenceTypes(slices.Concat(presetdeck.ReferenceTypes(), starterdeck.ReferenceTypes(), npcdeck.ReferenceTypes())...); err != nil {
		return nil, fmt.Errorf("reference types do not match catalog: %w", err)
	}
	if err := registry.VerifyQuotaTypes(slices.Concat(deck.QuotaTypes(), starterdeck.QuotaTypes(), avatar.QuotaTypes())...); err != nil {
		return nil, fmt.Errorf("quota types do not match catalog: %w", err)
	}
	return registry, nil
}

// HTTP を止めた後に呼ぶ
func (a *App) Stop() {
	for _, hub := range a.hubs {
		hub.Stop()
	}
}
