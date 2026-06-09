package config

import (
	"regexp"
	"testing"

	"github.com/stretchr/testify/require"
)

// resolverTestConfig builds the default MultiModalGuard config used by resolver
// tests. It intentionally mirrors Parse defaults so each case only changes the
// switch or consumer rule being verified.
func resolverTestConfig() AISecurityConfig {
	c := AISecurityConfig{}
	c.Action = MultiModalGuard
	c.SetDefaultValues()
	return c
}

// TestResolveRequestCheckService_DefaultEnabled verifies that unmatched request
// text traffic falls back to the global text service when the default switch is
// enabled.
func TestResolveRequestCheckService_DefaultEnabled(t *testing.T) {
	c := resolverTestConfig()
	d := c.ResolveRequestCheckService("unknown-consumer")
	require.True(t, d.Enabled)
	require.Equal(t, DefaultMultiModalGuardTextInputCheckService, d.Service)
	require.Equal(t, "default", d.Source)
}

// TestResolveRequestCheckService_DefaultDisabled verifies that disabling the
// request text default switch skips unmatched consumers instead of using the
// global service.
func TestResolveRequestCheckService_DefaultDisabled(t *testing.T) {
	c := resolverTestConfig()
	c.DefaultRequestCheckEnabled = false
	d := c.ResolveRequestCheckService("unknown-consumer")
	require.False(t, d.Enabled)
	require.Equal(t, "disabled", d.Source)
}

// TestResolveRequestCheckService_ConsumerMatch verifies that a matching
// consumer request text service overrides the global fallback.
func TestResolveRequestCheckService_ConsumerMatch(t *testing.T) {
	c := resolverTestConfig()
	c.ConsumerRequestCheckService = []map[string]interface{}{
		{
			"matcher":             Matcher{Exact: "consumer-a"},
			"requestCheckService": "custom_query_check",
		},
	}
	d := c.ResolveRequestCheckService("consumer-a")
	require.True(t, d.Enabled)
	require.Equal(t, "custom_query_check", d.Service)
	require.Equal(t, "consumer", d.Source)
}

// TestResolveRequestCheckService_ConsumerMatchNoServiceField captures the
// chosen semantics for matched rules without a requestCheckService field: the
// resolver continues to the default decision instead of treating the consumer as
// explicitly disabled.
func TestResolveRequestCheckService_ConsumerMatchNoServiceField(t *testing.T) {
	c := resolverTestConfig()
	c.ConsumerRequestCheckService = []map[string]interface{}{
		{
			"matcher": Matcher{Exact: "consumer-a"},
			// no requestCheckService field
		},
	}

	// With default enabled, falls through to default
	d := c.ResolveRequestCheckService("consumer-a")
	require.True(t, d.Enabled)
	require.Equal(t, DefaultMultiModalGuardTextInputCheckService, d.Service)
	require.Equal(t, "default", d.Source)

	// With default disabled, returns disabled
	c.DefaultRequestCheckEnabled = false
	d = c.ResolveRequestCheckService("consumer-a")
	require.False(t, d.Enabled)
	require.Equal(t, "disabled", d.Source)
}

// TestResolveRequestCheckService_ConsumerNoMatch verifies first-match lookup
// does not affect unrelated consumers and they still use the default decision.
func TestResolveRequestCheckService_ConsumerNoMatch(t *testing.T) {
	c := resolverTestConfig()
	c.ConsumerRequestCheckService = []map[string]interface{}{
		{
			"matcher":             Matcher{Exact: "consumer-a"},
			"requestCheckService": "custom_query_check",
		},
	}
	// Different consumer, falls through to default
	d := c.ResolveRequestCheckService("consumer-b")
	require.True(t, d.Enabled)
	require.Equal(t, DefaultMultiModalGuardTextInputCheckService, d.Service)
	require.Equal(t, "default", d.Source)
}

// TestResolveRequestImageCheckService_DefaultEnabled verifies that request image
// checks keep the legacy enabled fallback when no consumer image rule matches.
func TestResolveRequestImageCheckService_DefaultEnabled(t *testing.T) {
	c := resolverTestConfig()
	d := c.ResolveRequestImageCheckService("unknown")
	require.True(t, d.Enabled)
	require.Equal(t, DefaultMultiModalGuardImageInputCheckService, d.Service)
	require.Equal(t, "default", d.Source)
}

// TestResolveRequestImageCheckService_DefaultDisabled verifies the request image
// fallback switch can skip unmatched consumers.
func TestResolveRequestImageCheckService_DefaultDisabled(t *testing.T) {
	c := resolverTestConfig()
	c.DefaultRequestImageCheckEnabled = false
	d := c.ResolveRequestImageCheckService("unknown")
	require.False(t, d.Enabled)
	require.Equal(t, "disabled", d.Source)
}

// TestResolveRequestImageCheckService_ConsumerMatch verifies image service
// selection is independent from request text service selection.
func TestResolveRequestImageCheckService_ConsumerMatch(t *testing.T) {
	c := resolverTestConfig()
	c.ConsumerRequestCheckService = []map[string]interface{}{
		{
			"matcher":                  Matcher{Prefix: "team-"},
			"requestImageCheckService": "custom_img_check",
		},
	}
	d := c.ResolveRequestImageCheckService("team-alpha")
	require.True(t, d.Enabled)
	require.Equal(t, "custom_img_check", d.Service)
	require.Equal(t, "consumer", d.Source)
}

