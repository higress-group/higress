// Copyright 2026 Higress Authors
// Licensed under the Apache License, Version 2.0.

package main

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestPluginServerWorkflowFailsClosedBeforeCandidateMutation(t *testing.T) {
	data, err := os.ReadFile("../../.github/workflows/build-plugin-server-from-snapshot.yaml")
	if err != nil {
		t.Fatal(err)
	}
	workflow := string(data)
	lookup := strings.Index(workflow, "public tag lookup failed; refusing candidate/public mutation")
	build := strings.Index(workflow, "docker buildx build --platform")
	if lookup < 0 || build < 0 || lookup > build {
		t.Fatal("public lookup must fail closed before the candidate build/push")
	}
	for _, required := range []string{
		"docker/setup-qemu-action@29109295f81e9208d7d86ff1c6c12d2833863392", "docker/setup-buildx-action@e468171a9de216ec08956ac3ada2f0791b6bd435", "oras-project/setup-oras@8d34698a59f5ffe24821f0b48ab62a3de8b64b20",
		"attestation-manifest", "linux/amd64", "linux/arm64", "org.opencontainers.image.revision",
		"snapshot-inventory.json", "jsonrpc-converter", "unmanaged-plugins.lock.json",
	} {
		if !strings.Contains(workflow, required) {
			t.Fatalf("workflow lacks required acceptance contract %q", required)
		}
	}
	buildData, err := os.ReadFile("../../.github/workflows/build-plugin-server-from-snapshot.yaml")
	if err != nil || !strings.Contains(string(buildData), "plugin-release-batches:$SNAPSHOT_SHA256") {
		t.Fatal("plugin-server build must require the immutable latest-completion marker")
	}
}

func TestPluginServerIndexContractAcceptsOnlyTwoRunnablePlatformsAndPairedAttestations(t *testing.T) {
	workflows := []string{
		"build-plugin-server-from-snapshot.yaml",
		"authorize-higress-release-tag.yaml",
		"dispatch-standalone-release.yaml",
	}
	fixtures := map[string]struct {
		json string
		want bool
	}{
		"two-runnable":         {json: `{"manifests":[{"digest":"sha256:amd","platform":{"os":"linux","architecture":"amd64"}},{"digest":"sha256:arm","platform":{"os":"linux","architecture":"arm64"}}]}`, want: true},
		"paired-attestations":  {json: `{"manifests":[{"digest":"sha256:amd","platform":{"os":"linux","architecture":"amd64"}},{"digest":"sha256:arm","platform":{"os":"linux","architecture":"arm64"}},{"mediaType":"application/vnd.oci.image.manifest.v1+json","platform":{"os":"unknown","architecture":"unknown"},"annotations":{"vnd.docker.reference.type":"attestation-manifest","vnd.docker.reference.digest":"sha256:amd"}},{"mediaType":"application/vnd.oci.image.manifest.v1+json","platform":{"os":"unknown","architecture":"unknown"},"annotations":{"vnd.docker.reference.type":"attestation-manifest","vnd.docker.reference.digest":"sha256:arm"}}]}`, want: true},
		"missing-arm64":        {json: `{"manifests":[{"digest":"sha256:amd","platform":{"os":"linux","architecture":"amd64"}}]}`},
		"unexpected-platform":  {json: `{"manifests":[{"digest":"sha256:amd","platform":{"os":"linux","architecture":"amd64"}},{"digest":"sha256:arm","platform":{"os":"linux","architecture":"arm64"}},{"digest":"sha256:ppc","platform":{"os":"linux","architecture":"ppc64le"}}]}`},
		"unpaired-attestation": {json: `{"manifests":[{"digest":"sha256:amd","platform":{"os":"linux","architecture":"amd64"}},{"digest":"sha256:arm","platform":{"os":"linux","architecture":"arm64"}},{"mediaType":"application/vnd.oci.image.manifest.v1+json","platform":{"os":"unknown","architecture":"unknown"},"annotations":{"vnd.docker.reference.type":"attestation-manifest","vnd.docker.reference.digest":"sha256:amd"}},{"mediaType":"application/vnd.oci.image.manifest.v1+json","platform":{"os":"unknown","architecture":"unknown"},"annotations":{"vnd.docker.reference.type":"attestation-manifest","vnd.docker.reference.digest":"sha256:other"}}]}`},
		"unexpected-extra":     {json: `{"manifests":[{"digest":"sha256:amd","platform":{"os":"linux","architecture":"amd64"}},{"digest":"sha256:arm","platform":{"os":"linux","architecture":"arm64"}},{"mediaType":"application/vnd.oci.image.manifest.v1+json","platform":{"os":"unknown","architecture":"unknown"},"annotations":{"vnd.docker.reference.type":"other","vnd.docker.reference.digest":"sha256:amd"}},{"mediaType":"application/vnd.oci.image.manifest.v1+json","platform":{"os":"unknown","architecture":"unknown"},"annotations":{"vnd.docker.reference.type":"attestation-manifest","vnd.docker.reference.digest":"sha256:arm"}}]}`},
	}

	var canonical string
	for _, workflow := range workflows {
		contract := workflowShellContract(t, workflow, "plugin-server-index-contract")
		if canonical == "" {
			canonical = contract
		} else if contract != canonical {
			t.Fatalf("%s does not use the canonical plugin-server index contract", workflow)
		}
		for name, fixture := range fixtures {
			t.Run(workflow+"/"+name, func(t *testing.T) {
				path := filepath.Join(t.TempDir(), "index.json")
				if err := os.WriteFile(path, []byte(fixture.json), 0o600); err != nil {
					t.Fatal(err)
				}
				cmd := exec.Command("bash", "-c", "set -euo pipefail\n"+contract+fmt.Sprintf("\nverify_plugin_server_index %q", path))
				output, err := cmd.CombinedOutput()
				if fixture.want && err != nil {
					t.Fatalf("valid plugin-server index rejected: %v\n%s", err, output)
				}
				if !fixture.want && err == nil {
					t.Fatalf("invalid plugin-server index accepted: %s", fixture.json)
				}
			})
		}
	}
}

func TestPluginServerWorkflowReusesImmutableCandidateBeforeBuild(t *testing.T) {
	data, err := os.ReadFile("../../.github/workflows/build-plugin-server-from-snapshot.yaml")
	if err != nil {
		t.Fatal(err)
	}
	workflow := string(data)
	lookup := strings.Index(workflow, `if candidate_json=$(oras manifest fetch "$candidate" --descriptor`)
	build := strings.Index(workflow, `docker buildx build --platform linux/amd64,linux/arm64`)
	acceptance := strings.Index(workflow, `verify_plugin_server_index /tmp/candidate-index.json`)
	if lookup < 0 || build < 0 || acceptance < 0 || !(lookup < build && build < acceptance) {
		t.Fatal("plugin-server workflow must reuse or build its immutable candidate before the same acceptance boundary")
	}
	for _, required := range []string{
		`candidate_digest=$(jq -er .digest <<<"$candidate_json")`,
		`Reusing immutable plugin-server candidate`,
		`candidate tag lookup was refused; authorization failure is never absence`,
		`candidate tag lookup failed; refusing candidate/public mutation`,
	} {
		if !strings.Contains(workflow, required) {
			t.Fatalf("immutable candidate retry contract lacks %q", required)
		}
	}
}

func TestReleaseWorkflowsPairORASSetupMetadataWithPinnedCLI(t *testing.T) {
	const orasSetup = "oras-project/setup-oras@8d34698a59f5ffe24821f0b48ab62a3de8b64b20 # v1.2.3"
	const orasSetupWithCLI = orasSetup + "\n        with:\n          version: 1.2.3"
	const supersededSetup = "oras-project/setup-oras@ca28077386065e263c03428f4ae0c09024817c93"

	expectedCallers := map[string]int{
		"prepare-plugin-release.yaml":            2,
		"promote-plugin-release.yaml":            2,
		"emergency-overwrite-plugin-tag.yaml":    2,
		"build-plugin-server-from-snapshot.yaml": 1,
		"authorize-higress-release-tag.yaml":     1,
		"dispatch-standalone-release.yaml":       1,
		"validate-plugin-preparation-pr.yaml":    1,
	}
	for name, expected := range expectedCallers {
		data, err := os.ReadFile("../../.github/workflows/" + name)
		if err != nil {
			t.Fatal(err)
		}
		workflow := string(data)
		if strings.Contains(workflow, supersededSetup) {
			t.Fatalf("%s retains the setup-oras v1.2.0 metadata action", name)
		}
		if got := strings.Count(workflow, "version: 1.2.3"); got != expected {
			t.Fatalf("%s has %d ORAS CLI 1.2.3 declarations, want %d", name, got, expected)
		}
		if got := strings.Count(workflow, orasSetupWithCLI); got != expected {
			t.Fatalf("%s has %d setup-oras v1.2.3 action/CLI pairs, want %d", name, got, expected)
		}
	}
}

func TestReleaseWorkflowsUseSingleReferenceORASBlobFetch(t *testing.T) {
	required := map[string][]string{
		"build-plugin-server-from-snapshot.yaml": {
			`marker_repo=${marker%:*}`,
			`oras blob fetch "$marker_repo@$marker_layer" -o /tmp/latest-batch-marker.json`,
			`managed_layer=$(oras manifest fetch "$managed_ref@$managed_digest"`,
			`oras blob fetch "$managed_ref@$managed_layer" -o /tmp/managed-oci.wasm`,
		},
		"promote-plugin-release.yaml": {
			`oras blob fetch "$marker_repo@$marker_layer" -o /tmp/existing-latest-marker.json`,
			`oras blob fetch "$marker_repo@$marker_layer" -o "/tmp/plugin-release-latest-$SNAPSHOT_SHA256.json"`,
			`oras blob fetch "$marker_repo@$marker_layer" -o /tmp/racing-latest-marker.json`,
		},
		"authorize-higress-release-tag.yaml": {
			`oras blob fetch "$console_chart_repo@$console_chart_layer" -o /tmp/console-chart-oci.tgz`,
			`oras blob fetch "$plugin_server_repo@$config" -o -`,
		},
		"dispatch-standalone-release.yaml": {
			`oras blob fetch "$repo@$config" -o -`,
		},
	}
	for name, commands := range required {
		data, err := os.ReadFile("../../.github/workflows/" + name)
		if err != nil {
			t.Fatal(err)
		}
		workflow := string(data)
		for _, command := range commands {
			if !strings.Contains(workflow, command) {
				t.Errorf("%s lacks ORAS single-reference blob fetch contract %q", name, command)
			}
		}
	}

	legacyForms := map[string][]string{
		"build-plugin-server-from-snapshot.yaml": {`oras blob fetch "$marker" "$marker_layer"`, `oras blob fetch "$managed_ref@$managed_digest"`},
		"promote-plugin-release.yaml":            {`oras blob fetch "$marker" "$marker_layer"`},
		"authorize-higress-release-tag.yaml":     {`oras blob fetch "$console_chart_repo@$CONSOLE_CHART_DIGEST"`, `oras blob fetch "$plugin_server_repo@$child"`},
		"dispatch-standalone-release.yaml":       {`oras blob fetch "$repo@$child"`},
	}
	for name, commands := range legacyForms {
		data, err := os.ReadFile("../../.github/workflows/" + name)
		if err != nil {
			t.Fatal(err)
		}
		for _, command := range commands {
			if strings.Contains(string(data), command) {
				t.Errorf("%s retains ORAS two-reference blob fetch form %q", name, command)
			}
		}
	}
}

func TestReleaseAuthorizerRequiresCompleteImmutableEvidence(t *testing.T) {
	data, err := os.ReadFile("../../.github/workflows/authorize-higress-release-tag.yaml")
	if err != nil {
		t.Fatal(err)
	}
	workflow := string(data)
	if strings.Contains(workflow, "inputs.release_evidence") || strings.Contains(workflow, "RELEASE_EVIDENCE:") {
		t.Fatal("release authorizer must not accept operator-assembled release evidence JSON")
	}
	for _, required := range []string{
		"console_release_id", "schemaVersion", "gatewayVersion", "releaseCommit", "snapshotSha256", "consoleReleaseId", "consoleReleaseTag", "consoleReleaseCommit", "consoleChartOciRef", "consoleChartDigest", "consoleChartPackageSha256", "pluginServerCommit", "pluginServerHigressCommit", "pluginServerImage", "consoleProvenanceAssetId", "snapshot-console-map.json",
		"repos/higress-group/higress-console/releases/$CONSOLE_RELEASE_ID", "git/ref/tags/$CONSOLE_RELEASE_TAG",
		"oras manifest fetch \"$console_chart_repo@$CONSOLE_CHART_DIGEST\"", "git/ref/tags/$tag", "GitHub tag lookup failed", "snapshot-console-map.json", "jq -cnS", "/tmp/release-evidence.json",
		"resolved_plugin_server_digest", `[[ "$PLUGIN_SERVER_REGISTRY" != *"://"* ]]`, "helm/higress/Chart.yaml", "helm/higress/Chart.lock", "plugin-release-provenance.json",
	} {
		if !strings.Contains(workflow, required) {
			t.Fatalf("release authorizer lacks immutable evidence contract %q", required)
		}
	}
}

