package prefix_cache

import (
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"fmt"
	"math/rand"
	"strings"

	"github.com/alibaba/higress/plugins/wasm-go/extensions/ai-load-balancer/utils"

	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm"
	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm/types"
	"github.com/higress-group/wasm-go/pkg/log"
	"github.com/higress-group/wasm-go/pkg/wrapper"
	"github.com/tidwall/gjson"
	"github.com/tidwall/resp"
)

const (
	RedisKeyFormat = "higress:global_least_request_table:%s:%s"
	RedisLua       = `-- hex string => bytes
local function hex_to_bytes(hex)
    local bytes = {}
    for i = 1, #hex, 2 do
        local byte_str = hex:sub(i, i+1)
        local byte_val = tonumber(byte_str, 16)
        table.insert(bytes, byte_val)
    end
    return bytes
end

-- bytes => hex string
local function bytes_to_hex(bytes)
    local result = ""
    for _, byte in ipairs(bytes) do
        result = result .. string.format("%02X", byte)
    end
    return result
end

-- byte XOR
local function byte_xor(a, b)
    local result = 0
    for i = 0, 7 do
        local bit_val = 2^i
        if ((a % (bit_val * 2)) >= bit_val) ~= ((b % (bit_val * 2)) >= bit_val) then
            result = result + bit_val
        end
    end
    return result
end

-- hex string XOR
local function hex_xor(a, b)
    if #a ~= #b then
        error("Hex strings must be of equal length, first is " .. a .. " second is " .. b)
    end

    local a_bytes = hex_to_bytes(a)
    local b_bytes = hex_to_bytes(b)

    local result_bytes = {}
    for i = 1, #a_bytes do
        table.insert(result_bytes, byte_xor(a_bytes[i], b_bytes[i]))
    end

    return bytes_to_hex(result_bytes)
end

-- check host whether healthy
local function is_healthy(addr)
    for i = 4, #KEYS do
        if addr == KEYS[i] then
            return true
        end
    end
    return false
end

local function randomBool()
    return math.random() >= 0.5
end

local target = ""
local key = ""
local current_key = ""
local ttl = KEYS[1]
local hset_key = KEYS[2]
local default_target = KEYS[3]

-- find longest prefix
local index = 1
while index <= #ARGV do
    if current_key == "" then
        current_key = ARGV[index]
    else
        current_key = hex_xor(current_key, ARGV[index])
    end
    if redis.call("EXISTS", current_key) == 1 then
        key = current_key
        local tmp_target = redis.call("GET", key)
		if not is_healthy(tmp_target) then
			break
		end
		target = tmp_target
        -- update ttl for exist keys
        redis.call("EXPIRE", key, ttl)
        index = index + 1
    else
        break
    end
end


-- global least request
if target == "" then
	index = 1
	local current_count = 0
	target = default_target
	if redis.call('HEXISTS', hset_key, target) == 1 then
		current_count = redis.call('HGET', hset_key, target)
		for i = 4, #KEYS do
			if redis.call('HEXISTS', hset_key, KEYS[i]) == 1 then
				local count = redis.call('HGET', hset_key, KEYS[i])
				if tonumber(count) < tonumber(current_count) then
					target = KEYS[i]
					current_count = count
				elseif count == current_count and randomBool() then
					target = KEYS[i]
				end
			end
		end
	end
end

-- update request count
redis.call("HINCRBY", hset_key, target, 1)

-- add tree-path
while index <= #ARGV do
    if key == "" then
        key = ARGV[index]
    else
        key = hex_xor(key, ARGV[index])
    end
    redis.call("SET", key, target)
    redis.call("EXPIRE", key, ttl)
    index = index + 1
end

return target`
)

type PrefixCacheLoadBalancer struct {
	redisClient wrapper.RedisClient
	redisKeyTTL int
}

func NewPrefixCacheLoadBalancer(json gjson.Result) (PrefixCacheLoadBalancer, error) {
	lb := PrefixCacheLoadBalancer{}
	serviceFQDN := json.Get("serviceFQDN").String()
	servicePort := json.Get("servicePort").Int()
	if serviceFQDN == "" || servicePort == 0 {
		log.Errorf("invalid redis service, serviceFQDN: %s, servicePort: %d", serviceFQDN, servicePort)
		return lb, errors.New("invalid redis service config")
	}
	lb.redisClient = wrapper.NewRedisClusterClient(wrapper.FQDNCluster{
		FQDN: serviceFQDN,
		Port: servicePort,
	})
	username := json.Get("username").String()
	password := json.Get("password").String()
	timeout := json.Get("timeout").Int()
	if timeout == 0 {
		timeout = 3000
	}
	// database default is 0
	database := json.Get("database").Int()
	if json.Get("redisKeyTTL").Int() != 0 {
		lb.redisKeyTTL = int(json.Get("redisKeyTTL").Int())
	} else {
		lb.redisKeyTTL = 1800
	}
	return lb, lb.redisClient.Init(username, password, int64(timeout), wrapper.WithDataBase(int(database)))
}

