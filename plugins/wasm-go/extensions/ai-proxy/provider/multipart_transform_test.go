package provider

import (
	"bytes"
	"errors"
	"io"
	"mime/multipart"
	"strings"
	"testing"

	"github.com/higress-group/wasm-go/pkg/iface"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockMultipartHttpContext struct {
	contextMap map[string]interface{}
}

func newMockMultipartHttpContext() *mockMultipartHttpContext {
	return &mockMultipartHttpContext{contextMap: make(map[string]interface{})}
}

func (m *mockMultipartHttpContext) SetContext(key string, value interface{}) {
	m.contextMap[key] = value
}
func (m *mockMultipartHttpContext) GetContext(key string) interface{}                 { return m.contextMap[key] }
func (m *mockMultipartHttpContext) GetBoolContext(key string, def bool) bool          { return def }
func (m *mockMultipartHttpContext) GetStringContext(key, def string) string           { return def }
func (m *mockMultipartHttpContext) GetByteSliceContext(key string, def []byte) []byte { return def }
func (m *mockMultipartHttpContext) Scheme() string                                    { return "" }
func (m *mockMultipartHttpContext) Host() string                                      { return "" }
func (m *mockMultipartHttpContext) Path() string                                      { return "" }
func (m *mockMultipartHttpContext) Method() string                                    { return "" }
func (m *mockMultipartHttpContext) GetUserAttribute(key string) interface{}           { return nil }
func (m *mockMultipartHttpContext) SetUserAttribute(key string, value interface{})    {}
func (m *mockMultipartHttpContext) SetUserAttributeMap(kvmap map[string]interface{})  {}
func (m *mockMultipartHttpContext) GetUserAttributeMap() map[string]interface{}       { return nil }
func (m *mockMultipartHttpContext) WriteUserAttributeToLog() error                    { return nil }
func (m *mockMultipartHttpContext) WriteUserAttributeToLogWithKey(key string) error   { return nil }
func (m *mockMultipartHttpContext) WriteUserAttributeToTrace() error                  { return nil }
func (m *mockMultipartHttpContext) DontReadRequestBody()                              {}
func (m *mockMultipartHttpContext) DontReadResponseBody()                             {}
func (m *mockMultipartHttpContext) BufferRequestBody()                                {}
func (m *mockMultipartHttpContext) BufferResponseBody()                               {}
func (m *mockMultipartHttpContext) NeedPauseStreamingResponse()                       {}
func (m *mockMultipartHttpContext) PushBuffer(buffer []byte)                          {}
func (m *mockMultipartHttpContext) PopBuffer() []byte                                 { return nil }
func (m *mockMultipartHttpContext) BufferQueueSize() int                              { return 0 }
func (m *mockMultipartHttpContext) DisableReroute()                                   {}
func (m *mockMultipartHttpContext) SetRequestBodyBufferLimit(byteSize uint32)         {}
func (m *mockMultipartHttpContext) SetResponseBodyBufferLimit(byteSize uint32)        {}
func (m *mockMultipartHttpContext) RouteCall(method, url string, headers [][2]string, body []byte, callback iface.RouteResponseCallback) error {
	return nil
}
func (m *mockMultipartHttpContext) GetExecutionPhase() iface.HTTPExecutionPhase { return 0 }
func (m *mockMultipartHttpContext) HasRequestBody() bool                        { return false }
func (m *mockMultipartHttpContext) HasResponseBody() bool                       { return false }
func (m *mockMultipartHttpContext) IsWebsocket() bool                           { return false }
func (m *mockMultipartHttpContext) IsBinaryRequestBody() bool                   { return false }
func (m *mockMultipartHttpContext) IsBinaryResponseBody() bool                  { return false }

func buildProviderMultipartRequestBody(t *testing.T, fields map[string]string, files map[string][]byte) ([]byte, string) {
	t.Helper()

	var buffer bytes.Buffer
	writer := multipart.NewWriter(&buffer)

	for key, value := range fields {
		require.NoError(t, writer.WriteField(key, value))
	}
	for fieldName, data := range files {
		part, err := writer.CreateFormFile(fieldName, "upload-image.png")
		require.NoError(t, err)
		_, err = part.Write(data)
		require.NoError(t, err)
	}

	require.NoError(t, writer.Close())
	return buffer.Bytes(), writer.FormDataContentType()
}

type failAfterNWriteWriter struct {
	target     io.Writer
	failAtCall int
	writeCalls int
}

func (w *failAfterNWriteWriter) Write(p []byte) (int, error) {
	w.writeCalls++
	if w.writeCalls >= w.failAtCall {
		return 0, errors.New("injected write failure")
	}
	return w.target.Write(p)
}

func withInjectedMultipartWriterFactory(t *testing.T, failAtCall int, testFunc func()) {
	t.Helper()

	originalFactory := newMultipartWriter
	newMultipartWriter = func(target io.Writer) *multipart.Writer {
		return multipart.NewWriter(&failAfterNWriteWriter{
			target:     target,
			failAtCall: failAtCall,
		})
	}
	defer func() {
		newMultipartWriter = originalFactory
	}()

	testFunc()
}

func TestRewriteMultipartFormModel(t *testing.T) {
	t.Run("rewrites existing model field", func(t *testing.T) {
		body, contentType := buildProviderMultipartRequestBody(t, map[string]string{
			"model":  "gpt-image-1.5",
			"prompt": "Turn the dog white",
		}, map[string][]byte{
			"image[]": []byte("fake-image-content"),
		})

		transformed, err := rewriteMultipartFormModel(body, contentType, "gpt-image-1")
		require.NoError(t, err)

		req, err := parseMultipartImageRequest(transformed, contentType)
		require.NoError(t, err)
		assert.Equal(t, "gpt-image-1", req.Model)
		assert.Equal(t, "Turn the dog white", req.Prompt)
		assert.Len(t, req.ImageURLs, 1)
		assert.Contains(t, string(transformed), "fake-image-content")
	})

	t.Run("appends model field when missing", func(t *testing.T) {
		body, contentType := buildProviderMultipartRequestBody(t, map[string]string{
			"prompt": "Turn the dog white",
		}, map[string][]byte{
			"image": []byte("fake-image-content"),
		})

		transformed, err := rewriteMultipartFormModel(body, contentType, "gpt-image-1")
		require.NoError(t, err)

		req, err := parseMultipartImageRequest(transformed, contentType)
		require.NoError(t, err)
		assert.Equal(t, "gpt-image-1", req.Model)
		assert.Equal(t, "Turn the dog white", req.Prompt)
		assert.Len(t, req.ImageURLs, 1)
	})

	t.Run("returns error on invalid content type", func(t *testing.T) {
		_, err := rewriteMultipartFormModel([]byte("not-multipart"), "multipart/form-data", "gpt-image-1")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "missing multipart boundary")
	})

	t.Run("returns error when boundary cannot be set", func(t *testing.T) {
		longBoundary := strings.Repeat("a", 71)
		_, err := rewriteMultipartFormModel([]byte(""), "multipart/form-data; boundary="+longBoundary, "gpt-image-1")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unable to set multipart boundary")
	})

	t.Run("returns error on malformed multipart header", func(t *testing.T) {
		body := []byte("--abc\r\nnot-a-header\r\n\r\nvalue\r\n--abc--\r\n")
		_, err := rewriteMultipartFormModel(body, "multipart/form-data; boundary=abc", "gpt-image-1")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unable to read multipart part")
	})

	t.Run("returns error when multipart part copy fails", func(t *testing.T) {
		body := []byte("--abc\r\nContent-Disposition: form-data; name=\"image\"; filename=\"a.png\"\r\nContent-Type: image/png\r\n\r\nabc\r\n--ab")
		_, err := rewriteMultipartFormModel(body, "multipart/form-data; boundary=abc", "gpt-image-1")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unable to write multipart field image")
	})

	t.Run("returns error when creating rewritten multipart part fails", func(t *testing.T) {
		body, contentType := buildProviderMultipartRequestBody(t, map[string]string{
			"prompt": "Turn the dog white",
		}, map[string][]byte{
			"image": []byte("fake-image-content"),
		})

		withInjectedMultipartWriterFactory(t, 1, func() {
			_, err := rewriteMultipartFormModel(body, contentType, "gpt-image-1")
			require.Error(t, err)
			assert.Contains(t, err.Error(), "unable to create multipart field")
		})
	})

	t.Run("returns error when appending model field fails", func(t *testing.T) {
		withInjectedMultipartWriterFactory(t, 1, func() {
			_, err := rewriteMultipartFormModel([]byte("--abc--\r\n"), "multipart/form-data; boundary=abc", "gpt-image-1")
			require.Error(t, err)
			assert.Contains(t, err.Error(), "unable to append multipart model field")
		})
	})

	t.Run("returns error when finalizing multipart body fails", func(t *testing.T) {
		withInjectedMultipartWriterFactory(t, 1, func() {
			_, err := rewriteMultipartFormModel([]byte("--abc--\r\n"), "multipart/form-data; boundary=abc", "")
			require.Error(t, err)
			assert.Contains(t, err.Error(), "unable to finalize multipart body")
		})
	})
}

