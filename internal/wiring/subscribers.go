package wiring

import "gitlab.com/aimerzarashi/diecardomain/core/internal/eventdriven"

// 購読者（投影・ポリシー）の id と名前と、どのサービスのものかの一覧。全サービスの購読者を 1 か所で見渡せるよう、ここに置く。
// id は自動で振らない（DB ごと・起動の順番で変わりうるため）。1000 から順に振る。起動時に、そのサービスの DB に、そのサービスの
// 購読者だけを入れる（core#33）
var subscribers = []serviceSubscriber{
	{service: serviceStudio, def: eventdriven.ProjectionSubscriber(1000, "cards")},
	{service: serviceStudio, def: eventdriven.ProjectionSubscriber(1001, "preset_decks")},
	{service: serviceStudio, def: eventdriven.ProjectionSubscriber(1002, "starter_deck_sets")},
	{service: serviceGame, def: eventdriven.PolicySubscriber(1003, "starter_grant")},
	{service: serviceGame, def: eventdriven.ProjectionSubscriber(1004, "players")},
	{service: serviceGame, def: eventdriven.ProjectionSubscriber(1005, "player_starter_grants")},
	{service: serviceStudio, def: eventdriven.ProjectionSubscriber(1006, "game_rules")},
}

const (
	serviceStudio = "studio"
	serviceGame   = "game"
)

type serviceSubscriber struct {
	service string
	def     eventdriven.SubscriberDef
}

func subscribersOf(service string) []eventdriven.SubscriberDef {
	var defs []eventdriven.SubscriberDef
	for _, s := range subscribers {
		if s.service == service {
			defs = append(defs, s.def)
		}
	}
	return defs
}
