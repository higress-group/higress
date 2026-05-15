package provider

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestKlingProviderValidateConfig(t *testing.T) {
	initializer := &klingProviderInitializer{}

	t.Run("official credentials", func(t *testing.T) {
		err := initializer.ValidateConfig(&ProviderConfig{
			klingAccessKey: "ak",
			klingSecretKey: "sk",
		})
		require.NoError(t, err)
	})

	t.Run("gateway token", func(t *testing.T) {
		err := initializer.ValidateConfig(&ProviderConfig{
			apiTokens: []string{"gateway-token"},
		})
		require.NoError(t, err)
	})

	t.Run("official credentials preferred when both configured", func(t *testing.T) {
		err := initializer.ValidateConfig(&ProviderConfig{
			apiTokens:      []string{"gateway-token"},
			klingAccessKey: "ak",
			klingSecretKey: "sk",
		})
		require.NoError(t, err)
	})

	t.Run("partial official credentials rejected", func(t *testing.T) {
		err := initializer.ValidateConfig(&ProviderConfig{
			klingAccessKey: "ak",
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "klingAccessKey")
	})

	t.Run("missing auth rejected", func(t *testing.T) {
		err := initializer.ValidateConfig(&ProviderConfig{})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "missing kling authentication")
	})
}

func TestKlingProviderConfigFromJson(t *testing.T) {
	config := &ProviderConfig{}
	config.FromJson(gjson.Parse(`{
		"type": "kling",
		"klingAccessKey": "ak",
		"klingSecretKey": "sk",
		"klingTokenRefreshAhead": 120,
		"capabilities": {
			"kling/v1/image2video": "/gateway/image2video",
			"kling/v1/retrieveimagevideo": "/gateway/image-tasks/{video_id}"
		}
	}`))

	assert.Equal(t, "ak", config.klingAccessKey)
	assert.Equal(t, "sk", config.klingSecretKey)
	assert.Equal(t, int64(120), config.klingTokenRefreshAhead)
	assert.Equal(t, "/gateway/image2video", config.capabilities[string(ApiNameKlingImageToVideo)])
	assert.Equal(t, "/gateway/image-tasks/{video_id}", config.capabilities[string(ApiNameKlingRetrieveImageVideo)])

	defaultConfig := &ProviderConfig{}
	defaultConfig.FromJson(gjson.Parse(`{"type": "kling", "apiTokens": ["token"]}`))
	assert.Equal(t, klingDefaultRefreshAhead, defaultConfig.klingTokenRefreshAhead)
}

func TestKlingProviderInitializerCreateProvider(t *testing.T) {
	initializer := &klingProviderInitializer{}

	capabilities := initializer.DefaultCapabilities()
	assert.Equal(t, klingTextToVideoPath, capabilities[string(ApiNameVideos)])
	assert.Equal(t, klingImageToVideoPath, capabilities[string(ApiNameKlingImageToVideo)])
	assert.Equal(t, klingTextToVideoTaskPath, capabilities[string(ApiNameRetrieveVideo)])
	assert.Equal(t, klingImageToVideoTaskPath, capabilities[string(ApiNameKlingRetrieveImageVideo)])

	created, err := initializer.CreateProvider(ProviderConfig{
		protocol:       protocolOpenAI,
		klingAccessKey: "ak",
		klingSecretKey: "sk",
	})
	require.NoError(t, err)
	_, transformsOpenAIResponseBody := created.(TransformResponseBodyHandler)
	assert.True(t, transformsOpenAIResponseBody)
	_, transformsOpenAIRequestBody := created.(RequestBodyHandler)
	assert.True(t, transformsOpenAIRequestBody)

	provider := requireKlingBaseProvider(t, created)
	assert.Equal(t, providerTypeKling, provider.GetProviderType())
	assert.Equal(t, klingDefaultRefreshAhead, provider.config.klingTokenRefreshAhead)
	assert.Equal(t, klingTextToVideoPath, provider.config.capabilities[string(ApiNameVideos)])
	assert.Equal(t, klingImageToVideoPath, provider.config.capabilities[string(ApiNameKlingImageToVideo)])
	assert.Equal(t, klingImageToVideoTaskPath, provider.config.capabilities[string(ApiNameKlingRetrieveImageVideo)])

	original, err := initializer.CreateProvider(ProviderConfig{
		protocol:  protocolOriginal,
		apiTokens: []string{"token"},
	})
	require.NoError(t, err)
	_, transformsOriginalResponseBody := original.(TransformResponseBodyHandler)
	assert.False(t, transformsOriginalResponseBody)
	_, transformsOriginalRequestBody := original.(RequestBodyHandler)
	assert.False(t, transformsOriginalRequestBody)
	_, isBaseProvider := original.(*klingProvider)
	assert.True(t, isBaseProvider)
}

