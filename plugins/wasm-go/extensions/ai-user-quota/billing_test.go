// Copyright (c) 2024-2025
// Beijing Baolande Software Corporation
// All Rights Reserved

package main

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func utcDate(year int, month time.Month, day, hour, minute, second int) time.Time {
	return time.Date(year, month, day, hour, minute, second, 0, time.UTC)
}

func TestCalcCycle(t *testing.T) {
	cases := []struct {
		name          string
		anchor        time.Time
		now           time.Time
		wantCycle     time.Time
		wantNextCycle time.Time
	}{
		{
			name:          "month end stays in anchor cycle",
			anchor:        utcDate(2026, 1, 31, 0, 0, 0),
			now:           utcDate(2026, 1, 31, 0, 0, 0),
			wantCycle:     utcDate(2026, 1, 31, 0, 0, 0),
			wantNextCycle: utcDate(2026, 2, 28, 0, 0, 0),
		},
		{
			name:          "last second before cycle end",
			anchor:        utcDate(2026, 1, 31, 0, 0, 0),
			now:           utcDate(2026, 2, 27, 23, 59, 59),
			wantCycle:     utcDate(2026, 1, 31, 0, 0, 0),
			wantNextCycle: utcDate(2026, 2, 28, 0, 0, 0),
		},
		{
			name:          "clamped cycle boundary equals new cycle",
			anchor:        utcDate(2026, 1, 31, 0, 0, 0),
			now:           utcDate(2026, 2, 28, 0, 0, 0),
			wantCycle:     utcDate(2026, 2, 28, 0, 0, 0),
			wantNextCycle: utcDate(2026, 3, 31, 0, 0, 0),
		},
		{
			name:          "middle of clamped cycle",
			anchor:        utcDate(2026, 1, 31, 0, 0, 0),
			now:           utcDate(2026, 3, 30, 12, 0, 0),
			wantCycle:     utcDate(2026, 2, 28, 0, 0, 0),
			wantNextCycle: utcDate(2026, 3, 31, 0, 0, 0),
		},
		{
			name:          "month with 31 days",
			anchor:        utcDate(2026, 1, 31, 0, 0, 0),
			now:           utcDate(2026, 3, 31, 0, 0, 0),
			wantCycle:     utcDate(2026, 3, 31, 0, 0, 0),
			wantNextCycle: utcDate(2026, 4, 30, 0, 0, 0),
		},
		{
			name:          "month with 30 days",
			anchor:        utcDate(2026, 1, 31, 0, 0, 0),
			now:           utcDate(2026, 4, 30, 0, 0, 0),
			wantCycle:     utcDate(2026, 4, 30, 0, 0, 0),
			wantNextCycle: utcDate(2026, 5, 31, 0, 0, 0),
		},
		{
			name:          "year crossing boundary",
			anchor:        utcDate(2026, 1, 15, 0, 0, 0),
			now:           utcDate(2026, 12, 15, 0, 0, 0),
			wantCycle:     utcDate(2026, 12, 15, 0, 0, 0),
			wantNextCycle: utcDate(2027, 1, 15, 0, 0, 0),
		},
		{
			name:          "last second before new year cycle",
			anchor:        utcDate(2026, 1, 15, 0, 0, 0),
			now:           utcDate(2027, 1, 14, 23, 59, 59),
			wantCycle:     utcDate(2026, 12, 15, 0, 0, 0),
			wantNextCycle: utcDate(2027, 1, 15, 0, 0, 0),
		},
		{
			name:          "new cycle across year",
			anchor:        utcDate(2026, 1, 15, 0, 0, 0),
			now:           utcDate(2027, 1, 15, 0, 0, 0),
			wantCycle:     utcDate(2027, 1, 15, 0, 0, 0),
			wantNextCycle: utcDate(2027, 2, 15, 0, 0, 0),
		},
		{
			name:          "31st day clamped to 30 day month",
			anchor:        utcDate(2026, 8, 31, 0, 0, 0),
			now:           utcDate(2026, 9, 30, 0, 0, 0),
			wantCycle:     utcDate(2026, 9, 30, 0, 0, 0),
			wantNextCycle: utcDate(2026, 10, 31, 0, 0, 0),
		},
		{
			name:          "last second before seventh day cycle",
			anchor:        utcDate(2026, 11, 7, 0, 0, 0),
			now:           utcDate(2026, 12, 6, 23, 59, 59),
			wantCycle:     utcDate(2026, 11, 7, 0, 0, 0),
			wantNextCycle: utcDate(2026, 12, 7, 0, 0, 0),
		},
		{
			name:          "seventh day starts new cycle",
			anchor:        utcDate(2026, 11, 7, 0, 0, 0),
			now:           utcDate(2026, 12, 7, 0, 0, 0),
			wantCycle:     utcDate(2026, 12, 7, 0, 0, 0),
			wantNextCycle: utcDate(2027, 1, 7, 0, 0, 0),
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			cycleStart, nextCycleStart := calcCycle(c.anchor, c.now)
			assert.Equal(t, c.wantCycle, cycleStart)
			assert.Equal(t, c.wantNextCycle, nextCycleStart)
		})
	}
}

// 月末 clamp 每次从 anchor 独立计算,不链式递推
func TestCalcCycleNotChained(t *testing.T) {
	anchor := utcDate(2026, 2, 28, 0, 0, 0)
	_, nextCycleStart := calcCycle(anchor, anchor)
	assert.Equal(t, utcDate(2026, 3, 28, 0, 0, 0), nextCycleStart)
}

func TestCycleTTLSeconds(t *testing.T) {
	anchor := utcDate(2026, 1, 31, 0, 0, 0)
	now := utcDate(2026, 2, 27, 12, 0, 0)
	// 距下周期零点 12 小时,加两天宽限
	assert.Equal(t, int64(43200+ttlGraceSeconds), cycleTTLSeconds(anchor, now))
}

func TestRedisKeys(t *testing.T) {
	cycleStart := utcDate(2026, 1, 31, 0, 0, 0)
	assert.Equal(t, "user_token_quota:{u-1}:cfg", cfgKey("u-1"))
	assert.Equal(t, "user_token_quota:{u-1}:used:2:2026-01-31", usedKey("u-1", 2, cycleStart))
	assert.Equal(t, "user_token_quota:dirty:{snapshot}:current", dirtyCurrentKey())
	assert.Equal(t,
		`{"user_id":"u-1","usage_epoch":2,"cycle_start":"2026-01-31"}`,
		dirtyMember("u-1", 2, cycleStart))
}
