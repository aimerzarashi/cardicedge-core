package command

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/player"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/rdb"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/game/application/authentication"
)

type PlayerHandler struct {
	eventStore    domain.EventStore[player.ID, player.Event, player.State]
	identifier    domain.Identifier[player.ID]
	updateSubject player.UpdateSubjectFunc
	findBySubject player.FindBySubjectFunc
	now           func() time.Time
}

func NewPlayerHandler(
	eventStore domain.EventStore[player.ID, player.Event, player.State],
	identifier domain.Identifier[player.ID],
	updateSubject player.UpdateSubjectFunc,
	findBySubject player.FindBySubjectFunc,
	now func() time.Time,
) *PlayerHandler {
	return &PlayerHandler{
		eventStore: eventStore, identifier: identifier,
		updateSubject: updateSubject, findBySubject: findBySubject,
		now: now,
	}
}

// プレイヤーの公開 ID（core の内部の ID は返さない）
type RegisteredPlayer struct {
	ID uuid.UUID
}

// 認証ミドルウェアが、検証済みの JWT の値で呼ぶ。(iss, sub) を context に載せ、登録済みのプレイヤーがいればその ID も載せる。
// 未登録でも拒否しない（初めての registerPlayer がこの (iss, sub) で登録するため）。
// iss・sub の形が正しくなければ ErrInvalidCredentials
func (c *PlayerHandler) Authenticate(ctx context.Context, issuer, subject string) (context.Context, error) {
	iss, err := player.NewIssuer(issuer)
	if err != nil {
		return ctx, ErrInvalidCredentials
	}
	sub, err := player.NewSubject(subject)
	if err != nil {
		return ctx, ErrInvalidCredentials
	}
	ctx = authentication.WithSubject(ctx, iss, sub)
	id, found, err := c.findBySubject(ctx, iss, sub)
	if err != nil || !found {
		return ctx, err
	}
	return authentication.WithPlayer(ctx, id), nil
}

// context の (iss, sub) のプレイヤーを登録して返す。登録済みなら、何もせず（イベントを作らず）そのプレイヤーを返す
// （game は未登録のときに呼び直すだけで済む）。context に (iss, sub) が無ければ ErrNotAuthenticated
func (c *PlayerHandler) Register(ctx context.Context) (RegisteredPlayer, error) {
	issuer, subject, ok := authentication.Subject(ctx)
	if !ok {
		return RegisteredPlayer{}, ErrNotAuthenticated
	}
	// 登録済みなら、認証ミドルウェアが引いたプレイヤーが載っている（引き直さない）
	if id, ok := authentication.Player(ctx); ok {
		return c.load(ctx, id)
	}
	id, err := rdb.Transact(ctx, func(ctx context.Context) (player.ID, error) {
		id, publicID, err := c.identifier.Issue(ctx)
		if err != nil {
			return 0, err
		}
		if err := c.updateSubject(ctx, id, issuer, subject); err != nil {
			return 0, err
		}
		events, err := player.Decide(player.InitialState(), &player.RegisterCommand{ID: id, PublicID: publicID, Issuer: issuer, Subject: subject}, c.now())
		if err != nil {
			return 0, err
		}
		return id, c.eventStore.Save(ctx, events)
	})
	// 同時に呼ばれて（または認証ミドルウェアが引いた後に）先を越された。このトランザクションは rollback されているため、
	// 先に登録されたプレイヤーを引き直して返す。key が重なったのに見つからないのは、先の登録が引く前に消えたときだけで、
	// 今はプレイヤーを消す操作が無いため起きない（起きたら黙って作り直さず、重なりとして返す）
	if errors.Is(err, player.ErrAlreadyRegistered) {
		var found bool
		id, found, err = c.findBySubject(ctx, issuer, subject)
		if err == nil && !found {
			err = player.ErrAlreadyRegistered
		}
	}
	if err != nil {
		return RegisteredPlayer{}, err
	}
	return c.load(ctx, id)
}

// context のプレイヤー（認証ミドルウェアが載せたもの）を返す。載っていなければ ErrPlayerNotRegistered
func (c *PlayerHandler) Me(ctx context.Context) (RegisteredPlayer, error) {
	id, ok := authentication.Player(ctx)
	if !ok {
		return RegisteredPlayer{}, ErrPlayerNotRegistered
	}
	return c.load(ctx, id)
}

func (c *PlayerHandler) load(ctx context.Context, id player.ID) (RegisteredPlayer, error) {
	state, err := c.eventStore.Load(ctx, id)
	if err != nil {
		return RegisteredPlayer{}, err
	}
	loaded, ok := state.(player.Player)
	if !ok {
		return RegisteredPlayer{}, player.ErrInvalidState
	}
	return RegisteredPlayer{ID: loaded.PublicID}, nil
}
