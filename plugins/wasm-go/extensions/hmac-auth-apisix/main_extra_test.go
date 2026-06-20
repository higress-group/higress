// Copyright (c) 2025 Alibaba Group Holding Ltd.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0

package main

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm/types"
	"github.com/higress-group/wasm-go/pkg/test"
	"github.com/stretchr/testify/require"
)

// findHeader returns the value of the first matching header (case-insensitive).
func findHeader(headers [][2]string, key string) (string, bool) {
	k := strings.ToLower(key)
	for _, h := range headers {
		if strings.ToLower(h[0]) == k {
			return h[1], true
		}
	}
	return "", false
}

// gmt returns the current time formatted as the HTTP Date header expects.
func gmt() string { return time.Now().UTC().Format("Mon, 02 Jan 2006 15:04:05 GMT") }

// authHeaderRequestTargetDate builds a Signature header for (method, path) with
// a single signed "date" header in addition to @request-target.
func authHeaderRequestTargetDate(ak, sk, alg, method, path, date string) string {
	return generateAuthorizationHeader(ak, sk, alg, method, path,
		[]string{"@request-target", "date"},
		map[string]string{"date": date},
	)
}

// === Algorithm matrix ====================================================

func TestHmacAlgorithmMatrix_AllSupportedAlgs_RoundTripSucceeds(t *testing.T) {
	for _, alg := range []string{"hmac-sha1", "hmac-sha256", "hmac-sha512"} {
		t.Run(alg, func(t *testing.T) {
			test.RunTest(t, func(t *testing.T) {
				host, status := test.NewTestHost(createConfig(
					[]map[string]interface{}{{"name": "c1", "access_key": "ak1", "secret_key": "sk1"}},
					map[string]interface{}{"global_auth": true},
				))
				defer host.Reset()
				require.Equal(t, types.OnPluginStartStatusOK, status)

				d := gmt()
				ah := authHeaderRequestTargetDate("ak1", "sk1", alg, "GET", "/p", d)
				action := host.CallOnHttpRequestHeaders([][2]string{
					{":authority", "e.com"}, {":path", "/p"}, {":method", "GET"},
					{"authorization", ah}, {"date", d},
				})
				require.Equal(t, types.ActionContinue, action)
				require.Nil(t, host.GetLocalResponse())
			})
		})
	}
}

func TestHmacAlgorithm_RestrictedByAllowedAlgorithms_Rejects(t *testing.T) {
	// allowed_algorithms is narrowed to sha1; client sends sha256 → "Invalid algorithm".
	test.RunTest(t, func(t *testing.T) {
		host, status := test.NewTestHost(createConfig(
			[]map[string]interface{}{{"name": "c1", "access_key": "ak1", "secret_key": "sk1"}},
			map[string]interface{}{
				"global_auth":        true,
				"allowed_algorithms": []string{"hmac-sha1"},
			},
		))
		defer host.Reset()
		require.Equal(t, types.OnPluginStartStatusOK, status)

		d := gmt()
		ah := authHeaderRequestTargetDate("ak1", "sk1", "hmac-sha256", "GET", "/p", d)
		_ = host.CallOnHttpRequestHeaders([][2]string{
			{":authority", "e.com"}, {":path", "/p"}, {":method", "GET"},
			{"authorization", ah}, {"date", d},
		})
		resp := host.GetLocalResponse()
		require.NotNil(t, resp)
		require.Equal(t, uint32(401), resp.StatusCode)
		require.Contains(t, string(resp.Data), "Invalid algorithm")
	})
}

func TestHmacAlgorithm_UnknownAlgorithmInAuthHeader_Rejects(t *testing.T) {
	// Client crafts a Signature header advertising hmac-md5. allowed_algorithms
	// default is the three supported algs → rejected before generateHmacSignature
	// is even reached (hits the "Invalid algorithm" branch).
	test.RunTest(t, func(t *testing.T) {
		host, status := test.NewTestHost(createConfig(
			[]map[string]interface{}{{"name": "c1", "access_key": "ak1", "secret_key": "sk1"}},
			map[string]interface{}{"global_auth": true},
		))
		defer host.Reset()
		require.Equal(t, types.OnPluginStartStatusOK, status)

		d := gmt()
		ah := `Signature keyId="ak1",algorithm="hmac-md5",signature="abc",headers="@request-target date"`
		_ = host.CallOnHttpRequestHeaders([][2]string{
			{":authority", "e.com"}, {":path", "/p"}, {":method", "GET"},
			{"authorization", ah}, {"date", d},
		})
		resp := host.GetLocalResponse()
		require.NotNil(t, resp)
		require.Equal(t, uint32(401), resp.StatusCode)
		require.Contains(t, string(resp.Data), "Invalid algorithm")
	})
}