func TestReleaseAuthorizerBindsPluginServerToSnapshotAncestor(t *testing.T) {
	data, err := os.ReadFile("../../.github/workflows/authorize-higress-release-tag.yaml")
	if err != nil {
		t.Fatal(err)
	}
	workflow := string(data)
	for _, required := range []string{
		`child_higress_commit=$(jq -er '."io.higress.higress-source-commit"'`,
		`test "$child_higress_commit" = "$PLUGIN_SERVER_HIGRESS_COMMIT"`,
		`git merge-base --is-ancestor "$PLUGIN_SERVER_HIGRESS_COMMIT" "$RELEASE_COMMIT"`,
		`git show "$PLUGIN_SERVER_HIGRESS_COMMIT:$SNAPSHOT_PATH"`,
		`cmp /tmp/plugin-server-source-snapshot.json "$SNAPSHOT_PATH"`,
		`--committed-source "$PLUGIN_SERVER_HIGRESS_COMMIT"`,
	} {
		if !strings.Contains(workflow, required) {
			t.Fatalf("release authorizer lacks snapshot-source ancestor contract %q", required)
		}
	}
	if strings.Contains(workflow, `--arg source "$RELEASE_COMMIT"`) {
		t.Fatal("plugin-server immutable labels must not be required to name the later release metadata commit")
	}
}

func TestReleaseAuthorizerRequiresApprovedDryRunEvidenceHash(t *testing.T) {
	data, err := os.ReadFile("../../.github/workflows/authorize-higress-release-tag.yaml")
	if err != nil {
		t.Fatal(err)
	}
	workflow := string(data)
	for _, required := range []string{
		"approved_readiness_evidence_sha256", `if [ "$DRY_RUN" = true ]`,
		`test -z "$APPROVED_EVIDENCE_SHA256"`,
		`[[ "$APPROVED_EVIDENCE_SHA256" =~ ^[0-9a-f]{64}$ ]]`,
		`test "$evidence_sha256" = "$APPROVED_EVIDENCE_SHA256"`,
		"Upload canonical pre-tag evidence",
	} {
		if !strings.Contains(workflow, required) {
			t.Fatalf("formal authorization can rebaseline dry-run evidence: missing %q", required)
		}
	}
}

func TestReleaseAuthorizerBindsHTTPSChartPackageToOCIContent(t *testing.T) {
	data, err := os.ReadFile("../../.github/workflows/authorize-higress-release-tag.yaml")
	if err != nil {
		t.Fatal(err)
	}
	workflow := string(data)
	for _, required := range []string{
		"azure/setup-helm@1a275c3b69536ee54be43f2070a358922e12c8d4",
		"console_dependency()", `test "$chart_yaml_dependency" = "$chart_lock_dependency"`,
		`helm pull higress-console --repo "$CONSOLE_HELM_REPOSITORY" --version "$CONSOLE_CHART_VERSION"`,
		`test "$(helm show chart "$CONSOLE_HTTP_CHART"`,
		`layers | length == 1`, "application/vnd.cncf.helm.chart.content.v1.tar+gzip",
		`oras blob fetch "$console_chart_repo@$console_chart_layer"`,
		`test "sha256:$CONSOLE_CHART_PACKAGE_SHA256" = "$console_chart_layer"`,
	} {
		if !strings.Contains(workflow, required) {
			t.Fatalf("Console Helm dependency is not cryptographically bound to OCI provenance: missing %q", required)
		}
	}
}

func TestPreparationBootstrapPlansFromExactHistoricalBase(t *testing.T) {
	data, err := os.ReadFile("../../.github/workflows/prepare-plugin-release.yaml")
	if err != nil {
		t.Fatal(err)
	}
	workflow := string(data)
	for _, required := range []string{
		"bootstrap_comparison_base", `[[ "$BOOTSTRAP_BASE" =~ ^[0-9a-f]{40}$ ]]`,
		`test "$(git rev-parse refs/remotes/origin/main)" = "$TARGET_REF"`,
		`git merge-base --is-ancestor "$BOOTSTRAP_BASE" "$TARGET_REF"`,
		"capture-bootstrap-evidence", "--existing-evidence /tmp/bootstrap-evidence.json",
		"--previous /tmp/bootstrap-snapshot.json", `--base "$BOOTSTRAP_BASE"`,
		"apply-plan", "Build and publish content-addressed candidates",
		// The first managed snapshot carries the explicit committed bootstrap
		// evidence marker: render receives the same evidence bytes that are
		// committed at the deterministic gateway-versioned path.
		"--bootstrap-evidence /tmp/bootstrap-evidence.json",
		`cp /tmp/bootstrap-evidence.json "plugins/release/bootstrap-evidence/$GATEWAY_VERSION.json"`,
	} {
		if !strings.Contains(workflow, required) {
			t.Fatalf("preparation bootstrap lacks first-release contract %q", required)
		}
	}
	if strings.Count(workflow, `test "$(git rev-parse refs/remotes/origin/main)" = "$TARGET_REF"`) != 2 {
		t.Fatal("preparation must verify the exact main/code-freeze commit both before planning and before opening its base-main PR")
	}
	for _, forbidden := range []string{"inputs.existing_evidence", "PRODUCTION_REGISTRY_PASSWORD", "jq -n --arg gateway"} {
		if strings.Contains(workflow, forbidden) {
			t.Fatalf("preparation bootstrap retains unsafe/empty-plan contract %q", forbidden)
		}
	}
}

func TestPreparationCandidateTagRetainsFullHashesWithinOCILimit(t *testing.T) {
	data, err := os.ReadFile("../../.github/workflows/prepare-plugin-release.yaml")
	if err != nil {
		t.Fatal(err)
	}

	var candidateAssignment string
	for _, line := range strings.Split(string(data), "\n") {
		if strings.Contains(line, `candidate="$CANDIDATE_REGISTRY/candidates/$id:`) {
			if candidateAssignment != "" {
				t.Fatal("preparation workflow constructs the candidate reference more than once")
			}
			candidateAssignment = strings.TrimSpace(line)
		}
	}
	if candidateAssignment == "" {
		t.Fatal("preparation workflow lacks the candidate reference construction")
	}

	planHash := strings.Repeat("ab", 32)
	inputHash := strings.Repeat("cd", 32)
	script := candidateAssignment + `; printf '%s' "${candidate##*:}"`
	bash, err := exec.LookPath("bash")
	if err != nil {
		t.Fatal("bash is required to execute the candidate tag construction")
	}
	cmd := exec.Command(bash, "-euo", "pipefail", "-c", script)
	cmd.Env = []string{
		"CANDIDATE_REGISTRY=registry.example.invalid:5000",
		"id=example-plugin",
		"plan_id=" + planHash,
		"input_hash=sha256:" + inputHash,
	}
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("candidate tag construction failed: %v\n%s", err, output)
	}
	tag := string(output)
	if want := planHash + inputHash; tag != want {
		t.Fatalf("candidate tag must retain the complete plan and input hashes: got %q, want %q", tag, want)
	}
	if len(tag) > 128 {
		t.Fatalf("candidate tag is %d bytes, exceeding the OCI 128-byte limit", len(tag))
	}
	if !regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9_.-]{0,127}$`).MatchString(tag) {
		t.Fatalf("candidate tag %q does not match the OCI tag grammar", tag)
	}
}

func workflowShellContract(t *testing.T, workflowName, contractName string) string {
	t.Helper()
	data, err := os.ReadFile("../../.github/workflows/" + workflowName)
	if err != nil {
		t.Fatal(err)
	}
	begin := "# BEGIN " + contractName
	end := "# END " + contractName
	start := strings.Index(string(data), begin)
	finish := strings.Index(string(data), end)
	if start < 0 || finish < 0 || start >= finish {
		t.Fatalf("%s lacks executable shell contract %s", workflowName, contractName)
	}
	start += len(begin)
	return string(data)[start:finish]
}

func workflowShellContracts(t *testing.T, workflowName, contractName string) []string {
	t.Helper()
	data, err := os.ReadFile("../../.github/workflows/" + workflowName)
	if err != nil {
		t.Fatal(err)
	}
	begin := "# BEGIN " + contractName
	end := "# END " + contractName
	remainder := string(data)
	var contracts []string
	for {
		start := strings.Index(remainder, begin)
		if start < 0 {
			break
		}
		start += len(begin)
		finish := strings.Index(remainder[start:], end)
		if finish < 0 {
			t.Fatalf("%s has an unterminated executable shell contract %s", workflowName, contractName)
		}
		contracts = append(contracts, remainder[start:start+finish])
		remainder = remainder[start+finish+len(end):]
	}
	if len(contracts) == 0 {
		t.Fatalf("%s lacks executable shell contract %s", workflowName, contractName)
	}
	return contracts
}

func writeExecutableFixture(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(contents), 0o700); err != nil {
		t.Fatal(err)
	}
}

func TestPreparationRunsOptionalGoSetupBeforeTestsAndBuild(t *testing.T) {
	workflows := []string{"prepare-plugin-release.yaml", "validate-plugin-preparation-pr.yaml"}
	var canonical string
	for _, workflow := range workflows {
		t.Run(workflow, func(t *testing.T) {
			contract := workflowShellContract(t, workflow, "plugin-build-contract")
			if canonical == "" {
				canonical = contract
			} else if contract != canonical {
				t.Fatal("formal preparation and credential-free PR rebuild must execute the same plugin build contract")
			}

			tmp := t.TempDir()
			source := filepath.Join(tmp, "plugin")
			bin := filepath.Join(tmp, "bin")
			if err := os.MkdirAll(source, 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.MkdirAll(bin, 0o755); err != nil {
				t.Fatal(err)
			}
			log := filepath.Join(tmp, "build.log")
			writeExecutableFixture(t, filepath.Join(source, "prepare.sh"), `#!/bin/sh
set -eu
printf 'prepare\n' >> "$BUILD_LOG"
`)
			writeExecutableFixture(t, filepath.Join(bin, "go"), `#!/bin/sh
set -eu
printf 'test:%s\n' "$*" >> "$BUILD_LOG"
`)
			writeExecutableFixture(t, filepath.Join(bin, "make"), `#!/bin/sh
set -eu
printf 'build:%s\n' "$*" >> "$BUILD_LOG"
`)
			script := "set -euo pipefail\n" + contract + fmt.Sprintf("\nprepare_test_and_build_plugin go %q demo\n", source)
			cmd := exec.Command("bash", "-c", script)
			cmd.Env = append(os.Environ(), "PATH="+bin+":"+os.Getenv("PATH"), "BUILD_LOG="+log)
			if output, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("extracted workflow build contract failed: %v\n%s", err, output)
			}
			got, err := os.ReadFile(log)
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != "prepare\ntest:test ./...\nbuild:-C plugins/wasm-go PLUGIN_NAME=demo build\n" {
				t.Fatalf("optional setup/test/build order = %q", got)
			}

			if err := os.WriteFile(filepath.Join(source, "prepare.sh"), []byte("#!/bin/sh\nexit 23\n"), 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(log, nil, 0o600); err != nil {
				t.Fatal(err)
			}
			cmd = exec.Command("bash", "-c", script)
			cmd.Env = append(os.Environ(), "PATH="+bin+":"+os.Getenv("PATH"), "BUILD_LOG="+log)
			if output, err := cmd.CombinedOutput(); err == nil {
				t.Fatalf("failed optional setup did not stop the workflow contract: %s", output)
			}
			if got, err := os.ReadFile(log); err != nil || len(got) != 0 {
				t.Fatalf("test/build ran after failed optional setup: %q, %v", got, err)
			}
		})
	}
}

type candidateContractResult struct {
	output      string
	diagnostics string
	log         string
	err         error
}

func runCandidatePublishContract(t *testing.T, mode string) candidateContractResult {
	t.Helper()
	return runCandidatePublishContractWithCandidate(t, mode, "registry.example.invalid:5000/candidates/demo:plan-and-input-hash")
}

func runCandidatePublishContractWithCandidate(t *testing.T, mode, candidate string) candidateContractResult {
	t.Helper()
	tmp := t.TempDir()
	bin := filepath.Join(tmp, "bin")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	log := filepath.Join(tmp, "operations.log")
	source := filepath.Join(tmp, "plugin")
	wasm := filepath.Join(tmp, "plugin.wasm")
	wasmBytes := []byte("deterministic wasm fixture")
	if err := os.MkdirAll(source, 0o755); err != nil {
		t.Fatal(err)
	}
	wasm = filepath.Join(source, "plugin.wasm")
	wasmSum := sha256.Sum256(wasmBytes)
	writeExecutableFixture(t, filepath.Join(source, "prepare.sh"), `#!/bin/sh
set -eu
printf 'prepare\n' >> "$OPERATIONS_LOG"
if [ "$ORAS_MODE" = absent-build-error ]; then exit 23; fi
`)
	writeExecutableFixture(t, filepath.Join(bin, "go"), `#!/bin/sh
set -eu
printf 'test:%s\n' "$*" >> "$OPERATIONS_LOG"
printf 'go test fixture output\n'
`)
	writeExecutableFixture(t, filepath.Join(bin, "make"), `#!/bin/sh
