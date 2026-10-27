package command

import (
	"context"
	"errors"
	"time"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/studiorules"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/rdb"
)

// 今の studio_rules。確かめてから保存するコマンドが受け取る（ドメインの既定の値を直接読まず、
// 値を変えられるようになっても、テストで値を小さくしても、コマンドを変えないため）
type GetStudioRulesFunc func(ctx context.Context) (studiorules.Rules, error)

type StudioRulesHandler struct {
	eventStore domain.EventStore[studiorules.ID, studiorules.Event, studiorules.State]
	identifier domain.Identifier[studiorules.ID]
	find       studiorules.FindFunc
	create     studiorules.CreateFunc
	now        func() time.Time
}

func NewStudioRulesHandler(
	eventStore domain.EventStore[studiorules.ID, studiorules.Event, studiorules.State],
	identifier domain.Identifier[studiorules.ID],
	find studiorules.FindFunc,
	create studiorules.CreateFunc,
	now func() time.Time,
) *StudioRulesHandler {
	return &StudioRulesHandler{eventStore: eventStore, identifier: identifier, find: find, create: create, now: now}
}

// 起動時に呼ぶ。まだ無いときだけ、既定の値で作り、作ったかを返す（2 回目以降の起動で、既定の値が変わっていても上書きしない）。
// 複数台が同時に起動して先を越された（一意な key を先に確保された）のも「ある」とみなして成功にする
func (c *StudioRulesHandler) CreateInitial(ctx context.Context) (bool, error) {
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
		events, err := studiorules.Decide(studiorules.InitialState(), &studiorules.CreateCommand{ID: id, PublicID: publicID, Rules: studiorules.Defaults()}, c.now())
		if err != nil {
			return struct{}{}, err
		}
		return struct{}{}, c.eventStore.Save(ctx, events)
	})
	if errors.Is(err, studiorules.ErrAlreadyCreated) {
		return false, nil
	}
	return err == nil, err
}

// 今の studio_rules を、イベントストアで読む。read model を読まないのは、投影の遅れで古い値のまま確かめないため
func (c *StudioRulesHandler) Get(ctx context.Context) (studiorules.Rules, error) {
	id, found, err := c.find(ctx)
	if err != nil {
		return studiorules.Rules{}, err
	}
	if !found {
		return studiorules.Rules{}, ErrStudioRulesNotCreated
	}
	state, err := c.eventStore.Load(ctx, id)
	if err != nil {
		return studiorules.Rules{}, err
	}
	rules, ok := state.(studiorules.StudioRules)
	if !ok {
		return studiorules.Rules{}, ErrStudioRulesNotCreated
	}
	return rules.Rules, nil
}
