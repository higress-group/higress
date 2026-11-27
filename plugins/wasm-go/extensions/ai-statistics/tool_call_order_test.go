// Copyright (c) 2024 Alibaba Group Holding Ltd.
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

import (
	"reflect"
	"testing"
)

func TestToolCallSparseOrder(t *testing.T) {
	buffer := &StreamingToolCallsBuffer{ToolCalls: map[int]*ToolCall{10000000: {Index: 10000000}, 2: {Index: 2}, 0: {Index: 0}, -1: {Index: -1}}}
	got := getToolCallsFromBuffer(buffer)
	indexes := make([]int, len(got))
	for i, tc := range got {
		indexes[i] = tc.Index
	}
	if !reflect.DeepEqual(indexes, []int{0, 2, 10000000}) {
		t.Fatal(indexes)
	}
	if getToolCallsFromBuffer(nil) != nil || getToolCallsFromBuffer(&StreamingToolCallsBuffer{}) != nil {
		t.Fatal("empty buffer changed")
	}
}

var benchmarkToolCalls []ToolCall

func BenchmarkToolCallSparseOrder(b *testing.B) {
	for _, index := range []int{0, 10000000} {
		buffer := &StreamingToolCallsBuffer{ToolCalls: map[int]*ToolCall{index: {Index: index}}}
		b.Run(map[bool]string{true: "sparse", false: "dense"}[index > 0], func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				benchmarkToolCalls = getToolCallsFromBuffer(buffer)
			}
		})
	}
}

func TestToolCallMaxIndex(t *testing.T) {
	index := int(^uint(0) >> 1)
	got := getToolCallsFromBuffer(&StreamingToolCallsBuffer{ToolCalls: map[int]*ToolCall{index: {Index: index}}})
	if len(got) != 1 || got[0].Index != index {
		t.Fatal(got)
	}
}
