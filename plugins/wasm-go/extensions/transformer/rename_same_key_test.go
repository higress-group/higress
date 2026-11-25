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

func TestRenameSameKeyPreservesValue(t *testing.T) {
	test.RunGoTest(t, func(t *testing.T) {
		host, _ := test.NewTestHost(json.RawMessage(`{"reqRules":[{"operate":"remove","body":[{"key":"missing"}]}]}`))
		defer host.Reset()
		input := []byte(`{"label":{"items":[9007199254740993]},"untouched":1}`)
		got, err := (jsonHandler{kvtOps: []kvtOperation{{kvtOpType: RenameK, renameKvtGroup: []renameKvt{{oldKey: "label", newKey: "label"}}}}}).handle("example.com", "/", input, nil)
		require.NoError(t, err)
		require.Equal(t, string(input), string(got))
		for _, values := range [][]string{{"one"}, {"one", "two"}} {
			kvs := map[string][]string{"x-client-id": values, "untouched": {"keep"}}
			err := (kvHandler{kvtOps: []kvtOperation{{kvtOpType: RenameK, renameKvtGroup: []renameKvt{{oldKey: "x-client-id", newKey: "x-client-id"}}}}}).handle("example.com", "/", kvs, nil)
			require.NoError(t, err)
			require.Equal(t, values, kvs["x-client-id"])
			require.Equal(t, []string{"keep"}, kvs["untouched"])
		}
	})
}