func TestDefaultTransformMultipartRequestBody(t *testing.T) {
	t.Run("maps multipart model and keeps body valid", func(t *testing.T) {
		body, contentType := buildProviderMultipartRequestBody(t, map[string]string{
			"model":  "gpt-image-1.5",
			"prompt": "Turn the dog white",
			"size":   "1024x1024",
		}, map[string][]byte{
			"image[]": []byte("fake-image-content"),
		})

		config := &ProviderConfig{
			modelMapping: map[string]string{
				"gpt-image-1.5": "gpt-image-1",
			},
		}
		ctx := newMockMultipartHttpContext()

		transformed, err := config.defaultTransformMultipartRequestBody(ctx, ApiNameImageEdit, body, contentType)
		require.NoError(t, err)

		req, err := parseMultipartImageRequest(transformed, contentType)
		require.NoError(t, err)
		assert.Equal(t, "gpt-image-1.5", ctx.GetContext(ctxKeyOriginalRequestModel))
		assert.Equal(t, "gpt-image-1", ctx.GetContext(ctxKeyFinalRequestModel))
		assert.Equal(t, "gpt-image-1", req.Model)
		assert.Equal(t, "Turn the dog white", req.Prompt)
		assert.Len(t, req.ImageURLs, 1)
		assert.Contains(t, string(transformed), "fake-image-content")
	})

	t.Run("appends mapped model when multipart request omits model", func(t *testing.T) {
		body, contentType := buildProviderMultipartRequestBody(t, map[string]string{
			"prompt": "Turn the dog white",
		}, map[string][]byte{
			"image": []byte("fake-image-content"),
		})

		config := &ProviderConfig{
			modelMapping: map[string]string{
				"*": "gpt-image-1",
			},
		}
		ctx := newMockMultipartHttpContext()

		transformed, err := config.defaultTransformMultipartRequestBody(ctx, ApiNameImageVariation, body, contentType)
		require.NoError(t, err)

		req, err := parseMultipartImageRequest(transformed, contentType)
		require.NoError(t, err)
		assert.Equal(t, "", ctx.GetContext(ctxKeyOriginalRequestModel))
		assert.Equal(t, "gpt-image-1", ctx.GetContext(ctxKeyFinalRequestModel))
		assert.Equal(t, "gpt-image-1", req.Model)
	})

	t.Run("returns original body when multipart model is unchanged", func(t *testing.T) {
		body, contentType := buildProviderMultipartRequestBody(t, map[string]string{
			"model":  "gpt-image-1",
			"prompt": "Turn the dog white",
		}, map[string][]byte{
			"image": []byte("fake-image-content"),
		})

		config := &ProviderConfig{}
		ctx := newMockMultipartHttpContext()

		transformed, err := config.defaultTransformMultipartRequestBody(ctx, ApiNameImageEdit, body, contentType)
		require.NoError(t, err)
		assert.Equal(t, body, transformed)
		assert.Equal(t, "gpt-image-1", ctx.GetContext(ctxKeyOriginalRequestModel))
		assert.Equal(t, "gpt-image-1", ctx.GetContext(ctxKeyFinalRequestModel))
	})

	t.Run("ignores non image multipart apis", func(t *testing.T) {
		body, contentType := buildProviderMultipartRequestBody(t, map[string]string{
			"model": "gpt-image-1",
		}, nil)

		config := &ProviderConfig{
			modelMapping: map[string]string{
				"gpt-image-1": "mapped-model",
			},
		}
		ctx := newMockMultipartHttpContext()

		transformed, err := config.defaultTransformMultipartRequestBody(ctx, ApiNameChatCompletion, body, contentType)
		require.NoError(t, err)
		assert.Equal(t, body, transformed)
		assert.Nil(t, ctx.GetContext(ctxKeyOriginalRequestModel))
		assert.Nil(t, ctx.GetContext(ctxKeyFinalRequestModel))
	})

	t.Run("surfaces multipart parse errors", func(t *testing.T) {
		config := &ProviderConfig{}
		ctx := newMockMultipartHttpContext()

		_, err := config.defaultTransformMultipartRequestBody(ctx, ApiNameImageEdit, []byte("bad-body"), "multipart/form-data")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "missing multipart boundary")
	})
}