set -eu
printf 'build:%s\n' "$*" >> "$OPERATIONS_LOG"
printf '%s' 'deterministic wasm fixture' > "$WASM_PATH"
printf 'make fixture output\n'
`)
	writeExecutableFixture(t, filepath.Join(bin, "git"), `#!/bin/sh
set -eu
test "$1" = show
test "$2" = -s
test "$3" = --format=%cI
test "$4" = "$SOURCE_COMMIT"
printf '%s\n' "$SOURCE_CREATED"
`)
	verifier := filepath.Join(bin, "plugin-release")
	writeExecutableFixture(t, verifier, `#!/usr/bin/env bash
set -euo pipefail
printf 'verify:%s\n' "$*" >> "$OPERATIONS_LOG"
test "$1" = verify-pulled-plugin
config=''
while [ "$#" -gt 0 ]; do
  case "$1" in
    --config) config=$2; shift 2 ;;
    *) shift ;;
  esac
done
test "$(cat "$config")" = '{}'
if [ "$ORAS_MODE" = strict-verifier-error ]; then exit 29; fi
`)
	writeExecutableFixture(t, filepath.Join(bin, "oras"), `#!/usr/bin/env bash
set -euo pipefail
printf 'oras:%s\n' "$*" >> "$OPERATIONS_LOG"
if [ "$1 $2" = "manifest fetch" ]; then
  ref=$3
  if [ "${4:-}" = "--descriptor" ]; then
    case "$ORAS_MODE" in
      absent-404) echo "response status code 404: Not Found" >&2; exit 1 ;;
	  absent-manifest) echo "MANIFEST UNKNOWN" >&2; exit 1 ;;
	  absent-name) echo "name unknown" >&2; exit 1 ;;
	  absent-acr) echo "Error response from registry: $ref: not found" >&2; exit 1 ;;
	  absent-build-error|absent-push-error|absent-malformed-push) echo "response status code 404: Not Found" >&2; exit 1 ;;
      auth) echo "401 unauthorized" >&2; exit 1 ;;
      auth-401-status) echo "response status code 401" >&2; exit 1 ;;
      auth-403-http) echo "HTTP/1.1 403" >&2; exit 1 ;;
      auth-phrase-denied) echo "requested access is denied" >&2; exit 1 ;;
      auth-phrase-required) echo "authentication required" >&2; exit 1 ;;
      ambiguous) echo "not found" >&2; exit 1 ;;
      repository-missing) echo "repository does not exist" >&2; exit 1 ;;
      wrong-acr-ref) echo "Error response from registry: registry.example.invalid/candidates/other: not found" >&2; exit 1 ;;
      transport) echo "dial tcp: i/o timeout" >&2; exit 1 ;;
      transport-with-ref) echo "transport error while resolving $ref: dial tcp: i/o timeout" >&2; exit 1 ;;
      malformed-descriptor) echo '{'; exit 0 ;;
      wrong-descriptor-media) printf '{"mediaType":"application/example","digest":"%s"}\n' "$ORAS_MANIFEST_DIGEST"; exit 0 ;;
      *) printf '{"mediaType":"application/vnd.oci.image.manifest.v1+json","digest":"%s"}\n' "$ORAS_MANIFEST_DIGEST"; exit 0 ;;
    esac
  fi
  case "$ORAS_MODE" in
    manifest-error) echo "registry unavailable" >&2; exit 1 ;;
    malformed-manifest) echo '{'; exit 0 ;;
		wrong-manifest-media) manifest_media=application/example ;;
  esac
  revision=$SOURCE_COMMIT
  created=$SOURCE_CREATED
  version=$STABLE_VERSION
  input_hash=$INPUT_HASH
  oci_config_media=application/vnd.unknown.config.v1+json
  oci_config_digest=sha256:44136fa355b3678a1146ad16f7e8649e94fb4fc21fe77e8310c060f61caaff8a
  config_digest=sha256:44136fa355b3678a1146ad16f7e8649e94fb4fc21fe77e8310c060f61caaff8a
  config_media=application/vnd.module.wasm.config.v1+json
  config_title=config.json
  layer_digest=$WASM_DIGEST
	layer_media=application/vnd.module.wasm.content.layer.v1+wasm
	layer_title=plugin.wasm
	manifest_media=${manifest_media:-application/vnd.oci.image.manifest.v1+json}
	extra_layer=''
	case "$ORAS_MODE" in
	  annotation-mismatch) revision=bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb ;;
		created-mismatch) created=2026-02-03T04:05:06Z ;;
		version-mismatch) version=9.9.9 ;;
		input-hash-mismatch) input_hash=sha256:eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee ;;
		valid-different-layer) layer_digest=sha256:eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee ;;
		invalid-layer-digest) layer_digest=sha256:not-a-digest ;;
    layer-media-mismatch) layer_media=application/octet-stream ;;
    layer-count-mismatch) extra_layer=',{"mediaType":"application/octet-stream","digest":"sha256:ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff","size":9}' ;;
    oci-config-media-mismatch) oci_config_media=application/vnd.oci.empty.v1+json ;;
    oci-config-digest-mismatch) oci_config_digest=sha256:ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff ;;
    config-media-mismatch) config_media=application/octet-stream ;;
    config-digest-mismatch) config_digest=sha256:ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff ;;
    config-title-mismatch) config_title=other.json ;;
    wasm-title-mismatch) layer_title=other.wasm ;;
  esac
  if [ "${4:-}" = "--output" ]; then exec > "$5"; fi
  printf '{"schemaVersion":2,"mediaType":"%s","config":{"mediaType":"%s","digest":"%s","size":2},"annotations":{"org.opencontainers.image.created":"%s","org.opencontainers.image.revision":"%s","org.opencontainers.image.version":"%s","io.higress.plugin.input-hash":"%s"},"layers":[{"mediaType":"%s","digest":"%s","size":2,"annotations":{"org.opencontainers.image.title":"%s"}},{"mediaType":"%s","digest":"%s","size":%s,"annotations":{"org.opencontainers.image.title":"%s"}}%s]}\n' "$manifest_media" "$oci_config_media" "$oci_config_digest" "$created" "$revision" "$version" "$input_hash" "$config_media" "$config_digest" "$config_title" "$layer_media" "$layer_digest" "$WASM_SIZE" "$layer_title" "$extra_layer"
  exit 0
fi
if [ "$1" = pull ]; then
  test "$3" = --output
  mkdir -p "$4"
  if [ "$ORAS_MODE" = pulled-config-not-empty ]; then printf '%s' '{ }' > "$4/config.json"; else printf '%s' '{}' > "$4/config.json"; fi
  if [ "$ORAS_MODE" = pulled-different-layer ]; then printf '%s' 'different pulled bytes' > "$4/plugin.wasm"; else cp "$WASM_PATH" "$4/plugin.wasm"; fi
  exit 0
fi
if [ "$1" = push ]; then
	case "$ORAS_MODE" in
	  absent-push-error) echo "registry rejected candidate push" >&2; exit 1 ;;
	  absent-malformed-push) echo '{'; exit 0 ;;
	esac
  printf '{"digest":"%s"}\n' "$ORAS_PUSH_DIGEST"
  exit 0
fi
echo "unexpected fake oras invocation: $*" >&2
exit 2
`)

	const sourceCommit = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	const inputHash = "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	const manifestDigest = "sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"
	const pushDigest = "sha256:dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd"
	buildContract := workflowShellContract(t, "prepare-plugin-release.yaml", "plugin-build-contract")
	manifestContract := workflowShellContract(t, "prepare-plugin-release.yaml", "plugin-manifest-publish-contract")
	publishContract := workflowShellContract(t, "prepare-plugin-release.yaml", "candidate-publish-contract")
	script := "set -euo pipefail\n" + buildContract + manifestContract + publishContract + fmt.Sprintf("\ndigest=$(resolve_or_build_candidate %q go %q demo %q 1.2.3 %q %q)\nprintf '%%s\\n' \"$digest\"\n", candidate, source, sourceCommit, inputHash, verifier)
	cmd := exec.Command("bash", "-c", script)
	cmd.Env = append(os.Environ(),
		"PATH="+bin+":"+os.Getenv("PATH"),
		"OPERATIONS_LOG="+log,
		"WASM_PATH="+wasm,
		"ORAS_MODE="+mode,
		"ORAS_MANIFEST_DIGEST="+manifestDigest,
		"ORAS_PUSH_DIGEST="+pushDigest,
		"SOURCE_COMMIT="+sourceCommit,
		"SOURCE_CREATED=2026-01-02T03:04:05Z",
		"STABLE_VERSION=1.2.3",
		"INPUT_HASH="+inputHash,
		fmt.Sprintf("WASM_DIGEST=sha256:%x", wasmSum),
		fmt.Sprintf("WASM_SIZE=%d", len(wasmBytes)),
	)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	runErr := cmd.Run()
	logBytes, readErr := os.ReadFile(log)
	if readErr != nil && !os.IsNotExist(readErr) {
		t.Fatal(readErr)
	}
	return candidateContractResult{output: stdout.String(), diagnostics: stderr.String(), log: string(logBytes), err: runErr}
}

func TestCandidateAndEmergencyUseIdenticalDeterministicManifestContract(t *testing.T) {
	candidate := workflowShellContract(t, "prepare-plugin-release.yaml", "plugin-manifest-publish-contract")
	emergency := workflowShellContract(t, "emergency-overwrite-plugin-tag.yaml", "plugin-manifest-publish-contract")
	if candidate != emergency {
		t.Fatal("candidate and emergency publication contracts are not byte-for-byte identical")
	}

	root := t.TempDir()
	bin := filepath.Join(root, "bin")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	wasmPath := filepath.Join(root, "plugin.wasm")
	if err := os.WriteFile(wasmPath, []byte("same deterministic wasm bytes"), 0o600); err != nil {
		t.Fatal(err)
	}
	manifestPath := filepath.Join(root, "manifest.json")
	writeExecutableFixture(t, filepath.Join(bin, "git"), `#!/bin/sh
set -eu
test "$1" = show
test "$2" = -s
test "$3" = --format=%cI
test "$4" = "$SOURCE_COMMIT"
printf '%s\n' "$SOURCE_CREATED"
`)
	writeExecutableFixture(t, filepath.Join(bin, "oras"), `#!/usr/bin/env bash
