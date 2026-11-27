package main

import (
	"encoding/base64"
	"encoding/hex"
	"strings"
	"testing"
	"time"

	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm/types"
	"github.com/higress-group/wasm-go/pkg/test"
	"github.com/stretchr/testify/require"
)

const base64StdAlphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/"

// flipSignatureByte 返回一个与原签名等长、编码合法的错误签名。
func flipSignatureByte(t *testing.T, encoded string) string {
	t.Helper()
	raw, err := base64.StdEncoding.DecodeString(encoded)
	require.NoError(t, err)
	raw[0] ^= 0x01
	return base64.StdEncoding.EncodeToString(raw)
}

// nonCanonicalSignature 返回一个长度合法、解码结果相同但 padding bits 非零的编码。
func nonCanonicalSignature(t *testing.T, encoded string) string {
	t.Helper()
	require.True(t, strings.HasSuffix(encoded, "="))
	lastDataIndex := len(encoded) - 2
	alphabetIndex := strings.IndexByte(base64StdAlphabet, encoded[lastDataIndex])
	require.GreaterOrEqual(t, alphabetIndex, 0)
	replacement := base64StdAlphabet[alphabetIndex+1]
	return encoded[:lastDataIndex] + string(replacement) + encoded[lastDataIndex+1:]
}

func TestHmacSignatureEqual(t *testing.T) {
	sha1Signature, err := generateHmacSignature("sk1", "hmac-sha1", "signing string")
	require.NoError(t, err)
	sha256Signature, err := generateHmacSignature("sk1", "hmac-sha256", "signing string")
	require.NoError(t, err)
	sha512Signature, err := generateHmacSignature("sk1", "hmac-sha512", "signing string")
	require.NoError(t, err)
	require.Len(t, sha1Signature, 28)
	require.Len(t, sha256Signature, 44)
	require.Len(t, sha512Signature, 88)

	raw256, err := base64.StdEncoding.DecodeString(sha256Signature)
	require.NoError(t, err)

	base64URLSignature := base64.URLEncoding.EncodeToString(raw256)
	require.Len(t, base64URLSignature, len(sha256Signature))
	require.NotEqual(t, sha256Signature, base64URLSignature)

	nonCanonical := nonCanonicalSignature(t, sha256Signature)
	require.Len(t, nonCanonical, len(sha256Signature))
	_, err = base64.StdEncoding.DecodeString(nonCanonical)
	require.NoError(t, err, "non-strict decoder should accept non-zero padding bits")
	_, err = base64.StdEncoding.Strict().DecodeString(nonCanonical)
	require.Error(t, err, "strict decoder should reject non-zero padding bits")

	tests := []struct {
		name     string
		expected string
		actual   string
		want     bool
	}{
		{
			name:     "valid hmac-sha1 signature",
			expected: sha1Signature,
			actual:   sha1Signature,
			want:     true,
		},
		{
			name:     "valid hmac-sha256 signature",
			expected: sha256Signature,
			actual:   sha256Signature,
			want:     true,
		},
		{
			name:     "valid hmac-sha512 signature",
			expected: sha512Signature,
			actual:   sha512Signature,
			want:     true,
		},
		{
			name:     "equal length wrong signature",
			expected: sha256Signature,
			actual:   flipSignatureByte(t, sha256Signature),
			want:     false,
		},
		{
			name:     "empty actual signature",
			expected: sha256Signature,
			actual:   "",
			want:     false,
		},
		{
			name:     "short actual signature",
			expected: sha256Signature,
			actual:   sha256Signature[:len(sha256Signature)-4],
			want:     false,
		},
		{
			name:     "over-long actual signature",
			expected: sha256Signature,
			actual:   sha256Signature + "AAAA",
			want:     false,
		},
		{
			name:     "invalid base64 with matching length",
			expected: sha256Signature,
			actual:   "!" + sha256Signature[1:],
			want:     false,
		},
		{
			name:     "missing padding",
			expected: sha256Signature,
			actual:   strings.TrimRight(sha256Signature, "="),
			want:     false,
		},
		{
			name:     "base64url alphabet",
			expected: sha256Signature,
			actual:   base64URLSignature,
			want:     false,
		},
		{
			name:     "hex encoding",
			expected: sha256Signature,
			actual:   hex.EncodeToString(raw256),
			want:     false,
		},
		{
			name:     "crlf embedded",
			expected: sha256Signature,
			actual:   sha256Signature[:20] + "\r\n" + sha256Signature[22:],
			want:     false,
		},
		{
			name:     "non-zero padding bits",
			expected: sha256Signature,
			actual:   nonCanonical,
			want:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, hmacSignatureEqual(tt.expected, tt.actual))
		})
	}
}

