# Redis Client Reference

## Initialization

```go
type MyConfig struct {
    redis wrapper.RedisClient
    qpm   int
}

func parseConfig(json gjson.Result, config *MyConfig) error {
    serviceName := json.Get("serviceName").String()
    servicePort := json.Get("servicePort").Int()
    if servicePort == 0 {
        servicePort = 6379
    }
    
    config.redis = wrapper.NewRedisClusterClient(wrapper.FQDNCluster{
        FQDN: serviceName,
        Port: servicePort,
    })
    
    return config.redis.Init(
        json.Get("username").String(),
        json.Get("password").String(),
        json.Get("timeout").Int(), // milliseconds
        // Optional settings:
        // wrapper.WithDataBase(1),
        // wrapper.WithBufferFlushTimeout(3*time.Millisecond),
        // wrapper.WithMaxBufferSizeBeforeFlush(1024),
        // wrapper.WithDisableBuffer(), // For latency-sensitive scenarios
    )
}
```

## Callback Signature

```go
func(response resp.Value)

// Check for errors
if response.Error() != nil {
    // Handle error
}

// Get values
response.Integer()   // int
response.String()    // string
response.Bool()      // bool
response.Array()     // []resp.Value
response.Bytes()     // []byte
```

## Available Commands

### Key Operations

```go
redis.Del(key, callback)
redis.Exists(key, callback)
redis.Expire(key, ttlSeconds, callback)
redis.Persist(key, callback)
```

### String Operations

```go
redis.Get(key, callback)
redis.Set(key, value, callback)
redis.SetEx(key, value, ttlSeconds, callback)
redis.SetNX(key, value, ttlSeconds, callback)  // ttl=0 means no expiry
redis.MGet(keys, callback)
redis.MSet(kvMap, callback)
redis.Incr(key, callback)
redis.Decr(key, callback)
redis.IncrBy(key, delta, callback)
redis.DecrBy(key, delta, callback)
```

### List Operations

```go
redis.LLen(key, callback)
redis.RPush(key, values, callback)
redis.RPop(key, callback)
redis.LPush(key, values, callback)
redis.LPop(key, callback)
redis.LIndex(key, index, callback)
redis.LRange(key, start, stop, callback)
redis.LRem(key, count, value, callback)
redis.LInsertBefore(key, pivot, value, callback)
redis.LInsertAfter(key, pivot, value, callback)
```

### Hash Operations

```go
redis.HExists(key, field, callback)
redis.HDel(key, fields, callback)
redis.HLen(key, callback)
redis.HGet(key, field, callback)
redis.HSet(key, field, value, callback)
redis.HMGet(key, fields, callback)
redis.HMSet(key, kvMap, callback)
redis.HKeys(key, callback)
redis.HVals(key, callback)
redis.HGetAll(key, callback)
redis.HIncrBy(key, field, delta, callback)
redis.HIncrByFloat(key, field, delta, callback)
```

### Set Operations

```go
redis.SCard(key, callback)
redis.SAdd(key, values, callback)
redis.SRem(key, values, callback)
redis.SIsMember(key, value, callback)
redis.SMembers(key, callback)
redis.SDiff(key1, key2, callback)
redis.SDiffStore(dest, key1, key2, callback)
redis.SInter(key1, key2, callback)
redis.SInterStore(dest, key1, key2, callback)
redis.SUnion(key1, key2, callback)
redis.SUnionStore(dest, key1, key2, callback)
```

### Sorted Set Operations

```go
redis.ZCard(key, callback)
redis.ZAdd(key, memberScoreMap, callback)
redis.ZCount(key, min, max, callback)
redis.ZIncrBy(key, member, delta, callback)
redis.ZScore(key, member, callback)
redis.ZRank(key, member, callback)
redis.ZRevRank(key, member, callback)
redis.ZRem(key, members, callback)
redis.ZRange(key, start, stop, callback)
redis.ZRevRange(key, start, stop, callback)
```

### Lua Script

```go
redis.Eval(script, numkeys, keys, args, callback)
```

### Raw Command

```go
redis.Command([]interface{}{"SET", "key", "value"}, callback)
```

## Rate Limiting Example

```go
func onHttpRequestHeaders(ctx wrapper.HttpContext, config MyConfig) types.Action {
    now := time.Now()
    minuteAligned := now.Truncate(time.Minute)
    timeStamp := strconv.FormatInt(minuteAligned.Unix(), 10)
    
    err := config.redis.Incr(timeStamp, func(response resp.Value) {
        if response.Error() != nil {
            log.Errorf("redis error: %v", response.Error())
            proxywasm.ResumeHttpRequest()
            return
        }
        
        count := response.Integer()
        ctx.SetContext("timeStamp", timeStamp)
        ctx.SetContext("callTimeLeft", strconv.Itoa(config.qpm - count))
        
        if count == 1 {
            // First request in this minute, set expiry
            config.redis.Expire(timeStamp, 60, func(response resp.Value) {
                if response.Error() != nil {
                    log.Errorf("expire error: %v", response.Error())
                }
                proxywasm.ResumeHttpRequest()
            })
        } else if count > config.qpm {
            proxywasm.SendHttpResponse(429, [][2]string{
                {"timeStamp", timeStamp},
                {"callTimeLeft", "0"},
            }, []byte("Too many requests\n"), -1)
        } else {
            proxywasm.ResumeHttpRequest()
        }
    })
    
    if err != nil {
        log.Errorf("redis call failed: %v", err)
        return types.HeaderContinue
    }
    return types.HeaderStopAllIterationAndWatermark
}

func onHttpResponseHeaders(ctx wrapper.HttpContext, config MyConfig) types.Action {
    if ts := ctx.GetContext("timeStamp"); ts != nil {
        proxywasm.AddHttpResponseHeader("timeStamp", ts.(string))
    }
    if left := ctx.GetContext("callTimeLeft"); left != nil {
        proxywasm.AddHttpResponseHeader("callTimeLeft", left.(string))
    }
    return types.HeaderContinue
}
```

## Important Notes

1. **Check Ready()** - `redis.Ready()` returns false if init failed
2. **Auto-reconnect** - Client handles NOAUTH errors and re-authenticates automatically
3. **Buffering** - Default 3ms flush timeout and 1024 byte buffer; use `WithDisableBuffer()` for latency-sensitive scenarios
4. **Error handling** - Always check `response.Error()` in callbacks
