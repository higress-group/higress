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
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
)

const (
	hotfixReworkBaseline       = "cbe73567ab2e2092761d169c3c9418e9a5d7165e"
	hotfixReworkPromoteSHA256  = "25641862a33c4e09c74404c7a2f1a1739d55f3edc36ee672d29a5f5437e1b518"
	hotfixReworkContractSHA256 = "9c29a4245862eaa87f5832966c538f5bdccdba0a68c4095bd250302bd278a8cd"
	hotfixReworkDeliverySHA256 = "17282997eec2bb8d52ff33a398612b543977f5afbb570869385666aba8838e53"
	hotfixReworkSourceSHA256   = "e92381fb9f6ca37fe52840c97738a299058a717fbf2f3d771353d3945bc058b3"
)

func mustHotfixReworkBaseline(t *testing.T, name string) string {
	t.Helper()
	hashes := map[string]string{
		"promote-plugin-release.yaml":      hotfixReworkPromoteSHA256,
		"cpp-hotfix-release-contract.yaml": hotfixReworkContractSHA256,
		"delivery-files.txt":               hotfixReworkDeliverySHA256,
		"SOURCE.md":                        hotfixReworkSourceSHA256,
	}
	want, ok := hashes[name]
	if !ok {
		t.Fatalf("unknown hotfix rework baseline fixture %q", name)
	}
	data := mustRead(t, filepath.Join("testdata", "hotfix-rework-baseline", name))
	if got := sha256Hex(data); got != want {
		t.Fatalf("baseline %s from %s: SHA-256 got %s, want %s", name, hotfixReworkBaseline, got, want)
	}
	return string(data)
}

type hotfixSpyCall struct {
	Tool string   `json:"tool"`
	Args []string `json:"args"`
}

type hotfixStepResult struct {
	Err      error
	Output   string
	Calls    []hotfixSpyCall
	Journal  map[string]any
	ExitCode int
}

func workflowRunBody(t *testing.T, workflow, stepName string) string {
	t.Helper()
	marker := "      - name: " + stepName + "\n"
	start := strings.Index(workflow, marker)
	if start < 0 {
		t.Fatalf("workflow lacks step %s", stepName)
	}
	remaining := workflow[start+len(marker):]
	if next := strings.Index(remaining, "\n      - "); next >= 0 {
		remaining = remaining[:next]
	}
	runStart := strings.Index(remaining, "        run: |\n")
	if runStart < 0 {
		t.Fatalf("step %s has no literal run body", stepName)
	}
	lines := strings.Split(remaining[runStart+len("        run: |\n"):], "\n")
	body := []string{}
	for _, line := range lines {
		if line == "" {
			body = append(body, "")
			continue
		}
		if !strings.HasPrefix(line, "          ") {
			break
		}
		body = append(body, strings.TrimPrefix(line, "          "))
	}
	return strings.Join(body, "\n") + "\n"
}

func TestCPPHotfixStepBodyReworkRedGreen(t *testing.T) {
	previous := mustHotfixReworkBaseline(t, "promote-plugin-release.yaml")
	for _, fixture := range []struct {
		name     string
		workflow string
		wantPass bool
	}{
		{name: "before-H02-red", workflow: previous},
		{name: "after-H02-green", workflow: mustWorkflow(t, "promote-plugin-release.yaml"), wantPass: true},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			result := runHotfixStepBodies(t, fixture.workflow, "success", []string{"Verify exact snapshot provenance", "Promote candidate manifests by digest"})
			verifies := [][]string{}
			for _, call := range result.Calls {
				if call.Tool == "go" {
					verifies = append(verifies, call.Args)
				}
			}
			if len(verifies) != 2 {
				t.Fatalf("want two real step-body verify invocations, got %+v", result.Calls)
			}
			if !fixture.wantPass {
				if result.Err == nil || reflect.DeepEqual(verifies[0], verifies[1]) {
					t.Fatalf("old second verify did not reproduce H02: %+v", result)
				}
				t.Logf("H02 RED reproduced: exit=%d; first/second verification differ", result.ExitCode)
				return
			}
			if result.Err != nil || !reflect.DeepEqual(verifies[0], verifies[1]) {
				t.Fatalf("real write-step verification differs or failed: %+v\n%s", result, result.Output)
			}
			if result.Journal["phase"] != "version-complete" || len(versionJournalEntries(t, result.Journal)) != 9 {
				t.Fatalf("write-step did not complete the synthetic nine-version journal: %+v", result.Journal)
			}
			for _, call := range result.Calls {
				if strings.Contains(strings.Join(call.Args, " "), ":latest") || strings.Contains(strings.Join(call.Args, " "), "plugin-release-batches") {
					t.Fatalf("hotfix entered a latest phase: %+v", call)
				}
			}
			t.Log("H02 GREEN: both actual step bodies share exact cpp-hotfix plan/mode; version-complete; no latest")
		})
	}
}

