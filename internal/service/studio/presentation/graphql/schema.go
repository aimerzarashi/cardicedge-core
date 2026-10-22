package graphql

import (
	"io"

	"github.com/vektah/gqlparser/v2/formatter"
)

// gqlgen の生成コードがバイナリに埋め込んで動かしているスキーマを、そのまま書く
// （本番用のイメージには .graphqls がファイルとしては入っていないため。core#22）
func WriteSchema(w io.Writer) {
	formatter.NewFormatter(w).FormatSchema(NewExecutableSchema(Config{}).Schema())
}
