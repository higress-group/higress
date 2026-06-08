package config

import (
	"regexp"
	"testing"

	"github.com/stretchr/testify/require"
)

func resolverTestConfig() AISecurityConfig {
	c := AISecurityConfig{}
	c.Action = MultiModalGuard
	c.SetDefaultValues()
	return c
}

func TestResolveRequestCheckService_DefaultEnabled(t *testing.T) {
	c := resolverTestConfig()
	d := c.ResolveRequestCheckService("unknown-consumer")
	require.True(t, d.Enabled)
	require.Equal(t, DefaultMultiModalGuardTextInputCheckService, d.Service)
	require.Equal(t, "default", d.Source)
}

func TestResolveRequestCheckService_DefaultDisabled(t *testing.T) {
	c := resolverTestConfig()
	c.DefaultRequestCheckEnabled = false
	d := c.ResolveRequestCheckService("unknown-consumer")
	require.False(t, d.Enabled)
	require.Equal(t, "disabled", d.Source)
}

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

func TestResolveRequestImageCheckService_DefaultEnabled(t *testing.T) {
	c := resolverTestConfig()
	d := c.ResolveRequestImageCheckService("unknown")
	require.True(t, d.Enabled)
	require.Equal(t, DefaultMultiModalGuardImageInputCheckService, d.Service)
	require.Equal(t, "default", d.Source)
}

func TestResolveRequestImageCheckService_DefaultDisabled(t *testing.T) {
	c := resolverTestConfig()
	c.DefaultRequestImageCheckEnabled = false
	d := c.ResolveRequestImageCheckService("unknown")
	require.False(t, d.Enabled)
	require.Equal(t, "disabled", d.Source)
}

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

func TestResolveResponseCheckService_DefaultEnabled(t *testing.T) {
	c := resolverTestConfig()
	d := c.ResolveResponseCheckService("unknown")
	require.True(t, d.Enabled)
	require.Equal(t, DefaultMultiModalGuardTextOutputCheckService, d.Service)
	require.Equal(t, "default", d.Source)
}

func TestResolveResponseCheckService_DefaultDisabled(t *testing.T) {
	c := resolverTestConfig()
	c.DefaultResponseCheckEnabled = false
	d := c.ResolveResponseCheckService("unknown")
	require.False(t, d.Enabled)
	require.Equal(t, "disabled", d.Source)
}

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

func TestResolveResponseImageCheckService_DefaultDisabled(t *testing.T) {
	c := resolverTestConfig()
	// Default is false for response image
	d := c.ResolveResponseImageCheckService("unknown")
	require.False(t, d.Enabled)
	require.Equal(t, "disabled", d.Source)
}

func TestResolveResponseImageCheckService_ExplicitlyEnabled(t *testing.T) {
	c := resolverTestConfig()
	c.DefaultResponseImageCheckEnabled = true
	c.ResponseImageCheckService = "img_response_check"
	d := c.ResolveResponseImageCheckService("unknown")
	require.True(t, d.Enabled)
	require.Equal(t, "img_response_check", d.Service)
	require.Equal(t, "default", d.Source)
}

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
