package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestTitleToFilename(t *testing.T) {
	cases := []struct {
		input    string
		expected string
	}{
		{
			input:    "Show HN: WildCard, a retro Hypercard/HyperTalk simulator",
			expected: "wildcard-a-retro-hypercard-hypertalk-simulator.html",
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.input, func(t *testing.T) {
			t.Parallel()
			actual := titleToFilename(tc.input)
			assert.Equal(t, tc.expected, actual)
		})
	}
}
