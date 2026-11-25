// Copyright (c) 2023 Alibaba Group Holding Ltd.
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
	"github.com/higress-group/wasm-go/pkg/test"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestPropertyStringEncoding(t *testing.T) {
	test.RunGoTest(t, func(t *testing.T) {
		host, _ := test.NewTestHost(json.RawMessage(`{}`))
		defer host.Reset()
		cases := []struct{ value, want string }{
			{"", ""},
			{"plain", "plain"},
			{"引号\"与\\反斜杠\n\r\t", `引号\"与\\反斜杠\n\r\t`},
			{"<script>&", `\u003cscript\u003e\u0026`},
			{string([]byte{0, 1, 31, 255}), `\u0000\u0001\u001f\ufffd`},
			{"\u2028\u2029", `\u2028\u2029`},
		}
		for _, tc := range cases {
			setPropertyWithMarshal("encoding-test", tc.value)
			got, err := host.GetProperty([]string{"encoding-test"})
			if tc.value == "" {
				// SetProperty rejects an empty value; preserve the existing behavior.
				require.Error(t, err)
				continue
			}
			require.NoError(t, err)
			require.Equal(t, tc.want, string(got))
		}
	})
}
