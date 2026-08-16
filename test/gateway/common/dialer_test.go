// Copyright (c) 2022 Alibaba Group Holding Ltd.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package common

import "testing"

func TestServiceTargetPortFromJSON(t *testing.T) {
	tests := []struct {
		name           string
		port           string
		json           string
		want           string
		useEndpointPod bool
	}{
		{
			name:           "numeric target port",
			port:           "80",
			json:           `{"spec":{"ports":[{"port":80,"targetPort":30080}]}}`,
			want:           "30080",
			useEndpointPod: true,
		},
		{
			name: "default target port",
			port: "8080",
			json: `{"spec":{"ports":[{"port":8080}]}}`,
			want: "8080",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, useEndpointPod, err := serviceTargetPortFromJSON([]byte(tt.json), tt.port)
			if err != nil {
				t.Fatal(err)
			}
			if got != tt.want {
				t.Fatalf("serviceTargetPortFromJSON() = %q, want %q", got, tt.want)
			}
			if useEndpointPod != tt.useEndpointPod {
				t.Fatalf("serviceTargetPortFromJSON() useEndpointPod = %t, want %t", useEndpointPod, tt.useEndpointPod)
			}
		})
	}
}

func TestEndpointSlicePodFromJSON(t *testing.T) {
	pod, err := endpointSlicePodFromJSON([]byte(`{
  "items": [
    {"endpoints": [
      {"conditions": {"ready": false}, "targetRef": {"kind": "Pod", "name": "not-ready"}},
      {"conditions": {"ready": true}, "targetRef": {"kind": "Pod", "name": "gateway-123"}}
    ]}
  ]
}`))
	if err != nil {
		t.Fatal(err)
	}
	if pod != "gateway-123" {
		t.Fatalf("endpointSlicePodFromJSON() = %q, want gateway-123", pod)
	}
}
