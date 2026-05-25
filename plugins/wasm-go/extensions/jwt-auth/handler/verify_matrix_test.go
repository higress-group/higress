// Copyright (c) 2023 Alibaba Group Holding Ltd.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0

package handler

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"testing"
	"time"

	"github.com/alibaba/higress/plugins/wasm-go/extensions/jwt-auth/config"
	jose "github.com/go-jose/go-jose/v3"
	"github.com/go-jose/go-jose/v3/jwt"
	"github.com/stretchr/testify/require"
)

// signed builds (jwksJSON, token) for the given algorithm + claims + kid header.
func signed(t *testing.T, alg jose.SignatureAlgorithm, claims any, kid string, issuer string) (string, string) {
	t.Helper()
	var signKey any
	var jwk jose.JSONWebKey

	switch alg {
	case jose.RS256, jose.RS384, jose.RS512, jose.PS256:
		k, err := rsa.GenerateKey(rand.Reader, 2048)
		require.NoError(t, err)
		signKey = k
		jwk = jose.JSONWebKey{Key: &k.PublicKey, KeyID: kid, Algorithm: string(alg)}
	case jose.ES256:
		k, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		require.NoError(t, err)
		signKey = k
		jwk = jose.JSONWebKey{Key: &k.PublicKey, KeyID: kid, Algorithm: string(alg)}
	case jose.ES384:
		k, err := ecdsa.GenerateKey(elliptic.P384(), rand.Reader)
		require.NoError(t, err)
		signKey = k
		jwk = jose.JSONWebKey{Key: &k.PublicKey, KeyID: kid, Algorithm: string(alg)}
	case jose.ES512:
		k, err := ecdsa.GenerateKey(elliptic.P521(), rand.Reader)
		require.NoError(t, err)
		signKey = k
		jwk = jose.JSONWebKey{Key: &k.PublicKey, KeyID: kid, Algorithm: string(alg)}
	default:
		t.Fatalf("unsupported alg %v", alg)
	}
	if c, ok := claims.(jwt.Claims); ok && c.Issuer == "" {
		c.Issuer = issuer
		claims = c
	}

	sigOpts := (&jose.SignerOptions{}).WithType("JWT")
	if kid != "" {
		sigOpts = sigOpts.WithHeader(jose.HeaderKey("kid"), kid)
	}
	sig, err := jose.NewSigner(jose.SigningKey{Algorithm: alg, Key: signKey}, sigOpts)
	require.NoError(t, err)

	token, err := jwt.Signed(sig).Claims(claims).CompactSerialize()
	require.NoError(t, err)

	jwks := jose.JSONWebKeySet{Keys: []jose.JSONWebKey{jwk}}
	raw, err := json.Marshal(jwks)
	require.NoError(t, err)

	return string(raw), token
}

func consumerWith(jwks, issuer string) *config.Consumer {
	keep, skew := true, int64(60)
	return &config.Consumer{
		Name:             "c",
		JWKs:             jwks,
		Issuer:           issuer,
		ClaimsToHeaders:  &[]config.ClaimsToHeader{},
		FromHeaders:      &[]config.FromHeader{{Name: "Authorization", ValuePrefix: "Bearer "}},
		ClockSkewSeconds: &skew,
		KeepToken:        &keep,
	}
}

func validClaims() jwt.Claims {
	return jwt.Claims{
		Issuer:    "higress-test",
		Subject:   "sub",
		Audience:  jwt.Audience{"foo"},
		Expiry:    jwt.NewNumericDate(time.Now().Add(time.Hour)),
		NotBefore: jwt.NewNumericDate(time.Now().Add(-time.Hour)),
	}
}

func TestVerify_AlgorithmMatrix(t *testing.T) {
	algs := []jose.SignatureAlgorithm{jose.RS256, jose.RS384, jose.RS512, jose.PS256, jose.ES256, jose.ES384, jose.ES512}
	for _, alg := range algs {
		t.Run(string(alg), func(t *testing.T) {
			jwks, token := signed(t, alg, validClaims(), "k1", "higress-test")
			hp := &testProvider{headerMap: map[string]string{"authorization": "Bearer " + token}}
			err := consumerVerify(consumerWith(jwks, "higress-test"), time.Now(), hp, &testLogger{T: t})
			require.NoError(t, err)
		})
	}
}

func TestVerify_KidMissingFallsBackToFirstKey(t *testing.T) {
	jwks, token := signed(t, jose.ES256, validClaims(), "", "higress-test")
	hp := &testProvider{headerMap: map[string]string{"authorization": "Bearer " + token}}
	err := consumerVerify(consumerWith(jwks, "higress-test"), time.Now(), hp, &testLogger{T: t})
	require.NoError(t, err)
}

