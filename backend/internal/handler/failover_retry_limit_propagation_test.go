//go:build unit

package handler

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestHandleFailoverError_ProductionCallersPassAccountRetryLimit(t *testing.T) {
	_, currentFile, _, ok := runtime.Caller(0)
	require.True(t, ok)
	dir := filepath.Dir(currentFile)
	expected := map[string]int{
		"gateway_handler.go":                  2,
		"gateway_handler_chat_completions.go": 1,
		"gateway_handler_responses.go":        1,
		"gemini_v1beta_handler.go":            1,
	}

	paths, err := filepath.Glob(filepath.Join(dir, "*.go"))
	require.NoError(t, err)
	actual := make(map[string]int)
	for _, path := range paths {
		filename := filepath.Base(path)
		if strings.HasSuffix(filename, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		require.NoError(t, err)

		ast.Inspect(file, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok {
				return true
			}
			selector, ok := call.Fun.(*ast.SelectorExpr)
			if !ok || selector.Sel.Name != "HandleFailoverError" {
				return true
			}
			require.Len(t, call.Args, 6, filename)
			retryGetter, ok := call.Args[4].(*ast.CallExpr)
			require.True(t, ok, "%s must pass the selected account retry limit", filename)
			getterSelector, ok := retryGetter.Fun.(*ast.SelectorExpr)
			require.True(t, ok, filename)
			require.Equal(t, "GetPoolModeRetryCount", getterSelector.Sel.Name, filename)
			actual[filename]++
			return true
		})
	}
	require.Equal(t, expected, actual)
}
