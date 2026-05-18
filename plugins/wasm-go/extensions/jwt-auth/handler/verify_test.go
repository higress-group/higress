package handler

import (
	"encoding/base64"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/alibaba/higress/plugins/wasm-go/extensions/jwt-auth/config"
	"github.com/tidwall/gjson"
)

type testLogger struct {
	T *testing.T
}

func (l *testLogger) Warnf(format string, args ...interface{}) {
	l.T.Logf(format, args...)
}

type testProvider struct {
	headerMap map[string]string
}

func (p *testProvider) GetHttpRequestHeader(key string) (string, error) {
	if v, ok := p.headerMap[key]; ok {
		return v, nil
	}
	return "", errors.New("no found")
}

func (p *testProvider) ReplaceHttpRequestHeader(key string, value string) error {
	p.headerMap[key] = value
	return nil
}

func (p *testProvider) RemoveHttpRequestHeader(key string) error {
	delete(p.headerMap, key)
	return nil
}

const (
	ES256Allow   string = "eyJhbGciOiJFUzI1NiIsImtpZCI6InAyNTYiLCJ0eXAiOiJKV1QifQ.eyJhdWQiOlsiZm9vIiwiYmFyIl0sImV4cCI6MjAxOTY4NjQwMCwiaXNzIjoiaGlncmVzcy10ZXN0IiwibmJmIjoxNzA0MDY3MjAwLCJzdWIiOiJoaWdyZXNzLXRlc3QifQ.hm71YWfjALshUAgyOu-r9W2WBG_zfqIZZacAbc7oIH1r7dbB0sGQn3wKMWMmOzmxX0UyaVZ0KMk-HFTA1hDnBQ"
	ES256Expried string = "eyJhbGciOiJFUzI1NiIsImtpZCI6InAyNTYiLCJ0eXAiOiJKV1QifQ.eyJhdWQiOlsiZm9vIiwiYmFyIl0sImV4cCI6MTcwNDA2NzIwMCwiaXNzIjoiaGlncmVzcy10ZXN0IiwibmJmIjoxNzA0MDY3MjAwLCJzdWIiOiJoaWdyZXNzLXRlc3QifQ.9AnXd2rZ6FirHZQAoabyL4xZNz0jr-3LmcV4-pFV3JrdtUT4386Mw5Qan125fUB-rZf_ZBlv0Bft2tWY149fyg"
	RS256Allow   string = "eyJhbGciOiJSUzI1NiIsImtpZCI6InJzYSIsInR5cCI6IkpXVCJ9.eyJhdWQiOlsiZm9vIiwiYmFyIl0sImV4cCI6MjAxOTY4NjQwMCwiaXNzIjoiaGlncmVzcy10ZXN0IiwibmJmIjoxNzA0MDY3MjAwLCJzdWIiOiJoaWdyZXNzLXRlc3QifQ.iO0wPY91b_VNGUMZ1n-Ub-SRmEkDQMFLSi77z49tEzll3UZXwmBraP5udM_OPUAdk9ZO3dbb_fOgdcN9V1H9p5kiTr-l-pZTFTJHrPJj8wC519sYRcCk3wrZ9aXR5tNMwOsMdQb7waTBatDQLmHPWzAoTNBc8mwXkRcv1dmJLvsJgxyCl1I9CMOMPq0fYj1NBvaUDIdVSL1o7GGiriD8-0UIOmS72-I3mbaoCIyVb0h3wx7gnIW3zr0yYWaYoiIgmHLag-eEGxHp4-BjtCqcokU4QVMS91qpH7Mkl1iv2WHEkuDQRJ-nLzYGwXb7Dncx9K5tNWHJuZ-DihIU2oT0aA"
	RS256Expried string = "eyJhbGciOiJSUzI1NiIsImtpZCI6InJzYSIsInR5cCI6IkpXVCJ9.eyJhdWQiOlsiZm9vIiwiYmFyIl0sImV4cCI6MTcwNDA2NzIwMCwiaXNzIjoiaGlncmVzcy10ZXN0IiwibmJmIjoxNzA0MDY3MjAwLCJzdWIiOiJoaWdyZXNzLXRlc3QifQ.jqzlhBPk9mmvtTT5aCYf-_5uXXSEU5bQ32fx78XeboCnjR9K1CsI4KYUIkXEX3bk66XJQUeSes7lz3gA4Yzkd-v9oADHTgpKnIxzv_5mD0_afIwEFjcalqVbSvCmro4PessQZDnmU7AIzoo3RPSqbmq8xbPVYUH9I-OO8aUu2ATd1HozgxJH1XnRU8k9KMkVW8XhvJXLKZJmnqe3Tu6pCU_tawFlBfBC4fAhMf0yX2CGE0ABAHubcdiI6JXObQmQQ9Or2a-g2a8g_Bw697PoPOsAn0YpTrHst9GcyTpkbNTAq9X8fc5EM7hiDM1FGeMYcaQTdMnOh4HBhP0p4YEhvA"
	JWKs         string = "{\"keys\":[{\"kty\":\"EC\",\"kid\":\"p256\",\"crv\":\"P-256\",\"x\":\"GWym652nfByDbs4EzNpGXCkdjG03qFZHulNDHTo3YJU\",\"y\":\"5uVg_n-flqRJ5Zhf_aEKS0ow9SddTDgxGduSCgpoAZQ\"},{\"kty\":\"RSA\",\"kid\":\"rsa\",\"n\":\"pFKAKJ0V3vFwGTvBSHbPwrNdvPyr-zMTh7Y9IELFIMNUQfG9_d2D1wZcrX5CPvtEISHin3GdPyfqEX6NjPyqvCLFTuNh80-r5Mvld-A5CHwITZXz5krBdqY5Z0wu64smMbzst3HNxHbzLQvHUY-KS6hceOB84d9B4rhkIJEEAWxxIA7yPJYjYyIC_STpPddtJkkweVvoa0m0-_FQkDFsbRS0yGgMNG4-uc7qLIU4kSwMQWcw1Rwy39LUDP4zNzuZABbWsDDBsMlVUaszRdKIlk5AQ-Fkah3E247dYGUQjSQ0N3dFLlMDv_e62BT3IBXGLg7wvGosWFNT_LpIenIW6Q\",\"e\":\"AQAB\"}]}"
)
const (
	consumers = `{
		"consumers": [
			{
				"name": "consumer1",
				"issuer": "higress-test",
				"jwks": "{\n\"keys\": [\n{\n\"kty\": \"EC\",\n\"kid\": \"p256\",\n\"crv\": \"P-256\",\n\"x\": \"GWym652nfByDbs4EzNpGXCkdjG03qFZHulNDHTo3YJU\",\n\"y\": \"5uVg_n-flqRJ5Zhf_aEKS0ow9SddTDgxGduSCgpoAZQ\"\n},\n{\n\"kty\": \"RSA\",\n\"kid\": \"rsa\",\n\"n\": \"pFKAKJ0V3vFwGTvBSHbPwrNdvPyr-zMTh7Y9IELFIMNUQfG9_d2D1wZcrX5CPvtEISHin3GdPyfqEX6NjPyqvCLFTuNh80-r5Mvld-A5CHwITZXz5krBdqY5Z0wu64smMbzst3HNxHbzLQvHUY-KS6hceOB84d9B4rhkIJEEAWxxIA7yPJYjYyIC_STpPddtJkkweVvoa0m0-_FQkDFsbRS0yGgMNG4-uc7qLIU4kSwMQWcw1Rwy39LUDP4zNzuZABbWsDDBsMlVUaszRdKIlk5AQ-Fkah3E247dYGUQjSQ0N3dFLlMDv_e62BT3IBXGLg7wvGosWFNT_LpIenIW6Q\",\n\"e\": \"AQAB\"\n}\n]\n}"
			},
			{
				"name": "consumer_hedaer",
				"issuer": "higress-test",
				"jwks": "{\n\"keys\": [\n{\n\"kty\": \"EC\",\n\"kid\": \"p256\",\n\"crv\": \"P-256\",\n\"x\": \"GWym652nfByDbs4EzNpGXCkdjG03qFZHulNDHTo3YJU\",\n\"y\": \"5uVg_n-flqRJ5Zhf_aEKS0ow9SddTDgxGduSCgpoAZQ\"\n},\n{\n\"kty\": \"RSA\",\n\"kid\": \"rsa\",\n\"n\": \"pFKAKJ0V3vFwGTvBSHbPwrNdvPyr-zMTh7Y9IELFIMNUQfG9_d2D1wZcrX5CPvtEISHin3GdPyfqEX6NjPyqvCLFTuNh80-r5Mvld-A5CHwITZXz5krBdqY5Z0wu64smMbzst3HNxHbzLQvHUY-KS6hceOB84d9B4rhkIJEEAWxxIA7yPJYjYyIC_STpPddtJkkweVvoa0m0-_FQkDFsbRS0yGgMNG4-uc7qLIU4kSwMQWcw1Rwy39LUDP4zNzuZABbWsDDBsMlVUaszRdKIlk5AQ-Fkah3E247dYGUQjSQ0N3dFLlMDv_e62BT3IBXGLg7wvGosWFNT_LpIenIW6Q\",\n\"e\": \"AQAB\"\n}\n]\n}",
				"from_headers": [
					{
						"name": "jwt",
						"value_prefix": "Bearer "
					}
				]
			},
			{
				"name": "consumer_params",
				"issuer": "higress-test",
				"jwks": "{\n\"keys\": [\n{\n\"kty\": \"EC\",\n\"kid\": \"p256\",\n\"crv\": \"P-256\",\n\"x\": \"GWym652nfByDbs4EzNpGXCkdjG03qFZHulNDHTo3YJU\",\n\"y\": \"5uVg_n-flqRJ5Zhf_aEKS0ow9SddTDgxGduSCgpoAZQ\"\n},\n{\n\"kty\": \"RSA\",\n\"kid\": \"rsa\",\n\"n\": \"pFKAKJ0V3vFwGTvBSHbPwrNdvPyr-zMTh7Y9IELFIMNUQfG9_d2D1wZcrX5CPvtEISHin3GdPyfqEX6NjPyqvCLFTuNh80-r5Mvld-A5CHwITZXz5krBdqY5Z0wu64smMbzst3HNxHbzLQvHUY-KS6hceOB84d9B4rhkIJEEAWxxIA7yPJYjYyIC_STpPddtJkkweVvoa0m0-_FQkDFsbRS0yGgMNG4-uc7qLIU4kSwMQWcw1Rwy39LUDP4zNzuZABbWsDDBsMlVUaszRdKIlk5AQ-Fkah3E247dYGUQjSQ0N3dFLlMDv_e62BT3IBXGLg7wvGosWFNT_LpIenIW6Q\",\n\"e\": \"AQAB\"\n}\n]\n}",
				"from_params": [
					"jwt_token"
				]
			},
			{
				"name": "consumer_cookies",
				"issuer": "higress-test",
				"jwks": "{\n\"keys\": [\n{\n\"kty\": \"EC\",\n\"kid\": \"p256\",\n\"crv\": \"P-256\",\n\"x\": \"GWym652nfByDbs4EzNpGXCkdjG03qFZHulNDHTo3YJU\",\n\"y\": \"5uVg_n-flqRJ5Zhf_aEKS0ow9SddTDgxGduSCgpoAZQ\"\n},\n{\n\"kty\": \"RSA\",\n\"kid\": \"rsa\",\n\"n\": \"pFKAKJ0V3vFwGTvBSHbPwrNdvPyr-zMTh7Y9IELFIMNUQfG9_d2D1wZcrX5CPvtEISHin3GdPyfqEX6NjPyqvCLFTuNh80-r5Mvld-A5CHwITZXz5krBdqY5Z0wu64smMbzst3HNxHbzLQvHUY-KS6hceOB84d9B4rhkIJEEAWxxIA7yPJYjYyIC_STpPddtJkkweVvoa0m0-_FQkDFsbRS0yGgMNG4-uc7qLIU4kSwMQWcw1Rwy39LUDP4zNzuZABbWsDDBsMlVUaszRdKIlk5AQ-Fkah3E247dYGUQjSQ0N3dFLlMDv_e62BT3IBXGLg7wvGosWFNT_LpIenIW6Q\",\n\"e\": \"AQAB\"\n}\n]\n}",
				"from_cookies": [
					"jwt_token"
				]
			}
		]
	}`
)

