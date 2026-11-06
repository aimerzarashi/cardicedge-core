package model

import "github.com/google/uuid"

// gqlgen.yml でこの型を Room に使わせる（生成させない）。match は resolver が対戦を読んで返すため、
// 始まった対戦の公開 ID を resolver に渡すまで MatchID に持つ（スキーマの項目ではない）
type Room struct {
	ID           uuid.UUID          `json:"id"`
	Name         string             `json:"name"`
	Revision     int                `json:"revision"`
	Status       RoomStatus         `json:"status"`
	Participants []*RoomParticipant `json:"participants"`
	Seats        []*RoomSeat        `json:"seats"`
	MatchRules   *RoomMatchRules    `json:"matchRules,omitempty"`
	Matches      []*RoomMatch       `json:"matches"`
	Capabilities *RoomCapabilities  `json:"capabilities,omitempty"`
	MatchID      *uuid.UUID
}

func (Room) IsRoomResult() {}
