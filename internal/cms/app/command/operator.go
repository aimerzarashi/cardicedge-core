package command

import (
	"context"
	"errors"
	"time"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/cms/app/authentication"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/operator"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/rdb"
)

type OperatorCommands struct {
	eventStore          domain.EventStore[operator.ID, operator.Event, operator.State]
	identifier          domain.Identifier[operator.ID]
	updateEmail         operator.UpdateEmailFunc
	updateSubject       operator.UpdateSubjectFunc
	findBySubject       operator.FindBySubjectFunc
	findUnlinkedByEmail operator.FindUnlinkedByEmailFunc
	exists              operator.ExistsFunc
	now                 func() time.Time
}

func NewOperatorCommands(
	eventStore domain.EventStore[operator.ID, operator.Event, operator.State],
	identifier domain.Identifier[operator.ID],
	updateEmail operator.UpdateEmailFunc,
	updateSubject operator.UpdateSubjectFunc,
	findBySubject operator.FindBySubjectFunc,
	findUnlinkedByEmail operator.FindUnlinkedByEmailFunc,
	exists operator.ExistsFunc,
	now func() time.Time,
) *OperatorCommands {
	return &OperatorCommands{
		eventStore: eventStore, identifier: identifier,
		updateEmail: updateEmail, updateSubject: updateSubject,
		findBySubject: findBySubject, findUnlinkedByEmail: findUnlinkedByEmail, exists: exists,
		now: now,
	}
}

// 入力の誤りは InputErrors、登録済みのメールアドレスなら ErrOperatorEmailAlreadyRegistered
func (c *OperatorCommands) Register(ctx context.Context, email string) error {
	address, err := newEmail(email)
	if err != nil {
		return err
	}
	_, err = c.register(ctx, address)
	if errors.Is(err, operator.ErrAlreadyRegistered) {
		return ErrOperatorEmailAlreadyRegistered
	}
	return err
}

// 起動時に呼ぶ。オペレーターが 1 人もいないときだけ登録し、登録したかを返す（2 回目以降の起動で、環境変数を変えても増やさない）
func (c *OperatorCommands) RegisterInitial(ctx context.Context, email string) (bool, error) {
	address, err := newEmail(email)
	if err != nil {
		return false, err
	}
	exists, err := c.exists(ctx)
	if err != nil || exists {
		return false, err
	}
	_, err = c.register(ctx, address)
	// 複数台が同時に起動して先を越された（同じメールアドレスで登録済み）
	if errors.Is(err, operator.ErrAlreadyRegistered) {
		return false, nil
	}
	return err == nil, err
}

func (c *OperatorCommands) register(ctx context.Context, email operator.Email) (operator.ID, error) {
	return rdb.Transact(ctx, func(ctx context.Context) (operator.ID, error) {
		id, publicID, err := c.identifier.Issue(ctx)
		if err != nil {
			return 0, err
		}
		if err := c.updateEmail(ctx, id, email); err != nil {
			return 0, err
		}
		events, err := operator.Decide(operator.InitialState(), &operator.RegisterCommand{ID: id, PublicID: publicID, Email: email}, c.now())
		if err != nil {
			return 0, err
		}
		return id, c.eventStore.Save(ctx, events)
	})
}

// 認証ミドルウェアが、検証済みの JWT の値で呼ぶ。(iss, sub) に紐づいたオペレーターを context に載せて返す。
// まだ紐づいていなければ、確認済みのメールアドレス（emailVerified が true のときの email）で、登録済み・未紐づけの
// オペレーターを探して紐づける。どちらでもなければ、オペレーターを載せずに返す（入口では拒否せず、操作ごとの認可で断る）。
// iss・sub の形が正しくなければ ErrInvalidCredentials
func (c *OperatorCommands) Authenticate(ctx context.Context, issuer, subject, email string, emailVerified bool) (context.Context, error) {
	iss, err := operator.NewIssuer(issuer)
	if err != nil {
		return ctx, ErrInvalidCredentials
	}
	sub, err := operator.NewSubject(subject)
	if err != nil {
		return ctx, ErrInvalidCredentials
	}
	// 確認していないメールアドレスでは紐づけない（他人のメールアドレスでサインアップしても、オペレーターになれないように）
	var verifiedEmail *operator.Email
	if emailVerified {
		if address, err := operator.NewEmail(email); err == nil {
			verifiedEmail = &address
		}
	}
	id, found, err := c.identify(ctx, iss, sub, verifiedEmail)
	if err != nil || !found {
		return ctx, err
	}
	return authentication.WithOperator(ctx, id), nil
}

func (c *OperatorCommands) identify(ctx context.Context, issuer operator.Issuer, subject operator.Subject, verifiedEmail *operator.Email) (operator.ID, bool, error) {
	// ほとんどの呼び出しは紐づけ済みのため、先に引く
	if id, found, err := c.findBySubject(ctx, issuer, subject); err != nil || found || verifiedEmail == nil {
		return id, found, err
	}
	id, found, err := c.findUnlinkedByEmail(ctx, *verifiedEmail)
	if err != nil {
		return 0, false, err
	}
	// 同時に呼ばれて、最初に引いた後に先に紐づけられた場合も未紐づけでなくなるため、(iss, sub) で引き直す
	if !found {
		return c.findBySubject(ctx, issuer, subject)
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
		if err := c.updateSubject(ctx, id, issuer, subject); err != nil {
			return struct{}{}, err
		}
		return struct{}{}, c.eventStore.Save(ctx, events)
	})
	// 同時に呼ばれて先を越されたら、このトランザクションを rollback し、先に紐づいたものを引き直す。
	// 先を越されると、同じ (iss, sub) なら版の衝突（読んだ状態が古い）に、別の (iss, sub) が先なら ErrAlreadyLinked になる
	// （別の (iss, sub) に先に紐づいていれば、この (iss, sub) はオペレーターではない）
	if errors.Is(err, operator.ErrAlreadyLinked) || errors.Is(err, domain.ErrVersionConflict) {
		return c.findBySubject(ctx, issuer, subject)
	}
	if err != nil {
		return 0, false, err
	}
	return id, true, nil
}

func newEmail(value string) (operator.Email, error) {
	address, err := operator.NewEmail(value)
	if err != nil {
		return "", InputErrors{&InvalidFormatInput{Field: "email"}}
	}
	return address, nil
}