func TestVerify_KidNotInJWKsFallsBackToFirstKey(t *testing.T) {
	// Token signed with kid="k1"; JWKs contains the same key but under a different kid.
	jwks, token := signed(t, jose.ES256, validClaims(), "k1", "higress-test")
	// Rewrite kid in the JWKs to something else, simulating mismatch.
	munged := jose.JSONWebKeySet{}
	require.NoError(t, json.Unmarshal([]byte(jwks), &munged))
	munged.Keys[0].KeyID = "other"
	raw, err := json.Marshal(munged)
	require.NoError(t, err)

	hp := &testProvider{headerMap: map[string]string{"authorization": "Bearer " + token}}
	err = consumerVerify(consumerWith(string(raw), "higress-test"), time.Now(), hp, &testLogger{T: t})
	require.NoError(t, err)
}

func TestVerify_TokenMissingReturnsDenied(t *testing.T) {
	jwks, _ := signed(t, jose.ES256, validClaims(), "k1", "higress-test")
	hp := &testProvider{headerMap: map[string]string{}}
	err := consumerVerify(consumerWith(jwks, "higress-test"), time.Now(), hp, &testLogger{T: t})
	require.Error(t, err)
	ed, ok := err.(*ErrDenied)
	require.True(t, ok)
	require.Contains(t, ed.msg, "missing")
}

func TestVerify_InvalidTokenReturnsDenied(t *testing.T) {
	jwks, _ := signed(t, jose.ES256, validClaims(), "k1", "higress-test")
	hp := &testProvider{headerMap: map[string]string{"authorization": "Bearer not.a.jwt"}}
	err := consumerVerify(consumerWith(jwks, "higress-test"), time.Now(), hp, &testLogger{T: t})
	require.Error(t, err)
}

func TestVerify_IssuerMismatchReturnsDenied(t *testing.T) {
	jwks, token := signed(t, jose.ES256, validClaims(), "k1", "higress-test")
	hp := &testProvider{headerMap: map[string]string{"authorization": "Bearer " + token}}
	err := consumerVerify(consumerWith(jwks, "other-issuer"), time.Now(), hp, &testLogger{T: t})
	require.Error(t, err)
	ed, ok := err.(*ErrDenied)
	require.True(t, ok)
	require.Contains(t, ed.msg, "issuer does not equal")
}

func TestVerify_ExpiredReturnsDenied(t *testing.T) {
	cl := validClaims()
	cl.Expiry = jwt.NewNumericDate(time.Now().Add(-2 * time.Hour))
	cl.NotBefore = jwt.NewNumericDate(time.Now().Add(-3 * time.Hour))
	jwks, token := signed(t, jose.ES256, cl, "k1", "higress-test")
	hp := &testProvider{headerMap: map[string]string{"authorization": "Bearer " + token}}
	err := consumerVerify(consumerWith(jwks, "higress-test"), time.Now(), hp, &testLogger{T: t})
	require.Error(t, err)
	ed, ok := err.(*ErrDenied)
	require.True(t, ok)
	require.Contains(t, ed.msg, "expired")
}

func TestVerify_ClockSkewAllowsRecentlyExpired(t *testing.T) {
	cl := validClaims()
	cl.Expiry = jwt.NewNumericDate(time.Now().Add(-30 * time.Second))
	jwks, token := signed(t, jose.ES256, cl, "k1", "higress-test")
	// 60s default skew should accept a 30s-stale token.
	hp := &testProvider{headerMap: map[string]string{"authorization": "Bearer " + token}}
	err := consumerVerify(consumerWith(jwks, "higress-test"), time.Now(), hp, &testLogger{T: t})
	require.NoError(t, err)
}

func TestVerify_NotYetValidReturnsDenied(t *testing.T) {
	cl := validClaims()
	cl.NotBefore = jwt.NewNumericDate(time.Now().Add(time.Hour))
	jwks, token := signed(t, jose.ES256, cl, "k1", "higress-test")
	hp := &testProvider{headerMap: map[string]string{"authorization": "Bearer " + token}}
	err := consumerVerify(consumerWith(jwks, "higress-test"), time.Now(), hp, &testLogger{T: t})
	require.Error(t, err)
}

func TestVerify_InvalidJWKsInConsumerReturnsDenied(t *testing.T) {
	_, token := signed(t, jose.ES256, validClaims(), "k1", "higress-test")
	c := consumerWith("not-json", "higress-test")
	hp := &testProvider{headerMap: map[string]string{"authorization": "Bearer " + token}}
	err := consumerVerify(c, time.Now(), hp, &testLogger{T: t})
	require.Error(t, err)
}
