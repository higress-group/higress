package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func managedScript(t *testing.T, name string, args ...string) ([]byte, error) {
	t.Helper()
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	return exec.Command("python3", append([]string{filepath.Join(root, "tools/hack", name)}, args...)...).CombinedOutput()
}

func managedMatrixFixture(t *testing.T) (string, Catalog) {
	t.Helper()
	root := t.TempDir()
	catalog, _, err := loadCatalog("../../plugins/release/catalog.json")
	if err != nil {
		t.Fatal(err)
	}
	for _, plugin := range catalog.Plugins {
		if plugin.Implementation == "cpp" {
			mustWrite(t, filepath.Join(root, plugin.SourceDir, "VERSION"), "3.1.4\n")
		}
	}
	for _, source := range excludedCPPSources {
		mustWrite(t, filepath.Join(root, "plugins/wasm-cpp/extensions", source, ".keep"), "")
	}
	mustWrite(t, filepath.Join(root, "plugins/wasm-cpp/Dockerfile"), string(mustRead(t, "../../plugins/wasm-cpp/Dockerfile")))
	mustWrite(t, filepath.Join(root, "plugins/wasm-cpp/.bazelversion"), "6.0.0\n")
	mustCanonical(t, filepath.Join(root, "plugins/release/catalog.json"), catalog)
	return root, catalog
}

func TestManagedCPPMatrixIsCatalogDerivedAndNeverPruned(t *testing.T) {
	for _, mutation := range []string{"unmanaged", "managed", "shared-matcher", "different-version"} {
		t.Run(mutation, func(t *testing.T) {
			root, catalog := managedMatrixFixture(t)
			if mutation == "managed" {
				for index := range catalog.Plugins {
					if catalog.Plugins[index].Implementation == "cpp" {
						catalog.Plugins[index].ReleaseEligible = true
					}
				}
				mustCanonical(t, filepath.Join(root, "plugins/release/catalog.json"), catalog)
			}
			if mutation == "shared-matcher" {
				mustWrite(t, filepath.Join(root, "plugins/wasm-cpp/common/route_rule_matcher.h"), "changed shared matcher\n")
			}
			if mutation == "different-version" {
				mustWrite(t, filepath.Join(root, "plugins/wasm-cpp/extensions/basic_auth/VERSION"), "4.5.6\n")
			}
			output, err := managedScript(t, "cpp-managed-matrix.py", "--root", root)
			if err != nil {
				t.Fatalf("matrix failed: %v %s", err, output)
			}
			var entries []struct {
				LogicalID string `json:"logicalId"`
				Source    string `json:"source"`
			}
			if err := json.Unmarshal(output, &entries); err != nil {
				t.Fatal(err)
			}
			if len(entries) != 9 {
				t.Fatalf("pruned matrix has %d entries", len(entries))
			}
			for _, entry := range entries {
				if managedCPPSources[entry.LogicalID] != entry.Source {
					t.Fatal("matrix identity is not fixed-nine")
				}
			}
		})
	}
}

func TestManagedCPPMatrixRejectsUnknownSetAndUnpinnedToolchain(t *testing.T) {
	for _, mutation := range []string{"missing", "extra", "excluded", "source", "image", "inputs", "directory", "builder", "bazel"} {
		t.Run(mutation, func(t *testing.T) {
			root, catalog := managedMatrixFixture(t)
			index := 0
			for catalog.Plugins[index].Implementation != "cpp" {
				index++
			}
			switch mutation {
			case "missing":
				catalog.Plugins = append(catalog.Plugins[:index], catalog.Plugins[index+1:]...)
			case "extra":
				extra := catalog.Plugins[index]
				extra.LogicalID = "unknown"
				catalog.Plugins = append(catalog.Plugins, extra)
			case "excluded":
				catalog.Plugins[index].LogicalID = "model-router"
			case "source":
				catalog.Plugins[index].SourceDir = "plugins/wasm-cpp/extensions/model_router"
			case "image":
				catalog.Plugins[index].Image = "plugins/other"
			case "inputs":
				catalog.Plugins[index].SharedInputGroups = nil
			case "directory":
				mustWrite(t, filepath.Join(root, "plugins/wasm-cpp/extensions/new_plugin/.keep"), "")
			case "builder":
				mustWrite(t, filepath.Join(root, "plugins/wasm-cpp/Dockerfile"), "ARG BUILDER=mutable:tag\n")
			case "bazel":
				mustWrite(t, filepath.Join(root, "plugins/wasm-cpp/.bazelversion"), "latest\n")
			}
			mustCanonical(t, filepath.Join(root, "plugins/release/catalog.json"), catalog)
			if output, err := managedScript(t, "cpp-managed-matrix.py", "--root", root); err == nil {
				t.Fatalf("invalid matrix accepted: %s", output)
			}
		})
	}
}

