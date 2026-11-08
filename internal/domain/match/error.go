package match

import "errors"

var (
	ErrInvalidState   = errors.New("match: invalid state")
	ErrAlreadyCreated = errors.New("match: already created")
	// 写したボードに拠点が無い（公開したボードは拠点をちょうど 1 つ持つので、写し方の誤り）
	ErrBaseNotFound = errors.New("match: base not found")
)

var (
	ErrNotYourTurn         = errors.New("match: not your turn")
	ErrPhaseNotAllowed     = errors.New("match: phase not allowed")
	ErrHandIndexOutOfRange = errors.New("match: hand index out of range")
	ErrPathNotAllowed      = errors.New("match: path not allowed")
	// 配置できない手札（クリーチャーでない・コストが払えない）
	ErrCreatureNotDeployable = errors.New("match: creature not deployable")
	// 自分の領地でない位置は手放せない
	ErrTerritoryNotRelinquishable = errors.New("match: territory not relinquishable")
)

var (
	// 戦利品は NPC には配らない
	ErrRewardNotForNPC        = errors.New("match: reward is not for npc")
	ErrRewardAlreadyReceived  = errors.New("match: reward already received")
	ErrRewardPackNotAvailable = errors.New("match: reward pack not available")
)
