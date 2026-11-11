package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/board"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/card"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/deck"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/match"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/npcdeck"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/orm/sqlboiler/models"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/rdb"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/game/application/master"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/game/application/query"
)

// 対戦を始めるときに、ボードとデッキのカードを写す元を読む（decks・npc_decks・boards の read model）。
// デッキのカードは read model が公開 ID で持つので、game の内部 ID（domain_entities）に引き直す。
// デッキの集約はカードを持たない（Decide が読まないため）ので、集約ではなく read model から読む。
// 公開中かは見ない（選んだ後に使用不可になっても、対戦は始めた時点の写しで進むため）
type MatchSourceReader struct {
	entityRefs domain.EntityRefResolver
}

func NewMatchSourceReader(entityRefs domain.EntityRefResolver) *MatchSourceReader {
	return &MatchSourceReader{entityRefs: entityRefs}
}

type matchDeckMetadata struct {
	Cards []struct {
		CardID uuid.UUID `json:"card_id"`
		Count  int       `json:"count"`
	} `json:"cards"`
}

func (r *MatchSourceReader) publicID(ctx context.Context, ref domain.EntityRef) (uuid.UUID, error) {
	publicIDs, err := r.entityRefs.PublicIDs(ctx, []domain.EntityRef{ref})
	if err != nil {
		return uuid.Nil, err
	}
	publicID, ok := publicIDs[ref]
	if !ok {
		return uuid.Nil, fmt.Errorf("repository: %s %d has no public id: %w", ref.EntityType(), ref.ID(), domain.ErrEntityNotFound)
	}
	return publicID, nil
}

func (r *MatchSourceReader) PlayerDeckCards(ctx context.Context, id deck.ID) ([]master.DeckCard, error) {
	publicID, err := r.publicID(ctx, domain.NewEntityRef(id.EntityType(), id.Int64()))
	if err != nil {
		return nil, err
	}
	exec, err := rdb.GetExecutor(ctx)
	if err != nil {
		return nil, err
	}
	row, err := models.Decks(models.DeckWhere.DeckID.EQ(publicID.String())).One(ctx, exec)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("decks of %s is not projected yet: %w", publicID, domain.ErrEntityNotFound)
	}
	if err != nil {
		return nil, err
	}
	return r.deckCards(ctx, "decks", publicID, row.Metadata)
}

func (r *MatchSourceReader) NPCDeckCards(ctx context.Context, id npcdeck.ID) ([]master.DeckCard, error) {
	publicID, err := r.publicID(ctx, domain.NewEntityRef(id.EntityType(), id.Int64()))
	if err != nil {
		return nil, err
	}
	exec, err := rdb.GetExecutor(ctx)
	if err != nil {
		return nil, err
	}
	row, err := models.NPCDecks(models.NPCDeckWhere.NPCDeckID.EQ(publicID.String())).One(ctx, exec)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("npc_decks of %s is not published to game: %w", publicID, domain.ErrEntityNotFound)
	}
	if err != nil {
		return nil, err
	}
	return r.deckCards(ctx, "npc_decks", publicID, row.Metadata)
}

func (r *MatchSourceReader) deckCards(ctx context.Context, table string, id uuid.UUID, raw []byte) ([]master.DeckCard, error) {
	var metadata matchDeckMetadata
	if err := json.Unmarshal(raw, &metadata); err != nil {
		return nil, fmt.Errorf("%s.metadata of %s: %w", table, id, err)
	}
	cardIDs := make([]uuid.UUID, len(metadata.Cards))
	for i, c := range metadata.Cards {
		cardIDs[i] = c.CardID
	}
	refs, err := r.entityRefs.ResolveAll(ctx, cardIDs)
	if err != nil {
		return nil, err
	}
	cards := make([]master.DeckCard, 0, len(metadata.Cards))
	for _, c := range metadata.Cards {
		entityRef, ok := refs[c.CardID]
		if !ok {
			return nil, fmt.Errorf("%s.metadata of %s: card %s: %w", table, id, c.CardID, domain.ErrEntityNotFound)
		}
		ref, err := card.NewRef(entityRef.EntityType(), entityRef.ID())
		if err != nil {
			return nil, fmt.Errorf("%s.metadata of %s: card %s: %w", table, id, c.CardID, err)
		}
		cards = append(cards, master.DeckCard{Card: ref, Count: c.Count})
	}
	return cards, nil
}