// === signed_headers ======================================================

func TestSignedHeaders_RequiredHeaderMissingFromSigning_Rejects(t *testing.T) {
	// Server requires `host` in the signing list but client only signed
	// @request-target+date → 401 expected header "host" missing in signing.
	test.RunTest(t, func(t *testing.T) {
		host, status := test.NewTestHost(createConfig(
			[]map[string]interface{}{{"name": "c1", "access_key": "ak1", "secret_key": "sk1"}},
			map[string]interface{}{
				"global_auth":    true,
				"signed_headers": []string{"host"},
			},
		))
		defer host.Reset()
		require.Equal(t, types.OnPluginStartStatusOK, status)

		d := gmt()
		ah := authHeaderRequestTargetDate("ak1", "sk1", "hmac-sha256", "GET", "/p", d)
		_ = host.CallOnHttpRequestHeaders([][2]string{
			{":authority", "e.com"}, {":path", "/p"}, {":method", "GET"},
			{"authorization", ah}, {"date", d},
		})
		resp := host.GetLocalResponse()
		require.NotNil(t, resp)
		require.Equal(t, uint32(401), resp.StatusCode)
		require.Contains(t, string(resp.Data), `expected header "host" missing in signing`)
	})
}

func TestSignedHeaders_ClientSentEmptyHeadersField_Rejects(t *testing.T) {
	// signed_headers is configured, but client advertises headers="" so the
	// Authorization header carries no signed-header list at all → 401 headers missing.
	test.RunTest(t, func(t *testing.T) {
		host, status := test.NewTestHost(createConfig(
			[]map[string]interface{}{{"name": "c1", "access_key": "ak1", "secret_key": "sk1"}},
			map[string]interface{}{
				"global_auth":    true,
				"signed_headers": []string{"date"},
			},
		))
		defer host.Reset()
		require.Equal(t, types.OnPluginStartStatusOK, status)

		// Sign over no headers.
		ah := generateAuthorizationHeader("ak1", "sk1", "hmac-sha256", "GET", "/p", nil, nil)
		require.NotContains(t, ah, "headers=")
		_ = host.CallOnHttpRequestHeaders([][2]string{
			{":authority", "e.com"}, {":path", "/p"}, {":method", "GET"},
			{"authorization", ah},
		})
		resp := host.GetLocalResponse()
		require.NotNil(t, resp)
		require.Equal(t, uint32(401), resp.StatusCode)
		require.Contains(t, string(resp.Data), "headers missing")
	})
}

func TestSignedHeaders_AllRequiredHeadersSigned_Succeeds(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		host, status := test.NewTestHost(createConfig(
			[]map[string]interface{}{{"name": "c1", "access_key": "ak1", "secret_key": "sk1"}},
			map[string]interface{}{
				"global_auth":    true,
				"signed_headers": []string{"DATE"}, // case-insensitive lookup
			},
		))
		defer host.Reset()
		require.Equal(t, types.OnPluginStartStatusOK, status)

		d := gmt()
		ah := authHeaderRequestTargetDate("ak1", "sk1", "hmac-sha256", "GET", "/p", d)
		action := host.CallOnHttpRequestHeaders([][2]string{
			{":authority", "e.com"}, {":path", "/p"}, {":method", "GET"},
			{"authorization", ah}, {"date", d},
		})
		require.Equal(t, types.ActionContinue, action)
		require.Nil(t, host.GetLocalResponse())
	})
}

// === clock_skew ==========================================================

func TestClockSkew_DateMissing_Rejects(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		// Override the helper's default clock_skew=0 by re-marshalling.
		raw, _ := json.Marshal(map[string]interface{}{
			"consumers":   []map[string]interface{}{{"name": "c1", "access_key": "ak1", "secret_key": "sk1"}},
			"global_auth": true,
			"clock_skew":  300,
		})
		host, status := test.NewTestHost(raw)
		defer host.Reset()
		require.Equal(t, types.OnPluginStartStatusOK, status)

		// Sign only @request-target (no date header in the signature either).
		ah := generateAuthorizationHeader("ak1", "sk1", "hmac-sha256", "GET", "/p",
			[]string{"@request-target"}, nil)
		_ = host.CallOnHttpRequestHeaders([][2]string{
			{":authority", "e.com"}, {":path", "/p"}, {":method", "GET"},
			{"authorization", ah},
		})
		resp := host.GetLocalResponse()
		require.NotNil(t, resp)
		require.Equal(t, uint32(401), resp.StatusCode)
		require.Contains(t, string(resp.Data), "Date header missing")
	})
}

