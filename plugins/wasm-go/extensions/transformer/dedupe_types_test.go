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

func TestJSONDedupeKeepsDistinctTypes(t *testing.T) {
	cases := []struct{ input, want string }{
		{`[1,"1",true,"true",null,""]`, `[1,"1",true,"true",null,""]`},
		{`[{"a":1},"{\"a\":1}"]`, `[{"a":1},"{\"a\":1}"]`},
		{`[1,"1",1,"1"]`, `[1,"1"]`},
		{`[true,"true",true,"true"]`, `[true,"true"]`},
		{`["a","a","b"]`, `["a","b"]`},
		{`[1,1,2]`, `[1,2]`},
		{`["a","a"]`, `"a"`},
	}
	for _, tc := range cases {
		t.Run(tc.input, func(t *testing.T) {
			test.RunGoTest(t, func(t *testing.T) {
				host, _ := test.NewTestHost(json.RawMessage(`{"reqRules":[{"operate":"remove","body":[{"key":"missing"}]}]}`))
				defer host.Reset()
				got, err := (jsonHandler{kvtOps: []kvtOperation{{kvtOpType: DedupeK, dedupeKvtGroup: []dedupeKvt{{key: "values", strategy: "RETAIN_UNIQUE"}}}}}).handle("example.com", "/", []byte(`{"values":`+tc.input+`}`), nil)
				require.NoError(t, err)
				var fields map[string]json.RawMessage
				require.NoError(t, json.Unmarshal(got, &fields))
				require.JSONEq(t, tc.want, string(fields["values"]))
			})
		})
	}
}