func TestCreateKlingJWT(t *testing.T) {
	now := int64(1710000000)
	token, expireAt, err := createKlingJWT("access-key", "secret-key", now)
	require.NoError(t, err)
	assert.Equal(t, now+klingJWTLifetimeSeconds, expireAt)

	parts := strings.Split(token, ".")
	require.Len(t, parts, 3)

	headerJSON, err := base64.RawURLEncoding.DecodeString(parts[0])
	require.NoError(t, err)
	var header map[string]string
	require.NoError(t, json.Unmarshal(headerJSON, &header))
	assert.Equal(t, "HS256", header["alg"])
	assert.Equal(t, "JWT", header["typ"])

	payloadJSON, err := base64.RawURLEncoding.DecodeString(parts[1])
	require.NoError(t, err)
	var payload map[string]interface{}
	require.NoError(t, json.Unmarshal(payloadJSON, &payload))
	assert.Equal(t, "access-key", payload["iss"])
	assert.Equal(t, float64(now+klingJWTLifetimeSeconds), payload["exp"])
	assert.Equal(t, float64(now-klingJWTNotBeforeSkewSecond), payload["nbf"])

	mac := hmac.New(sha256.New, []byte("secret-key"))
	_, _ = mac.Write([]byte(parts[0] + "." + parts[1]))
	expectedSignature := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	assert.Equal(t, expectedSignature, parts[2])
}

