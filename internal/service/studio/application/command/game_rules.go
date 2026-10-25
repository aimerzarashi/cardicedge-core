package command

import (
	"context"
	"errors"
	"time"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/gamerules"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/rdb"
)

// 今のゲーム全体のルール。確かめてから保存するコマンドが受け取る（ドメインの既定の値を直接読まず、
// 値を変えられるようになっても、テストで値を小さくしても、コマンドを変えないため）
type GetGameRulesFunc func(ctx context.Context) (gamerules.Rules, error)

type GameRulesHandler struct {
	eventStore domain.EventStore[gamerules.ID, gamerules.Event, gamerules.State]
	identifier domain.Identifier[gamerules.ID]
	find       gamerules.FindFunc
	create     gamerules.CreateFunc
	now        func() time.Time
}

func NewGameRulesHandler(
	eventStore domain.EventStore[gamerules.ID, gamerules.Event, gamerules.State],
	identifier domain.Identifier[gamerules.ID],
	find gamerules.FindFunc,
	create gamerules.CreateFunc,
	now func() time.Time,
) *GameRulesHandler {
	return &GameRulesHandler{eventStore: eventStore, identifier: identifier, find: find, create: create, now: now}
}

// 起動時に呼ぶ。まだ無いときだけ、既定の値で作り、作ったかを返す（2 回目以降の起動で、既定の値が変わっていても上書きしない）。
// 複数台が同時に起動して先を越された（一意な key を先に確保された）のも「ある」とみなして成功にする
func (c *GameRulesHandler) CreateInitial(ctx context.Context) (bool, error) {
	_, found, err := c.find(ctx)
	if err != nil || found {
		return false, err
	}
	_, err = rdb.Transact(ctx, func(ctx context.Context) (struct{}, error) {
		id, publicID, err := c.identifier.Issue(ctx)
		if err != nil {
			return struct{}{}, err
		}
		if err := c.create(ctx, id); err != nil {
			return struct{}{}, err
		}
		events, err := gamerules.Decide(gamerules.InitialState(), &gamerules.CreateCommand{ID: id, PublicID: publicID, Rules: gamerules.Defaults()}, c.now())
		if err != nil {
			return struct{}{}, err
		}
		return struct{}{}, c.eventStore.Save(ctx, events)
	})
	if errors.Is(err, gamerules.ErrAlreadyCreated) {
		return false, nil
	}
	return err == nil, err
}

// 今のルールを、イベントストアで読む。read model を読まないのは、投影の遅れで古い値のまま確かめないため
func (c *GameRulesHandler) Get(ctx context.Context) (gamerules.Rules, error) {
	id, found, err := c.find(ctx)
	if err != nil {
		return gamerules.Rules{}, err
	}
	if !found {
		return gamerules.Rules{}, ErrGameRulesNotCreated
	}
	state, err := c.eventStore.Load(ctx, id)
	if err != nil {
		return gamerules.Rules{}, err
	}
	rules, ok := state.(gamerules.GameRules)
	if !ok {
		return gamerules.Rules{}, ErrGameRulesNotCreated
	}
	return rules.Rules, nil
}