set -euo pipefail
if [ "$1" = push ]; then
  test "$3" = "config.json:application/vnd.module.wasm.config.v1+json"
  test "$4" = "plugin.wasm:application/vnd.module.wasm.content.layer.v1+wasm"
  test "$5 $6" = "--image-spec v1.0"
  test "$7 $8" = "--annotation org.opencontainers.image.created=$SOURCE_CREATED"
  test "$(cat config.json)" = '{}'
  config_digest="sha256:$(sha256sum config.json | awk '{print $1}')"
  wasm_digest="sha256:$(sha256sum plugin.wasm | awk '{print $1}')"
  config_size=$(wc -c < config.json); config_size=${config_size//[[:space:]]/}
  wasm_size=$(wc -c < plugin.wasm); wasm_size=${wasm_size//[[:space:]]/}
  jq -cn --arg created "$SOURCE_CREATED" --arg revision "$SOURCE_COMMIT" --arg version "$STABLE_VERSION" --arg input "$INPUT_HASH" --arg config "$config_digest" --arg wasm "$wasm_digest" --argjson configSize "$config_size" --argjson wasmSize "$wasm_size" '{schemaVersion:2,mediaType:"application/vnd.oci.image.manifest.v1+json",config:{mediaType:"application/vnd.unknown.config.v1+json",digest:$config,size:$configSize},annotations:{"org.opencontainers.image.created":$created,"org.opencontainers.image.revision":$revision,"org.opencontainers.image.version":$version,"io.higress.plugin.input-hash":$input},layers:[{mediaType:"application/vnd.module.wasm.config.v1+json",digest:$config,size:$configSize,annotations:{"org.opencontainers.image.title":"config.json"}},{mediaType:"application/vnd.module.wasm.content.layer.v1+wasm",digest:$wasm,size:$wasmSize,annotations:{"org.opencontainers.image.title":"plugin.wasm"}}]}' > "$ORAS_MANIFEST"
  digest="sha256:$(sha256sum "$ORAS_MANIFEST" | awk '{print $1}')"
  jq -cn --arg digest "$digest" '{digest:$digest}'
  exit 0
fi
if [ "$1 $2" = "manifest fetch" ]; then
  cat "$ORAS_MANIFEST"
  exit 0
fi
echo "unexpected fake oras invocation: $*" >&2
exit 2
`)
	const sourceCommit = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	const inputHash = "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	run := func(contract, ref string) string {
		t.Helper()
		_ = os.Remove(manifestPath)
		script := "set -euo pipefail\n" + contract + fmt.Sprintf("\npublish_plugin_manifest %q %q %q 1.2.3 %q\n", ref, wasmPath, sourceCommit, inputHash)
		cmd := exec.Command("bash", "-c", script)
		cmd.Env = append(os.Environ(),
			"PATH="+bin+":"+os.Getenv("PATH"),
			"ORAS_MANIFEST="+manifestPath,
			"SOURCE_COMMIT="+sourceCommit,
			"SOURCE_CREATED=2026-01-02T03:04:05Z",
			"STABLE_VERSION=1.2.3",
			"INPUT_HASH="+inputHash,
		)
		output, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("manifest publication contract failed: %v\n%s", err, output)
		}
		return strings.TrimSpace(string(output))
	}
	candidateDigest := run(candidate, "registry.example.invalid/candidates/demo:content-addressed")
	emergencyDigest := run(emergency, "registry.example.invalid/plugins/demo:1.2.3")
	if !digestPattern.MatchString(candidateDigest) || emergencyDigest != candidateDigest {
		t.Fatalf("identical candidate/emergency inputs produced %q and %q", candidateDigest, emergencyDigest)
	}
}

func TestPreparationReusesOnlyCandidateMatchingDeterministicBuildAndPulledBlobs(t *testing.T) {
	result := runCandidatePublishContract(t, "identical")
	if result.err != nil {
		t.Fatalf("valid candidate was not reused: %v\n%s", result.err, result.diagnostics)
	}
	want := "sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc\n"
	if result.output != want {
		t.Fatalf("reused candidate digest = %q, want %q", result.output, want)
	}
	for _, required := range []string{"prepare\n", "test:test ./...\n", "build:-C plugins/wasm-go PLUGIN_NAME=demo build\n", "oras:pull ", "verify:verify-pulled-plugin "} {
		if strings.Count(result.log, required) != 1 {
			t.Fatalf("valid reuse did not execute exactly one %q operation:\n%s", required, result.log)
		}
	}
	if strings.Contains(result.log, "oras:push ") {
		t.Fatalf("valid reuse mutated the candidate tag:\n%s", result.log)
	}
	if strings.Count(result.log, "oras:manifest fetch ") != 2 || !strings.Contains(result.log, "@sha256:") {
		t.Fatalf("reuse did not resolve descriptor then immutable manifest digest:\n%s", result.log)
	}
}

func TestPreparationRejectsExistingCandidateWithDifferentOrInvalidPulledBytes(t *testing.T) {
	for _, mode := range []string{"valid-different-layer", "pulled-different-layer", "pulled-config-not-empty", "strict-verifier-error"} {
		t.Run(mode, func(t *testing.T) {
			result := runCandidatePublishContract(t, mode)
			if result.err == nil {
				t.Fatalf("poisoned candidate %s was reused: %s", mode, result.output)
			}
			if result.output != "" || strings.Contains(result.log, "oras:push ") {
				t.Fatalf("poisoned candidate %s returned evidence or mutated its tag: output=%q\n%s", mode, result.output, result.log)
			}
			for _, required := range []string{"prepare\n", "test:test ./...\n", "build:-C plugins/wasm-go PLUGIN_NAME=demo build\n"} {
				if strings.Count(result.log, required) != 1 {
					t.Fatalf("poisoned candidate %s was not compared with one deterministic source build (%q):\n%s", mode, required, result.log)
				}
			}
		})
	}
}

func TestPreparationPushesOnceOnlyForStrictlyProvenCandidateAbsence(t *testing.T) {
	for _, mode := range []string{"absent-404", "absent-manifest", "absent-name", "absent-acr"} {
		t.Run(mode, func(t *testing.T) {
			result := runCandidatePublishContract(t, mode)
			if result.err != nil {
				t.Fatalf("strict absence was not published: %v\n%s", result.err, result.output)
			}
			want := "sha256:dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd\n"
			if result.output != want {
				t.Fatalf("new candidate digest = %q, want %q", result.output, want)
			}
			for _, wantOperation := range []string{"prepare\n", "test:test ./...\n", "build:-C plugins/wasm-go PLUGIN_NAME=demo build\n"} {
				if strings.Count(result.log, wantOperation) != 1 {
					t.Fatalf("strict absence did not perform setup/test/build exactly once (%q):\n%s", wantOperation, result.log)
				}
			}
			if strings.Count(result.log, "oras:push ") != 1 {
				t.Fatalf("strict absence performed other than one push:\n%s", result.log)
			}
			previous := -1
			for _, operation := range []string{"oras:manifest fetch ", "prepare\n", "test:test ./...\n", "build:-C plugins/wasm-go PLUGIN_NAME=demo build\n", "oras:push "} {
				position := strings.Index(result.log, operation)
				if position <= previous {
					t.Fatalf("strict absence operations are not lookup -> setup -> test -> build -> push at %q:\n%s", operation, result.log)
				}
				previous = position
			}
		})
	}
}

func TestPreparationCandidateLookupAndManifestMismatchesFailWithoutMutation(t *testing.T) {
	modes := []string{
		"auth", "ambiguous", "repository-missing", "wrong-acr-ref", "transport", "malformed-descriptor", "wrong-descriptor-media",
		"manifest-error", "malformed-manifest", "wrong-manifest-media", "annotation-mismatch", "created-mismatch", "version-mismatch", "input-hash-mismatch",
		"layer-count-mismatch", "layer-media-mismatch", "invalid-layer-digest", "oci-config-media-mismatch", "oci-config-digest-mismatch", "config-media-mismatch", "config-digest-mismatch", "config-title-mismatch", "wasm-title-mismatch",
	}
	for _, mode := range modes {
		t.Run(mode, func(t *testing.T) {
			result := runCandidatePublishContract(t, mode)
			if result.err == nil {
				t.Fatalf("unsafe candidate state %s was accepted: %s", mode, result.output)
			}
			for _, forbidden := range []string{"prepare\n", "test:", "build:", "oras:push "} {
				if strings.Contains(result.log, forbidden) {
					t.Fatalf("unsafe candidate state %s performed %q:\n%s", mode, forbidden, result.log)
				}
			}
		})
	}
}

func TestPreparationCandidatePushFailuresDoNotReturnEvidence(t *testing.T) {
	for _, mode := range []string{"absent-push-error", "absent-malformed-push"} {
		t.Run(mode, func(t *testing.T) {
			result := runCandidatePublishContract(t, mode)
			if result.err == nil {
				t.Fatalf("candidate push failure %s was accepted: %s", mode, result.output)
			}
			if result.output != "" {
				t.Fatalf("candidate push failure %s returned evidence %q", mode, result.output)
			}
			if strings.Count(result.log, "oras:push ") != 1 {
				t.Fatalf("candidate push failure %s performed other than one bounded push attempt:\n%s", mode, result.log)
			}
		})
	}
}

func TestPreparationCandidateBuildFailureDoesNotPush(t *testing.T) {
	result := runCandidatePublishContract(t, "absent-build-error")
	if result.err == nil {
		t.Fatalf("candidate build failure was accepted: %s", result.output)
	}
	if result.output != "" || strings.Contains(result.log, "oras:push ") {
		t.Fatalf("candidate build failure returned evidence or pushed a candidate: output=%q\n%s", result.output, result.log)
	}
	if strings.Count(result.log, "prepare\n") != 1 || strings.Contains(result.log, "test:") || strings.Contains(result.log, "build:") {
		t.Fatalf("candidate setup failure did not stop test/build immediately:\n%s", result.log)
	}
}

func TestPreparationDoesNotReadEmbedded404FromRealCandidateTagAsHTTPAbsence(t *testing.T) {
	const aiProxyCandidate = "higress-registry.cn-hangzhou.cr.aliyuncs.com/candidates/ai-proxy:b0776c63af093544e840df5c88187a658767e51f7b0b424aca802bbcb531b351fc42f8a92ee198eacb59689be844cbeb00dab79b359b583cb404f4106fef5cee"
	result := runCandidatePublishContractWithCandidate(t, "transport-with-ref", aiProxyCandidate)
	if result.err == nil {
		t.Fatalf("transport error containing the ai-proxy tag's embedded 404 was accepted: %s", result.output)
	}
	if strings.Contains(result.log, "oras:push ") {
		t.Fatalf("transport error containing the ai-proxy tag's embedded 404 caused a push:\n%s", result.log)
	}
}

func TestPreparationClassifiesRealAIHashWithoutReadingEmbedded401AsAuthorization(t *testing.T) {
	const aiCacheCandidate = "higress-registry.cn-hangzhou.cr.aliyuncs.com/candidates/ai-cache:5a150abf7a1a76129eef4e2d61927116a59c9141f87cdfb1d4a0b0f58e049a544bf98709658a92d40b7bda75b5f458387526143e09b321a401ff7a258fdea47f"

	absent := runCandidatePublishContractWithCandidate(t, "absent-acr", aiCacheCandidate)
	if absent.err != nil {
		t.Fatalf("exact ACR absence for real ai-cache candidate was not published: %v\n%s", absent.err, absent.output)
	}
	if strings.Count(absent.log, "oras:push ") != 1 {
		t.Fatalf("exact ACR absence must perform one push:\n%s", absent.log)
	}

	transport := runCandidatePublishContractWithCandidate(t, "transport-with-ref", aiCacheCandidate)
	if transport.err == nil || strings.Contains(transport.log, "oras:push ") {
		t.Fatalf("transport error echoing real ai-cache candidate must fail without push: err=%v\n%s", transport.err, transport.log)
	}

	for _, mode := range []string{"auth-401-status", "auth-403-http", "auth", "auth-phrase-denied", "auth-phrase-required"} {
		t.Run(mode, func(t *testing.T) {
			result := runCandidatePublishContractWithCandidate(t, mode, aiCacheCandidate)
			if result.err == nil || strings.Contains(result.log, "oras:push ") {
				t.Fatalf("authorization evidence %s must fail without push: err=%v\n%s", mode, result.err, result.log)
			}
		})
	}
}

func runPromotionDescriptorContract(t *testing.T, contract, mode, ref string) (int, string) {
	t.Helper()
	tmp := t.TempDir()
	bin := filepath.Join(tmp, "bin")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	log := filepath.Join(tmp, "oras.log")
	writeExecutableFixture(t, filepath.Join(bin, "oras"), `#!/usr/bin/env bash
set -euo pipefail
printf '%s\n' "$*" >> "$ORAS_LOG"
ref=$3
case "$ORAS_MODE" in
  absent-acr) echo "Error response from registry: $ref: not found" >&2; exit 1 ;;
  transport-with-ref) echo "transport error while resolving $ref: dial tcp: i/o timeout" >&2; exit 1 ;;
  auth-401-status) echo "response status code 401" >&2; exit 1 ;;
  auth-403-http) echo "HTTP/1.1 403" >&2; exit 1 ;;
  auth-phrase) echo "authorization required" >&2; exit 1 ;;
  *) echo "unsupported fixture mode" >&2; exit 3 ;;
