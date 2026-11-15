package main

import (
	"context"
	"encoding/binary"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm/types"
	"github.com/higress-group/wasm-go/pkg/test"
	"github.com/stretchr/testify/require"
	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/imports/wasi_snapshot_preview1"
)

// The SDK emulator always accepts Redis dispatch and only stores the final
// stream action. This test host exercises the compiled plugin's ABI so dispatch
// failure and the exact number of resume/local-response calls are observable,
// without injecting hooks into the production plugin or modifying the SDK.
func TestHeaderKeyLookupABI(t *testing.T) {
	wasmPath := os.Getenv("WASM_FILE_PATH")
	if wasmPath == "" {
		wasmPath = filepath.Join(t.TempDir(), "response-cache.wasm")
		cmd := exec.Command("go", "build", "-buildmode=c-shared", "-o", wasmPath, "./")
		for _, env := range os.Environ() {
			if !strings.HasPrefix(env, "GOOS=") && !strings.HasPrefix(env, "GOARCH=") {
				cmd.Env = append(cmd.Env, env)
			}
		}
		cmd.Env = append(cmd.Env, "GOOS=wasip1", "GOARCH=wasm")
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, "%s", out)
	}
	wasm, err := os.ReadFile(wasmPath)
	require.NoError(t, err)
	ctx := context.Background()
	runtime := wazero.NewRuntime(ctx)
	t.Cleanup(func() { require.NoError(t, runtime.Close(ctx)) })
	_, err = wasi_snapshot_preview1.Instantiate(ctx, runtime)
	require.NoError(t, err)
	compiled, err := runtime.CompileModule(ctx, wasm)
	require.NoError(t, err)

	var host *headerLookupHost
	env := runtime.NewHostModuleBuilder("env")
	for _, definition := range compiled.ImportedFunctions() {
		module, name, _ := definition.Import()
		if module != "env" {
			continue
		}
		env.NewFunctionBuilder().WithGoModuleFunction(api.GoModuleFunc(
			func(ctx context.Context, mod api.Module, stack []uint64) {
				// The upper 32 bits of an i32 stack slot are unspecified.
				for i, typ := range definition.ParamTypes() {
					if typ == api.ValueTypeI32 {
						stack[i] = uint64(uint32(stack[i]))
					}
				}
				result := host.call(ctx, mod, name, stack)
				stack[0] = result
			}), definition.ParamTypes(), definition.ResultTypes()).Export(name)
	}
	_, err = env.Instantiate(ctx)
	require.NoError(t, err)

	for _, tc := range []struct {
		name           string
		key            string
		skip           bool
		dispatchStatus uint64
		callbackStatus uint64
		reply          []byte
		wantResume     int
		wantLocal      int
	}{
		{name: "hit", key: "key", reply: test.CreateRedisRespString("cached value"), wantLocal: 1},
		{name: "miss", key: "key", reply: test.CreateRedisRespNull(), wantResume: 1},
		{name: "RESP error", key: "key", reply: []byte("-ERR fixture error\r\n"), wantResume: 1},
		{name: "host error", key: "key", callbackStatus: 1, wantResume: 1},
		{name: "empty value", key: "key", reply: test.CreateRedisRespString(""), wantResume: 1},
		{name: "whitespace value", key: "key", reply: test.CreateRedisRespString(" \t\n"), wantResume: 1},
		{name: "synchronous dispatch failure", key: "key", dispatchStatus: 10},
		{name: "empty header"},
		{name: "explicit skip", key: "key", skip: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			host = &headerLookupHost{
				t: t, config: configWithHeaderKey,
				headers: map[string]string{
					":authority": "example.com", ":method": "POST", ":path": "/api/data",
					"content-type": "application/json", "accept-encoding": "gzip", "x-user-id": tc.key,
				},
				properties: map[string]string{}, dispatchStatus: tc.dispatchStatus,
			}
			if tc.skip {
				host.headers[SKIP_CACHE_HEADER] = "on"
			}
			mod, err := runtime.InstantiateModule(ctx, compiled, wazero.NewModuleConfig().
				WithName("").WithStartFunctions("_initialize").WithStdout(os.Stderr).WithStderr(os.Stderr))
			require.NoError(t, err)
			defer mod.Close(ctx)
			invoke := func(name string, args ...uint64) []uint64 {
				res, err := mod.ExportedFunction(name).Call(ctx, args...)
				require.NoError(t, err)
				return res
			}
			invoke("proxy_on_context_create", 1, 0)
			require.Equal(t, uint64(1), invoke("proxy_on_configure", 1, uint64(len(host.config)))[0])
			invoke("proxy_on_context_create", 2, 1)
			action := invoke("proxy_on_request_headers", 2, uint64(len(host.headers)), 0)[0]
			require.Zero(t, host.resumes)
			require.Zero(t, host.localResponses)
			if tc.key == "" || tc.skip {
				require.Equal(t, uint64(types.ActionContinue), action)
				require.Zero(t, host.dispatches)
				require.Equal(t, "gzip", host.headers["accept-encoding"])
			} else {
				require.Equal(t, 1, host.dispatches)
				require.Equal(t, "outbound|6379||redis.static", host.redisCluster)
				require.Equal(t, "off", host.properties["clear_route_cache"])
				require.NotContains(t, host.headers, "accept-encoding")
				require.Equal(t, []string{"dispatch", "disable-reroute", "remove-accept-encoding"}, host.effects)
				if tc.dispatchStatus != 0 {
					require.Equal(t, uint64(types.ActionContinue), action)
				} else {
					require.Equal(t, uint64(types.HeaderStopAllIterationAndWatermark), action)
					host.reply = tc.reply
					invoke("proxy_on_redis_call_response", 1, 1, tc.callbackStatus, uint64(len(tc.reply)))
				}
			}
			require.Equal(t, tc.wantResume, host.resumes)
			require.Equal(t, tc.wantLocal, host.localResponses)
			if tc.wantLocal != 0 {
				require.Equal(t, uint64(200), host.localStatus)
				require.Equal(t, "response-cache.hit", host.localDetail)
				require.Equal(t, "cached value", host.localBody)
				require.Equal(t, "hit", host.localHeaders["x-cache-status"])
				require.Equal(t, "application/json", host.localHeaders["content-type"])
			}
			// All header-key paths disable body processing, including synchronous
			// dispatch failure. A JSON body must not start a second cache lookup.
			before := host.dispatches
			invoke("proxy_on_request_body", 2, 20, 1)
			require.Equal(t, before, host.dispatches)
			require.Zero(t, host.requestBodyReads)
			require.Equal(t, tc.wantResume, host.resumes)
		})
	}
}

