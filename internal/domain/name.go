package domain

import (
	"strings"

	"golang.org/x/text/unicode/norm"
)

// 名前の比べ方（カード・プリセットデッキ・デッキの名前の一意性と、一覧の並び・検索）を 1 つに決める。
// 集約ごとに持つと決まりが割れ、関係の無い集約（card）に依存させることにもなるため、共通の契約に置く。
// 重複の判定と、一覧の並び・検索にだけ使い、表示には使わない（全角「ＡＢＣ」と半角「ABC」、「Apple」と「apple」を
// 同じ名前として扱うため）。検索の入力は空や上限超えもあり得るため、名前の型のメソッドにせず文字列を受け取る。
// 小文字にするのを NFKC の後にするのは、NFKC が「Ⅻ」を「XII」にするように大文字を作ることがあるため
func NormalizeName(value string) string {
	return strings.ToLower(norm.NFKC.String(value))
}
