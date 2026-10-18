package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/vektah/gqlparser/v2"
	"github.com/vektah/gqlparser/v2/ast"
	"github.com/vektah/gqlparser/v2/formatter"
)

func formatSchema(t *testing.T, sources ...*ast.Source) string {
	t.Helper()
	schema, err := gqlparser.LoadSchema(sources...)
	require.NoError(t, err)
	var b strings.Builder
	formatter.NewFormatter(&b).FormatSchema(schema)
	return b.String()
}

// 出力だけを読み直すと .graphqls と同じスキーマになる（書き出しで落ちるものが無い。studio は出力だけを codegen で読む）
func TestWriteSchema_WritesSameSchemaAsGraphqlsFiles(t *testing.T) {
	paths, err := filepath.Glob("../../internal/studio/uif/graphql/*.graphqls")
	require.NoError(t, err)
	require.NotEmpty(t, paths)
	var sources []*ast.Source
	for _, path := range paths {
		input, err := os.ReadFile(path)
		require.NoError(t, err)
		sources = append(sources, &ast.Source{Name: path, Input: string(input)})
	}
	var out bytes.Buffer

	require.NoError(t, writeSchema("studio", &out))

	require.Equal(t, formatSchema(t, sources...), formatSchema(t, &ast.Source{Name: "out", Input: out.String()}))
}

func TestWriteSchema_RejectsUnknownName(t *testing.T) {
	var out bytes.Buffer

	err := writeSchema("unknown", &out)

	require.ErrorContains(t, err, `unknown schema "unknown"`)
	require.Zero(t, out.Len())
}
