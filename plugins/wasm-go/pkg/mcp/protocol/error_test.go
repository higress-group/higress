// Copyright (c) 2022 Alibaba Group Holding Ltd.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package protocol

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestModernErrorDataSerialization(t *testing.T) {
	tests := []struct {
		name          string
		protocolError *Error
		wantCode      int
		wantData      ErrorData
	}{
		{
			name:          "unsupported version",
			protocolError: UnsupportedVersion(Version("2025-11-25"), SupportedVersions()),
			wantCode:      CodeUnsupportedVersion,
			wantData: ErrorData{
				Supported: SupportedVersions(),
				Requested: Version("2025-11-25"),
			},
		},
		{
			name: "missing required client capability",
			protocolError: MissingRequiredClientCapability(ClientCapabilities{
				Sampling: &SamplingCapabilities{Tools: &JSONObject{}},
			}),
			wantCode: CodeMissingRequiredClientCapability,
			wantData: ErrorData{RequiredCapabilities: &ClientCapabilities{
				Sampling: &SamplingCapabilities{Tools: &JSONObject{}},
			}},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			body := MarshalErrorResponse(ID{}, test.protocolError)
			var envelope struct {
				Error struct {
					Code int       `json:"code"`
					Data ErrorData `json:"data"`
				} `json:"error"`
			}
			if err := json.Unmarshal(body, &envelope); err != nil {
				t.Fatalf("Unmarshal() error = %v", err)
			}
			if envelope.Error.Code != test.wantCode || !reflect.DeepEqual(envelope.Error.Data, test.wantData) {
				t.Fatalf("error envelope = %+v, want code %d/data %+v", envelope.Error, test.wantCode, test.wantData)
			}
		})
	}
}