func TestConsumerVerify(t *testing.T) {
	log := &testLogger{
		T: t,
	}
	cs := []*config.Consumer{}

	c := gjson.Parse(consumers).Get("consumers")
	if !c.IsArray() {
		t.Error("failed to parse configuration for consumers: consumers is not a array")
		return
	}

	consumerNames := map[string]struct{}{}
	for _, v := range c.Array() {
		c, err := config.ParseConsumer(v, consumerNames)
		if err != nil {
			t.Log(err.Error())
			continue
		}
		cs = append(cs, c)
	}
	if len(cs) == 0 {
		t.Error("at least one consumer should be configured for a rule")
		return
	}

	header := &testProvider{headerMap: map[string]string{"jwt": "Bearer " + ES256Allow}}
	_, err := consumerVerify(&config.Consumer{
		Name:             "consumer1",
		JWKs:             JWKs,
		Issuer:           "higress-test",
		ClaimsToHeaders:  &[]config.ClaimsToHeader{},
		FromHeaders:      &[]config.FromHeader{{Name: "jwt", ValuePrefix: "Bearer "}},
		ClockSkewSeconds: &config.DefaultClockSkewSeconds,
		KeepToken:        &config.DefaultKeepToken,
	}, time.Now(), header, log)

	if err != nil {
		if v, ok := err.(*ErrDenied); ok {
			t.Error(v.msg)
		}
	}
}

