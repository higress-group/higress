package main

import (
	"encoding/json"
	"testing"

	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm/types"
	"github.com/higress-group/wasm-go/pkg/test"
	"github.com/stretchr/testify/require"
)

func TestNginxRewriteCompatible(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		t.Run("basic rewrite", func(t *testing.T) {
			host, status := test.NewTestHost(mustConfig(t, map[string]any{
				"rules": []map[string]any{
					{
						"regex":       "^/old/(.*)$",
						"replacement": "/new/$1",
					},
				},
			}))
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			action := host.CallOnHttpRequestHeaders(baseHeaders("/old/demo"))
			require.Equal(t, types.ActionContinue, action)
			requirePath(t, host.GetRequestHeaders(), "/new/demo")
		})

		t.Run("rewrite and query append", func(t *testing.T) {
			host, status := test.NewTestHost(mustConfig(t, map[string]any{
				"rules": []map[string]any{
					{
						"regex":        "^/api/(.*)$",
						"replacement":  "/internal",
						"query_append": "migrated=true",
					},
				},
			}))
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			action := host.CallOnHttpRequestHeaders(baseHeaders("/api/orders?id=1"))
			require.Equal(t, types.ActionContinue, action)
			requirePath(t, host.GetRequestHeaders(), "/internal?id=1&migrated=true")
		})

		t.Run("rewrite and set vars", func(t *testing.T) {
			host, status := test.NewTestHost(mustConfig(t, map[string]any{
				"rules": []map[string]any{
					{
						"regex":       "^/user/(.*)$",
						"replacement": "/profile",
						"set_vars": []map[string]any{
							{"name": "user_id", "capture_group": 1},
						},
					},
				},
			}))
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			action := host.CallOnHttpRequestHeaders(baseHeaders("/user/alice"))
			require.Equal(t, types.ActionContinue, action)
			requirePath(t, host.GetRequestHeaders(), "/profile")
			value, err := host.GetProperty([]string{"nginx_rewrite_compatible", "vars", "user_id"})
			require.NoError(t, err)
			require.Equal(t, "alice", string(value))
		})

		t.Run("special set var prefixes", func(t *testing.T) {
			host, status := test.NewTestHost(mustConfig(t, map[string]any{
				"rules": []map[string]any{
					{
						"regex":       "^/api/(.*)/(.*)$",
						"replacement": "/internal",
						"set_vars": []map[string]any{
							{"name": "original_path", "capture_group": 1},
							{"name": "http_x_original", "capture_group": 1},
							{"name": "arg_source", "capture_group": 2},
							{"name": "cookie_track_id", "capture_group": 1},
						},
					},
				},
			}))
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			action := host.CallOnHttpRequestHeaders(baseHeadersWithCookie("/api/orders/mobile?legacy=yes", "session=abc"))
			require.Equal(t, types.ActionContinue, action)
			requirePath(t, host.GetRequestHeaders(), "/internal?legacy=yes&source=mobile")
			requireHeader(t, host.GetRequestHeaders(), "x-original", "orders")
			requireHeader(t, host.GetRequestHeaders(), "cookie", "session=abc; track_id=orders")

			value, err := host.GetProperty([]string{"nginx_rewrite_compatible", "vars", "original_path"})
			require.NoError(t, err)
			require.Equal(t, "orders", string(value))

			_, err = host.GetProperty([]string{"nginx_rewrite_compatible", "vars", "http_x_original"})
			require.Error(t, err)
			_, err = host.GetProperty([]string{"nginx_rewrite_compatible", "vars", "arg_source"})
			require.Error(t, err)
			_, err = host.GetProperty([]string{"nginx_rewrite_compatible", "vars", "cookie_track_id"})
			require.Error(t, err)
		})

		t.Run("multiple capture groups", func(t *testing.T) {
			host, status := test.NewTestHost(mustConfig(t, map[string]any{
				"rules": []map[string]any{
					{
						"regex":          "^/a/(.*)/b/(.*)$",
						"replacement":    "/c",
						"query_template": "x=$1&y=$2",
						"set_vars": []map[string]any{
							{"name": "first", "capture_group": 1},
							{"name": "second", "capture_group": 2},
						},
					},
				},
			}))
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			action := host.CallOnHttpRequestHeaders(baseHeaders("/a/hello/b/world"))
			require.Equal(t, types.ActionContinue, action)
			requirePath(t, host.GetRequestHeaders(), "/c?x=hello&y=world")

			first, err := host.GetProperty([]string{"nginx_rewrite_compatible", "vars", "first"})
			require.NoError(t, err)
			require.Equal(t, "hello", string(first))
			second, err := host.GetProperty([]string{"nginx_rewrite_compatible", "vars", "second"})
			require.NoError(t, err)
			require.Equal(t, "world", string(second))
		})

		t.Run("break vs last", func(t *testing.T) {
			host, status := test.NewTestHost(mustConfig(t, map[string]any{
				"rules": []map[string]any{
					{
						"regex":       "^/stage/(.*)$",
						"replacement": "/mid/$1",
						"mode":        "break",
					},
					{
						"regex":       "^/mid/(.*)$",
						"replacement": "/final/$1",
					},
				},
			}))
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			action := host.CallOnHttpRequestHeaders(baseHeaders("/stage/item"))
			require.Equal(t, types.ActionContinue, action)
			requirePath(t, host.GetRequestHeaders(), "/mid/item")
		})

		t.Run("last continues matching", func(t *testing.T) {
			host, status := test.NewTestHost(mustConfig(t, map[string]any{
				"rules": []map[string]any{
					{
						"regex":       "^/stage/(.*)$",
						"replacement": "/mid/$1",
						"mode":        "last",
					},
					{
						"regex":       "^/mid/(.*)$",
						"replacement": "/final/$1",
					},
				},
			}))
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			action := host.CallOnHttpRequestHeaders(baseHeaders("/stage/item"))
			require.Equal(t, types.ActionContinue, action)
			requirePath(t, host.GetRequestHeaders(), "/final/item")
		})

		t.Run("no match keeps original request", func(t *testing.T) {
			host, status := test.NewTestHost(mustConfig(t, map[string]any{
				"rules": []map[string]any{
					{
						"regex":       "^/old/(.*)$",
						"replacement": "/new/$1",
					},
				},
			}))
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			action := host.CallOnHttpRequestHeaders(baseHeaders("/keep"))
			require.Equal(t, types.ActionContinue, action)
			requirePath(t, host.GetRequestHeaders(), "/keep")
		})

		t.Run("special characters are preserved", func(t *testing.T) {
			host, status := test.NewTestHost(mustConfig(t, map[string]any{
				"rules": []map[string]any{
					{
						"regex":          "^/raw/(.*)$",
						"replacement":    "/target",
						"query_template": "value=$1&flag=a+b&expr=%25done",
					},
				},
			}))
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			action := host.CallOnHttpRequestHeaders(baseHeaders("/raw/a+b%26=%25"))
			require.Equal(t, types.ActionContinue, action)
			requirePath(t, host.GetRequestHeaders(), "/target?value=a+b%26=%25&flag=a+b&expr=%25done")
		})

		t.Run("empty capture group", func(t *testing.T) {
			host, status := test.NewTestHost(mustConfig(t, map[string]any{
				"rules": []map[string]any{
					{
						"regex":       "^/empty/(.*)$",
						"replacement": "/done",
						"set_vars": []map[string]any{
							{"name": "tail", "capture_group": 1},
						},
					},
				},
			}))
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			action := host.CallOnHttpRequestHeaders(baseHeaders("/empty/"))
			require.Equal(t, types.ActionContinue, action)
			requirePath(t, host.GetRequestHeaders(), "/done")
			_, err := host.GetProperty([]string{"nginx_rewrite_compatible", "vars", "tail"})
			require.Error(t, err)
		})

		t.Run("query template replaces existing query", func(t *testing.T) {
			host, status := test.NewTestHost(mustConfig(t, map[string]any{
				"rules": []map[string]any{
					{
						"regex":          "^/query/(.*)$",
						"replacement":    "/target",
						"query_template": "id=$1",
					},
				},
			}))
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			action := host.CallOnHttpRequestHeaders(baseHeaders("/query/abc?legacy=yes"))
			require.Equal(t, types.ActionContinue, action)
			requirePath(t, host.GetRequestHeaders(), "/target?id=abc")
		})

		t.Run("query append preserves existing query", func(t *testing.T) {
			host, status := test.NewTestHost(mustConfig(t, map[string]any{
				"rules": []map[string]any{
					{
						"regex":        "^/query/(.*)$",
						"replacement":  "/target",
						"query_append": "id=$1",
					},
				},
			}))
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			action := host.CallOnHttpRequestHeaders(baseHeaders("/query/abc?legacy=yes"))
			require.Equal(t, types.ActionContinue, action)
			requirePath(t, host.GetRequestHeaders(), "/target?legacy=yes&id=abc")
		})

		t.Run("pass to upstream adds headers", func(t *testing.T) {
			host, status := test.NewTestHost(mustConfig(t, map[string]any{
				"rules": []map[string]any{
					{
						"regex":            "^/api/(.*)$",
						"replacement":      "/internal",
						"pass_to_upstream": true,
						"set_vars": []map[string]any{
							{"name": "original_endpoint", "capture_group": 1},
						},
					},
				},
			}))
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			action := host.CallOnHttpRequestHeaders(baseHeaders("/api/orders"))
			require.Equal(t, types.ActionContinue, action)
			requirePath(t, host.GetRequestHeaders(), "/internal")
			requireHeader(t, host.GetRequestHeaders(), "x-higress-rewrite-var-original-endpoint", "orders")
		})
	})
}

func mustConfig(t *testing.T, cfg map[string]any) []byte {
	t.Helper()
	data, err := json.Marshal(cfg)
	require.NoError(t, err)
	return data
}

func baseHeaders(path string) [][2]string {
	return [][2]string{
		{":authority", "example.com"},
		{":path", path},
		{":method", "GET"},
	}
}

func baseHeadersWithCookie(path string, cookie string) [][2]string {
	headers := baseHeaders(path)
	return append(headers, [2]string{"cookie", cookie})
}

func requirePath(t *testing.T, headers [][2]string, expected string) {
	t.Helper()
	requireHeader(t, headers, ":path", expected)
}

func requireHeader(t *testing.T, headers [][2]string, name string, expected string) {
	t.Helper()
	for _, header := range headers {
		if header[0] == name {
			require.Equal(t, expected, header[1])
			return
		}
	}
	t.Fatalf("header %s not found", name)
}