func TestCPPHotfixActualWriteStepFailsBeforeLoginOrCopy(t *testing.T) {
	workflow := mustWorkflow(t, "promote-plugin-release.yaml")
	for _, scenario := range []string{"401", "403", "conflict", "ambiguous-not-found"} {
		t.Run(scenario, func(t *testing.T) {
			result := runHotfixStepBodies(t, workflow, scenario, []string{"Verify exact snapshot provenance", "Promote candidate manifests by digest"})
			if result.Err == nil {
				t.Fatalf("unsafe descriptor %s was accepted", scenario)
			}
			assertNoHotfixLoginOrWrite(t, result.Calls)
			t.Logf("H02 %s rejected: exit=%d; login/copy/push=0", scenario, result.ExitCode)
		})
	}
	t.Run("missing-approval", func(t *testing.T) {
		result := runHotfixStepBodies(t, workflow, "missing-approval", []string{"Verify a merged labeled preparation PR authorizes promotion", "Promote candidate manifests by digest"})
		if result.Err == nil || !strings.Contains(result.Output, "expected exactly one PR") {
			t.Fatalf("missing merged approval was accepted: %+v", result)
		}
		assertNoHotfixLoginOrWrite(t, result.Calls)
	})
}

func TestCPPHotfixActualInputGuardRejectsNonDryRun(t *testing.T) {
	workflow := mustWorkflow(t, "promote-plugin-release.yaml")
	guardStart := strings.Index(workflow, "  input-guard:\n")
	if guardStart < 0 {
		t.Fatal("workflow lacks an independent first input guard")
	}
	guardEnd := strings.Index(workflow[guardStart:], "\n  verify-and-promote:\n")
	if guardEnd < 0 {
		t.Fatal("workflow lacks an independent first input guard")
	}
	guard := workflow[guardStart : guardStart+guardEnd]
	for _, forbidden := range []string{"environment:", "secrets.", "oras ", "always()"} {
		if strings.Contains(guard, forbidden) {
			t.Fatalf("input guard contains %s", forbidden)
		}
	}
	if !strings.Contains(workflow, "    needs: input-guard\n") {
		t.Fatal("credential job lacks the success dependency")
	}
	for _, scenario := range []struct {
		mode string
		dry  string
		pass bool
	}{{"cpp-hotfix", "false", false}, {"cpp-hotfix", "true", true}, {"standard", "false", true}, {"standard", "true", true}} {
		t.Run(scenario.mode+"/"+scenario.dry, func(t *testing.T) {
			result := runHotfixStepBodiesWithEnv(t, workflow, "guard", []string{"Reject disabled hotfix public promotion"}, []string{"RELEASE_MODE=" + scenario.mode, "DRY_RUN=" + scenario.dry})
			if (result.Err == nil) != scenario.pass {
				t.Fatalf("guard mode=%s dry=%s: %+v", scenario.mode, scenario.dry, result)
			}
			assertNoHotfixLoginOrWrite(t, result.Calls)
		})
	}
}

