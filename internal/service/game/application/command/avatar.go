package command

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/avatar"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/rdb"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/game/application/authentication"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/game/application/input"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/game/application/rules"
)

type AvatarHandler struct {
	eventStore    domain.EventStore[avatar.ID, avatar.Event, avatar.State]
	identifier    domain.Identifier[avatar.ID]
	updateName    avatar.UpdateNameFunc
	reserveQuota  avatar.ReserveQuotaFunc
	getQuotaLimit rules.GetQuotaLimitFunc
	now           func() time.Time
}

func NewAvatarHandler(
	eventStore domain.EventStore[avatar.ID, avatar.Event, avatar.State],
	identifier domain.Identifier[avatar.ID],
	updateName avatar.UpdateNameFunc,
	reserveQuota avatar.ReserveQuotaFunc,
	getQuotaLimit rules.GetQuotaLimitFunc,
	now func() time.Time,
) *AvatarHandler {
	return &AvatarHandler{
		eventStore: eventStore, identifier: identifier, updateName: updateName,
		reserveQuota: reserveQuota, getQuotaLimit: getQuotaLimit, now: now,
	}
}

// アバターの公開 ID と名前（core の内部の ID は返さない）
type RegisteredAvatar struct {
	ID   uuid.UUID
	Name string
}

// context のプレイヤー（認証ミドルウェアが載せたもの）のアバターを作る。登録済みのプレイヤーが載っていなければ ErrPlayerNotRegistered。
// 名前の誤りは input.Errors、同じプレイヤーの別のアバターが同じ名前なら ErrAvatarNameAlreadyInUse、上限に達していれば
// ErrAvatarLimitReached（上限の値は LimitReachedError）。ルールがまだ写されていなければ rules.ErrNotPublished。
// 制約は 1 つのトランザクションの中で名前 → 上限の順に確保し、どれかが失敗すれば全体が戻る（上限の数だけ増えることはない）
func (c *AvatarHandler) Register(ctx context.Context, name string) (RegisteredAvatar, error) {
	playerID, ok := authentication.Player(ctx)
	if !ok {
		return RegisteredAvatar{}, ErrPlayerNotRegistered
	}
	avatarName, err := avatar.NewName(name)
	switch {
	case errors.Is(err, avatar.ErrNameEmpty):
		return RegisteredAvatar{}, input.Errors{&input.Required{Field: "name"}}
	case errors.Is(err, avatar.ErrNameTooLong):
		return RegisteredAvatar{}, input.Errors{&input.TooLong{Field: "name", Max: avatar.MaxNameLength}}
	case err != nil:
		return RegisteredAvatar{}, err
	}
	// 上限の値はトランザクションの外で読む。上限に達したとき、確保した時点の値と同じ値を max に入れるため、ここで 1 度だけ読む
	limit, err := c.getQuotaLimit(ctx, avatar.PlayerQuotaType)
	if err != nil {
		return RegisteredAvatar{}, err
	}
	registered, err := rdb.Transact(ctx, func(ctx context.Context) (RegisteredAvatar, error) {
		id, publicID, err := c.identifier.Issue(ctx)
		if err != nil {
			return RegisteredAvatar{}, err
		}
		if err := c.updateName(ctx, id, playerID, avatarName); err != nil {
			return RegisteredAvatar{}, err
		}
		if err := c.reserveQuota(ctx, avatar.PlayerQuota(playerID), limit); err != nil {
			return RegisteredAvatar{}, err
		}
		events, err := avatar.Decide(avatar.InitialState(), &avatar.RegisterCommand{ID: id, PublicID: publicID, Player: playerID, Name: avatarName}, c.now())
		if err != nil {
			return RegisteredAvatar{}, err
		}
		if err := c.eventStore.Save(ctx, events); err != nil {
			return RegisteredAvatar{}, err
		}
		return RegisteredAvatar{ID: publicID, Name: avatarName.String()}, nil
	})
	switch {
	case errors.Is(err, avatar.ErrNameAlreadyInUse):
		return RegisteredAvatar{}, ErrAvatarNameAlreadyInUse
	case errors.Is(err, avatar.ErrLimitReached):
		return RegisteredAvatar{}, &LimitReachedError{reason: ErrAvatarLimitReached, Max: limit.Int()}
	case err != nil:
		return RegisteredAvatar{}, err
	}
	return registered, nil
}