func TestKlingProviderTransformRequestHeadersAuth(t *testing.T) {
	t.Run("official mode uses jwt bearer", func(t *testing.T) {
		provider := &klingProvider{
			config: ProviderConfig{
				protocol:       protocolOriginal,
				klingAccessKey: "access-key",
				klingSecretKey: "secret-key",
			},
		}
		headers := http.Header{}

		provider.TransformRequestHeaders(newMockMultipartHttpContext(), ApiNameVideos, headers)

		assert.Equal(t, klingDefaultDomain, headers.Get(":authority"))
		auth := headers.Get("Authorization")
		require.True(t, strings.HasPrefix(auth, "Bearer "))
		payload := decodeKlingJWTPayload(t, strings.TrimPrefix(auth, "Bearer "))
		assert.Equal(t, "access-key", payload["iss"])
	})

	t.Run("gateway mode uses static bearer token", func(t *testing.T) {
		ctx := newMockMultipartHttpContext()
		ctx.SetContext("kling-token-key", "gateway-token")
		provider := &klingProvider{
			config: ProviderConfig{
				protocol:  protocolOriginal,
				apiTokens: []string{"gateway-token"},
				failover:  &failover{ctxApiTokenInUse: "kling-token-key"},
			},
		}
		headers := http.Header{}

		provider.TransformRequestHeaders(ctx, ApiNameVideos, headers)

		assert.Equal(t, klingDefaultDomain, headers.Get(":authority"))
		assert.Equal(t, "Bearer gateway-token", headers.Get("Authorization"))
	})

	t.Run("provider domain skips default host overwrite", func(t *testing.T) {
		ctx := newMockMultipartHttpContext()
		ctx.SetContext("kling-token-key", "gateway-token")
		provider := &klingProvider{
			config: ProviderConfig{
				protocol:       protocolOriginal,
				apiTokens:      []string{"gateway-token"},
				failover:       &failover{ctxApiTokenInUse: "kling-token-key"},
				providerDomain: "api.302.ai",
			},
		}
		headers := http.Header{":authority": []string{"example.com"}}

		provider.TransformRequestHeaders(ctx, ApiNameVideos, headers)

		assert.Equal(t, "example.com", headers.Get(":authority"))
		assert.Equal(t, "Bearer gateway-token", headers.Get("Authorization"))
	})

	t.Run("original mode preserves content length", func(t *testing.T) {
		ctx := newMockMultipartHttpContext()
		ctx.SetContext("kling-token-key", "gateway-token")
		provider := &klingProvider{
			config: ProviderConfig{
				protocol:  protocolOriginal,
				apiTokens: []string{"gateway-token"},
				failover:  &failover{ctxApiTokenInUse: "kling-token-key"},
			},
		}
		headers := http.Header{"Content-Length": []string{"128"}}

		provider.TransformRequestHeaders(ctx, ApiNameVideos, headers)

		assert.Equal(t, "128", headers.Get("Content-Length"))
	})

	t.Run("openai mode rewrites capability path and removes content length", func(t *testing.T) {
		provider := &klingProvider{
			config: ProviderConfig{
				protocol:       protocolOpenAI,
				klingAccessKey: "access-key",
				klingSecretKey: "secret-key",
				capabilities: map[string]string{
					string(ApiNameVideos): klingTextToVideoPath,
				},
			},
		}
		headers := http.Header{
			":path":          []string{"/v1/videos?trace=1"},
			"Content-Length": []string{"12"},
		}

		provider.TransformRequestHeaders(newMockMultipartHttpContext(), ApiNameVideos, headers)

		assert.Equal(t, klingTextToVideoPath+"?trace=1", headers.Get(":path"))
		assert.Equal(t, klingDefaultDomain, headers.Get(":authority"))
		assert.Empty(t, headers.Get("Content-Length"))
	})

	t.Run("prefixed image task id routes retrieve to image endpoint", func(t *testing.T) {
		provider := &klingProvider{
			config: ProviderConfig{
				protocol: protocolOpenAI,
				capabilities: map[string]string{
					string(ApiNameRetrieveVideo): klingTextToVideoTaskPath,
				},
				apiTokens: []string{"gateway-token"},
				failover:  &failover{ctxApiTokenInUse: "kling-token-key"},
			},
		}
		ctx := newMockMultipartHttpContext()
		ctx.SetContext("kling-token-key", "gateway-token")
		headers := http.Header{":path": []string{"/v1/videos/" + klingImageTaskIDPrefix + "task-123?with_status=true"}}

		provider.TransformRequestHeaders(ctx, ApiNameRetrieveVideo, headers)

		assert.Equal(t, klingImageToVideoPath+"/task-123?with_status=true", headers.Get(":path"))
	})

	t.Run("prefixed image task id strips internal task type query", func(t *testing.T) {
		provider := &klingProvider{
			config: ProviderConfig{
				protocol: protocolOpenAI,
				capabilities: map[string]string{
					string(ApiNameRetrieveVideo): klingTextToVideoTaskPath,
				},
				apiTokens: []string{"gateway-token"},
				failover:  &failover{ctxApiTokenInUse: "kling-token-key"},
			},
		}
		ctx := newMockMultipartHttpContext()
		ctx.SetContext("kling-token-key", "gateway-token")
		headers := http.Header{":path": []string{"/v1/videos/" + klingImageTaskIDPrefix + "task-123?kling_task_type=image2video&with_status=true"}}

		provider.TransformRequestHeaders(ctx, ApiNameRetrieveVideo, headers)

		assert.Equal(t, klingImageToVideoPath+"/task-123?with_status=true", headers.Get(":path"))
	})

	t.Run("prefixed text task id routes retrieve to text endpoint", func(t *testing.T) {
		provider := &klingProvider{
			config: ProviderConfig{
				protocol: protocolOpenAI,
				capabilities: map[string]string{
					string(ApiNameRetrieveVideo): klingTextToVideoTaskPath,
				},
				apiTokens: []string{"gateway-token"},
				failover:  &failover{ctxApiTokenInUse: "kling-token-key"},
			},
		}
		ctx := newMockMultipartHttpContext()
		ctx.SetContext("kling-token-key", "gateway-token")
		headers := http.Header{":path": []string{"/v1/videos/" + klingTextTaskIDPrefix + "task-123?with_status=true"}}

		provider.TransformRequestHeaders(ctx, ApiNameRetrieveVideo, headers)

		assert.Equal(t, klingTextToVideoPath+"/task-123?with_status=true", headers.Get(":path"))
	})

	t.Run("prefixed text task id strips internal task type query", func(t *testing.T) {
		provider := &klingProvider{
			config: ProviderConfig{
				protocol: protocolOpenAI,
				capabilities: map[string]string{
					string(ApiNameRetrieveVideo): klingTextToVideoTaskPath,
				},
				apiTokens: []string{"gateway-token"},
				failover:  &failover{ctxApiTokenInUse: "kling-token-key"},
			},
		}
		ctx := newMockMultipartHttpContext()
		ctx.SetContext("kling-token-key", "gateway-token")
		headers := http.Header{":path": []string{"/v1/videos/" + klingTextTaskIDPrefix + "task-123?with_status=true&kling_task_type=t2v"}}

		provider.TransformRequestHeaders(ctx, ApiNameRetrieveVideo, headers)

		assert.Equal(t, klingTextToVideoPath+"/task-123?with_status=true", headers.Get(":path"))
	})

	t.Run("raw image task id uses explicit task type query without forwarding it", func(t *testing.T) {
		provider := &klingProvider{
			config: ProviderConfig{
				protocol: protocolOpenAI,
				capabilities: map[string]string{
					string(ApiNameRetrieveVideo): klingTextToVideoTaskPath,
				},
				apiTokens: []string{"gateway-token"},
				failover:  &failover{ctxApiTokenInUse: "kling-token-key"},
			},
		}
		ctx := newMockMultipartHttpContext()
		ctx.SetContext("kling-token-key", "gateway-token")
		headers := http.Header{":path": []string{"/v1/videos/raw-task-123?kling_task_type=image2video&with_status=true"}}

		provider.TransformRequestHeaders(ctx, ApiNameRetrieveVideo, headers)

		assert.Equal(t, klingImageToVideoPath+"/raw-task-123?with_status=true", headers.Get(":path"))
	})

	t.Run("raw text task id uses explicit task type query without forwarding it", func(t *testing.T) {
		provider := &klingProvider{
			config: ProviderConfig{
				protocol: protocolOpenAI,
				capabilities: map[string]string{
					string(ApiNameRetrieveVideo): klingTextToVideoTaskPath,
				},
				apiTokens: []string{"gateway-token"},
				failover:  &failover{ctxApiTokenInUse: "kling-token-key"},
			},
		}
		ctx := newMockMultipartHttpContext()
		ctx.SetContext("kling-token-key", "gateway-token")
		headers := http.Header{":path": []string{"/v1/videos/raw-task-123?with_status=true&kling_task_type=t2v"}}

		provider.TransformRequestHeaders(ctx, ApiNameRetrieveVideo, headers)

		assert.Equal(t, klingTextToVideoPath+"/raw-task-123?with_status=true", headers.Get(":path"))
	})

	t.Run("image task id routes retrieve through configured image capability", func(t *testing.T) {
		provider := &klingProvider{
			config: ProviderConfig{
				protocol: protocolOpenAI,
				capabilities: map[string]string{
					string(ApiNameRetrieveVideo):           klingTextToVideoTaskPath,
					string(ApiNameKlingRetrieveImageVideo): "/gateway/image-tasks/{video_id}",
				},
				apiTokens: []string{"gateway-token"},
				failover:  &failover{ctxApiTokenInUse: "kling-token-key"},
			},
		}
		ctx := newMockMultipartHttpContext()
		ctx.SetContext("kling-token-key", "gateway-token")
		headers := http.Header{":path": []string{"/v1/videos/" + klingImageTaskIDPrefix + "task-123?with_status=true"}}

		provider.TransformRequestHeaders(ctx, ApiNameRetrieveVideo, headers)

		assert.Equal(t, "/gateway/image-tasks/task-123?with_status=true", headers.Get(":path"))
	})

	t.Run("raw image task id routes retrieve through configured image capability", func(t *testing.T) {
		provider := &klingProvider{
			config: ProviderConfig{
				protocol: protocolOpenAI,
				capabilities: map[string]string{
					string(ApiNameRetrieveVideo):           klingTextToVideoTaskPath,
					string(ApiNameKlingRetrieveImageVideo): "/gateway/image-tasks/{video_id}",
				},
				apiTokens: []string{"gateway-token"},
				failover:  &failover{ctxApiTokenInUse: "kling-token-key"},
			},
		}
		ctx := newMockMultipartHttpContext()
		ctx.SetContext("kling-token-key", "gateway-token")
		headers := http.Header{":path": []string{"/v1/videos/raw-task-123?kling_task_type=i2v&with_status=true"}}

		provider.TransformRequestHeaders(ctx, ApiNameRetrieveVideo, headers)

		assert.Equal(t, "/gateway/image-tasks/raw-task-123?with_status=true", headers.Get(":path"))
	})

	t.Run("retrieve capability query merges with request query", func(t *testing.T) {
		provider := &klingProvider{
			config: ProviderConfig{
				protocol: protocolOpenAI,
				capabilities: map[string]string{
					string(ApiNameRetrieveVideo):           "/gateway/text-tasks/{video_id}?version=1",
					string(ApiNameKlingRetrieveImageVideo): "/gateway/image-tasks/{video_id}?version=1",
				},
				apiTokens: []string{"gateway-token"},
				failover:  &failover{ctxApiTokenInUse: "kling-token-key"},
			},
		}
		ctx := newMockMultipartHttpContext()
		ctx.SetContext("kling-token-key", "gateway-token")
		headers := http.Header{":path": []string{"/v1/videos/raw-task-123?kling_task_type=i2v&with_status=true"}}

		provider.TransformRequestHeaders(ctx, ApiNameRetrieveVideo, headers)

		assert.Equal(t, "/gateway/image-tasks/raw-task-123?version=1&with_status=true", headers.Get(":path"))
	})

	t.Run("retrieve path outside openai pattern falls back to capability mapping", func(t *testing.T) {
		provider := &klingProvider{
			config: ProviderConfig{
				capabilities: map[string]string{
					string(ApiNameRetrieveVideo): "/gateway/retrieve",
				},
			},
		}

		assert.Equal(t, "/gateway/retrieve?trace=1", provider.mapRetrieveVideoPath("/custom/retrieve?trace=1"))
	})

	t.Run("unknown task type hint is stripped before fallback retrieve mapping", func(t *testing.T) {
		provider := &klingProvider{
			config: ProviderConfig{
				capabilities: map[string]string{
					string(ApiNameRetrieveVideo): "/gateway/text-tasks/{video_id}?version=1",
				},
			},
		}

		assert.Equal(t, "/gateway/text-tasks/task-123?version=1&with_status=true", provider.mapRetrieveVideoPath("/v1/videos/task-123?kling_task_type=bad&with_status=true"))
	})
}