esac
`)
	script := "set -euo pipefail\n" + contract + fmt.Sprintf("\ndescriptor_or_absent %q\n", ref)
	cmd := exec.Command("bash", "-c", script)
	cmd.Env = append(os.Environ(), "PATH="+bin+":"+os.Getenv("PATH"), "ORAS_LOG="+log, "ORAS_MODE="+mode)
	output, err := cmd.CombinedOutput()
	status := 0
	if err != nil {
		var exitErr *exec.ExitError
		if !errors.As(err, &exitErr) {
			t.Fatalf("promotion descriptor fixture failed to execute: %v\n%s", err, output)
		}
		status = exitErr.ExitCode()
	}
	logBytes, readErr := os.ReadFile(log)
	if readErr != nil {
		t.Fatal(readErr)
	}
	return status, string(logBytes)
}

func TestPromotionDescriptorClassifiersIgnoreAuthDigitsInsideExpectedReference(t *testing.T) {
	contracts := workflowShellContracts(t, "promote-plugin-release.yaml", "promotion-descriptor-contract")
	if len(contracts) != 2 {
		t.Fatalf("promotion version/latest jobs have %d descriptor contracts, want 2", len(contracts))
	}
	const ref = "registry.example.invalid/plugins/plugin-401-and-403:hash401value403"
	for i, contract := range contracts {
		t.Run(fmt.Sprintf("classifier-%d", i+1), func(t *testing.T) {
			for _, tc := range []struct {
				mode string
				want int
			}{
				{mode: "absent-acr", want: 1},
				{mode: "transport-with-ref", want: 2},
				{mode: "auth-401-status", want: 2},
				{mode: "auth-403-http", want: 2},
				{mode: "auth-phrase", want: 2},
			} {
				t.Run(tc.mode, func(t *testing.T) {
					status, log := runPromotionDescriptorContract(t, contract, tc.mode, ref)
					if status != tc.want {
						t.Fatalf("descriptor status for %s = %d, want %d\n%s", tc.mode, status, tc.want, log)
					}
					if strings.Contains(log, "push ") || strings.Contains(log, "cp ") {
						t.Fatalf("descriptor classification mutated the registry:\n%s", log)
					}
				})
			}
		})
	}
}

func TestPromotionDescriptorClassifiersIgnore404InsideExpectedReferences(t *testing.T) {
	contracts := workflowShellContracts(t, "promote-plugin-release.yaml", "promotion-descriptor-contract")
	if len(contracts) != 2 {
		t.Fatalf("promotion version/latest jobs have %d descriptor contracts, want 2", len(contracts))
	}
	refs := map[string]string{
		"completion-marker":     "registry.example.invalid/candidates/plugin-release-batches:abc404def",
		"plugin-version-latest": "registry.example.invalid/plugins/plugin-404:hash404value",
	}
	for i, contract := range contracts {
		for name, ref := range refs {
			t.Run(fmt.Sprintf("classifier-%d/%s", i+1, name), func(t *testing.T) {
				status, log := runPromotionDescriptorContract(t, contract, "transport-with-ref", ref)
				if status != 2 {
					t.Fatalf("transport error echoing 404-containing ref returned %d, want fail-closed status 2\n%s", status, log)
				}
				if strings.Contains(log, "push ") || strings.Contains(log, "cp ") {
					t.Fatalf("transport error echoing 404-containing ref caused registry mutation:\n%s", log)
				}
			})
		}
	}
}

func TestPreparationVersionOverridesPreserveCanonicalObjectAndRejectMalformedInputs(t *testing.T) {
	data, err := os.ReadFile("../../.github/workflows/prepare-plugin-release.yaml")
	if err != nil {
		t.Fatal(err)
	}
	workflow := string(data)
	const commandPrefix = `jq `
	const filterDelimiter = ` '`
	const suffix = `' > /tmp/overrides.json`
	var validationCommand string
	for _, line := range strings.Split(workflow, "\n") {
		if strings.Contains(line, `$OVERRIDES`) && strings.Contains(line, suffix) {
			validationCommand = strings.TrimSpace(line)
			break
		}
	}
	commandStart := strings.Index(validationCommand, commandPrefix)
	if commandStart < 0 {
		t.Fatal("preparation workflow lacks canonical validate-and-return version override filter")
	}
	commandStart += len(commandPrefix)
	filterOffset := strings.Index(validationCommand[commandStart:], filterDelimiter)
	filterEnd := strings.LastIndex(validationCommand, suffix)
	if filterOffset < 0 || filterEnd < 0 {
		t.Fatal("preparation workflow does not write validated version overrides to the planner input")
	}
	flags := strings.Fields(validationCommand[commandStart : commandStart+filterOffset])
	filterStart := commandStart + filterOffset + len(filterDelimiter)
	filter := validationCommand[filterStart:filterEnd]
	if !strings.Contains(filter, `fromjson`) || strings.Contains(filter, `then true`) || !strings.Contains(filter, `then .`) {
		t.Fatal("version override validation must parse exactly one JSON input and return the validated object")
	}

	jq, err := exec.LookPath("jq")
	if err != nil {
		t.Fatal("jq is required to execute the preparation workflow contract")
	}
	run := func(input string) ([]byte, error) {
		args := append(append([]string{}, flags...), filter)
		cmd := exec.Command(jq, args...)
		cmd.Stdin = strings.NewReader(input)
		return cmd.CombinedOutput()
	}

	valid := map[string]string{
		`{}`: "{}\n",
		`{"z-plugin":"2.0.0","a_plugin":"1.2.3"}`: `{"a_plugin":"1.2.3","z-plugin":"2.0.0"}` + "\n",
	}
	for input, want := range valid {
		got, err := run(input)
		if err != nil {
			t.Fatalf("valid version overrides %s were rejected: %v\n%s", input, err, got)
		}
		if string(got) != want {
			t.Fatalf("valid version overrides %s were not preserved canonically: got %q, want %q", input, got, want)
		}
	}

	invalid := []string{
		``,
		`   `,
		`{} {}`,
		`[]`,
		`{"Unsafe/Plugin":"1.2.3"}`,
		`{"safe-plugin":1}`,
		`{"safe-plugin":"1.2.3-alpha"}`,
		`{"safe-plugin":"01.2.3"}`,
		`{"safe-plugin":`,
	}
	for _, input := range invalid {
		if output, err := run(input); err == nil {
			t.Fatalf("invalid version overrides %s were accepted as %s", input, output)
		}
	}
}

func TestPreparationPreflightsReadOnlyDeterministicCatalogPublicManifest(t *testing.T) {
	data, err := os.ReadFile("../../.github/workflows/prepare-plugin-release.yaml")
	if err != nil {
		t.Fatal(err)
	}
	workflow := string(data)
	preflightStart := strings.Index(workflow, "  preflight:\n")
	prepareStart := strings.Index(workflow, "  prepare:\n")
	bootstrap := strings.Index(workflow, "capture-bootstrap-evidence")
	if preflightStart < 0 || prepareStart < 0 || bootstrap < 0 || preflightStart > prepareStart || prepareStart > bootstrap {
		t.Fatal("read-only ORAS preflight job must run before the protected full bootstrap job")
	}
	preflight := workflow[preflightStart:prepareStart]
	if !strings.Contains(workflow[prepareStart:], "needs: preflight") {
		t.Fatal("protected preparation job must wait for the read-only preflight")
	}
	if strings.Contains(preflight, "environment:") || strings.Contains(preflight, "create-github-app-token") || strings.Contains(preflight, "oras login") {
		t.Fatal("ORAS preflight must remain read-only and outside the protected credential environment")
	}
	for _, required := range []string{
		"select(.releaseEligible)", "[$catalog.registry, .image, .sourceDir] | @tsv",
		`[ "${prerelease%%.*}" = "alpha" ]`, `attempted=$((attempted + 1))`,
		`descriptor=$(oras manifest fetch "$public_ref" --descriptor`, `jq -er '.digest | strings and test("^sha256:[0-9a-f]{64}$")'`,
		`present=$((present + 1))`, `absent=$((absent + 1))`, `authenticated_capture=0`,
		`authenticated_capture=$((authenticated_capture + 1))`,
		`auth_error=${auth_error//"$public_ref"/}`,
		`unauthorized|forbidden|denied|authentication required|authorization required`,
		`grep -Fq 'Error response from registry:'`, `grep -Fq "$public_ref: not found"`,
		"public registry preflight requires authenticated capture; authorization failure is never absence",
		"public registry preflight failed without explicit absence evidence",
		"Read-only ORAS preflight classified $attempted deterministic public references",
		"$authenticated_capture require authenticated capture",
	} {
		if !strings.Contains(preflight, required) {
			t.Fatalf("preflight lacks required read-only contract %q", required)
		}
	}
	if strings.Contains(preflight, "sort_by(.logicalId) | .[0]") || strings.Contains(preflight, "break\n") {
		t.Fatal("preflight must classify every deterministic non-alpha reference, not select or stop at the first entry")
	}
	if strings.Contains(preflight, "--descriptor --format") {
		t.Fatal("ORAS 1.2.3 descriptor preflight must not combine --descriptor and --format")
	}
	authStart := strings.Index(preflight, `auth_error=$(<"$lookup_error")`)
	absenceStart := strings.Index(preflight, "if grep -Eiq '404|manifest unknown|")
	if authStart < 0 || absenceStart < 0 || authStart > absenceStart {
		t.Fatal("uncredentialed preflight must classify authorization before explicit absence")
	}
	authBranch := preflight[authStart:absenceStart]
	if !strings.Contains(authBranch, "continue") || strings.Contains(authBranch, "exit 1") {
		t.Fatal("uncredentialed preflight must defer 401/403 to authenticated capture and continue without claiming absence")
	}
}

func TestPreparationPRUsesAppTokenForGitAndSignsGeneratedCommit(t *testing.T) {
	data, err := os.ReadFile("../../.github/workflows/prepare-plugin-release.yaml")
	if err != nil {
		t.Fatal(err)
	}
	workflow := string(data)
	renderStart := strings.Index(workflow, "      - name: Render canonical snapshot\n")
	refreshStart := strings.Index(workflow, "      - name: Refresh GitHub App token for preparation PR\n")
	stepStart := strings.Index(workflow, "      - name: Create or update deterministic preparation PR\n")
	if renderStart < 0 || refreshStart < 0 || stepStart < 0 || renderStart >= refreshStart || refreshStart >= stepStart {
		t.Fatal("preparation workflow must refresh its App token after snapshot rendering and immediately before PR publication")
	}
	checkout := workflow[strings.Index(workflow, "  prepare:\n"):renderStart]
	if !strings.Contains(checkout, "persist-credentials: false") {
		t.Fatal("preparation checkout must not persist the job-start App token across the long candidate build")
	}
	refresh := workflow[refreshStart:stepStart]
	for _, required := range []string{
		"if: ${{ !inputs.dry_run }}",
		"actions/create-github-app-token@d72941d797fd3113feb6b93fd0dec494b13a2547 # v1",
		"id: pr-app-token",
		"app-id: ${{ vars.RELEASE_PR_APP_ID }}",
		"private-key: ${{ secrets.RELEASE_PR_APP_PRIVATE_KEY }}",
		"owner: higress-group",
		"repositories: higress",
	} {
		if !strings.Contains(refresh, required) {
			t.Fatalf("preparation workflow lacks fresh post-build App-token contract %q", required)
		}
	}
	step := workflow[stepStart:]
	for _, required := range []string{
		`GH_TOKEN: ${{ steps.pr-app-token.outputs.token }}`,
		"gh auth setup-git",
		"git fetch --no-tags origin main",
		`test "$(git rev-parse refs/remotes/origin/main)" = "$TARGET_REF"`,
		`git commit -s -m "chore: prepare plugin snapshot $GATEWAY_VERSION"`,
		`git push --force-with-lease origin "$branch"`,
	} {
		if !strings.Contains(step, required) {
			t.Fatalf("preparation PR publication lacks App-authenticated/DCO contract %q", required)
		}
	}
	auth := strings.Index(step, "gh auth setup-git")
	fetch := strings.Index(step, "git fetch --no-tags origin main")
	push := strings.Index(step, `git push --force-with-lease origin "$branch"`)
	if auth < 0 || fetch < 0 || push < 0 || auth > fetch || fetch > push {
		t.Fatal("App-token Git authentication must precede exact-main fetch and deterministic branch push")
	}
	if strings.Contains(step, "x-access-token:${GH_TOKEN}") || strings.Contains(step, "x-access-token:$GH_TOKEN") {
		t.Fatal("preparation PR publication must not embed the App token in a remote URL")
	}
	if strings.Contains(step, `GH_TOKEN: ${{ steps.app-token.outputs.token }}`) {
		t.Fatal("preparation PR publication must not reuse the job-start App token after a long candidate build")
	}
}

func TestPreparationPRValidatorPinsAllActions(t *testing.T) {
	data, err := os.ReadFile("../../.github/workflows/validate-plugin-preparation-pr.yaml")
	if err != nil {
		t.Fatal(err)
	}
	workflow := string(data)
	for _, pinned := range []string{
		"actions/checkout@11d5960a326750d5838078e36cf38b85af677262 # v4",
		"actions/setup-go@40f1582b2485089dde7abd97c1529aa768e1baff # v5",
		"oras-project/setup-oras@8d34698a59f5ffe24821f0b48ab62a3de8b64b20 # v1.2.3",
	} {
		if !strings.Contains(workflow, pinned) {
			t.Fatalf("preparation PR validator lacks pinned action %q", pinned)
		}
	}
	for _, floating := range []string{"actions/checkout@v4", "actions/setup-go@v5"} {
		if strings.Contains(workflow, floating) {
			t.Fatalf("preparation PR validator retains floating action %q", floating)
		}
	}
}

func TestRegistryAbsenceShellClassifiersRequireStructuredQualifiedEvidence(t *testing.T) {
	files := map[string][]string{
		"prepare-plugin-release.yaml":            {`grep -Fq 'Error response from registry:'`, `grep -Fq "$public_ref: not found"`},
		"promote-plugin-release.yaml":            {`grep -Fq 'Error response from registry:'`, `grep -Fq "$ref: not found"`},
		"build-plugin-server-from-snapshot.yaml": {`grep -Fq 'Error response from registry:'`, `grep -Fq "$image: not found"`},
	}
	for name, required := range files {
		data, err := os.ReadFile("../../.github/workflows/" + name)
		if err != nil {
			t.Fatal(err)
		}
		workflow := string(data)
		for _, marker := range required {
			if !strings.Contains(workflow, marker) {
				t.Fatalf("%s lacks structured registry absence marker %q", name, marker)
			}
		}
		if strings.Contains(workflow, "not found|repository does not exist") {
			t.Fatalf("%s accepts generic not-found text", name)
		}
	}
}

func TestLatestPromotionMarkerSupportsFreshPartialAndCompletedRetries(t *testing.T) {
	data, err := os.ReadFile("../../.github/workflows/promote-plugin-release.yaml")
	if err != nil {
		t.Fatal(err)
	}
	workflow := string(data)
	if strings.Count(workflow, `marker="$REGISTRY/candidates/plugin-release-batches:$SNAPSHOT_SHA256"`) != 2 {
		t.Fatal("both version and latest jobs must independently define the completion marker")
	}
	for _, required := range []string{
		`phase:"version-complete"`, `phase == "latest-complete"`,
		`/tmp/plugin-release-version-$SNAPSHOT_SHA256.json`,
		`/tmp/plugin-release-latest-$SNAPSHOT_SHA256.json`,
		`no mutable latest tags were written`, `immutable latest marker conflict`,
	} {
		if !strings.Contains(workflow, required) {
			t.Fatalf("latest marker state machine lacks %q", required)
		}
	}
}

