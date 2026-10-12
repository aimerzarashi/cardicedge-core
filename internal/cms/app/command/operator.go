package command

import (
	"context"
	"errors"
	"time"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/operator"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/rdb"
)

// 実装は constraint.Operators（operators・operator_subjects）
type OperatorConstraints interface {
	// 登録済みのメールアドレスなら operator.ErrAlreadyRegistered
	ClaimEmail(ctx context.Context, id operator.ID, email operator.Email) error
	// (iss, sub) が別のオペレーターに紐づいている、またはオペレーターが紐づけ済みなら operator.ErrAlreadyLinked
	ClaimSubject(ctx context.Context, id operator.ID, issuer operator.Issuer, subject operator.Subject) error
	FindBySubject(ctx context.Context, issuer operator.Issuer, subject operator.Subject) (operator.ID, bool, error)
	FindUnlinkedByEmail(ctx context.Context, email operator.Email) (operator.ID, bool, error)
	Exists(ctx context.Context) (bool, error)
}

type OperatorCommands struct {
	eventStore  domain.EventStore[operator.ID, operator.Event, operator.State]
	identifier  domain.Identifier[operator.ID]
	constraints OperatorConstraints
	now         func() time.Time
}

func NewOperatorCommands(
	eventStore domain.EventStore[operator.ID, operator.Event, operator.State],
	identifier domain.Identifier[operator.ID],
	constraints OperatorConstraints,
	now func() time.Time,
) *OperatorCommands {
	return &OperatorCommands{eventStore: eventStore, identifier: identifier, constraints: constraints, now: now}
}

// 登録済みのメールアドレスなら operator.ErrAlreadyRegistered
func (c *OperatorCommands) Register(ctx context.Context, email operator.Email) (operator.ID, error) {
	return rdb.Transact(ctx, func(ctx context.Context) (operator.ID, error) {
		id, publicID, err := c.identifier.Issue(ctx)
		if err != nil {
			return 0, err
		}
		if err := c.constraints.ClaimEmail(ctx, id, email); err != nil {
			return 0, err
		}
		events, err := operator.Decide(operator.InitialState(), &operator.RegisterCommand{ID: id, PublicID: publicID, Email: email}, c.now())
		if err != nil {
			return 0, err
		}
		return id, c.eventStore.Save(ctx, events)
	})
}

// 起動時に呼ぶ。オペレーターが 1 人もいないときだけ登録し、登録したかを返す（2 回目以降の起動で、環境変数を変えても増やさない）
func (c *OperatorCommands) RegisterInitial(ctx context.Context, email operator.Email) (bool, error) {
	exists, err := c.constraints.Exists(ctx)
	if err != nil || exists {
		return false, err
	}
	_, err = c.Register(ctx, email)
	// 複数台が同時に起動して先を越された（同じメールアドレスで登録済み）
	if errors.Is(err, operator.ErrAlreadyRegistered) {
		return false, nil
	}
	return err == nil, err
}

// JWT の (iss, sub) のオペレーターを返す。まだ紐づいていなければ、確認済みのメールアドレス（verifiedEmail。無ければ nil）で
// 登録済み・未紐づけのオペレーターを探して紐づける。どちらでもなければ found は false
func (c *OperatorCommands) Identify(ctx context.Context, issuer operator.Issuer, subject operator.Subject, verifiedEmail *operator.Email) (id operator.ID, found bool, err error) {
	// ほとんどの呼び出しは紐づけ済みのため、先に引く
	if id, found, err := c.constraints.FindBySubject(ctx, issuer, subject); err != nil || found || verifiedEmail == nil {
		return id, found, err
	}
	id, found, err = c.constraints.FindUnlinkedByEmail(ctx, *verifiedEmail)
	if err != nil {
		return 0, false, err
	}
	// 同時に呼ばれて、最初に引いた後に先に紐づけられた場合も未紐づけでなくなるため、(iss, sub) で引き直す
	if !found {
		return c.constraints.FindBySubject(ctx, issuer, subject)
	}
	_, err = rdb.Transact(ctx, func(ctx context.Context) (struct{}, error) {
		state, err := c.eventStore.Load(ctx, id)
		if err != nil {
			return struct{}{}, err
		}
		events, err := operator.Decide(state, &operator.LinkCommand{Issuer: issuer, Subject: subject}, c.now())
		if err != nil {
			return struct{}{}, err
		}
		if err := c.constraints.ClaimSubject(ctx, id, issuer, subject); err != nil {
			return struct{}{}, err
		}
		return struct{}{}, c.eventStore.Save(ctx, events)
	})
	// 同時に呼ばれて先を越されたら、このトランザクションを rollback し、先に紐づいたものを引き直す
	// （別の (iss, sub) に先に紐づいていれば、この (iss, sub) はオペレーターではない）
	if errors.Is(err, operator.ErrAlreadyLinked) {
		return c.constraints.FindBySubject(ctx, issuer, subject)
	}
	if err != nil {
		return 0, false, err
	}
	return id, true, nil
}
