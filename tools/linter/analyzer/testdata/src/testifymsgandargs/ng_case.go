package testifymsgandargs

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// msgAndArgs あり: NG
func TestWithMsgAndArgs(t *testing.T) {
	assert.Equal(t, 1, 1, "values should be equal") // want "testify assertion function should not include msgAndArgs"
	assert.NoError(t, nil, "no error expected")     // want "testify assertion function should not include msgAndArgs"
	assert.True(t, true, "should be true")          // want "testify assertion function should not include msgAndArgs"
	assert.Nil(t, nil, "should be nil")             // want "testify assertion function should not include msgAndArgs"
}

// require での msgAndArgs: NG
func TestRequireWithMsgAndArgs(t *testing.T) {
	require.Equal(t, 1, 1, "values should match")           // want "testify assertion function should not include msgAndArgs"
	require.NoError(t, nil, "error occurred unexpectedly")  // want "testify assertion function should not include msgAndArgs"
	require.Contains(t, "hello", "he", "substring missing") // want "testify assertion function should not include msgAndArgs"
}

// 複数の msgAndArgs: NG
func TestWithMultipleMsgArgs(t *testing.T) {
	assert.Equal(t, 1, 1, "values", "should", "equal") // want "testify assertion function should not include msgAndArgs"
}