// ボードの写し。大きさの範囲は写した値から作る（範囲の決まりは studio がボードを作るときに確かめ済みで、ここでは値の型にするだけ）
func (r *MatchSourceReader) Board(ctx context.Context, id board.ID) (match.Board, error) {
	publicID, err := r.publicID(ctx, domain.NewEntityRef(id.EntityType(), id.Int64()))
	if err != nil {
		return match.Board{}, err
	}
	exec, err := rdb.GetExecutor(ctx)
	if err != nil {
		return match.Board{}, err
	}
	row, err := models.Boards(models.BoardWhere.BoardID.EQ(publicID.String())).One(ctx, exec)
	if errors.Is(err, sql.ErrNoRows) {
		return match.Board{}, fmt.Errorf("boards of %s is not published to game: %w", publicID, domain.ErrEntityNotFound)
	}
	if err != nil {
		return match.Board{}, err
	}
	var metadata boardMetadata
	if err := json.Unmarshal(row.Metadata, &metadata); err != nil {
		return match.Board{}, fmt.Errorf("boards.metadata of %s: %w", publicID, err)
	}
	if metadata.Pairs == nil {
		return match.Board{}, fmt.Errorf("boards.metadata of %s: pairs is missing", publicID)
	}
	snapshot, err := toMatchBoard(id, metadata)
	if err != nil {
		return match.Board{}, fmt.Errorf("boards.metadata of %s: %w", publicID, err)
	}
	return snapshot, nil
}

// 大きさの範囲は、今の値がそのまま入る広さをその場で作る。大きさは studio が範囲（studio_rules）で確かめてから公開したもので、
// game は studio_rules を持たず、読むときは確かめ直さないため
func toMatchBoard(id board.ID, metadata boardMetadata) (match.Board, error) {
	minSize, err := board.NewMinSize(1)
	if err != nil {
		return match.Board{}, err
	}
	maxSize, err := board.NewMaxSize(max(metadata.Width, metadata.Height, 1))
	if err != nil {
		return match.Board{}, err
	}
	within, err := board.NewSizeRange(minSize, maxSize)
	if err != nil {
		return match.Board{}, err
	}
	width, err := board.NewWidth(metadata.Width, within)
	if err != nil {
		return match.Board{}, err
	}
	height, err := board.NewHeight(metadata.Height, within)
	if err != nil {
		return match.Board{}, err
	}
	position := func(p boardPosition) (board.Position, error) {
		x, err := board.NewX(p.X)
		if err != nil {
			return board.Position{}, err
		}
		y, err := board.NewY(p.Y)
		if err != nil {
			return board.Position{}, err
		}
		return board.Position{X: x, Y: y}, nil
	}
	size := board.Size{Width: width, Height: height}
	snapshot := match.Board{ID: id, Size: size}
	for _, t := range metadata.Tiles {
		at, err := position(boardPosition{X: t.X, Y: t.Y})
		if err != nil {
			return match.Board{}, err
		}
		kind, err := board.NewTileKind(t.Kind)
		if err != nil {
			return match.Board{}, err
		}
		tile := board.Tile{Position: at, Kind: kind}
		if t.Element != "" {
			if tile.Element, err = board.NewElement(t.Element); err != nil {
				return match.Board{}, err
			}
		}
		snapshot.Tiles = append(snapshot.Tiles, tile)
	}
	for _, c := range metadata.Connections {
		a, err := position(c.A)
		if err != nil {
			return match.Board{}, err
		}
		b, err := position(c.B)
		if err != nil {
			return match.Board{}, err
		}
		snapshot.Connections = append(snapshot.Connections, board.NewConnection(a, b))
	}
	for _, p := range *metadata.Pairs {
		a, err := position(p.A)
		if err != nil {
			return match.Board{}, err
		}
		b, err := position(p.B)
		if err != nil {
			return match.Board{}, err
		}
		snapshot.Pairs = append(snapshot.Pairs, board.NewPair(a, b))
	}
	return snapshot, nil
}

