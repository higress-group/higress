// Repro plugin for higress issue #4034: worker CPU spin in
// WasmBase::doAfterVmCallActions when a deferred async callback synchronously
// calls injectEncodedDataToFilterChain (nesting SaveRestoreContext) so that the
// after-vm-call action re-queues itself forever.
//
// Repro shape (dead-Redis only, self-contained — no external HTTP dependency):
//   1. In the response-header phase, pause the response.
//   2. Fire N Redis commands against an UNREACHABLE redis cluster.
//   3. Each failure callback (onRedisCallFailure) synchronously calls the
//      inject_encoded_data_to_filter_chain foreign function. Because sibling
//      failure callbacks are still queued as after-vm-call actions, the nested
//      SaveRestoreContext leaves current_context_ != nullptr and the deferred
//      action re-queues itself -> CPU spin on the pre-fix host.
//
// With the Layer A drain-to-local fix in place, the drain terminates over a
// snapshot and worker CPU stays bounded.
package main

import (
	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm"
	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm/types"
	"github.com/higress-group/wasm-go/pkg/log"
	pb "github.com/higress-group/wasm-go/pkg/protos"
	"github.com/higress-group/wasm-go/pkg/wrapper"
	"github.com/tidwall/gjson"
	"github.com/tidwall/resp"
	"google.golang.org/protobuf/proto"
)

func main() {}

type Config struct {
	redisClient *wrapper.RedisClusterClient[wrapper.FQDNCluster]
	redisKey    string
	injectBody  string
	injectCount int
}

func init() {
	wrapper.SetCtx(
		"test-redis-inject-spin",
		wrapper.ParseConfig(parseConfig),
		wrapper.ProcessResponseHeaders(onHttpResponseHeaders),
	)
}

func parseConfig(json gjson.Result, config *Config) error {
	serviceName := json.Get("service_name").String()
	if serviceName == "" {
		serviceName = "dead-redis.dead-redis.svc.cluster.local"
	}
	servicePort := json.Get("service_port").Int()
	if servicePort == 0 {
		servicePort = 6379
	}
	config.redisKey = json.Get("redis_key").String()
	if config.redisKey == "" {
		config.redisKey = "higress-4034-key"
	}
	config.injectBody = json.Get("inject_body").String()
	if config.injectBody == "" {
		config.injectBody = "injected-by-4034-repro\n"
	}
	config.injectCount = int(json.Get("inject_count").Int())
	if config.injectCount == 0 {
		config.injectCount = 16
	}

	config.redisClient = wrapper.NewRedisClusterClient(wrapper.FQDNCluster{
		FQDN: serviceName,
		Port: servicePort,
	})
	// Init never returns error for an unreachable host; commands' callbacks will
	// fire with an error value, which is exactly the #4034 trigger path.
	return config.redisClient.Init("", "", 1000)
}

func onHttpResponseHeaders(ctx wrapper.HttpContext, config Config) types.Action {
	proxywasm.RemoveHttpResponseHeader("content-length")
	ctx.DontReadResponseBody()

	inject := func() {
		d := &pb.InjectEncodedDataToFilterChainArguments{
			Body:      config.injectBody,
			Endstream: true,
		}
		s, err := proto.Marshal(d)
		if err != nil {
			log.Errorf("marshal inject args failed: %+v", err)
			return
		}
		if _, err := proxywasm.CallForeignFunction("inject_encoded_data_to_filter_chain_on_header", s); err != nil {
			log.Errorf("call inject_encoded_data_to_filter_chain_on_header failed: %+v", err)
		}
	}

	scheduled := 0
	for i := 0; i < config.injectCount; i++ {
		err := config.redisClient.Get(config.redisKey, func(response resp.Value) {
			// Fires on redis-unreachable failure; synchronously inject to nest
			// SaveRestoreContext while sibling callbacks are still queued.
			if response.Error() != nil {
				log.Debugf("redis get failed as expected: %v", response.Error())
			}
			inject()
		})
		if err != nil {
			log.Errorf("redis Get dispatch failed: %+v", err)
			continue
		}
		scheduled++
	}

	if scheduled == 0 {
		// Nothing dispatched (e.g. cluster missing) — don't hang the response.
		return types.ActionContinue
	}
	return types.ActionPause
}