func (lb PrefixCacheLoadBalancer) HandleHttpRequestHeaders(ctx wrapper.HttpContext) types.Action {
	if !ctx.HasRequestBody() {
		return types.ActionContinue
	}
	// If return types.ActionContinue, SetUpstreamOverrideHost will not take effect
	return types.HeaderStopIteration
}

func (lb PrefixCacheLoadBalancer) HandleHttpRequestBody(ctx wrapper.HttpContext, body []byte) types.Action {
	var err error
	routeName, err := utils.GetRouteName()
	if err != nil || routeName == "" {
		ctx.SetContext("error", true)
		return types.ActionContinue
	} else {
		ctx.SetContext("routeName", routeName)
	}
	clusterName, err := utils.GetClusterName()
	if err != nil || clusterName == "" {
		ctx.SetContext("error", true)
		return types.ActionContinue
	} else {
		ctx.SetContext("clusterName", clusterName)
	}
	hostInfos, err := proxywasm.GetUpstreamHosts()
	if err != nil {
		ctx.SetContext("error", true)
		log.Error("get upstream cluster endpoints failed")
		return types.ActionContinue
	}
	healthyHosts := []string{}
	for _, hostInfo := range hostInfos {
		if gjson.Get(hostInfo[1], "health_status").String() == "Healthy" {
			healthyHosts = append(healthyHosts, hostInfo[0])
		}
	}
	if len(healthyHosts) == 0 {
		log.Info("upstream cluster has no healthy endpoints")
		return types.ActionContinue
	}
	defaultHost := healthyHosts[rand.Intn(len(healthyHosts))]
	params := []interface{}{}
	rawStr := ""
	messages := gjson.GetBytes(body, "messages").Array()
	for index, obj := range messages {
		if !obj.Get("role").Exists() || !obj.Get("content").Exists() {
			ctx.SetContext("error", true)
			log.Info("cannot extract role or content from request body, skip llm load balancing")
			return types.ActionContinue
		}
		role := obj.Get("role").String()
		content := obj.Get("content").String()
		rawStr += role + ":" + content
		if role == "user" || index == len(messages)-1 {
			sha1Str := computeSHA1(rawStr)
			params = append(params, sha1Str)
			rawStr = ""
		}
	}
	if len(params) == 0 {
		return types.ActionContinue
	}
	keys := []interface{}{lb.redisKeyTTL, fmt.Sprintf(RedisKeyFormat, routeName, clusterName), defaultHost}
	for _, v := range healthyHosts {
		keys = append(keys, v)
	}
	err = lb.redisClient.Eval(RedisLua, len(keys), keys, params, func(response resp.Value) {
		defer proxywasm.ResumeHttpRequest()
		if err := response.Error(); err != nil {
			ctx.SetContext("error", true)
			log.Errorf("Redis eval failed: %+v", err)
			return
		}
		hostSelected := response.String()
		if err := proxywasm.SetUpstreamOverrideHost([]byte(hostSelected)); err != nil {
			ctx.SetContext("error", true)
			log.Errorf("override upstream host failed, fallback to default lb policy, error informations: %+v", err)
		}
		log.Debugf("host_selected: %s", hostSelected)
		ctx.SetContext("host_selected", hostSelected)
	})
	if err != nil {
		ctx.SetContext("error", true)
		return types.ActionContinue
	}
	return types.ActionPause
}

func (lb PrefixCacheLoadBalancer) HandleHttpResponseHeaders(ctx wrapper.HttpContext) types.Action {
	return types.ActionContinue
}

func (lb PrefixCacheLoadBalancer) HandleHttpStreamingResponseBody(ctx wrapper.HttpContext, data []byte, endOfStream bool) []byte {
	return data
}

func (lb PrefixCacheLoadBalancer) HandleHttpResponseBody(ctx wrapper.HttpContext, body []byte) types.Action {
	return types.ActionContinue
}

func (lb PrefixCacheLoadBalancer) HandleHttpStreamDone(ctx wrapper.HttpContext) {
	isErr, _ := ctx.GetContext("error").(bool)
	if !isErr {
		routeName, _ := ctx.GetContext("routeName").(string)
		clusterName, _ := ctx.GetContext("clusterName").(string)
		host_selected, _ := ctx.GetContext("host_selected").(string)
		if host_selected != "" {
			lb.redisClient.HIncrBy(fmt.Sprintf(RedisKeyFormat, routeName, clusterName), host_selected, -1, nil)
		}
	}
}

func computeSHA1(data string) string {
	hasher := sha1.New()
	hasher.Write([]byte(data))
	return strings.ToUpper(hex.EncodeToString(hasher.Sum(nil)))
}