func TestKlingProviderGetJWTTokenUsesCache(t *testing.T) {
	provider := &klingProvider{
		config: ProviderConfig{
			klingAccessKey:         "access-key",
			klingSecretKey:         "secret-key",
			klingTokenRefreshAhead: 60,
		},
	}

	first := provider.getJWTToken()
	require.NotEmpty(t, first)
	second := provider.getJWTToken()
	assert.Equal(t, first, second)

	provider.jwtExpireAt = time.Now().Unix()
	refreshed := provider.getJWTToken()
	assert.NotEmpty(t, refreshed)
}

func TestKlingProviderTransformRequestBodyHeaders(t *testing.T) {
	provider := &klingProvider{
		config: ProviderConfig{
			modelMapping: map[string]string{"client-video": "kling-v2-1"},
		},
	}

	t.Run("text to video maps model to model_name", func(t *testing.T) {
		headers := http.Header{":path": []string{klingTextToVideoPath}}
		body := []byte(`{"model":"client-video","prompt":"sunrise","duration":"5","mode":"std"}`)

		result, err := provider.TransformRequestBodyHeaders(newMockMultipartHttpContext(), ApiNameVideos, body, headers)
		require.NoError(t, err)

		assert.Equal(t, klingTextToVideoPath, headers.Get(":path"))
		assert.False(t, gjson.GetBytes(result, "model").Exists())
		assert.Equal(t, "kling-v2-1", gjson.GetBytes(result, "model_name").String())
		assert.Equal(t, "sunrise", gjson.GetBytes(result, "prompt").String())
		assert.Equal(t, "5", gjson.GetBytes(result, "duration").String())
	})

	t.Run("image to video switches path", func(t *testing.T) {
		headers := http.Header{":path": []string{klingTextToVideoPath}}
		body := []byte(`{"model":"client-video","prompt":"animate","image":"https://example.com/a.png"}`)

		result, err := provider.TransformRequestBodyHeaders(newMockMultipartHttpContext(), ApiNameVideos, body, headers)
		require.NoError(t, err)

		assert.Equal(t, klingImageToVideoPath, headers.Get(":path"))
		assert.Equal(t, "kling-v2-1", gjson.GetBytes(result, "model_name").String())
		assert.Equal(t, "https://example.com/a.png", gjson.GetBytes(result, "image").String())
	})

	t.Run("text to video preserves query string", func(t *testing.T) {
		headers := http.Header{":path": []string{klingTextToVideoPath + "?trace=1"}}
		body := []byte(`{"model":"client-video","prompt":"sunrise"}`)

		_, err := provider.TransformRequestBodyHeaders(newMockMultipartHttpContext(), ApiNameVideos, body, headers)
		require.NoError(t, err)

		assert.Equal(t, klingTextToVideoPath+"?trace=1", headers.Get(":path"))
	})

	t.Run("image to video preserves query string", func(t *testing.T) {
		headers := http.Header{":path": []string{klingTextToVideoPath + "?trace=1"}}
		body := []byte(`{"model":"client-video","prompt":"animate","image":"https://example.com/a.png"}`)

		_, err := provider.TransformRequestBodyHeaders(newMockMultipartHttpContext(), ApiNameVideos, body, headers)
		require.NoError(t, err)

		assert.Equal(t, klingImageToVideoPath+"?trace=1", headers.Get(":path"))
	})

	t.Run("text create capability query merges with request query", func(t *testing.T) {
		customProvider := &klingProvider{
			config: ProviderConfig{
				capabilities: map[string]string{string(ApiNameVideos): "/gateway/text2video?version=1"},
				modelMapping: map[string]string{"client-video": "kling-v2-1"},
			},
		}
		headers := http.Header{":path": []string{"/v1/videos?trace=1"}}
		body := []byte(`{"model":"client-video","prompt":"sunrise"}`)

		_, err := customProvider.TransformRequestBodyHeaders(newMockMultipartHttpContext(), ApiNameVideos, body, headers)
		require.NoError(t, err)

		assert.Equal(t, "/gateway/text2video?version=1&trace=1", headers.Get(":path"))
	})

	t.Run("image create uses explicit image capability and merges query", func(t *testing.T) {
		customProvider := &klingProvider{
			config: ProviderConfig{
				capabilities: map[string]string{
					string(ApiNameVideos):            "/gateway/text2video",
					string(ApiNameKlingImageToVideo): "/gateway/image2video?version=1",
				},
				modelMapping: map[string]string{"client-video": "kling-v2-1"},
			},
		}
		headers := http.Header{":path": []string{"/v1/videos?trace=1"}}
		body := []byte(`{"model":"client-video","prompt":"animate","image":"https://example.com/a.png"}`)

		result, err := customProvider.TransformRequestBodyHeaders(newMockMultipartHttpContext(), ApiNameVideos, body, headers)
		require.NoError(t, err)

		assert.Equal(t, "/gateway/image2video?version=1&trace=1", headers.Get(":path"))
		assert.Equal(t, "kling-v2-1", gjson.GetBytes(result, "model_name").String())
	})

	t.Run("image create does not duplicate capability query after header mapping", func(t *testing.T) {
		customProvider := &klingProvider{
			config: ProviderConfig{
				capabilities: map[string]string{
					string(ApiNameKlingImageToVideo): "/gateway/image2video?version=1",
				},
				modelMapping: map[string]string{"client-video": "kling-v2-1"},
			},
		}
		headers := http.Header{":path": []string{"/gateway/image2video?version=1&trace=1"}}
		body := []byte(`{"model":"client-video","prompt":"animate","image":"https://example.com/a.png"}`)

		_, err := customProvider.TransformRequestBodyHeaders(newMockMultipartHttpContext(), ApiNameVideos, body, headers)
		require.NoError(t, err)

		assert.Equal(t, "/gateway/image2video?version=1&trace=1", headers.Get(":path"))
	})

	t.Run("image create does not inherit text capability query from header mapping", func(t *testing.T) {
		customProvider := &klingProvider{
			config: ProviderConfig{
				capabilities: map[string]string{
					string(ApiNameVideos):            "/gateway/text2video?mode=text",
					string(ApiNameKlingImageToVideo): "/gateway/image2video?mode=image",
				},
				modelMapping: map[string]string{"client-video": "kling-v2-1"},
			},
		}
		ctx := newMockMultipartHttpContext()
		ctx.SetContext(CtxRequestPath, "/v1/videos?trace=1")
		headers := http.Header{":path": []string{"/gateway/text2video?mode=text&trace=1"}}
		body := []byte(`{"model":"client-video","prompt":"animate","image":"https://example.com/a.png"}`)

		_, err := customProvider.TransformRequestBodyHeaders(ctx, ApiNameVideos, body, headers)
		require.NoError(t, err)

		assert.Equal(t, "/gateway/image2video?mode=image&trace=1", headers.Get(":path"))
	})

	t.Run("model_name is accepted and mapped in place", func(t *testing.T) {
		headers := http.Header{":path": []string{klingTextToVideoPath}}
		body := []byte(`{"model_name":"client-video","prompt":"sunrise"}`)

		result, err := provider.TransformRequestBodyHeaders(newMockMultipartHttpContext(), ApiNameVideos, body, headers)
		require.NoError(t, err)

		assert.Equal(t, "kling-v2-1", gjson.GetBytes(result, "model_name").String())
	})

	t.Run("missing model passes body through", func(t *testing.T) {
		headers := http.Header{":path": []string{klingTextToVideoPath}}
		body := []byte(`{"prompt":"sunrise"}`)

		result, err := provider.TransformRequestBodyHeaders(newMockMultipartHttpContext(), ApiNameVideos, body, headers)
		require.NoError(t, err)

		assert.Equal(t, string(body), string(result))
		assert.Equal(t, klingTextToVideoPath, headers.Get(":path"))
	})

}