// 公開中のカードの内部 ID（game の domain_entities）。公開 ID から引き直す
func (r *MatchSourceReader) PublishedCards(ctx context.Context) ([]card.Ref, error) {
	exec, err := rdb.GetExecutor(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := models.Cards(models.CardWhere.Status.EQ("published")).All(ctx, exec)
	if err != nil {
		return nil, err
	}
	ids := make([]uuid.UUID, len(rows))
	for i, row := range rows {
		if ids[i], err = uuid.Parse(row.CardID); err != nil {
			return nil, fmt.Errorf("cards.card_id %q: %w", row.CardID, err)
		}
	}
	resolved, err := r.entityRefs.ResolveAll(ctx, ids)
	if err != nil {
		return nil, err
	}
	cards := make([]card.Ref, 0, len(ids))
	for _, id := range ids {
		entityRef, ok := resolved[id]
		if !ok {
			return nil, fmt.Errorf("cards of %s: %w", id, domain.ErrEntityNotFound)
		}
		ref, err := card.NewRef(entityRef.EntityType(), entityRef.ID())
		if err != nil {
			return nil, fmt.Errorf("cards of %s: %w", id, err)
		}
		cards = append(cards, ref)
	}
	return cards, nil
}

// 対戦を始めるときに写すカードの性能を、写した cards から 1 回で読む（内部 ID を公開 ID に引き直す）。写していないカードはエラー
func (r *MatchSourceReader) CardSpecs(ctx context.Context, refs []card.Ref) (map[card.Ref]master.CardSpec, error) {
	entityRefs := make([]domain.EntityRef, len(refs))
	for i, ref := range refs {
		entityRefs[i] = domain.NewEntityRef(ref.EntityType, ref.ID)
	}
	publicIDs, err := r.entityRefs.PublicIDs(ctx, entityRefs)
	if err != nil {
		return nil, err
	}
	ids := make([]uuid.UUID, 0, len(publicIDs))
	for _, publicID := range publicIDs {
		ids = append(ids, publicID)
	}
	exec, err := rdb.GetExecutor(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := loadCards(ctx, exec, ids)
	if err != nil {
		return nil, err
	}
	specs := make(map[card.Ref]master.CardSpec, len(refs))
	for i, ref := range refs {
		publicID, ok := publicIDs[entityRefs[i]]
		if !ok {
			return nil, fmt.Errorf("repository: card %s/%d has no public id: %w", ref.EntityType, ref.ID, domain.ErrEntityNotFound)
		}
		row, ok := rows[publicID]
		if !ok {
			return nil, fmt.Errorf("cards of %s is not published: %w", publicID, domain.ErrEntityNotFound)
		}
		spec := master.CardSpec{Cost: row.card.Cost, Rarity: row.card.Rarity}
		if row.card.Creature != nil {
			spec.Creature = &master.CreatureSpec{Elements: row.card.Creature.Elements, Attack: row.card.Creature.Attack, HitPoints: row.card.Creature.HitPoints, ItemRestrictions: row.card.Creature.ItemRestrictions, PlacementRestrictions: row.card.Creature.PlacementRestrictions, Abilities: masterCreatureAbilities(row.card.Creature.Abilities)}
		}
		if row.card.Item != nil {
			spec.Item = &master.ItemSpec{Category: row.card.Item.Category, AttackBonus: row.card.Item.AttackBonus, HitPointsBonus: row.card.Item.HitPointsBonus}
		}
		if row.card.Spell != nil {
			spec.Spell = &master.SpellSpec{}
			effect := row.card.Spell.Effect
			if effect.DrawCards != nil {
				spec.Spell.DrawCards = &master.SpellDrawCards{Count: effect.DrawCards.Count}
			}
			if effect.GainMagic != nil {
				spec.Spell.GainMagic = &master.SpellGainMagic{PerLap: effect.GainMagic.PerLap}
			}
			if effect.RollDice != nil {
				spec.Spell.RollDice = &master.SpellRollDice{Count: effect.RollDice.Count}
			}
			if effect.FixedDiceCurse != nil {
				spec.Spell.FixedDiceCurse = &master.SpellFixedDiceCurse{Value: effect.FixedDiceCurse.Value, Turns: effect.FixedDiceCurse.Turns}
			}
			if effect.Damage != nil {
				spec.Spell.Damage = &master.SpellDamage{Amount: effect.Damage.Amount}
			}
		}
		specs[ref] = spec
	}
	return specs, nil
}

func masterCreatureAbilities(abilities []query.CreatureAbility) []master.CreatureAbility {
	values := make([]master.CreatureAbility, len(abilities))
	for i, ability := range abilities {
		values[i] = master.CreatureAbility{Defensive: ability.Defensive, TollMultiplierPercent: ability.TollMultiplierPercent, FirstStrike: ability.FirstStrike, LastStrike: ability.LastStrike}
	}
	return values
}
