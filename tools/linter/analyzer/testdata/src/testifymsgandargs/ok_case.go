package testifymsgandargs

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// msgAndArgs なし: OK
func TestNoMsgAndArgs(t *testing.T) {
	assert.Equal(t, 1, 1)
	assert.NoError(t, nil)
	assert.True(t, true)
	assert.Nil(t, nil)
	require.Equal(t, 1, 1)
	require.NoError(t, nil)
}

// 正規の用法: OK
func TestWithCorrectUsage(t *testing.T) {
	value := 42
	assert.Equal(t, 42, value)
	require.NoError(t, nil)
	assert.Contains(t, "hello world", "hello")
}