func TestClockSkew_InvalidDateFormat_Rejects(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		raw, _ := json.Marshal(map[string]interface{}{
			"consumers":   []map[string]interface{}{{"name": "c1", "access_key": "ak1", "secret_key": "sk1"}},
			"global_auth": true,
			"clock_skew":  300,
		})
		host, status := test.NewTestHost(raw)
		defer host.Reset()
		require.Equal(t, types.OnPluginStartStatusOK, status)

		badDate := "2025-05-25T10:00:00Z" // ISO-8601, not RFC 1123 GMT
		ah := authHeaderRequestTargetDate("ak1", "sk1", "hmac-sha256", "GET", "/p", badDate)
		_ = host.CallOnHttpRequestHeaders([][2]string{
			{":authority", "e.com"}, {":path", "/p"}, {":method", "GET"},
			{"authorization", ah}, {"date", badDate},
		})
		resp := host.GetLocalResponse()
		require.NotNil(t, resp)
		require.Equal(t, uint32(401), resp.StatusCode)
		require.Contains(t, string(resp.Data), "Invalid GMT format time")
	})
}

func TestClockSkew_StaleDate_Rejects(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		raw, _ := json.Marshal(map[string]interface{}{
			"consumers":   []map[string]interface{}{{"name": "c1", "access_key": "ak1", "secret_key": "sk1"}},
			"global_auth": true,
			"clock_skew":  10, // 10 seconds; we'll use a date an hour old
		})
		host, status := test.NewTestHost(raw)
		defer host.Reset()
		require.Equal(t, types.OnPluginStartStatusOK, status)

		stale := time.Now().Add(-1 * time.Hour).UTC().Format("Mon, 02 Jan 2006 15:04:05 GMT")
		ah := authHeaderRequestTargetDate("ak1", "sk1", "hmac-sha256", "GET", "/p", stale)
		_ = host.CallOnHttpRequestHeaders([][2]string{
			{":authority", "e.com"}, {":path", "/p"}, {":method", "GET"},
			{"authorization", ah}, {"date", stale},
		})
		resp := host.GetLocalResponse()
		require.NotNil(t, resp)
		require.Equal(t, uint32(401), resp.StatusCode)
		require.Contains(t, string(resp.Data), "Clock skew exceeded")
	})
}

func TestClockSkew_WithinTolerance_Succeeds(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		raw, _ := json.Marshal(map[string]interface{}{
			"consumers":   []map[string]interface{}{{"name": "c1", "access_key": "ak1", "secret_key": "sk1"}},
			"global_auth": true,
			"clock_skew":  300,
		})
		host, status := test.NewTestHost(raw)
		defer host.Reset()
		require.Equal(t, types.OnPluginStartStatusOK, status)

		d := gmt()
		ah := authHeaderRequestTargetDate("ak1", "sk1", "hmac-sha256", "GET", "/p", d)
		action := host.CallOnHttpRequestHeaders([][2]string{
			{":authority", "e.com"}, {":path", "/p"}, {":method", "GET"},
			{"authorization", ah}, {"date", d},
		})
		require.Equal(t, types.ActionContinue, action)
		require.Nil(t, host.GetLocalResponse())
	})
}

// === Signature validation ===============================================

func TestValidateSignature_WrongSecret_Rejects(t *testing.T) {
	// Client signs with a wrong secret. The Authorization header is well-formed
	// (keyId matches), but the HMAC won't match the server's recomputation.
	test.RunTest(t, func(t *testing.T) {
		host, status := test.NewTestHost(createConfig(
			[]map[string]interface{}{{"name": "c1", "access_key": "ak1", "secret_key": "real-secret"}},
			map[string]interface{}{"global_auth": true},
		))
		defer host.Reset()
		require.Equal(t, types.OnPluginStartStatusOK, status)

		d := gmt()
		ah := authHeaderRequestTargetDate("ak1", "wrong-secret", "hmac-sha256", "GET", "/p", d)
		_ = host.CallOnHttpRequestHeaders([][2]string{
			{":authority", "e.com"}, {":path", "/p"}, {":method", "GET"},
			{"authorization", ah}, {"date", d},
		})
		resp := host.GetLocalResponse()
		require.NotNil(t, resp)
		require.Equal(t, uint32(401), resp.StatusCode)
		require.Contains(t, string(resp.Data), "Invalid signature")
	})
}

