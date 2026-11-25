// Copyright (c) 2022 Alibaba Group Holding Ltd.
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
	"encoding/json"
	"testing"

	"github.com/higress-group/wasm-go/pkg/test"
	"github.com/stretchr/testify/require"
)

func TestJSONRenameMapPreserveValue(t *testing.T) {
	for _, value := range []string{`9007199254740993`, `-9007199254740993`, `1.234567890123456789`, `{"items":[9007199254740993]}`, `[9007199254740993,null,true,"x"]`, `"quoted\"value"`, `null`, `false`} {
		t.Run(value, func(t *testing.T) {
			test.RunGoTest(t, func(t *testing.T) {
				host, _ := test.NewTestHost(json.RawMessage(`{"reqRules":[{"operate":"remove","body":[{"key":"missing"}]}]}`))
				defer host.Reset()
				input := []byte(`{"old":` + value + `}`)
				for _, operation := range []kvtOperation{
					{kvtOpType: RenameK, renameKvtGroup: []renameKvt{{oldKey: "old", newKey: "new"}}},
					{kvtOpType: MapK, mapSource: "body", mapKvtGroup: []mapKvt{{fromKey: "old", toKey: "new"}}},
				} {
					got, err := (jsonHandler{kvtOps: []kvtOperation{operation}}).handle("example.com", "/", input, map[string]MapSourceData{"body": {mapSourceType: "bodyJson", json: input}})
					require.NoError(t, err)
					var fields map[string]json.RawMessage
					require.NoError(t, json.Unmarshal(got, &fields))
					require.Equal(t, value, string(fields["new"]))
				}
			})
		})
	}
}