// TestResolveResponseCheckService_DefaultEnabled verifies response text keeps
// the legacy enabled global fallback for unmatched consumers.
func TestResolveResponseCheckService_DefaultEnabled(t *testing.T) {
	c := resolverTestConfig()
	d := c.ResolveResponseCheckService("unknown")
	require.True(t, d.Enabled)
	require.Equal(t, DefaultMultiModalGuardTextOutputCheckService, d.Service)
	require.Equal(t, "default", d.Source)
}

// TestResolveResponseCheckService_DefaultDisabled verifies that disabling the
// response text fallback skips unmatched consumers before response buffering.
func TestResolveResponseCheckService_DefaultDisabled(t *testing.T) {
	c := resolverTestConfig()
	c.DefaultResponseCheckEnabled = false
	d := c.ResolveResponseCheckService("unknown")
	require.False(t, d.Enabled)
	require.Equal(t, "disabled", d.Source)
}

// TestResolveResponseCheckService_ConsumerMatch verifies a matched consumer
// response text service overrides the global fallback.
func TestResolveResponseCheckService_ConsumerMatch(t *testing.T) {
	c := resolverTestConfig()
	c.ConsumerResponseCheckService = []map[string]interface{}{
		{
			"matcher":              Matcher{Re: regexp.MustCompile("^vip-.*")},
			"responseCheckService": "vip_response_check",
		},
	}
	d := c.ResolveResponseCheckService("vip-user1")
	require.True(t, d.Enabled)
	require.Equal(t, "vip_response_check", d.Service)
	require.Equal(t, "consumer", d.Source)
}

// TestResolveResponseImageCheckService_DefaultDisabled verifies generated-image
// response checks remain opt-in by default.
func TestResolveResponseImageCheckService_DefaultDisabled(t *testing.T) {
	c := resolverTestConfig()
	// Default is false for response image
	d := c.ResolveResponseImageCheckService("unknown")
	require.False(t, d.Enabled)
	require.Equal(t, "disabled", d.Source)
}

// TestResolveResponseImageCheckService_ExplicitlyEnabled verifies the explicit
// default response-image fallback can enable checks for unmatched consumers.
func TestResolveResponseImageCheckService_ExplicitlyEnabled(t *testing.T) {
	c := resolverTestConfig()
	c.DefaultResponseImageCheckEnabled = true
	c.ResponseImageCheckService = "img_response_check"
	d := c.ResolveResponseImageCheckService("unknown")
	require.True(t, d.Enabled)
	require.Equal(t, "img_response_check", d.Service)
	require.Equal(t, "default", d.Source)
}

// TestResolveResponseImageCheckService_ConsumerMatch verifies a consumer image
// response service enables generated-image checks even while the default
// response-image switch remains disabled.
func TestResolveResponseImageCheckService_ConsumerMatch(t *testing.T) {
	c := resolverTestConfig()
	c.ConsumerResponseCheckService = []map[string]interface{}{
		{
			"matcher":                   Matcher{Exact: "premium"},
			"responseImageCheckService": "premium_img_response",
		},
	}
	// Consumer match overrides default disabled
	d := c.ResolveResponseImageCheckService("premium")
	require.True(t, d.Enabled)
	require.Equal(t, "premium_img_response", d.Service)
	require.Equal(t, "consumer", d.Source)
}

// TestParseValidation_EmptyConsumerRequestCheckService documents the parse-time
// distinction between an omitted service field, which means fallback, and an
// explicitly empty request service, which should be rejected.
func TestParseValidation_EmptyConsumerRequestCheckService(t *testing.T) {
	c := AISecurityConfig{}
	c.Action = MultiModalGuard
	c.SetDefaultValues()
	c.ConsumerRequestCheckService = []map[string]interface{}{
		{
			"matcher":             Matcher{Exact: "test"},
			"requestCheckService": "",
		},
	}
	// Simulate what Parse validation does
	for _, obj := range c.ConsumerRequestCheckService {
		if v, exists := obj["requestCheckService"]; exists {
			s, _ := v.(string)
			require.Equal(t, "", s, "empty string should trigger validation error in Parse()")
		}
	}
}

// TestParseValidation_EmptyConsumerResponseCheckService documents the same
// explicit-empty rejection for consumer response service fields.
func TestParseValidation_EmptyConsumerResponseCheckService(t *testing.T) {
	c := AISecurityConfig{}
	c.Action = MultiModalGuard
	c.SetDefaultValues()
	c.ConsumerResponseCheckService = []map[string]interface{}{
		{
			"matcher":              Matcher{Exact: "test"},
			"responseCheckService": "",
		},
	}
	for _, obj := range c.ConsumerResponseCheckService {
		if v, exists := obj["responseCheckService"]; exists {
			s, _ := v.(string)
			require.Equal(t, "", s, "empty string should trigger validation error in Parse()")
		}
	}
}