func TestLatestPromotionMarkerPushUsesRelativeLayerPath(t *testing.T) {
	data, err := os.ReadFile("../../.github/workflows/promote-plugin-release.yaml")
	if err != nil {
		t.Fatal(err)
	}
	workflow := string(data)
	for _, required := range []string{
		`marker_journal="plugin-release-latest-$SNAPSHOT_SHA256.json"`,
		`marker_digest=$(cd /tmp && oras push "$marker" "$marker_journal:application/vnd.higress.plugin-release-batch.v1+json" --format json | jq -r .digest)`,
	} {
		if !strings.Contains(workflow, required) {
			t.Fatalf("latest marker push lacks relative ORAS layer path contract %q", required)
		}
	}
	if strings.Contains(workflow, `oras push "$marker" "/tmp/plugin-release-latest-`) {
		t.Fatal("latest marker push passes an absolute layer path rejected by ORAS path validation")
	}
}

func TestPromotionBackfillsVersionTagAndJoinsMonotonicLatest(t *testing.T) {
	data, err := os.ReadFile("../../.github/workflows/promote-plugin-release.yaml")
	if err != nil {
		t.Fatal(err)
	}
	workflow := string(data)
	// The version phase iterates every snapshot entry; an absent tag is created
	// only from candidate provenance (which is how a backfill gets its stable
	// tag), an identical existing digest is accepted, and any conflict or a
	// missing carried-forward public artifact fails closed.
	for _, required := range []string{
		`done < <(jq -c '.plugins[]' "$SNAPSHOT_PATH")`,
		`select(.preflight == "absent" and .provenanceMode == "candidate")`,
		`test "$existing" = "$digest" || { echo "immutable tag conflict: $ref" >&2; exit 1; }`,
		`state=already-present`,
		`previous public artifact is missing`,
	} {
		if !strings.Contains(workflow, required) {
			t.Fatalf("version promotion lacks the backfill/idempotence contract %q", required)
		}
	}
	// Every selected stable plugin participates in the serialized monotonic
	// latest policy, including historical bootstrap backfill entries: create
	// when absent, accept an identical digest without a write, advance an older
	// reliably annotated stable version, and fail closed on an unclassifiable
	// alias, a downgrade, or a same-version digest conflict. The final
	// re-resolve rechecks the complete set, which keeps retries idempotent.
	for _, required := range []string{
		`existing latest lacks a stable version annotation`,
		`semver-compare --current "$old" --candidate "$version"`,
		`latest alias conflict`,
		`state=identical`,
		`select(.pullGate.status == "passed" and .preflight != "identical")`,
	} {
		if !strings.Contains(workflow, required) {
			t.Fatalf("latest promotion lacks the monotonic complete-set contract %q", required)
		}
	}
	if strings.Count(workflow, `done < <(jq -c '.plugins[]' "$SNAPSHOT_PATH")`) != 2 {
		t.Fatal("version and latest phases must both iterate the complete selected snapshot set")
	}
	if strings.Contains(workflow, `select(.backfill != true)`) {
		t.Fatal("backfill is provenance/migration state, never a blanket exclusion from latest")
	}
	latestDigest := strings.Index(workflow, `if [ "$old_digest" = "$digest" ]; then`)
	latestAnnotation := strings.Index(workflow, `old=$(oras manifest fetch "$latest" --format json`)
	if latestDigest < 0 || latestAnnotation < 0 || latestDigest > latestAnnotation {
		t.Fatal("latest promotion must accept an identical digest before requiring a legacy version annotation")
	}
}

func TestPromotionDryRunResolvesCandidateProvenanceWithoutMutation(t *testing.T) {
	data, err := os.ReadFile("../../.github/workflows/promote-plugin-release.yaml")
	if err != nil {
		t.Fatal(err)
	}
	workflow := string(data)
	verifyStart := strings.Index(workflow, "      - name: Verify exact snapshot provenance\n")
	dryRunBoundary := strings.Index(workflow, "      - name: Confirm dry-run boundary\n")
	if verifyStart < 0 || dryRunBoundary < 0 || verifyStart > dryRunBoundary {
		t.Fatal("promotion must verify exact provenance before reaching its dry-run boundary")
	}
	verify := workflow[verifyStart:dryRunBoundary]
	for _, required := range []string{
		`--plan "../../$plan"`, `--committed-source "$SOURCE_COMMIT"`,
		`--resolve --oci-source candidate`, `args+=(--previous "../../$previous_path")`,
	} {
		if !strings.Contains(verify, required) {
			t.Fatalf("promotion dry run lacks exact provenance resolution contract %q", required)
		}
	}
	for _, mutation := range []string{"oras login", "oras cp", "oras push"} {
		if strings.Contains(verify, mutation) {
			t.Fatalf("promotion dry-run verification contains registry mutation %q", mutation)
		}
	}
}

func TestPluginServerDryRunChecksExactBuildInputsAndCandidateProvenance(t *testing.T) {
	data, err := os.ReadFile("../../.github/workflows/build-plugin-server-from-snapshot.yaml")
	if err != nil {
		t.Fatal(err)
	}
	workflow := string(data)
	dryRunBoundary := strings.Index(workflow, "      - name: Confirm dry-run boundary\n")
	if dryRunBoundary < 0 {
		t.Fatal("plugin-server workflow lacks a dry-run boundary")
	}
	verify := workflow[:dryRunBoundary]
	for _, required := range []string{
		"oras-project/setup-oras@8d34698a59f5ffe24821f0b48ab62a3de8b64b20 # v1.2.3",
		`repository: higress-group/plugin-server`, `ref: ${{ inputs.plugin_server_commit }}`,
		`[[ "$GATEWAY_VERSION" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]]`,
		`test "$SNAPSHOT_PATH" = "plugins/release/snapshots/$GATEWAY_VERSION.json"`,
		`test "$(jq -er .gatewayVersion "$SNAPSHOT_PATH")" = "$GATEWAY_VERSION"`,
		`test "$(git -C plugin-server rev-parse HEAD)" = "$PLUGIN_SERVER_COMMIT"`,
		`(cd plugin-server && python3 -m unittest test_pull_plugins.py)`,
		`--plan "../../$plan"`, `--resolve --oci-source candidate`,
		`args+=(--previous "../../$previous_path")`,
	} {
		if !strings.Contains(verify, required) {
			t.Fatalf("plugin-server dry run lacks exact input contract %q", required)
		}
	}
	for _, mutation := range []string{"docker login", "docker buildx build", "docker buildx imagetools create", "gh api --method POST"} {
		if strings.Contains(verify, mutation) {
			t.Fatalf("plugin-server dry-run verification crosses mutation boundary with %q", mutation)
		}
	}
}

func TestReleaseAuthorizerBindsConsoleLockToCanonicalPluginServer(t *testing.T) {
	data, err := os.ReadFile("../../.github/workflows/authorize-higress-release-tag.yaml")
	if err != nil {
		t.Fatal(err)
	}
	workflow := string(data)
	image := strings.Index(workflow, `PLUGIN_SERVER_IMAGE="$plugin_server_tag@$resolved_plugin_server_digest"`)
	lock := strings.Index(workflow, `.pluginLock.pluginServerCommit == $plugin and .pluginLock.pluginServerImage == $image`)
	inspect := strings.Index(workflow, `oras manifest fetch "$PLUGIN_SERVER_IMAGE" >/tmp/plugin-server-index.json`)
	if image < 0 || lock < 0 || inspect < 0 || image > lock || lock > inspect {
		t.Fatal("release authorizer must bind the Console lock to the canonical plugin-server image before inspecting it")
	}
}

func TestReleaseAuthorizerConfiguresTaggerIdentityBeforeAnnotatedTag(t *testing.T) {
	data, err := os.ReadFile("../../.github/workflows/authorize-higress-release-tag.yaml")
	if err != nil {
		t.Fatal(err)
	}
	workflow := string(data)
	name := strings.Index(workflow, `git config user.name "higress-release-manager[bot]"`)
	email := strings.Index(workflow, `git config user.email "higress-release-manager[bot]@users.noreply.github.com"`)
	tag := strings.Index(workflow, `git tag -a "$tag" "$RELEASE_COMMIT"`)
	if name < 0 || email < 0 || tag < 0 || name > email || email > tag {
		t.Fatal("release authorizer must configure a local App tagger identity before creating the annotated tag")
	}
}

func TestReleaseWorkflowsUseORAS12ManifestOutput(t *testing.T) {
	required := map[string][]string{
		"authorize-higress-release-tag.yaml": {
			`oras manifest fetch "$PLUGIN_SERVER_IMAGE" >/tmp/plugin-server-index.json`,
		},
		"dispatch-standalone-release.yaml": {
			`oras manifest fetch "$ref" >/tmp/index.json`,
			`oras manifest fetch "$repo@$digest" >/tmp/index.json`,
		},
	}
	for name, commands := range required {
		data, err := os.ReadFile("../../.github/workflows/" + name)
		if err != nil {
			t.Fatal(err)
		}
		workflow := string(data)
		for _, command := range commands {
			if !strings.Contains(workflow, command) {
				t.Errorf("%s lacks ORAS 1.2 manifest output contract %q", name, command)
			}
		}
		if strings.Contains(workflow, "oras manifest fetch ") && regexp.MustCompile(`oras manifest fetch [^\n;]+ --raw`).MatchString(workflow) {
			t.Errorf("%s retains unsupported ORAS manifest fetch --raw", name)
		}
	}
}

func TestStandaloneDispatchBindsUniqueConsoleEvidenceToPluginServer(t *testing.T) {
	data, err := os.ReadFile("../../.github/workflows/dispatch-standalone-release.yaml")
	if err != nil {
		t.Fatal(err)
	}
	workflow := string(data)
	for _, required := range []string{
		`test "$(jq '[.assets[] | select(.name == "plugin-release-provenance.json")] | length' <<<"$console_release")" = 1`,
		`test "$(jq -er .gatewayVersion "$snapshot")" = "${TAG#v}"`,
		`SNAPSHOT_SOURCE_COMMIT=$(jq -er .sourceCommit "$snapshot")`,
		`git merge-base --is-ancestor "$SNAPSHOT_SOURCE_COMMIT" "$COMMIT"`,
		`plugin_server_image="$plugin_server_ref@$plugin_server"`,
		`PLUGIN_SERVER_COMMIT=$(jq -er .pluginLock.pluginServerCommit /tmp/console-provenance.json)`,
		`.pluginLock.sourceCommit == $sourceCommit and .pluginLock.snapshotSha256 == $snapshot`,
		`.pluginLock.pluginServerCommit == $pluginCommit and .pluginLock.pluginServerImage == $pluginImage`,
		`test "$digest" = "$plugin_server"`,
		`PLUGIN_SERVER_HIGRESS_COMMIT=""`,
		`child_higress_commit=$(jq -er '."io.higress.higress-source-commit"' <<<"$label")`,
		`test "$child_higress_commit" = "$PLUGIN_SERVER_HIGRESS_COMMIT"`,
		`git merge-base --is-ancestor "$PLUGIN_SERVER_HIGRESS_COMMIT" "$COMMIT"`,
		`git show "$PLUGIN_SERVER_HIGRESS_COMMIT:$snapshot"`,
		`cmp /tmp/plugin-server-carrier-snapshot.json "$snapshot"`,
		`jq -cn --arg ref "$ref" --arg digest "$digest"`,
		`plugin_json=$(inspect_plugin_server "$plugin_server_ref")`,
		`io.higress.higress-source-commit`,
		`org.opencontainers.image.revision`,
	} {
		if !strings.Contains(workflow, required) {
			t.Fatalf("standalone dispatch lacks exact Console/plugin-server binding %q", required)
		}
	}
	commit := strings.Index(workflow, `PLUGIN_SERVER_COMMIT=$(jq -er .pluginLock.pluginServerCommit`)
	inspect := strings.Index(workflow, `inspect_plugin_server() {`)
	if commit < 0 || inspect < 0 || commit > inspect {
		t.Fatal("standalone dispatch must derive the plugin-server commit from signed Console provenance before image inspection")
	}
	if strings.Contains(workflow, `.pluginLock.snapshotSha256 != null`) {
		t.Fatal("standalone dispatch must compare the exact snapshot hash, not merely require a non-null lock field")
	}
	if strings.Contains(workflow, `jq -cn --arg ref "$plugin_server_image"`) {
		t.Fatal("standalone evidence must keep pluginServer.ref as the gateway-version tag and carry its digest separately")
	}
	if strings.Contains(workflow, `git show "$SNAPSHOT_SOURCE_COMMIT:$snapshot"`) || strings.Contains(workflow, `--arg source "$SNAPSHOT_SOURCE_COMMIT" --arg snapshot`) {
		t.Fatal("standalone dispatch must not confuse the plugin source baseline with the later snapshot carrier commit proven by plugin-server labels")
	}
}

