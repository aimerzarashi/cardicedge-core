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
	ErrCreatureNotPlaceable = errors.New("match: creature not placeable")
	// 侵略できない手札（クリーチャーでない・コストが払えない）
	ErrCreatureNotInvadable = errors.New("match: creature not invadable")
	// 出せない手札（無い・アイテムでない・コストが払えない）
	ErrItemNotUsable = errors.New("match: item not usable")
	// アイテムとして出せない手札（戦うクリーチャーが援護を持たない・クリーチャーでない・属性が合わない・コストが払えない）
	ErrCreatureNotUsableAsItem = errors.New("match: creature not usable as item")
	// 自分の領地でない位置は手放せない
	ErrTerritoryNotRelinquishable = errors.New("match: territory not relinquishable")
	ErrTerritoryNotLevelUpable    = errors.New("match: territory not level-upable")
	ErrCreatureNotExchangeable    = errors.New("match: creature not exchangeable")
	ErrLandElementNotChangeable   = errors.New("match: land element not changeable")
	ErrCreatureNotMovable         = errors.New("match: creature not movable")
	// 使えない手札（スペルでない・効果が無い・コストが払えない・このターンにもう使った）
	ErrSpellNotUsable = errors.New("match: spell not usable")
	// 対象を選ぶスペルで、対象が無い
	ErrSpellTargetRequired = errors.New("match: spell target required")
	// 選べない対象（対象を選ばないスペルに対象がある・参加者でない）
	ErrSpellTargetNotAllowed = errors.New("match: spell target not allowed")
	// クリーチャーを選ぶスペルで、対象の領地の位置が無い
	ErrSpellTargetPositionRequired = errors.New("match: spell target position required")
	// 選べない対象の領地（クリーチャーを選ばないスペルに位置がある・領地でない）
	ErrSpellTargetPositionNotAllowed = errors.New("match: spell target position not allowed")
)

var (
	// 戦利品は NPC には配らない
	ErrRewardNotForNPC        = errors.New("match: reward is not for npc")
	ErrRewardAlreadyReceived  = errors.New("match: reward already received")
	ErrRewardPackNotAvailable = errors.New("match: reward pack not available")
)
