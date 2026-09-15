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

package mcp_session

import (
	"strings"
	"testing"

	xds "github.com/cncf/xds/go/xds/type/v3"
	"github.com/envoyproxy/envoy/contrib/golang/common/go/api"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/structpb"
)

type configTestCommonCAPI struct{}

func (configTestCommonCAPI) Log(api.LogType, string) {}

func (configTestCommonCAPI) LogLevel() api.LogType {
	return api.Debug
}

func TestParserParseRequiresRedisOnlyForUserLevelServer(t *testing.T) {
	api.SetCommonCAPI(configTestCommonCAPI{})

	tests := []struct {
		name                  string
		enableUserLevelServer any
		wantErr               bool
	}{
		{
			name:    "missing flag",
			wantErr: false,
		},
		{
			name:                  "explicitly disabled",
			enableUserLevelServer: false,
			wantErr:               false,
		},
		{
			name:                  "enabled",
			enableUserLevelServer: true,
			wantErr:               true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			value := map[string]any{
				"sse_path_suffix": "/sse",
			}
			if tt.enableUserLevelServer != nil {
				value["enable_user_level_server"] = tt.enableUserLevelServer
			}

			configAny := newMCPConfigAny(t, value)
			parsed, err := (&Parser{}).Parse(configAny, nil)
			if tt.wantErr {
				if err == nil {
					t.Fatal("Parse() error = nil, want Redis requirement error")
				}
				if !strings.Contains(err.Error(), "redis configuration is not provided") {
					t.Fatalf("Parse() error = %q, want Redis requirement error", err)
				}
				return
			}

			if err != nil {
				t.Fatalf("Parse() unexpected error: %v", err)
			}
			if parsed.(*config).enableUserLevelServer {
				t.Fatal("enableUserLevelServer = true, want false")
			}
		})
	}
}

func newMCPConfigAny(t *testing.T, value map[string]any) *anypb.Any {
	t.Helper()

	configValue, err := structpb.NewStruct(value)
	if err != nil {
		t.Fatalf("create config struct: %v", err)
	}
	configAny, err := anypb.New(&xds.TypedStruct{Value: configValue})
	if err != nil {
		t.Fatalf("create config any: %v", err)
	}
	return configAny
}