func TestStandaloneDispatchSurvivesGitHubTokenReleaseSuppression(t *testing.T) {
	data, err := os.ReadFile("../../.github/workflows/dispatch-standalone-release.yaml")
	if err != nil {
		t.Fatal(err)
	}
	workflow := string(data)
	for _, required := range []string{
		`workflow_run:`,
		`workflows: ["Build Docker Images and Push to Image Registry"]`,
		`github.event.workflow_run.conclusion == 'success'`,
		`github.event.workflow_run.head_branch`,
		`workflow_dispatch:`,
		`release_tag:`,
		`release=$(gh api "repos/higress-group/higress/releases/tags/$TAG")`,
		`RELEASE_ID=$(jq -er .id <<<"$release")`,
		`evidence=$(jq -n`,
		`| canonical_evidence)`,
		`evidence_sha=$(printf '%s' "$evidence" | sha256sum`,
		`key=$evidence_sha`,
		`-f "client_payload[evidence]=$evidence"`,
	} {
		if !strings.Contains(workflow, required) {
			t.Fatalf("standalone dispatch lacks release-event recovery contract %q", required)
		}
	}
	if !strings.Contains(workflow, `(github.event_name == 'workflow_run' && github.event.workflow_run.conclusion == 'success' && startsWith(github.event.workflow_run.head_branch, 'v'))`) {
		t.Fatal("standalone dispatch must reject failed or non-release image workflow runs")
	}
	if strings.Contains(workflow, `/tmp/evidence.json`) {
		t.Fatal("standalone dispatch must hash the no-newline canonical JSON string, not jq's newline-terminated file output")
	}
}

func TestStandaloneDispatchCanonicalEvidenceMatchesPythonReceiverBytes(t *testing.T) {
	contract := workflowShellContract(t, "dispatch-standalone-release.yaml", "standalone-evidence-canonical-contract")
	for _, tc := range []struct {
		name     string
		input    string
		expected string
	}{
		{name: "key-order-and-whitespace", input: ` { "z" : 2, "a" : 1 } `, expected: `{"a":1,"z":2}`},
		{name: "non-ascii", input: `{"z":"雪","a":1}`, expected: `{"a":1,"z":"\u96ea"}`},
		{name: "embedded-newline", input: "{\"line\":\"a\\nb\"}", expected: `{"line":"a\nb"}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			script := "set -euo pipefail\n" + contract + fmt.Sprintf("\nprintf %%s %q | canonical_evidence", tc.input)
			output, err := exec.Command("bash", "-c", script).CombinedOutput()
			if err != nil {
				t.Fatalf("canonical evidence contract failed: %v\n%s", err, output)
			}
			if string(output) != tc.expected {
				t.Fatalf("canonical evidence = %q, want %q", output, tc.expected)
			}
			if strings.HasSuffix(string(output), "\n") {
				t.Fatal("canonical evidence must not include a terminal newline")
			}
		})
	}
}

func TestStandaloneOSSReceiverHashesTheExactNoNewlineSenderPayload(t *testing.T) {
	data, err := os.ReadFile("../../.github/workflows/deploy-standalone-to-oss.yaml")
	if err != nil {
		t.Fatal(err)
	}
	workflow := string(data)
	for _, required := range []string{
		`canonical=$(printf '%s' "$payload" | canonical_standalone_oss_payload)`,
		`test "$(printf '%s' "$canonical" | sha256sum`,
	} {
		if !strings.Contains(workflow, required) {
			t.Fatalf("standalone OSS receiver lacks exact canonical-byte contract %q", required)
		}
	}
	if strings.Contains(workflow, `printf '%s' "$payload" | jq -cS . | sha256sum`) {
		t.Fatal("standalone OSS receiver must not hash jq's terminal newline")
	}

	contract := workflowShellContract(t, "deploy-standalone-to-oss.yaml", "standalone-oss-idempotency-canonical-contract")
	script := "set -euo pipefail\n" + contract + `
payload=$(jq -cn --argjson releaseId 370295985 --arg tag v2.2.4 --arg commit 9b83988013d80aa82527600841b1ce7c8fdb67b9 --arg archive 92e30705b8479a829b9b82ca95b90e0115b01e00f0ffcc5af30e0623b728d17c --arg installer e648f63ec13f0cbe54eb50e2aadb518df69f5df77e6926e31418d020fa3231fb '{releaseId:$releaseId,tag:$tag,commit:$commit,archiveSha256:$archive,installerSha256:$installer,dryRun:false}')
canonical=$(printf '%s' "$payload" | canonical_standalone_oss_payload)
printf '%s\n' "$(printf '%s' "$canonical" | sha256sum | awk '{print $1}')"
printf '%s\n' "$(printf '%s\n' "$canonical" | sha256sum | awk '{print $1}')"
`
	output, err := exec.Command("bash", "-c", script).CombinedOutput()
	if err != nil {
		t.Fatalf("standalone OSS canonical hash fixture failed: %v\n%s", err, output)
	}
	lines := strings.Fields(string(output))
	if len(lines) != 2 {
		t.Fatalf("standalone OSS canonical hash fixture returned %q", output)
	}
	const eventKey = "90427933289ee0646f707db301328db9d917b1fcb9bc5c293f8d6b718881f755"
	if lines[0] != eventKey {
		t.Fatalf("no-newline canonical hash = %s, want dispatched event key %s", lines[0], eventKey)
	}
	if lines[1] == eventKey {
		t.Fatal("newline-terminated jq bytes must not match the dispatched event key")
	}
}

func TestPreparationBootstrapReusesProtectedCandidateRegistryCredential(t *testing.T) {
	data, err := os.ReadFile("../../.github/workflows/prepare-plugin-release.yaml")
	if err != nil {
		t.Fatal(err)
	}
	workflow := string(data)
	for _, required := range []string{
		`CANDIDATE_REGISTRY: ${{ vars.PLUGIN_CANDIDATE_REGISTRY }}`,
		`REGISTRY_USERNAME: ${{ secrets.CANDIDATE_REGISTRY_USERNAME }}`,
		`REGISTRY_PASSWORD: ${{ secrets.CANDIDATE_REGISTRY_PASSWORD }}`,
		`[[ "$CANDIDATE_REGISTRY" =~ ^[a-z0-9][a-z0-9.-]*(\:[0-9]+)?$ ]]`,
		`test "$CANDIDATE_REGISTRY" = "$(jq -er .registry plugins/release/catalog.json)"`,
		`test -n "$REGISTRY_USERNAME"; test -n "$REGISTRY_PASSWORD"`,
	} {
		if !strings.Contains(workflow, required) {
			t.Fatalf("bootstrap capture does not reuse the protected candidate registry contract %q", required)
		}
	}
	branch := strings.Index(workflow, `if [ -z "$PREVIOUS" ]; then`)
	login := strings.Index(workflow, `echo "$REGISTRY_PASSWORD" | oras login "$CANDIDATE_REGISTRY" -u "$REGISTRY_USERNAME" --password-stdin`)
	capture := strings.Index(workflow, "capture-bootstrap-evidence")
	if branch < 0 || login < 0 || capture < 0 || branch > login || login > capture {
		t.Fatal("the existing candidate registry login must stay inside the bootstrap branch and precede read-only evidence capture")
	}
	for _, obsolete := range []string{"PLUGIN_REGISTRY_READER_USERNAME", "PLUGIN_REGISTRY_READER_PASSWORD", "PLUGIN_PUBLIC_REGISTRY"} {
		if strings.Contains(workflow, obsolete) {
			t.Fatalf("preparation must not require obsolete registry configuration %q", obsolete)
		}
	}
}

func TestPreparationPRValidationBindsMixedSnapshotToBootstrapEvidence(t *testing.T) {
	data, err := os.ReadFile("../../.github/workflows/validate-plugin-preparation-pr.yaml")
	if err != nil {
		t.Fatal(err)
	}
	workflow := string(data)
	for _, required := range []string{
		"candidate | mixed)",
		// The jq gate checks exact backfill equality between plan and snapshot
		// in both directions before the Go verifier runs.
		`(($published.backfill // false) == ($entry.backfill // false))`,
		// Bootstrap mode is explicit committed provenance: the snapshot marker
		// names the deterministic evidence file the preparation PR carries, and
		// verify-snapshot recomputes its digest and binds every claim to it.
		`marker=$(jq -r '.bootstrapEvidence.path // empty' "$snapshot")`,
		`[[ "$marker" =~ ^plugins/release/bootstrap-evidence/[0-9]+\.[0-9]+\.[0-9]+\.json$ ]]`,
		`test -f "$marker"`,
		// The snapshot is always bound to the exact committed plan; a managed
		// predecessor is bound through --previous only when no marker is
		// present (the bootstrap baseline itself is never committed).
		`--plan "../../$plan"`,
		`previous_path="plugins/release/snapshots/$previous.json"`,
		`args+=(--previous "../../$previous_path")`,
		"--oci-source candidate",
		"--oci-source public",
	} {
		if !strings.Contains(workflow, required) {
			t.Fatalf("preparation PR validation lacks the committed-evidence binding contract %q", required)
		}
	}
	// Bootstrap mode must never be inferred from a previous snapshot file that
	// the prepare workflow never commits; that inference silently skipped the
	// evidence cross-check.
	if strings.Contains(workflow, `provenanceMode // empty`) {
		t.Fatal("validation must not infer bootstrap mode from a previous snapshot provenanceMode file lookup")
	}
	if strings.Count(workflow, "verify-snapshot") != 2 {
		t.Fatal("candidate/mixed and bootstrap-public snapshots must each keep one verify-snapshot gate")
	}
	// verify-snapshot --resolve executes the oras CLI and the workflow runs the
	// release tool tests, so this required PR gate must install the pinned ORAS
	// 1.2.3 before either step: without it the resolver fails closed with
	// executable-not-found and the executable descriptor contract test would
	// silently skip instead of exercising the pinned CLI.
	orasSetup := strings.Index(workflow, "oras-project/setup-oras@8d34698a59f5ffe24821f0b48ab62a3de8b64b20 # v1.2.3")
	toolTests := strings.Index(workflow, "go test ./...")
	verify := strings.Index(workflow, "verify-snapshot")
	if orasSetup < 0 || toolTests < 0 || verify < 0 || orasSetup > toolTests || orasSetup > verify {
		t.Fatal("validation must install the pinned ORAS 1.2.3 CLI before running the release tool tests and verify-snapshot --resolve")
	}
}

func TestGatewayStableAliasesAreStagedAndImmutable(t *testing.T) {
	data, err := os.ReadFile("../../.github/workflows/build-image-and-push.yaml")
	if err != nil {
		t.Fatal(err)
	}
	workflow := string(data)
	for _, required := range []string{
		`release-build-$GITHUB_SHA`, `release-provenance-$GITHUB_SHA`,
		`immutable stable image tag conflict`, `:v$version`,
		`BUILDX_NO_DEFAULT_ATTESTATIONS=1`, `stable image tag lookup failed; refusing mutation`,
		`authorization failure is never absence`,
		`401|403|unauthorized|forbidden|denied|authentication required|authorization required`,
		`404|manifest unknown|name unknown|repository does not exist`,
		`grep -Fqx "ERROR: $ref: not found" "$err"`,
		`verify_release_staging "$staging"`,
	} {
		if !strings.Contains(workflow, required) {
			t.Fatalf("gateway release publication lacks %q", required)
		}
	}
	if strings.Contains(workflow, "not found|repository does not exist") {
		t.Fatal("generic \"not found\" text is never registry absence evidence; only explicit 404-class markers qualify")
	}
	if strings.Count(workflow, `grep -Fqx "ERROR: $ref: not found" "$err"`) != 3 {
		t.Fatal("every image publisher must recognize only the exact ACR missing-reference diagnostic")
	}
	if strings.Count(workflow, "descriptor_or_absent()") != 3 || strings.Count(workflow, "verify_release_staging()") != 3 {
		t.Fatal("every controller/pilot/gateway publisher must use its own fail-closed lookup and staging acceptance helper")
	}
}

func TestGatewayReleasePublishesProxyV2CompatibilityAlias(t *testing.T) {
	data, err := os.ReadFile("../../.github/workflows/build-image-and-push.yaml")
	if err != nil {
		t.Fatal(err)
	}
	workflow := string(data)
	for _, required := range []string{
		`gateway_stable="${IMAGES[0]%:*}:$version"`,
		`proxyv2_stable="${BUILT_IMAGE%:*}:$version"`,
		`immutable proxyv2 compatibility tag conflict`,
		`docker buildx imagetools create --tag "$proxyv2_stable" "$gateway_stable@$desired"`,
		`test "$(descriptor_or_absent "$proxyv2_stable")" = "$desired"`,
	} {
		if !strings.Contains(workflow, required) {
			t.Fatalf("gateway release publication lacks proxyv2 compatibility contract %q", required)
		}
	}
	if strings.Count(workflow, "# BEGIN gateway-proxyv2-compatibility-contract") != 1 {
		t.Fatal("gateway release publication must contain exactly one proxyv2 compatibility contract")
	}
	buildGateway := strings.Index(workflow, "make build-gateway")
	compatibility := strings.Index(workflow, "# BEGIN gateway-proxyv2-compatibility-contract")
	if buildGateway < 0 || compatibility < buildGateway {
		t.Fatal("proxyv2 compatibility alias must be derived after the gateway image publication")
	}
}

func runGatewayProxyV2PublicationContract(t *testing.T, contract, mode string, release bool) (int, string) {
	t.Helper()
	tmp := t.TempDir()
	state := filepath.Join(tmp, "created")
	log := filepath.Join(tmp, "docker.log")
	desired := "sha256:" + strings.Repeat("a", 64)
	conflict := "sha256:" + strings.Repeat("b", 64)
	script := `set -euo pipefail
descriptor_or_absent() {
  local ref=$1
  if [ "$ref" = "registry.example.invalid/higress/gateway:2.2.4" ]; then
    printf '%s' "$PUBLICATION_DESIRED"
    return 0
  fi
  case "$PUBLICATION_MODE" in
    absent)
      if [ -f "$PUBLICATION_STATE" ]; then printf '%s' "$PUBLICATION_DESIRED"; else return 1; fi
      ;;
    identical) printf '%s' "$PUBLICATION_DESIRED" ;;
    conflict) printf '%s' "$PUBLICATION_CONFLICT" ;;
    lookup-error) return 2 ;;
    *) return 3 ;;
  esac
}
docker() {
  printf '%s\n' "$*" >>"$PUBLICATION_LOG"
  touch "$PUBLICATION_STATE"
}
IMAGES=(registry.example.invalid/higress/gateway:sha-deadbeef)
BUILT_IMAGE=registry.example.invalid/higress/proxyv2:release-build-deadbeef
version=2.2.4
` + fmt.Sprintf("release_provenance=%t\n", release) + contract
	cmd := exec.Command("bash", "-c", script)
	cmd.Env = append(os.Environ(),
		"PUBLICATION_MODE="+mode,
		"PUBLICATION_STATE="+state,
		"PUBLICATION_LOG="+log,
		"PUBLICATION_DESIRED="+desired,
		"PUBLICATION_CONFLICT="+conflict,
	)
	output, err := cmd.CombinedOutput()
	status := 0
	if err != nil {
		var exitErr *exec.ExitError
		if !errors.As(err, &exitErr) {
			t.Fatalf("gateway proxyv2 publication fixture failed to execute: %v\n%s", err, output)
		}
		status = exitErr.ExitCode()
	}
	logData, readErr := os.ReadFile(log)
	if readErr != nil && !os.IsNotExist(readErr) {
		t.Fatal(readErr)
	}
	return status, string(logData)
}

func TestGatewayProxyV2PublicationContractIsIdempotentAndFailsClosed(t *testing.T) {
	contract := workflowShellContract(t, "build-image-and-push.yaml", "gateway-proxyv2-compatibility-contract")
	wantCreate := "buildx imagetools create --tag registry.example.invalid/higress/proxyv2:2.2.4 registry.example.invalid/higress/gateway:2.2.4@sha256:" + strings.Repeat("a", 64)
	for _, tc := range []struct {
		name       string
		mode       string
		release    bool
		wantStatus int
		wantLog    string
	}{
		{name: "missing creates from gateway stable digest", mode: "absent", release: true, wantLog: wantCreate + "\n"},
		{name: "existing identical alias is idempotent", mode: "identical", release: true},
		{name: "existing conflicting alias fails closed", mode: "conflict", release: true, wantStatus: 1},
		{name: "lookup errors fail closed", mode: "lookup-error", release: true, wantStatus: 2},
		{name: "non-release build does not create compatibility alias", mode: "absent"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			status, log := runGatewayProxyV2PublicationContract(t, contract, tc.mode, tc.release)
			if status != tc.wantStatus || log != tc.wantLog {
				t.Fatalf("status/log = %d/%q, want %d/%q", status, log, tc.wantStatus, tc.wantLog)
			}
		})
	}
}

