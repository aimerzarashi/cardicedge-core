#!/bin/sh
# make gen-api（gqlgen generate）をやり直しても、生成コードに差分が出ないことの検査。
# .graphqls を変えて生成をし忘れると、gqlgen が埋め込んで動かすスキーマ（入力の検査と `app schema` の出力）と、
# 生成した実行のコードがずれ、studio・game にも動かない項目を含むスキーマを渡してしまうため（core#22）。
# lint はファイルを書き換えないため、複製した木で生成して比べる
set -eu

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

# CI の cache（.cache-ci。core#78）は写さない（大きく、生成には要らないため）
tar --exclude=.git --exclude=./.cache-ci -cf - . | tar -xf - -C "$tmp"

status=0
for dir in internal/service/studio/presentation/graphql internal/service/game/presentation/graphql; do
  (cd "$tmp/$dir" && go tool gqlgen generate)
  if ! diff -r "$dir" "$tmp/$dir"; then
    echo "check-gen-api: $dir の生成コードが .graphqls と合っていない。make gen-api を実行してコミットする" >&2
    status=1
  fi
done
exit $status
