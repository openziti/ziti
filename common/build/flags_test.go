package build

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseBuildFlags(t *testing.T) {
	tests := []struct {
		name     string
		raw      string
		expected []string
	}{
		{
			name:     "empty string yields an empty, non nil slice",
			raw:      "",
			expected: []string{},
		},
		{
			name:     "single name",
			raw:      "ALPHA",
			expected: []string{"ALPHA"},
		},
		{
			name:     "multiple names keep their order",
			raw:      "ALPHA,BRAVO,CHARLIE",
			expected: []string{"ALPHA", "BRAVO", "CHARLIE"},
		},
		{
			name:     "surrounding whitespace is trimmed",
			raw:      "  ALPHA , BRAVO\t,\nCHARLIE  ",
			expected: []string{"ALPHA", "BRAVO", "CHARLIE"},
		},
		{
			name:     "blank tokens are dropped",
			raw:      ",ALPHA,,  ,BRAVO,",
			expected: []string{"ALPHA", "BRAVO"},
		},
		{
			name:     "duplicates are dropped, first occurrence wins",
			raw:      "ALPHA,BRAVO,ALPHA",
			expected: []string{"ALPHA", "BRAVO"},
		},
		{
			name:     "digits and underscores are accepted",
			raw:      "ALPHA_2,BRAVO_MODE,3",
			expected: []string{"ALPHA_2", "BRAVO_MODE", "3"},
		},
		{
			name:     "malformed tokens are dropped, well formed ones survive",
			raw:      "alpha,BRAVO,Char-lie,DELTA ECHO,FOXTROT!,GOLF",
			expected: []string{"BRAVO", "GOLF"},
		},
		{
			name:     "names longer than the cap are dropped",
			raw:      "ALPHA," + strings.Repeat("B", maxBuildFlagNameLength+1) + ",CHARLIE",
			expected: []string{"ALPHA", "CHARLIE"},
		},
		{
			name:     "names exactly at the length cap are kept",
			raw:      strings.Repeat("B", maxBuildFlagNameLength),
			expected: []string{strings.Repeat("B", maxBuildFlagNameLength)},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := parseBuildFlags(test.raw)

			require.Equal(t, test.expected, result)
		})
	}
}

func TestParseBuildFlagsStopsAtTheCountCap(t *testing.T) {
	names := make([]string, 0, maxBuildFlags+5)
	for i := 0; i < maxBuildFlags+5; i++ {
		names = append(names, "NAME_"+strings.Repeat("X", i))
	}

	result := parseBuildFlags(strings.Join(names, ","))

	require.Len(t, result, maxBuildFlags, "accepted names must be capped")
	require.Equal(t, names[:maxBuildFlags], result, "the first names up to the cap are the ones kept")
}
