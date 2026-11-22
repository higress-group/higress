// Copyright 2026 Higress Authors
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

package main

import (
	"strings"
	"testing"
)

func TestCPPHotfixPrepareUsesStableInterfaceAndSeparateReleaseID(t *testing.T) {
	prepare := mustWorkflow(t, "prepare-plugin-release.yaml")
	for _, required := range []string{
		"release_mode:",
		"- cpp-hotfix",
		"plan --mode=cpp-hotfix",
		"render-snapshot --catalog",
		"args+=(--mode=cpp-hotfix)",
		"release_id=cpp-hotfix-2.0.1",
		`bazel test --test_output=errors "//extensions/$build_name:${build_name}_test"`,
		`make -C plugins/wasm-cpp PLUGIN_NAME="$build_name" build`,
		`test "$(git rev-parse refs/remotes/origin/main)" = "$TARGET_REF"`,
	} {
		if !strings.Contains(prepare, required) {
			t.Fatalf("prepare workflow lacks %q", required)
		}
	}
	if strings.Contains(prepare, "plugins/model-") || strings.Contains(prepare, "sni_misdirect") {
		t.Fatal("prepare workflow contains an out-of-scope C++ plugin")
	}
}

func TestCPPHotfixPromotionIsApprovedVersionOnlyAndHashesRawBytes(t *testing.T) {
	promote := mustWorkflow(t, "promote-plugin-release.yaml")
	for _, required := range []string{
		"environment: plugin-release-cpp-hotfix-version",
		"test \"$SNAPSHOT_PATH\" = plugins/release/snapshots/cpp-hotfix-2.0.1.json",
		"args+=(--mode=cpp-hotfix)",
		"if: ${{ inputs.release_mode != 'cpp-hotfix' && !inputs.dry_run }}",
		`test "$(sha256sum "$SNAPSHOT_PATH" | awk '{print $1}')" = "$SNAPSHOT_SHA256"`,
		`git merge-base --is-ancestor "$SOURCE_COMMIT" refs/remotes/origin/main`,
		"descriptor_or_absent()",
	} {
		if !strings.Contains(promote, required) {
			t.Fatalf("promote workflow lacks %q", required)
		}
	}
	latestJob := workflowJobSection(t, promote, "latest")
	if !strings.Contains(latestJob, "inputs.release_mode != 'cpp-hotfix'") {
		t.Fatal("hotfix can enter the latest job")
	}
}

func TestCPPHotfixContractCIIsReadOnlyAndRunsRealTests(t *testing.T) {
	workflow := mustWorkflow(t, "cpp-hotfix-release-contract.yaml")
	for _, required := range []string{
		"runs-on: ubuntu-22.04",
		"GOMAXPROCS=1 go test -p 1 -count=1 ./...",
		"go run . validate-catalog --root ../.. --catalog ../../plugins/release/catalog.json",
		"python3 -m unittest -v test_pull_plugins.py",
		"contents: read",
		"persist-credentials: false",
	} {
		if !strings.Contains(workflow, required) {
			t.Fatalf("contract CI lacks %q", required)
		}
	}
	for _, forbidden := range []string{"oras login", "oras push", "docker build", "docker run", "kind create", "secrets."} {
		if strings.Contains(workflow, forbidden) {
			t.Fatalf("contract CI contains forbidden production or local-cluster operation %q", forbidden)
		}
	}
}