func TestManagedCPPEvidenceFailsClosedOnSkippedNativeMissingBytesAndWrongIdentity(t *testing.T) {
	for _, mutation := range []string{"valid", "missing-matrix", "missing-native", "skipped-native", "failed-native", "missing-bytes", "hash", "source", "tooling-head", "run", "imports"} {
		t.Run(mutation, func(t *testing.T) {
			root := t.TempDir()
			artifactSource := strings.Repeat("a", 40)
			verificationHead := strings.Repeat("b", 40)
			for _, source := range managedCPPSources {
				directory := filepath.Join(root, "artifacts", "cpp-managed-"+source+"-"+artifactSource)
				files := map[string]string{"plugin.wasm": string([]byte{0, 'a', 's', 'm', 1, 0, 0, 0}), "source-head.txt": artifactSource + "\n", "verification-head.txt": verificationHead + "\n", "native-result.txt": "success\n", "native.log": "native target passed\n", "build.log": "wasm built\n", "toolchain.txt": "pinned\n", "bazel-version.txt": "bazel 6.0.0\n", "targets.txt": "native before wasm\n"}
				hash := sha256Hex([]byte(files["plugin.wasm"]))
				imports, _ := json.Marshal(map[string]any{"sha256": hash, "size": 8, "imports": []any{}})
				files["imports.json"] = string(imports)
				metadata, _ := json.Marshal(map[string]string{"source": source, "artifactSourceCommit": artifactSource, "verificationHead": verificationHead, "runId": "123", "runAttempt": "1", "wasmSha256": hash})
				files["metadata.json"] = string(metadata)
				if source == "basic_auth" {
					switch mutation {
					case "missing-matrix":
						continue
					case "missing-native":
						delete(files, "native-result.txt")
					case "skipped-native":
						files["native-result.txt"] = "skipped\n"
					case "failed-native":
						files["native-result.txt"] = "failure\n"
					case "missing-bytes":
						delete(files, "plugin.wasm")
					case "source":
						files["source-head.txt"] = verificationHead + "\n"
					case "tooling-head":
						files["verification-head.txt"] = artifactSource + "\n"
					case "run":
						files["metadata.json"] = strings.ReplaceAll(files["metadata.json"], `"123"`, `"124"`)
					case "imports":
						files["imports.json"] = `{"sha256":"bad","size":8,"imports":[]}`
					}
				}
				var sums strings.Builder
				for filename, data := range files {
					mustWrite(t, filepath.Join(directory, filename), data)
					fmt.Fprintf(&sums, "%s  %s\n", sha256Hex([]byte(data)), filename)
				}
				mustWrite(t, filepath.Join(directory, "SHA256SUMS"), sums.String())
				if source == "basic_auth" && mutation == "hash" {
					mustWrite(t, filepath.Join(directory, "plugin.wasm"), "wrong final bytes")
				}
			}
			outputPath := filepath.Join(root, "index")
			output, err := managedScript(t, "cpp-managed-evidence.py", "--artifact-dir", filepath.Join(root, "artifacts"), "--head", artifactSource, "--verification-head", verificationHead, "--run-id", "123", "--run-attempt", "1", "--output-dir", outputPath)
			if mutation == "valid" {
				if err != nil {
					t.Fatalf("valid evidence: %v %s", err, output)
				}
			} else {
				if err == nil {
					t.Fatalf("incomplete evidence accepted: %s", output)
				}
				if _, err := os.Stat(outputPath); !os.IsNotExist(err) {
					t.Fatal("failed validation wrote an index")
				}
			}
		})
	}
}

func TestManagedCPPWorkflowPreservesNonSkippingReadOnlyRuntimeContract(t *testing.T) {
	workflow := mustWorkflow(t, "wasm-cpp-managed-verification.yaml")
	for _, required := range []string{"cpp-managed-matrix.py", "fromJSON(needs.matrix.outputs.entries)", "needs.matrix.outputs.count == '9'", "if: always()", "test \"$MATRIX_RESULT\" = success", "test \"$BUILD_RESULT\" = success", "all(.[]; .result == \"success\")", "persist-credentials: false", "--jobs=1 --test_output=errors", "steps.native.outcome == 'success'", "cpp-managed-evidence.py", "cpp-managed-run-e2e.sh", "go-version: '1.26'"} {
		if !strings.Contains(workflow, required) {
			t.Fatalf("managed workflow lacks %s", required)
		}
	}
	for _, forbidden := range []string{"VERSION\")\" = '2.0.1'", "392b64", "oras push", "oras tag", "docker push", "secrets.", "releaseEligible == true"} {
		if strings.Contains(workflow, forbidden) {
			t.Fatalf("managed workflow contains obsolete or mutating contract %s", forbidden)
		}
	}
	prepare := mustWorkflow(t, "prepare-plugin-release.yaml")
	validation := mustWorkflow(t, "validate-plugin-preparation-pr.yaml")
	for _, required := range []string{"candidate_import:", "history_import:", "--candidate-import", "capture-history-evidence", "plugins/wasm-cpp/extensions plugins/release/imports", `--committed-source "$preparation_head"`} {
		if !strings.Contains(prepare, required) {
			t.Fatalf("prepare is not wired for %s", required)
		}
	}
	for _, text := range []string{prepare, validation} {
		if !strings.Contains(text, `if jq -e '.candidateImport != null' <<<"$entry" >/dev/null; then continue; fi`) {
			t.Fatal("imported candidate can fall through into rebuild/publish")
		}
	}
	if !strings.Contains(validation, `$entry.artifactSourceCommit // $plan[0].sourceCommit`) || !strings.Contains(validation, `--committed-source "$PREPARATION_HEAD"`) {
		t.Fatal("PR validation confuses artifact source with Q")
	}
	runtime := string(mustRead(t, "../hack/cpp-managed-run-e2e.sh"))
	rollout := strings.Index(runtime, "kubectl rollout status --timeout=10m")
	firstHash := strings.Index(runtime, "\nassert_wasm_hashes\n")
	if rollout < 0 || firstHash < rollout || !strings.Contains(runtime, `.metadata.deletionTimestamp == null`) || !strings.Contains(runtime, `.ready == true`) {
		t.Fatal("runtime hashing can race deployment availability or select an unready/terminating gateway pod")
	}
}

