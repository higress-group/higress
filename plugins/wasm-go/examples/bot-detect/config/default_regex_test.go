package config

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDefaultBotRegexRequiresLiteralDotInHostname(t *testing.T) {
	rule := DefaultBotRegex[len(DefaultBotRegex)-1]

	require.True(t, rule.MatchString("boitho.com-dc/1.0"))
	require.False(t, rule.MatchString("boithoXcom-dc/1.0"))
}
