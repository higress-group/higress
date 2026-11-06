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

package install

import (
	"encoding/json"
	"fmt"
	k8s "github.com/alibaba/higress/hgctl/pkg/kubernetes"
	"github.com/alibaba/higress/hgctl/pkg/plugin/utils"
	"io"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInstallUsesSelectedNamespace(t *testing.T) {
	for _, tc := range []struct{ name, selected, manifest, want string }{
		{"explicit", "custom-ns", "custom-ns", "custom-ns"},
		{"missing manifest namespace", "custom-ns", "", "custom-ns"},
		{"mismatched manifest namespace", "custom-ns", "other-ns", "custom-ns"},
		{"default", "", "", "higress-system"},
		{"second custom namespace", "second-ns", "custom-ns", "second-ns"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			old := k8s.HigressNamespace
			k8s.HigressNamespace = k8s.DefaultHigressNamespace
			defer func() { k8s.HigressNamespace = old }()
			var requestPath, objectNamespace string
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requestPath = r.URL.Path
				var obj map[string]interface{}
				if err := json.NewDecoder(r.Body).Decode(&obj); err != nil {
					http.Error(w, err.Error(), 400)
					return
				}
				objectNamespace = obj["metadata"].(map[string]interface{})["namespace"].(string)
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(obj)
			}))
			defer server.Close()
			cfg := clientcmdapi.Config{Clusters: map[string]*clientcmdapi.Cluster{"test": {Server: server.URL}}, Contexts: map[string]*clientcmdapi.Context{"test": {Cluster: "test"}}, CurrentContext: "test"}
			dyn, err := k8s.NewDynamicClient(clientcmd.NewNonInteractiveClientConfig(cfg, "test", &clientcmd.ConfigOverrides{}, nil))
			if err != nil {
				t.Fatal(err)
			}
			manifest := filepath.Join(t.TempDir(), "plugin.yaml")
			if err := os.WriteFile(manifest, []byte(fmt.Sprintf("apiVersion: extensions.higress.io/v1alpha1\nkind: WasmPlugin\nmetadata:\n  name: diagnostic\n  namespace: %q\nspec: {}\n", tc.manifest)), 0600); err != nil {
				t.Fatal(err)
			}
			ins := &installer{cli: k8s.NewWasmPluginClient(dyn), w: io.Discard, Debugger: utils.NewDefaultDebugger(false, io.Discard)}
			ins.insOpts.Namespace = tc.selected
			ins.insOpts.FromYaml = manifest
			if err := ins.doInstall(false); err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(requestPath, "/namespaces/"+tc.want+"/") || objectNamespace != tc.want {
				t.Fatalf("unexpected path=%s objectNamespace=%s", requestPath, objectNamespace)
			}
			if k8s.HigressNamespace != k8s.DefaultHigressNamespace {
				t.Fatal("installer changed global namespace")
			}
			t.Logf("API path=%s object namespace=%s", requestPath, objectNamespace)

		})
	}
}