func TestKlingProviderTransformResponseBody(t *testing.T) {
	provider := &klingOpenAIProvider{klingProvider: &klingProvider{}}

	t.Run("image creation prefixes returned task ids", func(t *testing.T) {
		ctx := newMockMultipartHttpContext()
		ctx.SetContext(ctxKeyKlingVideoTaskType, klingTaskTypeImageToVideo)

		result, err := provider.TransformResponseBody(ctx, ApiNameVideos, []byte(`{"id":"root-task","task_id":"top-task","data":{"task_id":"data-task"}}`))
		require.NoError(t, err)

		assert.Equal(t, "root-task", gjson.GetBytes(result, "id").String())
		assert.Equal(t, klingImageTaskIDPrefix+"top-task", gjson.GetBytes(result, "task_id").String())
		assert.Equal(t, klingImageTaskIDPrefix+"data-task", gjson.GetBytes(result, "data.task_id").String())
	})

	t.Run("text creation prefixes returned task ids", func(t *testing.T) {
		ctx := newMockMultipartHttpContext()
		ctx.SetContext(ctxKeyKlingVideoTaskType, klingTaskTypeTextToVideo)

		result, err := provider.TransformResponseBody(ctx, ApiNameVideos, []byte(`{"data":{"task_id":"data-task"}}`))
		require.NoError(t, err)

		assert.Equal(t, klingTextTaskIDPrefix+"data-task", gjson.GetBytes(result, "data.task_id").String())
	})

	t.Run("retrieve video response body passes through", func(t *testing.T) {
		ctx := newMockMultipartHttpContext()
		ctx.SetContext(ctxKeyKlingVideoTaskType, klingTaskTypeImageToVideo)
		body := []byte(`{"id":"root-task","data":{"task_id":"data-task"}}`)

		result, err := provider.TransformResponseBody(ctx, ApiNameRetrieveVideo, body)
		require.NoError(t, err)

		assert.Equal(t, string(body), string(result))
	})

	t.Run("video response without task type passes through", func(t *testing.T) {
		ctx := newMockMultipartHttpContext()
		body := []byte(`{"data":{"task_id":"data-task"}}`)

		result, err := provider.TransformResponseBody(ctx, ApiNameVideos, body)
		require.NoError(t, err)

		assert.Equal(t, string(body), string(result))
	})
}

