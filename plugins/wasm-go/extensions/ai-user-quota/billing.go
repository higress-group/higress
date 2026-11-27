// Copyright (c) 2024-2025
// Beijing Baolande Software Corporation
// All Rights Reserved

package main

import (
	"encoding/json"
	"strconv"
	"time"
)

const (
	// utc8Offset 固定偏移,不依赖进程时区
	utc8Offset = 8 * time.Hour
	// ttlGraceSeconds 旧周期计数键的宽限时间
	ttlGraceSeconds = int64(172800)
	// redisKeyPrefix 配额相关键统一前缀
	redisKeyPrefix = "user_token_quota"
)

// nowUTC8 取 UTC+8 时间
func nowUTC8() time.Time {
	return time.Now().UTC().Add(utc8Offset)
}

func daysInMonth(year int, month time.Month) int {
	return time.Date(year, month+1, 0, 0, 0, 0, 0, time.UTC).Day()
}

// addMonthsClamped anchor 加 months 个月,目标月天数不足时取目标月最后一天
func addMonthsClamped(anchor time.Time, months int) time.Time {
	total := anchor.Year()*12 + int(anchor.Month()) - 1 + months
	year := total / 12
	month := time.Month(total%12 + 1)
	day := anchor.Day()
	if last := daysInMonth(year, month); day > last {
		day = last
	}
	return time.Date(year, month, day, 0, 0, 0, 0, time.UTC)
}

// calcCycle 计算 now 所在的滚动月周期,返回 [cycleStart, nextCycleStart)
// 每次从 anchor 独立推算,不链式递推;周期起点等值归新周期
func calcCycle(anchor, now time.Time) (time.Time, time.Time) {
	anchor = time.Date(anchor.Year(), anchor.Month(), anchor.Day(), 0, 0, 0, 0, time.UTC)
	now = time.Date(now.Year(), now.Month(), now.Day(), now.Hour(), now.Minute(), now.Second(), now.Nanosecond(), time.UTC)
	k := (now.Year()-anchor.Year())*12 + int(now.Month()) - int(anchor.Month())
	candidate := addMonthsClamped(anchor, k)
	if now.Before(candidate) {
		k--
	}
	return addMonthsClamped(anchor, k), addMonthsClamped(anchor, k+1)
}

// cycleTTLSeconds 当前周期键 TTL:距下周期零点的秒数加宽限
func cycleTTLSeconds(anchor, now time.Time) int64 {
	_, next := calcCycle(anchor, now)
	return int64((next.Sub(now)) / time.Second) + ttlGraceSeconds
}

func cfgKey(userID string) string {
	return redisKeyPrefix + ":{" + userID + "}:cfg"
}

func usedKey(userID string, epoch int64, cycleStart time.Time) string {
	return redisKeyPrefix + ":{" + userID + "}:used:" + strconv.FormatInt(epoch, 10) + ":" + cycleStart.Format("2006-01-02")
}

// dirtyCurrentKey 待同步集合键
func dirtyCurrentKey() string {
	return redisKeyPrefix + ":dirty:{snapshot}:current"
}

// dirtyMember 待同步成员,与平台侧快照任务解析格式一致
func dirtyMember(userID string, epoch int64, cycleStart time.Time) string {
	member := struct {
		UserID     string `json:"user_id"`
		UsageEpoch int64  `json:"usage_epoch"`
		CycleStart string `json:"cycle_start"`
	}{
		UserID:     userID,
		UsageEpoch: epoch,
		CycleStart: cycleStart.Format("2006-01-02"),
	}
	data, _ := json.Marshal(member)
	return string(data)
}