// === hide_credentials ===================================================

func TestHideCredentials_RemovesAuthorizationHeaderAfterSuccess(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		host, status := test.NewTestHost(createConfig(
			[]map[string]interface{}{{"name": "c1", "access_key": "ak1", "secret_key": "sk1"}},
			map[string]interface{}{
				"global_auth":      true,
				"hide_credentials": true,
			},
		))
		defer host.Reset()
		require.Equal(t, types.OnPluginStartStatusOK, status)

		d := gmt()
		ah := authHeaderRequestTargetDate("ak1", "sk1", "hmac-sha256", "GET", "/p", d)
		action := host.CallOnHttpRequestHeaders([][2]string{
			{":authority", "e.com"}, {":path", "/p"}, {":method", "GET"},
			{"authorization", ah}, {"date", d},
		})
		require.Equal(t, types.ActionContinue, action)
		require.Nil(t, host.GetLocalResponse())

		_, present := findHeader(host.GetRequestHeaders(), "authorization")
		require.False(t, present, "authorization header should be removed after successful verification")
		consumer, ok := findHeader(host.GetRequestHeaders(), "X-Mse-Consumer")
		require.True(t, ok)
		require.Equal(t, "c1", consumer)
	})
}

// === global_auth=true + allow exclusion =================================

func TestGlobalAuthTrue_AllowExcludesConsumer_Rejects(t *testing.T) {
	// Token verifies as c1 but allow=[c2] → "consumer 'c1' is not allowed".
	test.RunTest(t, func(t *testing.T) {
		host, status := test.NewTestHost(createConfig(
			[]map[string]interface{}{
				{"name": "c1", "access_key": "ak1", "secret_key": "sk1"},
				{"name": "c2", "access_key": "ak2", "secret_key": "sk2"},
			},
			map[string]interface{}{
				"global_auth": true,
				"_rules_": []map[string]interface{}{{
					"_match_route_": []string{"r1"},
					"allow":         []string{"c2"},
				}},
			},
		))
		defer host.Reset()
		require.Equal(t, types.OnPluginStartStatusOK, status)
		require.NoError(t, host.SetRouteName("r1"))

		d := gmt()
		ah := authHeaderRequestTargetDate("ak1", "sk1", "hmac-sha256", "GET", "/p", d)
		_ = host.CallOnHttpRequestHeaders([][2]string{
			{":authority", "e.com"}, {":path", "/p"}, {":method", "GET"},
			{"authorization", ah}, {"date", d},
		})
		resp := host.GetLocalResponse()
		require.NotNil(t, resp)
		require.Equal(t, uint32(401), resp.StatusCode)
		require.Contains(t, string(resp.Data), `consumer 'c1' is not allowed`)
	})
}

// === Body-validation HeaderStopIteration =================================

func TestBodyValidation_HeaderStageReturnsHeaderStopIteration(t *testing.T) {
	// validate_request_body=true with a request that has a body → header stage
	// must pause until the body arrives. The wasm-go test framework surfaces
	// this as ActionPause.
	test.RunTest(t, func(t *testing.T) {
		host, status := test.NewTestHost(createConfig(
			[]map[string]interface{}{{"name": "c1", "access_key": "ak1", "secret_key": "sk1"}},
			map[string]interface{}{
				"global_auth":           true,
				"validate_request_body": true,
			},
		))
		defer host.Reset()
		require.Equal(t, types.OnPluginStartStatusOK, status)

		body := []byte(`{"hello":"world"}`)
		digest := calculateBodyDigest(body)
		d := gmt()
		ah := authHeaderRequestTargetDate("ak1", "sk1", "hmac-sha256", "POST", "/p", d)
		action := host.CallOnHttpRequestHeaders([][2]string{
			{":authority", "e.com"}, {":path", "/p"}, {":method", "POST"},
			{"authorization", ah}, {"date", d},
			{"digest", digest}, {"content-type", "application/json"},
			{"content-length", "17"},
		})
		require.Equal(t, types.ActionPause, action,
			"validate_request_body=true should pause at header stage")

		// Body stage with matching digest must release the request.
		require.Equal(t, types.ActionContinue, host.CallOnHttpRequestBody(body))
		require.Nil(t, host.GetLocalResponse())
	})
}

