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

package helm

import (
	"reflect"
	"testing"
)

func TestUnsupportedLocalDockerUpgradeOverlayPaths(t *testing.T) {
	baseline := &Profile{
		Profile:            "local-docker",
		InstallPackagePath: "/opt/higress",
		HigressVersion:     "2.1.0",
		Global:             ProfileGlobal{Install: InstallLocalDocker},
		Console:            ProfileConsole{Port: 8080},
		Gateway:            ProfileGateway{HttpPort: 80},
		Storage:            ProfileStorage{Url: "file:///opt/higress", Ns: "higress"},
		Values:             map[string]any{"feature": map[string]any{"enabled": true}},
		Charts:             ProfileCharts{Standalone: Chart{Url: "https://example.com/get-higress.sh"}},
	}

	tests := []struct {
		name     string
		file     string
		setFlags []string
		want     []string
	}{
		{name: "no overlay"},
		{name: "same value", setFlags: []string{"gateway.httpPort=80"}},
		{name: "install package path", setFlags: []string{"installPackagePath=/tmp/higress"}},
		{name: "standalone url", setFlags: []string{"charts.standalone.url=https://example.com/next.sh"}},
		{name: "gateway", setFlags: []string{"gateway.httpPort=18080"}, want: []string{"gateway.httpPort"}},
		{name: "storage", setFlags: []string{"storage.ns=other"}, want: []string{"storage.ns"}},
		{name: "console", setFlags: []string{"console.port=18081"}, want: []string{"console.port"}},
		{name: "values", file: "values:\n  feature:\n    enabled: false\n", want: []string{"values.feature.enabled"}},
		{name: "install mode", setFlags: []string{"global.install=k8s"}, want: []string{"global.install"}},
		{name: "version", setFlags: []string{"higressVersion=9.9.9"}, want: []string{"higressVersion"}},
		{
			name:     "sorted paths",
			setFlags: []string{"storage.ns=other", "gateway.httpPort=18080", "higressVersion=9.9.9"},
			want:     []string{"gateway.httpPort", "higressVersion", "storage.ns"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			overlay, err := GetProfileOverlay(tt.file, tt.setFlags)
			if err != nil {
				t.Fatalf("GetProfileOverlay() error = %v", err)
			}
			got, err := UnsupportedLocalDockerUpgradeOverlayPaths(baseline, overlay)
			if err != nil {
				t.Fatalf("UnsupportedLocalDockerUpgradeOverlayPaths() error = %v", err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("UnsupportedLocalDockerUpgradeOverlayPaths() = %v, want %v", got, tt.want)
			}
		})
	}
}
