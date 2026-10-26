package constraint

import (
	"context"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/player"
)

var (
	_ player.UpdateSubjectFunc = UpdatePlayerSubject
	_ player.FindBySubjectFunc = FindPlayerBySubject
)

func UpdatePlayerSubject(ctx context.Context, id player.ID, issuer player.Issuer, subject player.Subject) error {
	return updateUniqueKey(ctx, id.Int64(), player.SubjectKey(issuer, subject), player.ErrAlreadyRegistered)
}

func FindPlayerBySubject(ctx context.Context, issuer player.Issuer, subject player.Subject) (player.ID, bool, error) {
	id, found, err := findUniqueKeyOwner(ctx, player.SubjectKey(issuer, subject))
	if err != nil || !found {
		return 0, false, err
	}
	playerID, err := player.NewID(id)
	return playerID, err == nil, err
}