func TestConsumerVerifyWithCachedRemoteJWKs(t *testing.T) {
	log := &testLogger{T: t}
	cacheRemoteJWKsForTest("consumer-remote", "https://auth.example.com/.well-known/jwks.json", JWKs, time.Now().Add(time.Minute))
	defer clearRemoteJWKsCacheForTest()

	header := &testProvider{headerMap: map[string]string{"jwt": "Bearer " + ES256Allow}}
	_, err := consumerVerify(&config.Consumer{
		Name:              "consumer-remote",
		JWKsURI:           "https://auth.example.com/.well-known/jwks.json",
		Issuer:            "higress-test",
		ClaimsToHeaders:   &[]config.ClaimsToHeader{},
		FromHeaders:       &[]config.FromHeader{{Name: "jwt", ValuePrefix: "Bearer "}},
		ClockSkewSeconds:  &config.DefaultClockSkewSeconds,
		KeepToken:         &config.DefaultKeepToken,
		JWKsCacheDuration: &config.DefaultJWKsCacheDuration,
		JWKsFetchTimeout:  &config.DefaultJWKsFetchTimeout,
	}, time.Now(), header, log)

	if err != nil {
		if v, ok := err.(*ErrDenied); ok {
			t.Error(v.msg)
		} else {
			t.Error(err)
		}
	}
}

