package npcdecision

import "errors"

// 判断する参加者が NPC でない（思考パターンの写しを持たない）
var ErrNotNPC = errors.New("npcdecision: participant is not an npc")
