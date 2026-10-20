// 部品を組み立てて動かす（照合・初期オペレーター・イベント駆動・HTTP の入口）。cmd/app と結合テストの両方が使い、
// 本番と同じ組み立てをテストで通す。cmd/app に残すのはプロセスとしての起動（設定・シグナル・HTTP の Listen・Serve）だけ
package wiring

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"slices"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/card"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/creature"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/item"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/operator"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/presetdeck"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/spell"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domaindef"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/eventdriven"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/rdb"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/wiring/studio"
)

type Config struct {
	DB *sql.DB
	// Listener が自分で接続するため、DB とは別に渡す
	DSN                  string
	Version              string
	Studio               studio.Config
	InitialOperatorEmail string
	EventDriven          eventdriven.Options
}

type App struct {
	Handler http.Handler
	hub     *eventdriven.Hub
}

// HTTP の入口を返す前に LISTEN を張る（コマンドが commit したイベントの知らせを受け取るため）。
// 止めるのは呼び出し側が HTTP を止めた後に Stop を呼んで行う
func Start(ctx context.Context, config Config) (*App, error) {
	registry, err := domaindef.Load(rdb.WithDB(ctx, config.DB))
	if err != nil {
		return nil, err
	}
	// 集約が宣言した種別名と DB（マイグレーション）の食い違いを、使うときではなく起動時に検出する
	if err := registry.Verify(operator.Types(), creature.Types(), item.Types(), spell.Types(), presetdeck.Types()); err != nil {
		return nil, fmt.Errorf("domain types do not match migrations: %w", err)
	}
	if err := registry.VerifyUniqueKeyTypes(slices.Concat(operator.UniqueKeyTypes(), card.UniqueKeyTypes(), presetdeck.UniqueKeyTypes())...); err != nil {
		return nil, fmt.Errorf("unique key types do not match migrations: %w", err)
	}
	if err := registry.VerifyReferenceTypes(presetdeck.ReferenceTypes()...); err != nil {
		return nil, fmt.Errorf("reference types do not match migrations: %w", err)
	}

	studioParts := studio.New(registry, config.Studio)
	if err := studioParts.RegisterInitialOperator(ctx, config.DB, config.InitialOperatorEmail); err != nil {
		return nil, err
	}
	// 取り消されない ctx を渡す（シグナルで取り消される ctx だと、HTTP より先に止まり、止める途中のリクエストが
	// commit したイベントの知らせを受け取れない）
	hub, err := startEventDriven(context.WithoutCancel(ctx), config.DB, config.DSN, registry, config.EventDriven, studioParts.RegisterProjections)
	if err != nil {
		return nil, err
	}

	mux := NewRouter(config.Version)
	studioParts.Mount(mux, config.DB, config.Version)
	return &App{Handler: mux, hub: hub}, nil
}

// HTTP を止めた後に呼ぶ
func (a *App) Stop() {
	a.hub.Stop()
}
