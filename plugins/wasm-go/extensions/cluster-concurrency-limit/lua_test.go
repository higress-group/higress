// Copyright (c) 2026 Alibaba Group Holding Ltd.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

// Lua 脚本边界用例测试,基于 miniredis(纯内存,不依赖真实 Redis)。
// 覆盖设计文档 §8:
//  1. 恰好到限:ZADD 后 cur == concurrency → 放行;再来一个 → 拒绝
//  2. 先检后占无幻影:第 N 个维度超限拒绝后,所有维度 ZCARD 不变
//  3. 释放幂等:ZREM 后 ZCARD 回基线;重复 ZREM 无副作用
//  4. 租约过期:ZADD 后不 ZREM,推进时间后过期 member 被清理、ZCARD 回落
//  5. 崩溃恢复:模拟进程死亡(不调用 done),租约到期后新请求正常占槽
//  6. 长流超租约:member 过期后新请求可占槽,迟到 ZREM 不报错

import (
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/gomodule/redigo/redis"
	"github.com/stretchr/testify/require"
)

func setupMiniredis(t *testing.T) (*miniredis.Miniredis, redis.Conn) {
	t.Helper()
	mr, err := miniredis.Run()
	require.NoError(t, err)
	t.Cleanup(mr.Close)
	conn, err := redis.Dial("tcp", mr.Addr())
	require.NoError(t, err)
	t.Cleanup(func() { conn.Close() })
	return mr, conn
}

// acquire 执行占槽脚本,返回脚本的整数数组结果
func acquire(t *testing.T, conn redis.Conn, nowMs, leaseMs int64, member string, keys []string, limits []int64) []int64 {
	t.Helper()
	args := []interface{}{AcquireScript, len(keys)}
	for _, k := range keys {
		args = append(args, k)
	}
	args = append(args, nowMs, leaseMs, member)
	for _, l := range limits {
		args = append(args, l)
	}
	reply, err := conn.Do("EVAL", args...)
	require.NoError(t, err)
	result, err := redis.Int64s(reply, nil)
	require.NoError(t, err)
	return result
}

// release 执行释放脚本
func release(t *testing.T, conn redis.Conn, member string, keys ...string) {
	t.Helper()
	args := []interface{}{ReleaseScript, len(keys)}
	for _, k := range keys {
		args = append(args, k)
	}
	args = append(args, member)
	_, err := conn.Do("EVAL", args...)
	require.NoError(t, err)
}

func zcard(t *testing.T, conn redis.Conn, key string) int64 {
	t.Helper()
	n, err := redis.Int64(conn.Do("ZCARD", key))
	require.NoError(t, err)
	return n
}

// 1. 恰好到限:占满 cur == concurrency 放行,再来一个拒绝
func TestAcquireAtLimit(t *testing.T) {
	_, conn := setupMiniredis(t)
	key := "{test}:concurrency:global"

	// 第 1 个请求:占槽成功,占槽后计数为 1
	result := acquire(t, conn, 1000, 60000, "m1", []string{key}, []int64{1})
	require.Equal(t, []int64{0, 1}, result)

	// 第 2 个请求:cur(1) >= limit(1),拒绝,返回 {reject=1, cur=1}
	result = acquire(t, conn, 1001, 60000, "m2", []string{key}, []int64{1})
	require.Equal(t, []int64{1, 1}, result)
	require.Equal(t, int64(1), zcard(t, conn, key))
}

// 2. 先检后占无幻影:第 2 个维度超限拒绝后,所有维度 ZCARD 不变
func TestCheckBeforeOccupyNoPhantom(t *testing.T) {
	_, conn := setupMiniredis(t)
	key1 := "{test}:concurrency:global"
	key2 := "{test}:concurrency:consumer=alice"
	keys := []string{key1, key2}
	limits := []int64{10, 1}

	// 第 1 个请求:两个维度都占槽成功
	result := acquire(t, conn, 1000, 60000, "m1", keys, limits)
	require.Equal(t, []int64{0, 1, 1}, result)

	// 第 2 个请求:维度 2 超限拒绝
	result = acquire(t, conn, 1001, 60000, "m2", keys, limits)
	require.Equal(t, []int64{2, 1}, result)

	// 无幻影占槽:两个维度的计数都不变
	require.Equal(t, int64(1), zcard(t, conn, key1))
	require.Equal(t, int64(1), zcard(t, conn, key2))
}

