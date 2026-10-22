package graphql

import (
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/studio/application/query"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/studio/presentation/graphql/model"
)

func fromQueryStarterDeckSet(set query.StarterDeckSet) *model.StarterDeckSet {
	decks := make([]*model.StarterDeck, 0, len(set.Decks))
	for _, deck := range set.Decks {
		counts := make([]model.DeckCardCount, 0, len(deck.Cards))
		for _, card := range deck.Cards {
			counts = append(counts, model.DeckCardCount{CardID: card.CardID, Count: card.Count})
		}
		source := &model.StarterDeckSource{
			PresetDeckID:       deck.Source.PresetDeckID,
			Revision:           int(deck.Source.Revision),
			ChangedSinceCopied: deck.Source.ChangedSinceCopied,
			PresetDeckDeleted:  deck.Source.PresetDeckDeleted,
		}
		decks = append(decks, &model.StarterDeck{Name: deck.Name, Source: source, CardCounts: counts})
	}
	return &model.StarterDeckSet{Decks: decks, Revision: int(set.Revision), UpdatedAt: set.UpdatedAt}
}
