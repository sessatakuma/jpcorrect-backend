package domain

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// eventModeValues returns the value of every constant declared with type
// EventMode in the non-test files of this package, so the test below follows the
// real declarations instead of a list it carries itself.
func eventModeValues(t *testing.T) []string {
	t.Helper()

	entries, err := os.ReadDir(".")
	require.NoError(t, err)

	var values []string
	fset := token.NewFileSet()
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(fset, name, nil, 0)
		require.NoError(t, err)

		for _, decl := range file.Decls {
			gen, ok := decl.(*ast.GenDecl)
			if !ok || gen.Tok != token.CONST {
				continue
			}
			for _, spec := range gen.Specs {
				valueSpec, ok := spec.(*ast.ValueSpec)
				if !ok {
					continue
				}
				typeName, ok := valueSpec.Type.(*ast.Ident)
				if !ok || typeName.Name != "EventMode" {
					continue
				}
				for _, value := range valueSpec.Values {
					lit, ok := value.(*ast.BasicLit)
					require.True(t, ok, "EventMode constants must be string literals")
					unquoted, err := strconv.Unquote(lit.Value)
					require.NoError(t, err)
					values = append(values, unquoted)
				}
			}
		}
	}
	sort.Strings(values)
	return values
}

// TestEventModeHasNoDiscussion pins spec §2.1: discussion practice was
// confirmed as Won't Have before implementation started, so
// `discussion` is not a supported EventMode. It reads the constant
// declarations, so adding EventModeDiscussion fails it.
func TestEventModeHasNoDiscussion(t *testing.T) {
	modes := eventModeValues(t)

	assert.NotContains(t, modes, "discussion", "discussion practice is Won't Have, so it is not an EventMode")
	assert.Equal(t, []string{"conversation", "report", "review"}, modes)
}

// TestMistakeEnums pins the spec §2.1 enum values of the new Mistake columns.
func TestMistakeEnums(t *testing.T) {
	assert.Equal(t, []MistakeConfidence{"low", "medium", "high"}, []MistakeConfidence{
		MistakeConfidenceLow,
		MistakeConfidenceMedium,
		MistakeConfidenceHigh,
	})
	assert.Equal(t, []MistakeCorrectionSource{"initial", "chatbot"}, []MistakeCorrectionSource{
		MistakeCorrectionSourceInitial,
		MistakeCorrectionSourceChatbot,
	})
}