func TestPrefixKlingTaskIDs(t *testing.T) {
	t.Run("already prefixed task ids are unchanged", func(t *testing.T) {
		body := []byte(`{"task_id":"kling-i2v-top-task","data":{"task_id":"kling-t2v-data-task"}}`)

		result, err := prefixKlingTaskIDs(body, klingImageTaskIDPrefix)
		require.NoError(t, err)

		assert.Equal(t, "kling-i2v-top-task", gjson.GetBytes(result, "task_id").String())
		assert.Equal(t, "kling-t2v-data-task", gjson.GetBytes(result, "data.task_id").String())
	})

	t.Run("missing task ids are ignored", func(t *testing.T) {
		body := []byte(`{"data":{"status":"submitted"}}`)

		result, err := prefixKlingTaskIDs(body, klingImageTaskIDPrefix)
		require.NoError(t, err)

		assert.Equal(t, string(body), string(result))
	})
}

func TestKlingProviderGetApiName(t *testing.T) {
	provider := &klingProvider{}

	tests := []struct {
		name string
		path string
		want ApiName
	}{
		{
			name: "text to video create",
			path: "/proxy/v1/videos/text2video",
			want: ApiNameVideos,
		},
		{
			name: "image to video create",
			path: "/proxy/v1/videos/image2video",
			want: ApiNameVideos,
		},
		{
			name: "openai retrieve",
			path: "/proxy/v1/videos/task-123",
			want: ApiNameRetrieveVideo,
		},
		{
			name: "native text retrieve",
			path: "/proxy/v1/videos/text2video/task-123",
			want: ApiNameRetrieveVideo,
		},
		{
			name: "native image retrieve",
			path: "/proxy/v1/videos/image2video/task-123",
			want: ApiNameRetrieveVideo,
		},
		{
			name: "native text create is not retrieve",
			path: "/proxy/v1/videos/text2video",
			want: ApiNameVideos,
		},
		{
			name: "unsupported path",
			path: "/proxy/v1/images/generations",
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, provider.GetApiName(tt.path))
		})
	}
}