// === Anonymous consumer fallback for malformed Authorization =============

func TestAnonymousConsumer_AppliedWhenAuthorizationCannotBeParsed(t *testing.T) {
	// Authorization header present but the prefix isn't "Signature " →
	// retrieveHmacFieldsAndConsumer returns an error → because
	// anonymous_consumer is set, the request continues under that identity.
	test.RunTest(t, func(t *testing.T) {
		host, status := test.NewTestHost(createConfig(
			[]map[string]interface{}{{"name": "c1", "access_key": "ak1", "secret_key": "sk1"}},
			map[string]interface{}{
				"anonymous_consumer": "guest",
			},
		))
		defer host.Reset()
		require.Equal(t, types.OnPluginStartStatusOK, status)

		action := host.CallOnHttpRequestHeaders([][2]string{
			{":authority", "e.com"}, {":path", "/p"}, {":method", "GET"},
			{"authorization", "Bearer not-a-signature"},
		})
		require.Equal(t, types.ActionContinue, action)
		require.Nil(t, host.GetLocalResponse())
		got, ok := findHeader(host.GetRequestHeaders(), "X-Mse-Consumer")
		require.True(t, ok)
		require.Equal(t, "guest", got)
	})
}

// === Name fallback to access_key ========================================

func TestConsumerName_OmittedFallsBackToAccessKey(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		host, status := test.NewTestHost(createConfig(
			[]map[string]interface{}{
				{"access_key": "ak1", "secret_key": "sk1"}, // no name
			},
			map[string]interface{}{"global_auth": true},
		))
		defer host.Reset()
		require.Equal(t, types.OnPluginStartStatusOK, status)

		d := gmt()
		ah := authHeaderRequestTargetDate("ak1", "sk1", "hmac-sha256", "GET", "/p", d)
		require.Equal(t, types.ActionContinue, host.CallOnHttpRequestHeaders([][2]string{
			{":authority", "e.com"}, {":path", "/p"}, {":method", "GET"},
			{"authorization", ah}, {"date", d},
		}))
		got, ok := findHeader(host.GetRequestHeaders(), "X-Mse-Consumer")
		require.True(t, ok)
		require.Equal(t, "ak1", got)
	})
}

// === Authorization parsing edge cases ===================================

func TestAuthorization_AlgorithmFieldMissing_Rejects(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		host, status := test.NewTestHost(createConfig(
			[]map[string]interface{}{{"name": "c1", "access_key": "ak1", "secret_key": "sk1"}},
			map[string]interface{}{"global_auth": true},
		))
		defer host.Reset()
		require.Equal(t, types.OnPluginStartStatusOK, status)

		ah := `Signature keyId="ak1",signature="abc"` // no algorithm field
		_ = host.CallOnHttpRequestHeaders([][2]string{
			{":authority", "e.com"}, {":path", "/p"}, {":method", "GET"},
			{"authorization", ah},
		})
		resp := host.GetLocalResponse()
		require.NotNil(t, resp)
		require.Equal(t, uint32(401), resp.StatusCode)
		require.Contains(t, string(resp.Data), "algorithm missing")
	})
}

func TestAuthorization_UnknownFieldsIgnored(t *testing.T) {
	// Extra unknown keys must not break parsing.
	test.RunTest(t, func(t *testing.T) {
		host, status := test.NewTestHost(createConfig(
			[]map[string]interface{}{{"name": "c1", "access_key": "ak1", "secret_key": "sk1"}},
			map[string]interface{}{"global_auth": true},
		))
		defer host.Reset()
		require.Equal(t, types.OnPluginStartStatusOK, status)

		d := gmt()
		signed := authHeaderRequestTargetDate("ak1", "sk1", "hmac-sha256", "GET", "/p", d)
		// Inject a junk field between known ones.
		ah := strings.Replace(signed, "keyId=", `extra="ignored",keyId=`, 1)
		action := host.CallOnHttpRequestHeaders([][2]string{
			{":authority", "e.com"}, {":path", "/p"}, {":method", "GET"},
			{"authorization", ah}, {"date", d},
		})
		require.Equal(t, types.ActionContinue, action)
		require.Nil(t, host.GetLocalResponse())
	})
}