func TestExtractMultipartModel(t *testing.T) {
	t.Run("extracts model value", func(t *testing.T) {
		body, contentType := buildProviderMultipartRequestBody(t, map[string]string{
			"model":  "gpt-image-1.5",
			"prompt": "Turn the dog white",
		}, nil)

		model, err := extractMultipartModel(body, contentType)
		require.NoError(t, err)
		assert.Equal(t, "gpt-image-1.5", model)
	})

	t.Run("returns empty model when field missing", func(t *testing.T) {
		body, contentType := buildProviderMultipartRequestBody(t, map[string]string{
			"prompt": "Turn the dog white",
		}, nil)

		model, err := extractMultipartModel(body, contentType)
		require.NoError(t, err)
		assert.Equal(t, "", model)
	})

	t.Run("returns parse error for invalid content type", func(t *testing.T) {
		_, err := extractMultipartModel([]byte("bad-body"), "multipart/form-data; boundary=\"")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unable to parse content-type")
	})

	t.Run("returns parse error for malformed multipart header", func(t *testing.T) {
		body := []byte("--abc\r\nnot-a-header\r\n\r\nvalue\r\n--abc--\r\n")
		_, err := extractMultipartModel(body, "multipart/form-data; boundary=abc")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unable to read multipart part")
	})

	t.Run("returns field read error on truncated model part", func(t *testing.T) {
		body := []byte("--abc\r\nContent-Disposition: form-data; name=\"model\"\r\n\r\nvalue\r\n--ab")
		_, err := extractMultipartModel(body, "multipart/form-data; boundary=abc")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unable to read multipart field model")
	})
}

func TestParseMultipartImageRequestContentTypeError(t *testing.T) {
	_, err := parseMultipartImageRequest([]byte("bad-body"), "multipart/form-data; boundary=\"")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unable to parse content-type")
}

func TestIsMultipartFormData(t *testing.T) {
	assert.True(t, isMultipartFormData("multipart/form-data; boundary=abc"))
	assert.False(t, isMultipartFormData("application/json"))
	assert.False(t, isMultipartFormData("multipart/form-data; boundary=\""))
}