func TestKlingQueryMerge(t *testing.T) {
	tests := []struct {
		name       string
		targetPath string
		query      string
		want       string
	}{
		{
			name:       "empty query leaves target unchanged",
			targetPath: "/gateway/image2video",
			query:      "",
			want:       "/gateway/image2video",
		},
		{
			name:       "request query is appended",
			targetPath: "/gateway/image2video",
			query:      "?trace=1",
			want:       "/gateway/image2video?trace=1",
		},
		{
			name:       "capability query and request query are merged",
			targetPath: "/gateway/image2video?version=1",
			query:      "?trace=1",
			want:       "/gateway/image2video?version=1&trace=1",
		},
		{
			name:       "duplicate capability query from mapped path is not repeated",
			targetPath: "/gateway/image2video?version=1",
			query:      "?version=1&trace=1",
			want:       "/gateway/image2video?version=1&trace=1",
		},
		{
			name:       "query without question mark is accepted",
			targetPath: "/gateway/image2video?version=1",
			query:      "trace=1",
			want:       "/gateway/image2video?version=1&trace=1",
		},
		{
			name:       "empty question mark query leaves target unchanged",
			targetPath: "/gateway/image2video",
			query:      "?",
			want:       "/gateway/image2video",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, appendKlingQuery(tt.targetPath, tt.query))
		})
	}

	t.Run("merge skips empty and duplicate parts", func(t *testing.T) {
		assert.Equal(t, "version=1&trace=1", mergeKlingQueryParts("version=1&&", "&trace=1&&version=1"))
	})
}

