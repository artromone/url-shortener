package shortcode

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGenerate(t *testing.T) {
	code, err := Generate()
	require.NoError(t, err, "Generate should not return error")
	assert.Len(t, code, 7, "Generated code should be 7 characters")
	assert.Regexp(t, "^[a-zA-Z0-9]{7}$", code, "Code should be alphanumeric")
}

func TestGenerateUniqueness(t *testing.T) {
	seen := make(map[string]bool)
	iterations := 1000

	for i := 0; i < iterations; i++ {
		code, err := Generate()
		require.NoError(t, err)
		assert.False(t, seen[code], "Generated duplicate code: %s", code)
		seen[code] = true
	}

	assert.Len(t, seen, iterations, "Should generate unique codes")
}

func TestGenerateCharacterDistribution(t *testing.T) {
	charCounts := make(map[rune]int)
	iterations := 10000

	for i := 0; i < iterations; i++ {
		code, err := Generate()
		require.NoError(t, err)

		for _, ch := range code {
			charCounts[ch]++
		}
	}

	assert.GreaterOrEqual(t, len(charCounts), 30,
		"Should use diverse character set, got %d unique chars", len(charCounts))
}

func BenchmarkGenerate(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_, _ = Generate()
	}
}