func TestCPPHotfixPrepareAllowsNonDryRunCandidatesOnly(t *testing.T) {
	prepare := mustWorkflow(t, "prepare-plugin-release.yaml")
	for _, forbidden := range []string{
		"  input-guard:\n", "needs: input-guard", "cpp-hotfix is dry-run-only",
		":latest", "PLUGIN_PUBLIC_REGISTRY", "oras cp ", "oras tag ", "go run . promote",
	} {
		if strings.Contains(prepare, forbidden) {
			t.Fatalf("candidate-only preparation contains %q", forbidden)
		}
	}
	for _, required := range []string{
		"jobs:\n  preflight:\n    runs-on: ubuntu-latest\n",
		"  prepare:\n    needs: preflight\n",
		"CANDIDATE_REGISTRY: ${{ vars.PLUGIN_CANDIDATE_REGISTRY }}",
		`candidate="$CANDIDATE_REGISTRY/candidates/$id:${plan_id}${input_hash#sha256:}"`,
		`publish_plugin_manifest "$candidate"`,
		"release_id=cpp-hotfix-2.0.1",
		`branch="release/plugin-snapshot-$release_id"`,
	} {
		if !strings.Contains(prepare, required) {
			t.Fatalf("candidate-only preparation lacks %q", required)
		}
	}
	for _, step := range []string{
		"Build and publish content-addressed candidates",
		"Render canonical snapshot",
		"Create or update deterministic preparation PR",
	} {
		if !strings.Contains(prepare, "      - name: "+step+"\n        if: ${{ !inputs.dry_run }}\n") {
			t.Fatalf("candidate-only step %q must allow non-dry-run cpp-hotfix", step)
		}
	}
	t.Cleanup(func() { _ = os.Remove("/tmp/overrides.json") })
	result := runHotfixStepBodiesWithEnv(t, prepare, "candidate-only", []string{"Validate immutable inputs"}, []string{
		"RELEASE_MODE=cpp-hotfix", "DRY_RUN=false", "GATEWAY_VERSION=2.2.7",
		"TARGET_REF=" + strings.Repeat("a", 40), "PREVIOUS=", "BOOTSTRAP_BASE=", "OVERRIDES={}",
	})
	if result.Err != nil {
		t.Fatalf("non-dry-run cpp-hotfix candidate inputs were rejected: %+v\n%s", result, result.Output)
	}
	assertNoHotfixLoginOrWrite(t, result.Calls)
	promote := mustWorkflow(t, "promote-plugin-release.yaml")
	blocked := runHotfixStepBodiesWithEnv(t, promote, "guard", []string{"Reject disabled hotfix public promotion"}, []string{
		"RELEASE_MODE=cpp-hotfix", "DRY_RUN=false",
	})
	if blocked.Err == nil {
		t.Fatal("non-dry-run cpp-hotfix public promotion must remain rejected")
	}
	assertNoHotfixLoginOrWrite(t, blocked.Calls)
	t.Log("non-dry-run cpp-hotfix candidate inputs accepted; public promotion rejected before login/write")
}

func assertNoHotfixLoginOrWrite(t *testing.T, calls []hotfixSpyCall) {
	t.Helper()
	for _, call := range calls {
		if call.Tool == "oras" && len(call.Args) > 0 && (call.Args[0] == "login" || call.Args[0] == "cp" || call.Args[0] == "push") {
			t.Fatalf("rejected request reached registry login/write: %+v", call)
		}
	}
}

func runHotfixStepBodies(t *testing.T, workflow, scenario string, steps []string) hotfixStepResult {
	t.Helper()
	return runHotfixStepBodiesWithEnv(t, workflow, scenario, steps, nil)
}