func TestKlingTaskTypeQuery(t *testing.T) {
	t.Run("extract task type query", func(t *testing.T) {
		tests := []struct {
			name           string
			query          string
			wantTaskType   string
			wantForwarding string
		}{
			{
				name:           "empty query",
				query:          "",
				wantTaskType:   "",
				wantForwarding: "",
			},
			{
				name:           "only task type",
				query:          "?kling_task_type=image2video",
				wantTaskType:   klingTaskTypeImageToVideo,
				wantForwarding: "",
			},
			{
				name:           "task type is stripped and other query params are forwarded",
				query:          "?trace=1&kling_task_type=t2v&with_status=true",
				wantTaskType:   klingTaskTypeTextToVideo,
				wantForwarding: "?trace=1&with_status=true",
			},
			{
				name:           "url encoded value",
				query:          "?kling_task_type=image%32video&trace=1",
				wantTaskType:   klingTaskTypeImageToVideo,
				wantForwarding: "?trace=1",
			},
			{
				name:           "url encoded unknown value",
				query:          "?kling_task_type=image%202video&trace=1",
				wantTaskType:   "",
				wantForwarding: "?trace=1",
			},
			{
				name:           "repeated task type uses the last value",
				query:          "?kling_task_type=t2v&trace=1&kling_task_type=i2v",
				wantTaskType:   klingTaskTypeImageToVideo,
				wantForwarding: "?trace=1",
			},
			{
				name:           "invalid encoded key is forwarded",
				query:          "?%zz=image2video&trace=1",
				wantTaskType:   "",
				wantForwarding: "?%zz=image2video&trace=1",
			},
			{
				name:           "invalid encoded value falls back before normalization",
				query:          "?kling_task_type=%zz&trace=1",
				wantTaskType:   "",
				wantForwarding: "?trace=1",
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				taskType, forwarding := extractKlingTaskTypeQuery(tt.query)
				assert.Equal(t, tt.wantTaskType, taskType)
				assert.Equal(t, tt.wantForwarding, forwarding)
			})
		}
	})

	t.Run("normalize task type", func(t *testing.T) {
		tests := []struct {
			name string
			raw  string
			want string
		}{
			{
				name: "image alias",
				raw:  "image",
				want: klingTaskTypeImageToVideo,
			},
			{
				name: "text alias",
				raw:  "t2v",
				want: klingTaskTypeTextToVideo,
			},
			{
				name: "unknown value",
				raw:  "image 2video",
				want: "",
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				assert.Equal(t, tt.want, normalizeKlingTaskType(tt.raw))
			})
		}
	})
}

func TestKlingProviderOnRequestBodyUnsupportedAPI(t *testing.T) {
	provider := &klingOpenAIProvider{
		klingProvider: &klingProvider{
			config: ProviderConfig{
				capabilities: map[string]string{},
			},
		},
	}

	action, err := provider.OnRequestBody(newMockMultipartHttpContext(), ApiNameVideos, []byte(`{}`))
	assert.Equal(t, types.ActionContinue, action)
	assert.ErrorIs(t, err, errUnsupportedApiName)
}

func decodeKlingJWTPayload(t *testing.T, token string) map[string]interface{} {
	t.Helper()

	parts := strings.Split(token, ".")
	require.Len(t, parts, 3)
	payloadJSON, err := base64.RawURLEncoding.DecodeString(parts[1])
	require.NoError(t, err)
	var payload map[string]interface{}
	require.NoError(t, json.Unmarshal(payloadJSON, &payload))
	return payload
}

func requireKlingBaseProvider(t *testing.T, created Provider) *klingProvider {
	t.Helper()

	switch provider := created.(type) {
	case *klingProvider:
		return provider
	case *klingOpenAIProvider:
		return provider.klingProvider
	default:
		t.Fatalf("expected kling provider, got %T", created)
		return nil
	}
}
