package domaindef_test

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domaindef"
)

// 集約は 1000 から 100 刻み、イベントはその集約の 100 の中、一意性の key・参照は 1000 から。どれも id・名前が重ならない。
// 決まりから外れた行を、名前つきで集めて比べる（どの行かが分かるように）
func TestTypes_FollowsNumberingRules(t *testing.T) {
	var violations []string
	entityIDs := map[int16]bool{}
	entityNames := map[string]bool{}
	eventIDs := map[int16]bool{}
	for _, entityType := range domaindef.Types.EntityTypes {
		if entityType.ID < 1000 || entityType.ID%100 != 0 || entityIDs[entityType.ID] || entityNames[entityType.Name] {
			violations = append(violations, fmt.Sprintf("entity %d: %s", entityType.ID, entityType.Name))
		}
		entityIDs[entityType.ID] = true
		entityNames[entityType.Name] = true
		eventNames := map[string]bool{}
		for _, event := range entityType.Events {
			if event.ID <= entityType.ID || event.ID >= entityType.ID+100 || eventIDs[event.ID] || eventNames[event.Name] {
				violations = append(violations, fmt.Sprintf("event %d: %s.%s", event.ID, entityType.Name, event.Name))
			}
			eventIDs[event.ID] = true
			eventNames[event.Name] = true
		}
	}
	for _, named := range [][]domaindef.NamedTypeDef{domaindef.Types.UniqueKeyTypes, domaindef.Types.ReferenceTypes} {
		ids := map[int16]bool{}
		names := map[string]bool{}
		for _, def := range named {
			if def.ID < 1000 || ids[def.ID] || names[def.Name] {
				violations = append(violations, fmt.Sprintf("%d: %s", def.ID, def.Name))
			}
			ids[def.ID] = true
			names[def.Name] = true
		}
	}

	require.Empty(t, violations)
}

// 一覧の名前と、各集約の宣言の名前がちょうど一致する（一覧への書き忘れ・名前の食い違いを、起動より前に見つける）
func TestTypes_MatchesDeclarations(t *testing.T) {
	declaredEvents := map[string][]string{}
	for _, aggregate := range aggregates() {
		var events []string
		for _, name := range aggregate.EventTypes {
			events = append(events, string(name))
		}
		declaredEvents[string(aggregate.EntityType)] = events
	}
	catalogEvents := map[string][]string{}
	for _, entityType := range domaindef.Types.EntityTypes {
		var events []string
		for _, event := range entityType.Events {
			events = append(events, event.Name)
		}
		catalogEvents[entityType.Name] = events
	}
	require.Equal(t, declaredEvents, catalogEvents)

	var declaredKeys, catalogKeys []string
	for _, keyType := range uniqueKeyTypes() {
		declaredKeys = append(declaredKeys, keyType.String())
	}
	for _, def := range domaindef.Types.UniqueKeyTypes {
		catalogKeys = append(catalogKeys, def.Name)
	}
	require.ElementsMatch(t, declaredKeys, catalogKeys)

	var declaredReferences, catalogReferences []string
	for _, referenceType := range referenceTypes() {
		declaredReferences = append(declaredReferences, referenceType.String())
	}
	for _, def := range domaindef.Types.ReferenceTypes {
		catalogReferences = append(catalogReferences, def.Name)
	}
	require.ElementsMatch(t, declaredReferences, catalogReferences)
}
