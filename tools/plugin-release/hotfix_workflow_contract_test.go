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
	"os"
	"os/exec"
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
	validate := mustWorkflow(t, "validate-plugin-preparation-pr.yaml")
	for _, required := range []string{"cpp-hotfix-2.0.1", "args+=(--mode=cpp-hotfix)"} {
		if !strings.Contains(validate, required) {
			t.Fatalf("preparation validation workflow lacks %q", required)
		}
	}
}

func TestCPPReleaseNativeTestsUsePinnedBuilder(t *testing.T) {
	for _, name := range []string{"prepare-plugin-release.yaml", "validate-plugin-preparation-pr.yaml"} {
		t.Run(name, func(t *testing.T) {
			workflow := mustWorkflow(t, name)
			build := workflowShellContract(t, name, "plugin-build-contract")
			for _, required := range []string{
				`builder="$(sed -n 's/^ARG BUILDER=//p' plugins/wasm-cpp/Dockerfile)"`,
				`case "$builder" in
                  *@sha256:*) ;;
                  *) echo 'Builder must be pinned by digest' >&2; return 1 ;;
                esac`,
				`docker run --rm \
                  -v "$GITHUB_WORKSPACE/plugins/wasm-cpp:/workspace" \
                  -w /workspace \
                  "$builder" \
                  bash -euo pipefail -c 'bazel test --test_output=errors "//extensions/$1:$1_test"' -- "$build_name"`,
				`make -C plugins/wasm-cpp PLUGIN_NAME="$build_name" build`,
			} {
				if !strings.Contains(build, required) {
					t.Fatalf("%s build contract lacks %q", name, required)
				}
			}
			if strings.Contains(workflow, "(cd plugins/wasm-cpp && bazel test") {
				t.Fatalf("%s still runs native C++ tests with host bazel", name)
			}
		})
	}
}

func TestCPPHotfixPromotionIsDryRunOnlyAndHashesRawBytes(t *testing.T) {
	promote := mustWorkflow(t, "promote-plugin-release.yaml")
	for _, required := range []string{
		"needs: input-guard",
		"test \"$SNAPSHOT_PATH\" = plugins/release/snapshots/cpp-hotfix-2.0.1.json",
		"build_promotion_verify_args",
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
	if strings.Contains(promote, "hotfix-version-approval") || strings.Contains(promote, "plugin-release-cpp-hotfix-version") {
		t.Fatal("dry-run-only hotfix must not instantiate an approval environment")
	}
	if strings.Count(promote, "build_promotion_verify_args\n") != 2 {
		t.Fatal("both verification steps must use the same argument builder")
	}
	if !strings.Contains(latestJob, "inputs.release_mode != 'cpp-hotfix'") {
		t.Fatal("hotfix can enter the latest job")
	}
}

func TestCPPHotfixContractCIIsReadOnlyAndRunsRealTests(t *testing.T) {
	workflow := mustWorkflow(t, "cpp-hotfix-release-contract.yaml")
	for _, required := range []string{
		"runs-on: ubuntu-22.04",
		"GOMAXPROCS=1 go test -v -p 1 -count=1 ./...",
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

func TestCPPHotfixContractCIBindsTwoFullSHAsOnlyOnDispatch(t *testing.T) {
	workflow := mustWorkflow(t, "cpp-hotfix-release-contract.yaml")
	for _, required := range []string{
		`[[ "$EXPECTED_HEAD" =~ ^[0-9a-f]{40}$ ]]`,
		`[[ "$PLUGIN_SERVER_REF" =~ ^[0-9a-f]{40}$ ]]`,
		`test "$(git rev-parse HEAD)" = "$PLUGIN_SERVER_REF"`,
		"Higress release contract only",
		"Two-SHA release and history contract",
		"Higress-only contract: plugin-server history tests did not run",
	} {
		if !strings.Contains(workflow, required) {
			t.Fatalf("two-SHA contract lacks %q", required)
		}
	}
	if strings.Contains(workflow, "default: main") || strings.Count(workflow, "if: ${{ github.event_name == 'workflow_dispatch' }}") != 2 {
		t.Fatal("plugin-server checkout and tests must require explicit dispatch SHAs")
	}
}

func TestCPPHotfixTemporaryDeliveryIsNotTracked(t *testing.T) {
	paths, err := runGit("../..", "ls-files", "delivery")
	if err != nil || paths != "" {
		t.Fatalf("temporary delivery files remain tracked: %s (%v)", paths, err)
	}
	if _, err := os.Stat("promotion-verify.sh"); err != nil {
		t.Fatal(err)
	}
}

func TestCPPHotfixContractCIRejectsFloatingRepositoryRefs(t *testing.T) {
	body := workflowRunBody(t, mustWorkflow(t, "cpp-hotfix-release-contract.yaml"), "Validate immutable source inputs")
	for _, fixture := range []struct {
		name       string
		higress    string
		server     string
		event      string
		shouldPass bool
	}{
		{"two-full-shas", strings.Repeat("a", 40), strings.Repeat("b", 40), "workflow_dispatch", true},
		{"server-main-rejected", strings.Repeat("a", 40), "main", "workflow_dispatch", false},
		{"server-missing-rejected", strings.Repeat("a", 40), "", "workflow_dispatch", false},
		{"higress-branch-rejected", "task/branch", strings.Repeat("b", 40), "workflow_dispatch", false},
		{"higress-only-pr", strings.Repeat("a", 40), "", "pull_request", true},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			cmd := exec.Command("bash", "-c", body)
			cmd.Env = append(os.Environ(), "EXPECTED_HEAD="+fixture.higress, "PLUGIN_SERVER_REF="+fixture.server, "EVENT_NAME="+fixture.event)
			output, err := cmd.CombinedOutput()
			if (err == nil) != fixture.shouldPass {
				t.Fatalf("input gate pass=%t expected=%t: %s (%v)", err == nil, fixture.shouldPass, output, err)
			}
		})
	}
}

func TestCPPHotfixReworkBaselineFindingsAreReproduced(t *testing.T) {
	source := mustHotfixReworkBaseline(t, "SOURCE.md")
	if !strings.Contains(source, hotfixReworkBaseline) {
		t.Fatal("baseline provenance lacks the exact source commit")
	}
	previous := mustHotfixReworkBaseline(t, "promote-plugin-release.yaml")
	if !strings.Contains(previous, "environment: plugin-release-cpp-hotfix-version") || strings.Contains(previous, "  input-guard:\n") {
		t.Fatal("H01 baseline evidence changed")
	}
	previousCI := mustHotfixReworkBaseline(t, "cpp-hotfix-release-contract.yaml")
	if !strings.Contains(previousCI, "default: main") {
		t.Fatal("H03 baseline no longer accepts floating server main")
	}
	previousDelivery := mustHotfixReworkBaseline(t, "delivery-files.txt")
	wantDelivery := "delivery/HANDOFF.json\ndelivery/PROGRESS.json\ndelivery/RESULT.md\n"
	if previousDelivery != wantDelivery {
		t.Fatalf("H04 baseline must reproduce exactly three tracked temporary paths: got %q, want %q", previousDelivery, wantDelivery)
	}
	t.Log("H01/H03/H04 RED baseline evidence reproduced; current guards, exact-SHA gates, and untracked delivery are checked independently")
}
