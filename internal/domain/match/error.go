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
	// 侵略できない手札（クリーチャーでない・コストが払えない）
	ErrCreatureNotInvadable = errors.New("match: creature not invadable")
	// 出せない手札（無い・アイテムでない・コストが払えない）
	ErrItemNotUsable = errors.New("match: item not usable")
	// 自分の領地でない位置は手放せない
	ErrTerritoryNotRelinquishable = errors.New("match: territory not relinquishable")
	ErrTerritoryNotLevelUpable    = errors.New("match: territory not level-upable")
	ErrCreatureNotExchangeable    = errors.New("match: creature not exchangeable")
	ErrLandElementNotChangeable   = errors.New("match: land element not changeable")
)

var (
	// 戦利品は NPC には配らない
	ErrRewardNotForNPC        = errors.New("match: reward is not for npc")
	ErrRewardAlreadyReceived  = errors.New("match: reward already received")
	ErrRewardPackNotAvailable = errors.New("match: reward pack not available")
)