func TestGatewayHelmDefaultsUsePublishedGatewayImage(t *testing.T) {
	values, err := os.ReadFile("../../helm/core/values.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(values), "image: proxyv2") {
		t.Fatal("Helm values must not advertise the legacy proxyv2 repository as a runtime default")
	}
	for _, required := range []string{
		"  proxy:\n    image: gateway\n",
		"  proxy_init:\n    # -- Base name for the proxy_init container, used to configure iptables.\n    image: gateway\n",
	} {
		if !strings.Contains(string(values), required) {
			t.Fatalf("Helm values lack gateway proxy default %q", required)
		}
	}

	readme, err := os.ReadFile("../../helm/higress/README.md")
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{
		`| global.proxy.image | string | ` + "`\"gateway\"`" + ` |`,
		`| global.proxy_init.image | string | ` + "`\"gateway\"`" + ` |`,
	} {
		if !strings.Contains(string(readme), required) {
			t.Fatalf("generated Helm documentation lacks gateway default %q", required)
		}
	}
}

func TestGatewayReleaseAliasRecoveryPromotesOnlyVerifiedStaging(t *testing.T) {
	data, err := os.ReadFile("../../.github/workflows/recover-gateway-release-images.yaml")
	if err != nil {
		t.Fatal(err)
	}
	workflow := string(data)
	for _, required := range []string{
		`workflow_dispatch:`,
		`ref: refs/tags/v${{ inputs.gateway_version }}`,
		`test "$resolved" = "$RELEASE_COMMIT"`,
		`test "$(git rev-parse HEAD)" = "$RELEASE_COMMIT"`,
		`test "$(sha256sum "$snapshot" | awk '{print $1}')" = "$SNAPSHOT_SHA256"`,
		`staging="$repo:release-provenance-$RELEASE_COMMIT"`,
		`docker buildx imagetools inspect "$staging" --raw`,
		`(.manifests | length == 2)`,
		`["linux/amd64","linux/arm64"]`,
		`grep -Fqx "ERROR: $ref: not found" "$err"`,
		`immutable release alias conflict`,
		`docker buildx imagetools create --tag "$target" "$staging@$desired"`,
		`test "$(descriptor_or_absent "$target")" = "$desired"`,
		`compat_image_name: higress/proxyv2`,
		`COMPAT_IMAGE_NAME: ${{ matrix.compat_image_name }}`,
		`compat_target="$IMAGE_REGISTRY/$COMPAT_IMAGE_NAME:$GATEWAY_VERSION"`,
		`immutable proxyv2 compatibility tag conflict`,
		`docker buildx imagetools create --tag "$compat_target" "$staging@$desired"`,
		`test "$(descriptor_or_absent "$compat_target")" = "$desired"`,
	} {
		if !strings.Contains(workflow, required) {
			t.Fatalf("gateway image recovery lacks immutable staging contract %q", required)
		}
	}
	for _, forbidden := range []string{"docker build ", "make docker", "make build-istio", "make build-gateway"} {
		if strings.Contains(workflow, forbidden) {
			t.Fatalf("gateway image recovery must never rebuild release images: found %q", forbidden)
		}
	}
	if strings.Index(workflow, `docker buildx imagetools inspect "$staging" --raw`) > strings.Index(workflow, `docker buildx imagetools create --tag "$target"`) {
		t.Fatal("gateway image recovery must validate staging before any alias mutation")
	}
}

func runGatewayProxyV2RecoveryContract(t *testing.T, contract, mode string, dryRun bool) (int, string) {
	t.Helper()
	tmp := t.TempDir()
	state := filepath.Join(tmp, "created")
	log := filepath.Join(tmp, "docker.log")
	summary := filepath.Join(tmp, "summary")
	desired := "sha256:" + strings.Repeat("a", 64)
	conflict := "sha256:" + strings.Repeat("b", 64)
	script := `set -euo pipefail
descriptor_or_absent() {
  local ref=$1
  case "$RECOVERY_MODE" in
    absent)
      if [ -f "$RECOVERY_STATE" ]; then printf '%s' "$RECOVERY_DESIRED"; else return 1; fi
      ;;
    identical) printf '%s' "$RECOVERY_DESIRED" ;;
    conflict) printf '%s' "$RECOVERY_CONFLICT" ;;
    lookup-error) return 2 ;;
    *) return 3 ;;
  esac
}
docker() {
  printf '%s\n' "$*" >>"$RECOVERY_LOG"
  touch "$RECOVERY_STATE"
}
` + contract
	cmd := exec.Command("bash", "-c", script)
	cmd.Env = append(os.Environ(),
		"RECOVERY_MODE="+mode,
		"RECOVERY_STATE="+state,
		"RECOVERY_LOG="+log,
		"RECOVERY_DESIRED="+desired,
		"RECOVERY_CONFLICT="+conflict,
		"GITHUB_STEP_SUMMARY="+summary,
		"IMAGE_REGISTRY=registry.example.invalid",
		"COMPAT_IMAGE_NAME=higress/proxyv2",
		"GATEWAY_VERSION=2.2.4",
		"DRY_RUN="+fmt.Sprint(dryRun),
		"staging=registry.example.invalid/higress/gateway:release-provenance-deadbeef",
		"desired="+desired,
	)
	output, err := cmd.CombinedOutput()
	status := 0
	if err != nil {
		var exitErr *exec.ExitError
		if !errors.As(err, &exitErr) {
			t.Fatalf("gateway proxyv2 recovery fixture failed to execute: %v\n%s", err, output)
		}
		status = exitErr.ExitCode()
	}
	logData, readErr := os.ReadFile(log)
	if readErr != nil && !os.IsNotExist(readErr) {
		t.Fatal(readErr)
	}
	return status, string(logData)
}

func TestGatewayProxyV2RecoveryContractIsIdempotentAndFailsClosed(t *testing.T) {
	contract := workflowShellContract(t, "recover-gateway-release-images.yaml", "gateway-proxyv2-recovery-contract")
	wantCreate := "buildx imagetools create --tag registry.example.invalid/higress/proxyv2:2.2.4 registry.example.invalid/higress/gateway:release-provenance-deadbeef@sha256:" + strings.Repeat("a", 64)
	for _, tc := range []struct {
		name       string
		mode       string
		dryRun     bool
		wantStatus int
		wantLog    string
	}{
		{name: "missing creates from verified staging digest", mode: "absent", wantLog: wantCreate + "\n"},
		{name: "existing identical alias is idempotent", mode: "identical"},
		{name: "existing conflicting alias fails closed", mode: "conflict", wantStatus: 1},
		{name: "lookup errors fail closed", mode: "lookup-error", wantStatus: 2},
		{name: "dry run does not mutate", mode: "absent", dryRun: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			status, log := runGatewayProxyV2RecoveryContract(t, contract, tc.mode, tc.dryRun)
			if status != tc.wantStatus || log != tc.wantLog {
				t.Fatalf("status/log = %d/%q, want %d/%q", status, log, tc.wantStatus, tc.wantLog)
			}
		})
	}
}

func runGatewayRecoveryDescriptorContract(t *testing.T, contract, mode, ref string) int {
	t.Helper()
	tmp := t.TempDir()
	bin := filepath.Join(tmp, "bin")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	writeExecutableFixture(t, filepath.Join(bin, "docker"), `#!/usr/bin/env bash
set -euo pipefail
ref=$4
case "$RECOVERY_MODE" in
  absent-acr) echo "ERROR: $ref: not found" >&2; exit 1 ;;
  absent-http-404) echo "response status code 404: Not Found" >&2; exit 1 ;;
  transport-with-ref) echo "transport error while resolving $ref: dial tcp: i/o timeout" >&2; exit 1 ;;
  auth-401-status) echo "response status code 401" >&2; exit 1 ;;
  auth-403-http) echo "HTTP/1.1 403 Forbidden" >&2; exit 1 ;;
  generic-not-found) echo "lookup failed: artifact not found" >&2; exit 1 ;;
  *) echo "unsupported fixture mode" >&2; exit 3 ;;
esac
`)
	script := "set -euo pipefail\n" + contract + fmt.Sprintf("\ndescriptor_or_absent %q\n", ref)
	cmd := exec.Command("bash", "-c", script)
	cmd.Env = append(os.Environ(), "PATH="+bin+":"+os.Getenv("PATH"), "RECOVERY_MODE="+mode)
	output, err := cmd.CombinedOutput()
	if err == nil {
		return 0
	}
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("gateway recovery descriptor fixture failed to execute: %v\n%s", err, output)
	}
	return exitErr.ExitCode()
}

func TestGatewayReleaseAliasRecoveryDescriptorClassifierFailsClosed(t *testing.T) {
	contracts := workflowShellContracts(t, "recover-gateway-release-images.yaml", "gateway-recovery-descriptor-contract")
	if len(contracts) != 1 {
		t.Fatalf("gateway recovery workflow has %d descriptor contracts, want 1", len(contracts))
	}
	const ref = "registry.example.invalid/higress/gateway:sha-401403404"
	for _, tc := range []struct {
		mode string
		want int
	}{
		{mode: "absent-acr", want: 1},
		{mode: "absent-http-404", want: 1},
		{mode: "transport-with-ref", want: 2},
		{mode: "auth-401-status", want: 2},
		{mode: "auth-403-http", want: 2},
		{mode: "generic-not-found", want: 2},
	} {
		t.Run(tc.mode, func(t *testing.T) {
			if status := runGatewayRecoveryDescriptorContract(t, contracts[0], tc.mode, ref); status != tc.want {
				t.Fatalf("descriptor status for %s = %d, want %d", tc.mode, status, tc.want)
			}
		})
	}
}
