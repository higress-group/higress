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

package helm

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

const emptyDevelopmentEntriesIndex = `apiVersion: v1
entries:
  higress: []
`

const nonEmptyChartEntriesIndex = `apiVersion: v1
entries:
  higress:
    - apiVersion: v2
      name: higress
      version: 2.1.0
      appVersion: 2.1.0
    - apiVersion: v2
      name: higress
      version: 2.2.0-alpha.1
      appVersion: 2.2.0-dev
`

func TestParseLatestVersionEmptyDevelopmentEntries(t *testing.T) {
	repoURL := serveIndex(t, emptyDevelopmentEntriesIndex)

	version, err := ParseLatestVersion(repoURL, RepoLatestVersion, true)
	if version != "" {
		t.Fatalf("expected empty version, got %q", version)
	}
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if err.Error() != "invalid index.yaml: no versions found for higress chart" {
		t.Fatalf("expected invalid index error, got %q", err.Error())
	}
}

func TestParseLatestVersionDevelopmentSelection(t *testing.T) {
	repoURL := serveIndex(t, nonEmptyChartEntriesIndex)

	version, err := ParseLatestVersion(repoURL, RepoLatestVersion, true)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if version != "2.2.0-dev" {
		t.Fatalf("expected development app version 2.2.0-dev, got %q", version)
	}
}

func TestParseLatestVersionStableSelection(t *testing.T) {
	repoURL := serveIndex(t, nonEmptyChartEntriesIndex)

	version, err := ParseLatestVersion(repoURL, RepoLatestVersion, false)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if version != "2.1.0" {
		t.Fatalf("expected stable chart version 2.1.0, got %q", version)
	}
}

func serveIndex(t *testing.T, index string) string {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(index))
	}))
	t.Cleanup(server.Close)

	return server.URL
}