func runHotfixStepBodiesWithEnv(t *testing.T, workflow, scenario string, steps, extraEnv []string) hotfixStepResult {
	t.Helper()
	root := t.TempDir()
	bin := filepath.Join(root, "bin")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, relative := range []string{"tools/hack/oras-retry.sh", "tools/plugin-release/promotion-verify.sh"} {
		data, err := os.ReadFile(filepath.Join("../..", relative))
		if err != nil {
			t.Fatal(err)
		}
		mustWrite(t, filepath.Join(root, relative), string(data))
	}
	const snapshotPath = "plugins/release/snapshots/cpp-hotfix-2.0.1.json"
	const sourceCommit = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	snapshot := Snapshot{SchemaVersion: 1, GatewayVersion: "2.2.7", SourceCommit: sourceCommit, ProvenanceMode: "candidate", Plugins: []SnapshotEntry{}}
	ids := make([]string, 0, len(cppHotfixSources))
	for id := range cppHotfixSources {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		digest := testDigest(id)
		snapshot.Plugins = append(snapshot.Plugins, SnapshotEntry{LogicalID: id, Version: cppHotfixVersion, Digest: digest, OCIRef: "registry.example.invalid/plugins/" + id + ":2.0.1", CandidateRef: "registry.example.invalid/candidates/" + id + "@" + digest, ProvenanceMode: "candidate"})
	}
	if err := writeCanonical(filepath.Join(root, snapshotPath), snapshot); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Join(root, "plugins/release/plans/cpp-hotfix-2.0.1.json"), "{}\n")
	journalSHA := sha256Hex(mustRead(t, filepath.Join(root, snapshotPath)))
	journalPath := filepath.Join("/tmp", "plugin-release-version-"+journalSHA+".json")
	t.Cleanup(func() { _ = os.Remove(journalPath) })
	mustWrite(t, filepath.Join(root, "registry.json"), "{}\n")
	mustWrite(t, filepath.Join(root, "calls.ndjson"), "")
	writeExecutableFixture(t, filepath.Join(bin, "go"), `#!/usr/bin/env bash
set -euo pipefail
jq -cn --arg tool go --args '{tool:$tool,args:$ARGS.positional}' -- "$@" >> "$SPY_LOG"
[[ "$*" == *"--mode=cpp-hotfix"* ]] || exit 41
[[ "$*" == *"plans/cpp-hotfix-2.0.1.json"* ]] || exit 42
[[ "$*" != *"--previous"* ]] || exit 43
`)
	writeExecutableFixture(t, filepath.Join(bin, "git"), `#!/usr/bin/env bash
set -euo pipefail
jq -cn --arg tool git --args '{tool:$tool,args:$ARGS.positional}' -- "$@" >> "$SPY_LOG"
case "$1" in
  rev-parse) printf '%s\n' "$SOURCE_COMMIT" ;;
  merge-base|fetch) exit 0 ;;
  *) exit 90 ;;
esac
`)
	writeExecutableFixture(t, filepath.Join(bin, "gh"), `#!/usr/bin/env bash
set -euo pipefail
jq -cn --arg tool gh --args '{tool:$tool,args:$ARGS.positional}' -- "$@" >> "$SPY_LOG"
test "$1" = api
printf '[]\n'
`)
	writeExecutableFixture(t, filepath.Join(bin, "oras"), `#!/usr/bin/env bash
set -euo pipefail
jq -cn --arg tool oras --args '{tool:$tool,args:$ARGS.positional}' -- "$@" >> "$SPY_LOG"
if [ "$1" = login ]; then cat >/dev/null; exit 0; fi
if [ "$1 $2" = "manifest fetch" ]; then
  ref=$3
  case "$SPY_SCENARIO" in
    401|403) echo "response status code $SPY_SCENARIO" >&2; exit 1 ;;
    conflict) jq -cn '{digest:("sha256:" + ("b" * 64))}'; exit 0 ;;
    ambiguous-not-found) echo 'not found' >&2; exit 1 ;;
  esac
  if record=$(jq -cer --arg ref "$ref" '.[$ref]' "$SPY_STATE"); then
    printf '%s\n' "$record"; exit 0
  fi
  echo 'response status code 404: Not Found' >&2; exit 1
fi
if [ "$1" = cp ]; then
  jq --arg ref "$3" --arg digest "${2##*@}" '.[$ref]={digest:$digest}' "$SPY_STATE" > "$SPY_STATE.next"
  mv "$SPY_STATE.next" "$SPY_STATE"; exit 0
fi
echo "unsupported no-network ORAS spy command" >&2
exit 90
`)
	script := "set -euo pipefail\n"
	for _, step := range steps {
		script += workflowRunBody(t, workflow, step) + "\n"
	}
	mustWrite(t, filepath.Join(root, "step-bodies.sh"), script)
	cmd := exec.Command("bash", filepath.Join(root, "step-bodies.sh"))
	cmd.Dir = root
	cmd.Env = append(os.Environ(),
		"PATH="+bin+":"+os.Getenv("PATH"), "SPY_LOG="+filepath.Join(root, "calls.ndjson"),
		"SPY_STATE="+filepath.Join(root, "registry.json"), "SPY_SCENARIO="+scenario,
		"REGISTRY=registry.example.invalid", "REGISTRY_USERNAME=fixture", "REGISTRY_PASSWORD=fixture-not-a-credential",
		"CANDIDATE_REGISTRY=registry.example.invalid", "CANDIDATE_USERNAME=fixture", "CANDIDATE_PASSWORD=fixture-not-a-credential",
		"RELEASE_MODE=cpp-hotfix", "DRY_RUN=false", "ADOPT_UNANNOTATED_LATEST=false",
		"SOURCE_COMMIT="+sourceCommit, "SNAPSHOT_PATH="+snapshotPath, "SNAPSHOT_SHA256="+journalSHA,
		"GITHUB_REPOSITORY=higress-group/higress", "GITHUB_STEP_SUMMARY="+filepath.Join(root, "summary.md"),
	)
	cmd.Env = append(cmd.Env, extraEnv...)
	output, runErr := cmd.CombinedOutput()
	result := hotfixStepResult{Err: runErr, Output: string(output)}
	if exitErr, ok := runErr.(*exec.ExitError); ok {
		result.ExitCode = exitErr.ExitCode()
	}
	for _, line := range strings.Split(strings.TrimSpace(string(mustRead(t, filepath.Join(root, "calls.ndjson")))), "\n") {
		if line == "" {
			continue
		}
		var call hotfixSpyCall
		if err := json.Unmarshal([]byte(line), &call); err != nil {
			t.Fatalf("invalid spy journal %q: %v", line, err)
		}
		result.Calls = append(result.Calls, call)
	}
	if data, err := os.ReadFile(journalPath); err == nil {
		if err := json.Unmarshal(data, &result.Journal); err != nil {
			t.Fatal(err)
		}
		mustWrite(t, filepath.Join(root, "version-journal.json"), string(data))
	}
	mustWrite(t, filepath.Join(root, "output.log"), string(output))
	if err := writeCanonical(filepath.Join(root, "outcome.json"), map[string]any{"exitCode": result.ExitCode, "scenario": scenario, "realRegistryWrites": 0, "evidenceKind": "isolated-step-body-spy-not-public-promotion"}); err != nil {
		t.Fatal(err)
	}
	if evidenceRoot := os.Getenv("CPP_HOTFIX_CONTRACT_EVIDENCE"); evidenceRoot != "" {
		directory := filepath.Join(evidenceRoot, strings.ReplaceAll(t.Name(), "/", "_"))
		for _, file := range []string{"calls.ndjson", "step-bodies.sh", "output.log", "outcome.json", "version-journal.json"} {
			if data, err := os.ReadFile(filepath.Join(root, file)); err == nil {
				mustWrite(t, filepath.Join(directory, file), string(data))
			}
		}
	}
	return result
}
