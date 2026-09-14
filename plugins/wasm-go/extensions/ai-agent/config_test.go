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

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestInitAPIsRejectsOpenAPIWithoutServerURL(t *testing.T) {
	tests := []struct {
		name    string
		servers string
	}{
		{name: "missing servers"},
		{name: "empty server URL", servers: "servers:\n  - url: ''\n"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			configJSON, err := json.Marshal(map[string]any{
				"apis": []map[string]any{
					{
						"apiProvider": map[string]any{
							"serviceName": "api-service",
							"servicePort": 8080,
							"domain":      "api.example.com",
						},
						"api": `openapi: 3.0.0
info:
  title: API without servers
  version: 1.0.0
` + tt.servers + `
paths:
  /items:
    get:
      operationId: listItems`,
					},
				},
			})
			require.NoError(t, err)

			var initErr error
			require.NotPanics(t, func() {
				initErr = initAPIs(gjson.ParseBytes(configJSON), &PluginConfig{})
			})
			require.EqualError(t, initErr, "api servers is required")
		})
	}
}