func TestConsumerVerifyWithRemoteJWKsReturnsCacheMissOnUnknownKid(t *testing.T) {
	log := &testLogger{T: t}
	staleJWKs := "{\"keys\":[{\"kty\":\"RSA\",\"kid\":\"rsa\",\"n\":\"pFKAKJ0V3vFwGTvBSHbPwrNdvPyr-zMTh7Y9IELFIMNUQfG9_d2D1wZcrX5CPvtEISHin3GdPyfqEX6NjPyqvCLFTuNh80-r5Mvld-A5CHwITZXz5krBdqY5Z0wu64smMbzst3HNxHbzLQvHUY-KS6hceOB84d9B4rhkIJEEAWxxIA7yPJYjYyIC_STpPddtJkkweVvoa0m0-_FQkDFsbRS0yGgMNG4-uc7qLIU4kSwMQWcw1Rwy39LUDP4zNzuZABbWsDDBsMlVUaszRdKIlk5AQ-Fkah3E247dYGUQjSQ0N3dFLlMDv_e62BT3IBXGLg7wvGosWFNT_LpIenIW6Q\",\"e\":\"AQAB\"}]}"
	cacheRemoteJWKsForTest("consumer-remote", "https://auth.example.com/.well-known/jwks.json", staleJWKs, time.Now().Add(time.Minute))
	defer clearRemoteJWKsCacheForTest()

	header := &testProvider{headerMap: map[string]string{"jwt": "Bearer " + ES256Allow}}
	_, err := consumerVerify(&config.Consumer{
		Name:              "consumer-remote",
		JWKsURI:           "https://auth.example.com/.well-known/jwks.json",
		Issuer:            "higress-test",
		ClaimsToHeaders:   &[]config.ClaimsToHeader{},
		FromHeaders:       &[]config.FromHeader{{Name: "jwt", ValuePrefix: "Bearer "}},
		ClockSkewSeconds:  &config.DefaultClockSkewSeconds,
		KeepToken:         &config.DefaultKeepToken,
		JWKsCacheDuration: &config.DefaultJWKsCacheDuration,
		JWKsFetchTimeout:  &config.DefaultJWKsFetchTimeout,
	}, time.Now(), header, log)

	if !isRemoteJWKsCacheMiss(err) {
		t.Fatalf("expected remote jwks cache miss for unknown kid, got: %v", err)
	}
}