func TestHmacSignatureVerificationLogs(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		consumerConfig := createConfig(
			[]map[string]interface{}{
				{
					"name":       "consumer1",
					"access_key": "ak1",
					"secret_key": "sk1",
				},
			},
			map[string]interface{}{
				"global_auth": true,
			},
		)

		fixedTime := time.Now().UTC().Format("Mon, 02 Jan 2006 15:04:05 GMT")
		signedHeaders := []string{"@request-target", "date"}
		headerValues := map[string]string{"date": fixedTime}
		expectedSignature := generateValidSignature("ak1", "sk1", "hmac-sha256", "GET", "/api/test", signedHeaders, headerValues)
		signingString := "ak1\nGET /api/test\ndate: " + fixedTime + "\n"

		assertNoSignatureMaterialLogged := func(t *testing.T, host test.TestHost, actualSignature string) {
			t.Helper()
			logs := append(append([]string{}, host.GetDebugLogs()...), host.GetWarnLogs()...)
			for _, entry := range logs {
				require.NotContains(t, entry, expectedSignature, "expected signature must not be logged")
				require.NotContains(t, entry, actualSignature, "client signature must not be logged")
				require.NotContains(t, entry, signingString, "signing string must not be logged")
				require.NotContains(t, entry, "signature=", "signature field must not be logged")
			}
		}

		t.Run("invalid signature returns 401 without logging signature material", func(t *testing.T) {
			host, status := test.NewTestHost(consumerConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			actualSignature := flipSignatureByte(t, expectedSignature)
			require.Len(t, actualSignature, len(expectedSignature))
			require.NotEqual(t, expectedSignature, actualSignature)

			authHeader := strings.Replace(generateAuthorizationHeader("ak1", "sk1", "hmac-sha256", "GET", "/api/test", signedHeaders, headerValues),
				expectedSignature, actualSignature, 1)
			require.Contains(t, authHeader, actualSignature)

			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/api/test"},
				{":method", "GET"},
				{"authorization", authHeader},
				{"date", fixedTime},
			})
			require.Equal(t, types.ActionContinue, action)

			localResponse := host.GetLocalResponse()
			require.NotNil(t, localResponse)
			require.Equal(t, uint32(401), localResponse.StatusCode)
			require.Equal(t, `{"message":"client request can't be validated: Invalid signature"}`, string(localResponse.Data))

			assertNoSignatureMaterialLogged(t, host, actualSignature)
			require.Contains(t, strings.Join(host.GetWarnLogs(), "\n"), "Signature validation failed")

			host.CompleteHttp()
		})

		t.Run("valid signature logs parameters without signature material", func(t *testing.T) {
			host, status := test.NewTestHost(consumerConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/api/test"},
				{":method", "GET"},
				{"authorization", generateAuthorizationHeader("ak1", "sk1", "hmac-sha256", "GET", "/api/test", signedHeaders, headerValues)},
				{"date", fixedTime},
			})
			require.Equal(t, types.ActionContinue, action)
			require.Nil(t, host.GetLocalResponse())

			debugLogs := strings.Join(host.GetDebugLogs(), "\n")
			require.Contains(t, debugLogs, "HMAC params extracted:")
			assertNoSignatureMaterialLogged(t, host, expectedSignature)

			host.CompleteHttp()
		})
	})
}
