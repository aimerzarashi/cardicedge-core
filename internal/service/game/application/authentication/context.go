package authentication

import (
	"context"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/player"
)

type subjectKey struct{}

type playerKey struct{}

type subject struct {
	issuer  player.Issuer
	subject player.Subject
}

// 検証済みの JWT の (iss, sub) を載せる。プレイヤーに紐づいていなくても載せるのは、初めての registerPlayer が
// この (iss, sub) でプレイヤーを登録するため。入力ではなく JWT だけから決めるのは、他人として操作させないため
func WithSubject(ctx context.Context, issuer player.Issuer, sub player.Subject) context.Context {
	return context.WithValue(ctx, subjectKey{}, subject{issuer: issuer, subject: sub})
}

func Subject(ctx context.Context) (player.Issuer, player.Subject, bool) {
	s, ok := ctx.Value(subjectKey{}).(subject)
	return s.issuer, s.subject, ok
}

// (iss, sub) に登録済みのプレイヤーを載せる
func WithPlayer(ctx context.Context, id player.ID) context.Context {
	return context.WithValue(ctx, playerKey{}, id)
}

func Player(ctx context.Context) (player.ID, bool) {
	id, ok := ctx.Value(playerKey{}).(player.ID)
	return id, ok
}