func TestConsumerVerifyWithRemoteJWKsRejectsMissingKid(t *testing.T) {
	log := &testLogger{T: t}
	uri := "https://auth.example.com/.well-known/jwks.json"
	cacheRemoteJWKsForTest("consumer-remote", uri, JWKs, time.Now().Add(time.Minute))
	defer clearRemoteJWKsCacheForTest()

	tokenWithoutKid := jwtWithHeader(ES256Allow, `{"alg":"ES256","typ":"JWT"}`)
	header := &testProvider{headerMap: map[string]string{"jwt": "Bearer " + tokenWithoutKid}}
	_, err := consumerVerify(&config.Consumer{
		Name:              "consumer-remote",
		JWKsURI:           uri,
		Issuer:            "higress-test",
		ClaimsToHeaders:   &[]config.ClaimsToHeader{},
		FromHeaders:       &[]config.FromHeader{{Name: "jwt", ValuePrefix: "Bearer "}},
		ClockSkewSeconds:  &config.DefaultClockSkewSeconds,
		KeepToken:         &config.DefaultKeepToken,
		JWKsCacheDuration: &config.DefaultJWKsCacheDuration,
		JWKsFetchTimeout:  &config.DefaultJWKsFetchTimeout,
	}, time.Now(), header, log)

	if err == nil {
		t.Fatalf("expected remote jwks token without kid to fail")
	}
	if isRemoteJWKsCacheMiss(err) {
		t.Fatalf("missing kid should be denied without remote jwks refresh")
	}
}