// 3. 释放幂等:ZREM 后 ZCARD 回基线,重复 ZREM / 删不存在 member 均无副作用
func TestReleaseIdempotent(t *testing.T) {
	_, conn := setupMiniredis(t)
	key := "{test}:concurrency:global"

	acquire(t, conn, 1000, 60000, "m1", []string{key}, []int64{10})
	require.Equal(t, int64(1), zcard(t, conn, key))

	release(t, conn, "m1", key)
	require.Equal(t, int64(0), zcard(t, conn, key))

	// 重复释放、释放不存在的 member 均不报错、无副作用
	release(t, conn, "m1", key)
	release(t, conn, "not-exist", key)
	require.Equal(t, int64(0), zcard(t, conn, key))
}

// 4. 租约过期:不 ZREM,推进时间后过期 member 被惰性清理、ZCARD 回落
func TestLeaseExpiry(t *testing.T) {
	_, conn := setupMiniredis(t)
	key := "{test}:concurrency:global"

	// now=1000, lease=1000ms → member 过期时间戳 2000
	result := acquire(t, conn, 1000, 1000, "m1", []string{key}, []int64{1})
	require.Equal(t, []int64{0, 1}, result)

	// 租约内再来一个 → 拒绝
	result = acquire(t, conn, 1500, 1000, "m2", []string{key}, []int64{1})
	require.Equal(t, []int64{1, 1}, result)

	// 推进到 now=2001(超过租约):m1 被清理,新请求正常占槽
	result = acquire(t, conn, 2001, 1000, "m2", []string{key}, []int64{1})
	require.Equal(t, []int64{0, 1}, result)
	require.Equal(t, int64(1), zcard(t, conn, key))
}

// 5. 崩溃恢复:模拟进程死亡(占槽后永不释放),租约到期后新请求正常占槽
func TestCrashRecovery(t *testing.T) {
	_, conn := setupMiniredis(t)
	key := "{test}:concurrency:global"

	// 进程占槽后崩溃,done 未执行,永不 ZREM(member 过期时间戳 = 1000+60000 = 61000)
	result := acquire(t, conn, 1000, 60000, "crashed-member", []string{key}, []int64{1})
	require.Equal(t, []int64{0, 1}, result)

	// 租约到期前:拒绝
	result = acquire(t, conn, 60999, 60000, "m2", []string{key}, []int64{1})
	require.Equal(t, []int64{1, 1}, result)

	// 租约到期后:崩溃 member 被清理,新请求正常占槽
	result = acquire(t, conn, 61000, 60000, "m2", []string{key}, []int64{1})
	require.Equal(t, []int64{0, 1}, result)
}

// 6. 长流超租约:member 过期后新请求可占槽(暂时性轻微超额放行),迟到 ZREM 不报错
func TestLongStreamBeyondLease(t *testing.T) {
	_, conn := setupMiniredis(t)
	key := "{test}:concurrency:global"

	// 长流请求 m1 占槽,租约 1000ms,但流存活 5000ms
	result := acquire(t, conn, 1000, 1000, "m1", []string{key}, []int64{1})
	require.Equal(t, []int64{0, 1}, result)

	// now=3000:m1 的 member 已过期被清理,m2 可占槽(符合预期的暂时性超额放行窗口)
	result = acquire(t, conn, 3000, 1000, "m2", []string{key}, []int64{1})
	require.Equal(t, []int64{0, 1}, result)

	// now=6000:m1 的流终于结束,迟到的 ZREM 不报错,且不影响 m2
	release(t, conn, "m1", key)
	require.Equal(t, int64(1), zcard(t, conn, key))

	release(t, conn, "m2", key)
	require.Equal(t, int64(0), zcard(t, conn, key))
}

// PEXPIRE 整 key 垃圾回收:所有 member 释放后 key 随 TTL 过期,不留垃圾 key
func TestKeyGarbageCollection(t *testing.T) {
	mr, conn := setupMiniredis(t)
	key := "{test}:concurrency:global"

	acquire(t, conn, 1000, 60000, "m1", []string{key}, []int64{10})
	require.True(t, mr.Exists(key))
	require.Greater(t, mr.TTL(key), time.Duration(0))

	release(t, conn, "m1", key)
	// member 空了但 key 还在(空 zset 被 Redis 自动删除则不存在;若存在必带 TTL)
	if mr.Exists(key) {
		mr.FastForward(61 * time.Second)
		require.False(t, mr.Exists(key))
	}
}
