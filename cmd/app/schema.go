package main

import (
	"fmt"
	"io"
	"maps"
	"slices"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/game"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/studio"
)

// 入口ごとの GraphQL のスキーマ。studio・game は、前提にする版のイメージからこれを取り出して型を生成する（core#22）
var schemaWriters = map[string]func(io.Writer){
	"game":   game.WriteSchema,
	"studio": studio.WriteSchema,
}

func writeSchema(name string, w io.Writer) error {
	write, ok := schemaWriters[name]
	if !ok {
		return fmt.Errorf("unknown schema %q (known: %v)", name, slices.Sorted(maps.Keys(schemaWriters)))
	}
	write(w)
	return nil
}