func TestConsumerVerifyWithRemoteJWKsThrottlesUnknownKidRefresh(t *testing.T) {
	log := &testLogger{T: t}
	uri := "https://auth.example.com/.well-known/jwks.json"
	staleJWKs := "{\"keys\":[{\"kty\":\"RSA\",\"kid\":\"rsa\",\"n\":\"pFKAKJ0V3vFwGTvBSHbPwrNdvPyr-zMTh7Y9IELFIMNUQfG9_d2D1wZcrX5CPvtEISHin3GdPyfqEX6NjPyqvCLFTuNh80-r5Mvld-A5CHwITZXz5krBdqY5Z0wu64smMbzst3HNxHbzLQvHUY-KS6hceOB84d9B4rhkIJEEAWxxIA7yPJYjYyIC_STpPddtJkkweVvoa0m0-_FQkDFsbRS0yGgMNG4-uc7qLIU4kSwMQWcw1Rwy39LUDP4zNzuZABbWsDDBsMlVUaszRdKIlk5AQ-Fkah3E247dYGUQjSQ0N3dFLlMDv_e62BT3IBXGLg7wvGosWFNT_LpIenIW6Q\",\"e\":\"AQAB\"}]}"
	now := time.Now()
	cacheRemoteJWKsForTest("consumer-remote", uri, staleJWKs, now.Add(time.Minute))
	markRemoteJWKsFetchCompletedForTest("consumer-remote", uri, now)
	defer clearRemoteJWKsCacheForTest()

	header := &testProvider{headerMap: map[string]string{"jwt": "Bearer " + ES256Allow}}
	_, err := consumerVerify(&config.Consumer{
		Name:              "consumer-remote",
		JWKsURI:           uri,
		Issuer:            "higress-test",
		ClaimsToHeaders:   &[]config.ClaimsToHeader{},
		FromHeaders:       &[]config.FromHeader{{Name: "jwt", ValuePrefix: "Bearer "}},
		ClockSkewSeconds:  &config.DefaultClockSkewSeconds,
		KeepToken:         &config.DefaultKeepToken,
		JWKsCacheDuration: &config.DefaultJWKsCacheDuration,
		JWKsFetchTimeout:  &config.DefaultJWKsFetchTimeout,
	}, now, header, log)

	if isRemoteJWKsCacheMiss(err) {
		t.Fatalf("recently fetched remote jwks should deny unknown kid instead of fetching again")
	}
	if err == nil {
		t.Fatalf("expected unknown kid to fail")
	}
}

func TestConsumerVerifyWithRemoteJWKsSkipsIssuerMismatchBeforeFetch(t *testing.T) {
	log := &testLogger{T: t}
	defer clearRemoteJWKsCacheForTest()

	header := &testProvider{headerMap: map[string]string{"jwt": "Bearer " + ES256Allow}}
	_, err := consumerVerify(&config.Consumer{
		Name:              "consumer-remote",
		JWKsURI:           "https://auth.example.com/.well-known/jwks.json",
		Issuer:            "other-issuer",
		ClaimsToHeaders:   &[]config.ClaimsToHeader{},
		FromHeaders:       &[]config.FromHeader{{Name: "jwt", ValuePrefix: "Bearer "}},
		ClockSkewSeconds:  &config.DefaultClockSkewSeconds,
		KeepToken:         &config.DefaultKeepToken,
		JWKsCacheDuration: &config.DefaultJWKsCacheDuration,
		JWKsFetchTimeout:  &config.DefaultJWKsFetchTimeout,
	}, time.Now(), header, log)

	if isRemoteJWKsCacheMiss(err) {
		t.Fatalf("issuer mismatch should not trigger remote jwks fetch")
	}
	if err == nil {
		t.Fatalf("expected issuer mismatch to fail")
	}
}

func TestExtractTokenRemovesQueryParamWhenKeepTokenFalse(t *testing.T) {
	header := &testProvider{headerMap: map[string]string{":path": "/resource?access_token=token-value&keep=1"}}
	token := extractToken(false, &config.Consumer{
		FromParams: &[]string{"access_token"},
	}, header, &testLogger{T: t})

	if token != "token-value" {
		t.Fatalf("unexpected token: %q", token)
	}
	if got := header.headerMap[":path"]; got != "/resource?keep=1" {
		t.Fatalf("expected token query param to be removed, got: %q", got)
	}
}

func jwtWithHeader(token, headerJSON string) string {
	parts := strings.Split(token, ".")
	parts[0] = base64.RawURLEncoding.EncodeToString([]byte(headerJSON))
	return strings.Join(parts, ".")
}
