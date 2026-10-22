package model

// gqlgen.yml でこの型を StarterDeck に使わせる（生成させない）。cards は resolver がカードを引いて並べてから返すため、
// read model から読んだ公開 ID と枚数を、resolver に渡すまで CardCounts に持つ（PresetDeck と同じ）
type StarterDeck struct {
	Name       string
	Source     *StarterDeckSource
	CardCounts []DeckCardCount
}
