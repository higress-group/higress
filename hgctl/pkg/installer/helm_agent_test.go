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

package installer

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/alibaba/higress/hgctl/pkg/helm"
)

func TestHelmAgentIsHigressInstalled(t *testing.T) {
	profile := &helm.Profile{Global: helm.ProfileGlobal{Namespace: "higress-system"}}
	tests := []struct {
		name          string
		mode          string
		wantInstalled bool
		wantErr       string
	}{
		{name: "missing binary", mode: "missing", wantErr: "start helm ownership check"},
		{name: "command failure", mode: "failure", wantErr: "helm ownership check failed"},
		{name: "not installed", mode: "absent"},
		{name: "deployed release", mode: "deployed", wantInstalled: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			binary := os.Args[0]
			if tt.mode == "missing" {
				binary = "helm-not-found-for-test"
			} else {
				t.Setenv("HGCTL_HELM_TEST_MODE", tt.mode)
			}
			agent := NewHelmAgent(profile, &bytes.Buffer{}, true, WithHelmBinaryName(binary))
			installed, err := agent.IsHigressInstalled()
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("IsHigressInstalled() error = %v, want substring %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("IsHigressInstalled() error = %v", err)
			}
			if installed != tt.wantInstalled {
				t.Fatalf("IsHigressInstalled() = %t, want %t", installed, tt.wantInstalled)
			}
		})
	}
}

func TestHelmAgentListReleasesFiltersExactParentChartAndStatus(t *testing.T) {
	t.Setenv("HGCTL_HELM_TEST_MODE", "list-mixed")

	agent := NewHelmAgent(nil, &bytes.Buffer{}, true, WithHelmBinaryName(os.Args[0]))
	candidates, err := agent.ListReleases(HelmReleaseSelector{})
	if err != nil {
		t.Fatalf("ListReleases() error = %v", err)
	}

	want := []HelmReleaseCandidate{
		{Name: "custom-higress", Namespace: "prod", ChartName: "higress", ChartVersion: "2.2.4", AppVersion: "2.2.4", Status: "deployed"},
		{Name: "future-higress", Namespace: "prod", ChartName: "higress", ChartVersion: "2.3.0", AppVersion: "2.3.0", Status: "deployed"},
	}
	if !reflect.DeepEqual(candidates, want) {
		t.Fatalf("ListReleases() = %#v, want %#v", candidates, want)
	}
}

func TestHelmAgentListReleasesPaginatesPastDefaultLimit(t *testing.T) {
	t.Setenv("HGCTL_HELM_TEST_MODE", "list-paginated-hidden")

	agent := NewHelmAgent(nil, &bytes.Buffer{}, true, WithHelmBinaryName(os.Args[0]))
	candidates, err := agent.ListReleases(HelmReleaseSelector{})
	if err != nil {
		t.Fatalf("ListReleases() error = %v", err)
	}

	want := []HelmReleaseCandidate{
		{Name: "first-higress", Namespace: "prod", ChartName: "higress", ChartVersion: "2.2.4", AppVersion: "2.2.4", Status: "deployed"},
		{Name: "hidden-higress", Namespace: "staging", ChartName: "higress", ChartVersion: "2.2.4", AppVersion: "2.2.4", Status: "deployed"},
	}
	if !reflect.DeepEqual(candidates, want) {
		t.Fatalf("ListReleases() = %#v, want %#v", candidates, want)
	}
}

func TestHelmAgentGetRetainedValuesParsesStructuredOutput(t *testing.T) {
	t.Setenv("HGCTL_HELM_TEST_MODE", "get-values")

	agent := NewHelmAgent(nil, &bytes.Buffer{}, true, WithHelmBinaryName(os.Args[0]))
	values, err := agent.GetRetainedValues(HelmReleaseCandidate{Name: "custom-higress", Namespace: "prod"})
	if err != nil {
		t.Fatalf("GetRetainedValues() error = %v", err)
	}
	core, ok := values["higress-core"].(map[string]any)
	if !ok {
		t.Fatalf("higress-core = %#v, want map", values["higress-core"])
	}
	gateway, ok := core["gateway"].(map[string]any)
	if !ok {
		t.Fatalf("gateway = %#v, want map", core["gateway"])
	}
	if gateway["replicas"] != float64(2) {
		t.Fatalf("gateway replicas = %#v, want 2", gateway["replicas"])
	}
}

func TestMain(m *testing.M) {
	mode := os.Getenv("HGCTL_HELM_TEST_MODE")
	if mode != "" {
		runHelmHelper(mode)
		return
	}
	os.Exit(m.Run())
}

func runHelmHelper(mode string) {
	switch mode {
	case "failure":
		fmt.Fprint(os.Stderr, "helm cluster access failed")
		os.Exit(1)
	case "absent":
		fmt.Fprint(os.Stdout, "NAME\tNAMESPACE\tREVISION\tUPDATED\tSTATUS\tCHART\tAPP VERSION\n")
	case "deployed":
		fmt.Fprint(os.Stdout, "higress\thigress-system\t1\t2026-01-01\tdeployed\thigress\t2.2.4\n")
	case "list-mixed":
		fmt.Fprint(os.Stdout, `[
  {"name":"custom-higress","namespace":"prod","status":"deployed","chart":"higress-2.2.4","app_version":"2.2.4"},
  {"name":"core-dependency","namespace":"prod","status":"deployed","chart":"higress-core-2.2.4","app_version":"2.2.4"},
  {"name":"pending-higress","namespace":"prod","status":"pending-upgrade","chart":"higress-2.2.4","app_version":"2.2.4"},
  {"name":"future-higress","namespace":"prod","status":"deployed","chart":"higress-2.3.0","app_version":"2.3.0"}
]`)
	case "list-paginated-hidden":
		writePaginatedReleaseList()
	case "get-values":
		fmt.Fprint(os.Stdout, `{"higress-core":{"gateway":{"replicas":2}}}`)
	default:
		fmt.Fprintf(os.Stderr, "unknown test mode %q", mode)
		os.Exit(2)
	}
}

func writePaginatedReleaseList() {
	offset := helmHelperIntFlag("--offset")
	if offset >= 256 {
		fmt.Fprint(os.Stdout, `[{"name":"hidden-higress","namespace":"staging","status":"deployed","chart":"higress-2.2.4","app_version":"2.2.4"}]`)
		return
	}

	releases := make([]map[string]string, 0, 256)
	releases = append(releases, map[string]string{
		"name":        "first-higress",
		"namespace":   "prod",
		"status":      "deployed",
		"chart":       "higress-2.2.4",
		"app_version": "2.2.4",
	})
	for i := 1; i < 256; i++ {
		releases = append(releases, map[string]string{
			"name":        fmt.Sprintf("other-%03d", i),
			"namespace":   "prod",
			"status":      "deployed",
			"chart":       "other-1.0.0",
			"app_version": "1.0.0",
		})
	}
	out, err := json.Marshal(releases)
	if err != nil {
		fmt.Fprintf(os.Stderr, "marshal paginated releases: %v", err)
		os.Exit(2)
	}
	fmt.Fprint(os.Stdout, string(out))
}

func helmHelperIntFlag(name string) int {
	for i, arg := range os.Args {
		if arg == name && i+1 < len(os.Args) {
			value, err := strconv.Atoi(os.Args[i+1])
			if err == nil {
				return value
			}
		}
		if strings.HasPrefix(arg, name+"=") {
			value, err := strconv.Atoi(strings.TrimPrefix(arg, name+"="))
			if err == nil {
				return value
			}
		}
	}
	return 0
}
