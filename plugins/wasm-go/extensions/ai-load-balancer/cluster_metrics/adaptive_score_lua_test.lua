-- Mock Redis environment for AdaptiveScore global in-flight Lua.
local redis_data = {
    hset = {},
    ttl = {}
}

local redis = {
    call = function(cmd, ...)
        local args = {...}
        if cmd == "HGET" then
            local field = args[2]
            return redis_data.hset[field]
        elseif cmd == "HINCRBY" then
            local field, increment = args[2], tonumber(args[3])
            local val = tonumber(redis_data.hset[field] or 0)
            redis_data.hset[field] = tostring(val + increment)
            return redis_data.hset[field]
        elseif cmd == "EXPIRE" then
            local key, ttl = args[1], tonumber(args[2])
            redis_data.ttl[key] = ttl
        end
    end
}

local function run_adaptive_score_global_inflight(KEYS)
    local seed = tonumber(KEYS[1])
    local hset_key = KEYS[2]
    local service_count = tonumber(KEYS[3])
    local ttl = tonumber(KEYS[4])

    math.randomseed(seed)

    local selected = ""
    local selected_score = 0
    local same_score_hits = 0

    for i = 1, service_count do
        local offset = 4 + (i - 1) * 2
        local service = KEYS[offset + 1]
        local local_score = tonumber(KEYS[offset + 2])
        local inflight = 0
        local val = redis.call("HGET", hset_key, service)
        if val then
            inflight = tonumber(val) or 0
        end
        local score = local_score * (inflight + 1)

        if same_score_hits == 0 or score < selected_score then
            selected = service
            selected_score = score
            same_score_hits = 1
        elseif score == selected_score then
            same_score_hits = same_score_hits + 1
            if math.random(same_score_hits) == 1 then
                selected = service
                selected_score = score
            end
        end
    end

    redis.call("HINCRBY", hset_key, selected, 1)
    if ttl > 0 then
        redis.call("EXPIRE", hset_key, ttl)
    end
    local new_count = redis.call("HGET", hset_key, selected)
    return {selected, new_count, selected_score}
end

local function assert_equal(name, got, expected)
    if got ~= expected then
        error(string.format("%s: got %s, expected %s", name, tostring(got), tostring(expected)))
    end
end

-- svc-a has better local score, but much higher global in-flight:
-- svc-a final score = 10 * (10 + 1) = 110
-- svc-b final score = 40 * (1 + 1) = 80
redis_data.hset = {
    ["svc-a"] = "10",
    ["svc-b"] = "1",
    ["svc-old"] = "99"
}
redis_data.ttl = {}
local redis_key = "higress:adaptive_score_inflight:route-a:AdaptiveScore"
local result = run_adaptive_score_global_inflight({
    1000000,
    redis_key,
    2,
    1800,
    "svc-a", 10,
    "svc-b", 40
})
assert_equal("selected service", result[1], "svc-b")
assert_equal("incremented selected count", redis_data.hset["svc-b"], "2")
assert_equal("untouched non-selected count", redis_data.hset["svc-a"], "10")
assert_equal("non-candidate count preserved", redis_data.hset["svc-old"], "99")
assert_equal("ttl set", redis_data.ttl[redis_key], 1800)

-- Missing Redis counts default to zero and should still be incremented.
redis_data.hset = {}
result = run_adaptive_score_global_inflight({
    2000000,
    "higress:adaptive_score_inflight:route-a:AdaptiveScore",
    2,
    1800,
    "svc-a", 5,
    "svc-b", 6
})
assert_equal("selected service with missing counts", result[1], "svc-a")
assert_equal("new selected count", redis_data.hset["svc-a"], "1")

print("AdaptiveScore global in-flight Lua tests passed.")