type headerLookupHost struct {
	t                *testing.T
	config           []byte
	headers          map[string]string
	properties       map[string]string
	reply            []byte
	dispatchStatus   uint64
	dispatches       int
	resumes          int
	localResponses   int
	requestBodyReads int
	redisCluster     string
	localStatus      uint64
	localDetail      string
	localBody        string
	localHeaders     map[string]string
	effects          []string
}

func (h *headerLookupHost) call(ctx context.Context, mod api.Module, name string, p []uint64) uint64 {
	read := func(ptr, size uint64) string {
		data, ok := mod.Memory().Read(uint32(ptr), uint32(size))
		require.True(h.t, ok)
		return string(data)
	}
	write := func(data string, ptr, size uint64) {
		address := uint64(0)
		if len(data) != 0 {
			allocated, err := mod.ExportedFunction("proxy_on_memory_allocate").Call(ctx, uint64(len(data)))
			require.NoError(h.t, err)
			address = allocated[0]
			require.True(h.t, mod.Memory().Write(uint32(address), []byte(data)))
		}
		require.True(h.t, mod.Memory().WriteUint32Le(uint32(ptr), uint32(address)))
		require.True(h.t, mod.Memory().WriteUint32Le(uint32(size), uint32(len(data))))
	}
	switch name {
	case "proxy_log":
		message := read(p[1], p[2])
		require.NotContains(h.t, message, "panic")
	case "proxy_redis_init", "proxy_set_effective_context":
		// No network connection is needed by this focused ABI host.
	case "proxy_get_buffer_bytes":
		var data []byte
		switch p[0] {
		case 7: // plugin configuration
			data = h.config
		case 9: // Redis callback response
			data = h.reply
		case 0: // request body; must not be requested in header-key mode
			h.requestBodyReads++
		default:
			h.t.Fatalf("unexpected buffer type %d", p[0])
		}
		start := int(p[1])
		require.LessOrEqual(h.t, start, len(data))
		end := min(start+int(p[2]), len(data))
		write(string(data[start:end]), p[3], p[4])
	case "proxy_get_property":
		value, ok := h.properties[read(p[0], p[1])]
		if !ok {
			return 1 // NotFound
		}
		write(value, p[2], p[3])
	case "proxy_set_property":
		key, value := read(p[0], p[1]), read(p[2], p[3])
		h.properties[key] = value
		if key == "clear_route_cache" {
			h.effects = append(h.effects, "disable-reroute")
		}
	case "proxy_get_header_map_value":
		require.Zero(h.t, p[0]) // request headers
		value, ok := h.headers[strings.ToLower(read(p[1], p[2]))]
		if !ok {
			return 1
		}
		write(value, p[3], p[4])
	case "proxy_remove_header_map_value":
		require.Zero(h.t, p[0])
		key := strings.ToLower(read(p[1], p[2]))
		require.Equal(h.t, "accept-encoding", key)
		delete(h.headers, key)
		h.effects = append(h.effects, "remove-accept-encoding")
	case "proxy_redis_call":
		h.dispatches++
		h.redisCluster = read(p[0], p[1])
		h.effects = append(h.effects, "dispatch")
		if h.dispatchStatus != 0 {
			return h.dispatchStatus
		}
		require.True(h.t, mod.Memory().WriteUint32Le(uint32(p[4]), 1))
	case "proxy_continue_stream":
		require.Zero(h.t, p[0]) // request stream
		h.resumes++
	case "proxy_send_local_response":
		h.localResponses++
		h.localStatus = p[0]
		h.localDetail = read(p[1], p[2])
		h.localBody = read(p[3], p[4])
		h.localHeaders = map[string]string{}
		data := []byte(read(p[5], p[6]))
		count := int(binary.LittleEndian.Uint32(data))
		offset := 4 + count*8
		for i := 0; i < count; i++ {
			keySize := int(binary.LittleEndian.Uint32(data[4+i*8:]))
			valueSize := int(binary.LittleEndian.Uint32(data[8+i*8:]))
			key := string(data[offset : offset+keySize])
			offset += keySize + 1
			h.localHeaders[key] = string(data[offset : offset+valueSize])
			offset += valueSize + 1
		}
	default:
		h.t.Fatalf("unexpected hostcall %s", name)
	}
	return 0
}