func TestManagedCPPVerifierCallersBindPlanAndPrevious(t *testing.T) {
	authorization := mustWorkflow(t, "authorize-higress-release-tag.yaml")
	pluginServer := mustWorkflow(t, "build-plugin-server-from-snapshot.yaml")
	for _, caller := range []struct {
		name     string
		workflow string
		required []string
	}{
		{
			name:     "gateway-release-commit",
			workflow: authorization,
			required: []string{
				`test "$(git rev-parse HEAD)" = "$RELEASE_COMMIT"`,
				`previous_path="$GITHUB_WORKSPACE/plugins/release/snapshots/$previous.json"`,
				`previous_path=/tmp/release-bootstrap-previous.json`,
				`--existing-evidence "../../$marker" --output "$previous_path"`,
				`test -f "$previous_path"`,
				`verify-snapshot --root ../.. --catalog ../../plugins/release/catalog.json --snapshot "../../$SNAPSHOT_PATH" --plan "../../$plan" --previous "$previous_path" --committed-source "$RELEASE_COMMIT" --resolve --oci-source public)`,
			},
		},
		{
			name:     "gateway-plugin-server-source",
			workflow: authorization,
			required: []string{
				`ref: ${{ inputs.release_commit }}`,
				`git show "$PLUGIN_SERVER_HIGRESS_COMMIT:$SNAPSHOT_PATH"`,
				`cmp /tmp/plugin-server-source-snapshot.json "$SNAPSHOT_PATH"`,
				`verify-snapshot --root ../.. --catalog ../../plugins/release/catalog.json --snapshot "../../$SNAPSHOT_PATH" --plan "../../$plan" --previous "$previous_path" --committed-source "$PLUGIN_SERVER_HIGRESS_COMMIT" --oci-source public)`,
			},
		},
		{
			name:     "plugin-server-bootstrap",
			workflow: pluginServer,
			required: []string{
				`test "$(git rev-parse HEAD)" = "$HIGRESS_COMMIT"`,
				`args=(verify-snapshot --root ../.. --catalog ../../plugins/release/catalog.json --snapshot "../../$SNAPSHOT_PATH" --plan "../../$plan" --committed-source "$HIGRESS_COMMIT" --resolve --oci-source candidate)`,
				`if [ -z "$marker" ]; then`,
				`previous_path="plugins/release/snapshots/$previous.json"`,
				`test -f "$previous_path"`,
				`args+=(--previous "../../$previous_path")`,
				`--existing-evidence "../../$marker" --output /tmp/plugin-server-bootstrap-previous.json)`,
				`args+=(--previous /tmp/plugin-server-bootstrap-previous.json)`,
				`(cd tools/plugin-release && go run . "${args[@]}")`,
			},
		},
	} {
		t.Run(caller.name, func(t *testing.T) {
			bindings := []string{
				`test "$(jq -er .gatewayVersion "$SNAPSHOT_PATH")" = "$GATEWAY_VERSION"`,
				`plan="plugins/release/plans/$GATEWAY_VERSION.json"`,
				`test -f "$plan"`,
				`snapshot_source=$(jq -er .sourceCommit "$SNAPSHOT_PATH")`,
				`test "$(jq -er .gatewayVersion "$plan")" = "$GATEWAY_VERSION"`,
				`test "$(jq -er .sourceCommit "$plan")" = "$snapshot_source"`,
				`previous=$(jq -er .previousRelease "$SNAPSHOT_PATH")`,
				`[[ "$previous" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]]`,
				`marker=$(jq -r '.bootstrapEvidence.path // empty' "$SNAPSHOT_PATH")`,
				`test -f "$marker"`,
				`bootstrap-snapshot --root ../.. --catalog ../../plugins/release/catalog.json --gateway-version "$previous" --source "$snapshot_source" --existing-evidence "../../$marker"`,
			}
			for _, required := range append(bindings, caller.required...) {
				if !strings.Contains(caller.workflow, required) {
					t.Fatalf("verifier caller lacks plan/previous contract %q", required)
				}
			}
		})
	}
}
