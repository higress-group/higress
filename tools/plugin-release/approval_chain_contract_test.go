// Copyright 2026 Higress Authors
// Licensed under the Apache License, Version 2.0.

package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// TestApprovalChainHasExactlyTwoHumanGates proves SPEC-4634005 in the checked-in
// workflows: the candidate phase requests no environment approval, promote's
// version phase is authorized by the merged labeled preparation PR instead of an
// environment, and the latest move keeps the single independent production gate.
func TestApprovalChainHasExactlyTwoHumanGates(t *testing.T) {
	prepare := mustWorkflow(t, "prepare-plugin-release.yaml")
	if strings.Contains(prepare, "plugin-release-candidate") {
		t.Fatal("prepare still references the removed plugin-release-candidate environment")
	}
	prepareJob := workflowJobSection(t, prepare, "prepare")
	if strings.Contains(prepareJob, "environment:") {
		t.Fatal("the prepare job must not request an environment approval")
	}

	promote := mustWorkflow(t, "promote-plugin-release.yaml")
	versionJob := workflowJobSection(t, promote, "verify-and-promote")
	if strings.Contains(versionJob, "environment:") {
		t.Fatal("promote's version phase must not request an environment approval; the merged labeled preparation PR authorizes it")
	}
	latestJob := workflowJobSection(t, promote, "latest")
	if !strings.Contains(latestJob, "environment: plugin-release-production") {
		t.Fatal("the latest move lost its independent plugin-release-production gate")
	}
	if strings.Count(promote, "environment: plugin-release-production") != 1 {
		t.Fatal("promote must keep exactly one production environment gate, on the latest job")
	}
	if !strings.Contains(promote, "  pull-requests: read\n") {
		t.Fatal("promote cannot resolve the authorizing preparation PR without read-only pull-requests permission")
	}
	for _, forbidden := range []string{"pull-requests: write", "contents: write", "issues: write"} {
		if strings.Contains(promote, forbidden) {
			t.Fatalf("promote grants more than read-only permission: %q", forbidden)
		}
	}

	// Emergency overwrite self-authorizes the triggering actor through the
	// collaborators API instead of consuming the independent latest gate.
	emergency := mustWorkflow(t, "emergency-overwrite-plugin-tag.yaml")
	if strings.Contains(emergency, "environment:") {
		t.Fatal("emergency overwrite must not request an environment approval")
	}
	if !strings.Contains(emergency, "github.triggering_actor") || !strings.Contains(emergency, "collaborators/$actor/permission") || !strings.Contains(emergency, ".role_name") {
		t.Fatal("emergency overwrite must authorize the triggering actor through the collaborators API role_name")
	}
}

func TestEmergencyAuthorizationRequiresTriggeringMaintainer(t *testing.T) {
	contract := workflowShellContract(t, "emergency-overwrite-plugin-tag.yaml", "emergency-authorization-contract")
	for _, tc := range []struct {
		name       string
		actor      string
		permission string
		roleName   string
		apiFails   bool
		omitRole   bool
		want       string
		wantError  bool
	}{
		{name: "maintainer-with-legacy-write", actor: "release-maintainer", permission: "write", roleName: "maintain", want: "actor=release-maintainer role_name=maintain"},
		{name: "administrator", actor: "release-admin", permission: "admin", roleName: "admin", want: "actor=release-admin role_name=admin"},
		{name: "write-collaborator", actor: "writer", permission: "write", roleName: "write", want: "role_name=write", wantError: true},
		{name: "read-collaborator", actor: "reader", permission: "read", roleName: "read", want: "role_name=read", wantError: true},
		{name: "missing-role-name", actor: "malformed", permission: "admin", omitRole: true, want: "role_name=invalid-response", wantError: true},
		{name: "api-failure", actor: "maintainer", apiFails: true, want: "role_name=api-error", wantError: true},
		{name: "invalid-actor", actor: "bad/actor", permission: "admin", roleName: "admin", want: "role_name=invalid-actor", wantError: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			bin := filepath.Join(root, "bin")
			if err := os.MkdirAll(bin, 0o755); err != nil {
				t.Fatal(err)
			}
			writeExecutableFixture(t, filepath.Join(bin, "gh"), `#!/usr/bin/env bash
set -euo pipefail
if [ "$GH_API_FAILS" = true ]; then
  exit 23
fi
test "$1" = api
test "$2" = "repos/higress-group/higress/collaborators/$TRIGGERING_ACTOR/permission"
if [ "$OMIT_ROLE" = true ]; then
  jq -cn --arg permission "$FIXTURE_PERMISSION" '{permission:$permission}'
else
  jq -cn --arg permission "$FIXTURE_PERMISSION" --arg role_name "$FIXTURE_ROLE_NAME" '{permission:$permission,role_name:$role_name}'
fi
`)
			summary := filepath.Join(root, "summary.md")
			apiFails := strconv.FormatBool(tc.apiFails)
			omitRole := strconv.FormatBool(tc.omitRole)
			cmd := exec.Command("bash", "-c", "set -euo pipefail\n"+contract)
			cmd.Env = append(os.Environ(),
				"PATH="+bin+":"+os.Getenv("PATH"),
				"GH_TOKEN=fixture-token",
				"GH_API_FAILS="+apiFails,
				"OMIT_ROLE="+omitRole,
				"FIXTURE_PERMISSION="+tc.permission,
				"FIXTURE_ROLE_NAME="+tc.roleName,
				"GITHUB_REPOSITORY=higress-group/higress",
				"GITHUB_STEP_SUMMARY="+summary,
				"TRIGGERING_ACTOR="+tc.actor,
			)
			output, err := cmd.CombinedOutput()
			if tc.wantError && err == nil {
				t.Fatalf("unauthorized emergency actor was accepted: %s", output)
			}
			if !tc.wantError && err != nil {
				t.Fatalf("authorized emergency actor was rejected: %v\n%s", err, output)
			}
			summaryBytes, readErr := os.ReadFile(summary)
			if readErr != nil {
				t.Fatal(readErr)
			}
			if !strings.Contains(string(summaryBytes), tc.want) {
				t.Fatalf("authorization summary lacks %q: %s", tc.want, summaryBytes)
			}
			if strings.Contains(string(output), "fixture-token") || strings.Contains(string(summaryBytes), "fixture-token") {
				t.Fatal("emergency authorization leaked its GitHub token")
			}
		})
	}
}

func TestEmergencyWorkflowStagesAndBindsPublicationBeforeLatest(t *testing.T) {
	emergency := mustWorkflow(t, "emergency-overwrite-plugin-tag.yaml")
	for _, required := range []string{
		"gateway_version:",
		"required: true",
		"group: emergency-overwrite-${{ inputs.logical_id }}-${{ inputs.version }}",
		"cancel-in-progress: false",
		"ref: ${{ github.sha }}",
		`test "$WORKFLOW_REF" = refs/heads/main`,
		`git merge-base --is-ancestor "$evidence_base" refs/remotes/origin/main`,
		"go build -p 1 -o /tmp/emergency-overwrite-artifact/plugin-release",
		"git checkout -q \"$SOURCE_COMMIT\"",
		"git merge-base --is-ancestor \"$CANDIDATE_SOURCE_COMMIT\" \"$SOURCE_COMMIT\"",
		"ref: ${{ needs.validate.outputs.evidence_base }}",
		"Upload current tool and built artifact",
		`public_registry: ${{ steps.resolve.outputs.public_registry }}`,
		`public_repository: ${{ steps.resolve.outputs.public_repository }}`,
		`CONFIGURED_REGISTRY: ${{ vars.PLUGIN_PUBLIC_REGISTRY }}`,
		`validate_emergency_repository "$CONFIGURED_REGISTRY" "$REGISTRY" "$REPOSITORY" "$IMAGE_REF"`,
		`emergency_candidate="$REPOSITORY:emergency-${SOURCE_COMMIT}-${INPUT_HASH#sha256:}"`,
		"chmod 0700 \"$tool\"",
		"append-emergency-lineage",
		`if ((.lineage // []) | length) > 0 then .lineage[-1].digest else .digest end`,
		"actions/create-github-app-token@",
		`branch="release/plugin-emergency-evidence-$GATEWAY_VERSION-$LOGICAL_ID-$WORKFLOW_RUN_ID"`,
		`gh pr create --base main --head "$branch"`,
		`|| gh pr edit "$branch"`,
		"registry write precedes this PR",
		"Move latest only after evidence publication",
	} {
		if !strings.Contains(emergency, required) {
			t.Fatalf("emergency workflow lacks %q", required)
		}
	}
	buildTool := strings.Index(emergency, "go build -p 1 -o /tmp/emergency-overwrite-artifact/plugin-release")
	checkoutSource := strings.Index(emergency, "git checkout -q \"$SOURCE_COMMIT\"")
	validateEvidence := strings.Index(emergency, `candidate=$(jq -cer --arg id "$LOGICAL_ID"`)
	registryBinding := strings.Index(emergency, `validate_emergency_repository "$CONFIGURED_REGISTRY"`)
	registryLogin := strings.Index(emergency, "oras login")
	candidatePush := strings.Index(emergency, `digest=$(publish_plugin_manifest "$emergency_candidate"`)
	stageLineage := strings.Index(emergency, `--output /tmp/staged-emergency-evidence.json`)
	stableCopy := strings.Index(emergency, `oras cp "$candidate_repo@$desired" "$stable_ref"`)
	openPR := strings.Index(emergency, "gh pr create")
	latestCopy := strings.Index(emergency, `oras cp "$REPOSITORY@$DIGEST" "$REPOSITORY:latest"`)
	if buildTool < 0 || checkoutSource < 0 || validateEvidence < 0 || registryBinding < 0 || registryLogin < 0 || candidatePush < 0 || stageLineage < 0 || stableCopy < 0 || openPR < 0 || latestCopy < 0 {
		t.Fatal("emergency workflow lost a required ordering boundary")
	}
	if !(buildTool < validateEvidence && validateEvidence < checkoutSource && checkoutSource < registryBinding && registryBinding < registryLogin && registryLogin < candidatePush && candidatePush < stageLineage && stageLineage < stableCopy && stableCopy < openPR && openPR < latestCopy) {
		t.Fatal("emergency workflow must validate/build, stage the candidate and lineage, conditionally copy stable, publish evidence, then optionally move latest")
	}
}

func TestEmergencyRegistryBindingRejectsConfigurationDrift(t *testing.T) {
	contract := workflowShellContract(t, "emergency-overwrite-plugin-tag.yaml", "emergency-registry-binding-contract")
	for _, tc := range []struct {
		name       string
		configured string
		validated  string
		repository string
		image      string
		wantError  bool
	}{
		{name: "exact", configured: "registry.example.invalid:5000", validated: "registry.example.invalid:5000", repository: "registry.example.invalid:5000/plugins/demo", image: "plugins/demo"},
		{name: "registry-drift", configured: "other.example.invalid", validated: "registry.example.invalid", repository: "registry.example.invalid/plugins/demo", image: "plugins/demo", wantError: true},
		{name: "repository-drift", configured: "registry.example.invalid", validated: "registry.example.invalid", repository: "registry.example.invalid/plugins/other", image: "plugins/demo", wantError: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			script := "set -euo pipefail\n" + contract + "\nvalidate_emergency_repository \"$CONFIGURED\" \"$VALIDATED\" \"$REPOSITORY\" \"$IMAGE\"\n"
			cmd := exec.Command("bash", "-c", script)
			cmd.Env = append(os.Environ(), "CONFIGURED="+tc.configured, "VALIDATED="+tc.validated, "REPOSITORY="+tc.repository, "IMAGE="+tc.image)
			output, err := cmd.CombinedOutput()
			if tc.wantError && err == nil {
				t.Fatalf("registry mismatch was accepted: %s", output)
			}
			if !tc.wantError && err != nil {
				t.Fatalf("valid registry binding failed: %v\n%s", err, output)
			}
		})
	}
}

func TestEmergencyStableOverwriteRequiresCommittedPredecessorAndHealsRetry(t *testing.T) {
	contract := workflowShellContract(t, "emergency-overwrite-plugin-tag.yaml", "emergency-stable-overwrite-contract")
	predecessor := testDigest("predecessor")
	desired := testDigest("desired")
	conflict := testDigest("conflict")
	for _, tc := range []struct {
		name       string
		current    string
		runAttempt string
		wantError  bool
		wantCopies int
	}{
		{name: "committed-predecessor", current: predecessor, runAttempt: "1", wantCopies: 1},
		{name: "same-run-retry-at-desired", current: desired, runAttempt: "2"},
		{name: "first-attempt-cannot-claim-desired", current: desired, runAttempt: "1", wantError: true},
		{name: "predecessor-conflict", current: conflict, runAttempt: "1", wantError: true},
		{name: "missing-stable-tag", runAttempt: "1", wantError: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			bin := filepath.Join(root, "bin")
			if err := os.MkdirAll(bin, 0o755); err != nil {
				t.Fatal(err)
			}
			state := filepath.Join(root, "stable-digest")
			logPath := filepath.Join(root, "oras.log")
			if tc.current != "" {
				if err := os.WriteFile(state, []byte(tc.current), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			writeExecutableFixture(t, filepath.Join(bin, "oras"), `#!/usr/bin/env bash
set -euo pipefail
printf '%s\n' "$*" >> "$ORAS_LOG"
if [ "$1 $2" = "manifest fetch" ]; then
  if [ ! -f "$ORAS_STATE" ]; then echo "manifest unknown" >&2; exit 1; fi
  jq -cn --arg digest "$(cat "$ORAS_STATE")" '{mediaType:"application/vnd.oci.image.manifest.v1+json",digest:$digest}'
  exit 0
fi
if [ "$1" = cp ]; then
  test "$2" = "registry.example.invalid/plugins/demo@$DESIRED"
  test "$3" = "registry.example.invalid/plugins/demo:1.2.3"
  printf '%s' "$DESIRED" > "$ORAS_STATE"
  exit 0
fi
exit 2
`)
			script := "set -euo pipefail\n" + contract + "\npromote_emergency_candidate registry.example.invalid/plugins/demo:emergency-content registry.example.invalid/plugins/demo:1.2.3 \"$PREDECESSOR\" \"$DESIRED\" \"$RUN_ATTEMPT\"\n"
			cmd := exec.Command("bash", "-c", script)
			cmd.Env = append(os.Environ(),
				"PATH="+bin+":"+os.Getenv("PATH"),
				"ORAS_LOG="+logPath,
				"ORAS_STATE="+state,
				"PREDECESSOR="+predecessor,
				"DESIRED="+desired,
				"RUN_ATTEMPT="+tc.runAttempt,
			)
			output, err := cmd.CombinedOutput()
			if tc.wantError && err == nil {
				t.Fatalf("unsafe stable state was accepted: %s", output)
			}
			if !tc.wantError && err != nil {
				t.Fatalf("safe stable transition failed: %v\n%s", err, output)
			}
			logBytes, readErr := os.ReadFile(logPath)
			if readErr != nil {
				t.Fatal(readErr)
			}
			if got := strings.Count(string(logBytes), "cp "); got != tc.wantCopies {
				t.Fatalf("stable copy count = %d, want %d:\n%s", got, tc.wantCopies, logBytes)
			}
		})
	}
}

// TestEmergencyPullVerificationGatesTheStableTagOverwrite proves the incident
// channel clears the same loadability gate as the immutable pipeline before it
// may replace a public stable tag: the staged candidate is re-pulled from the
// registry by digest and verified as the registry serves it, never as the local
// build produced it, and any failure aborts before a stable-tag mutation.
func TestEmergencyPullVerificationGatesTheStableTagOverwrite(t *testing.T) {
	emergency := mustWorkflow(t, "emergency-overwrite-plugin-tag.yaml")
	contract := workflowShellContract(t, "emergency-overwrite-plugin-tag.yaml", "emergency-pull-verification-contract")
	for _, required := range []string{
		`oras manifest fetch "$repository@$digest" --output "$pull_dir/manifest.json"`,
		`oras pull "$repository@$digest" --output "$pull_dir"`,
		`"$tool" verify-pulled-plugin --manifest "$pull_dir/manifest.json" --config "$pull_dir/config.json" --wasm "$pull_dir/plugin.wasm"`,
		`--digest "$digest" --source-commit "$source_commit" --source-created "$source_created" --version "$version" --input-hash "$input_hash"`,
		"the stable tag was not modified",
	} {
		if !strings.Contains(contract, required) {
			t.Fatalf("emergency pull verification lacks %q", required)
		}
	}
	// The gate verifies registry state; the local build output is not an input.
	if strings.Contains(contract, "/tmp/emergency-overwrite-artifact/plugin.wasm") {
		t.Fatal("emergency pull verification inspected the local build output instead of the pulled artifact")
	}
	stagingPush := strings.Index(emergency, `digest=$(publish_plugin_manifest "$emergency_candidate"`)
	gate := strings.Index(emergency, `verify_emergency_pull "$tool" "$REPOSITORY" "$digest"`)
	lineage := strings.Index(emergency, `--output /tmp/staged-emergency-evidence.json`)
	stableCopy := strings.Index(emergency, `oras cp "$candidate_repo@$desired" "$stable_ref"`)
	latestCopy := strings.Index(emergency, `oras cp "$REPOSITORY@$DIGEST" "$REPOSITORY:latest"`)
	if stagingPush < 0 || gate < 0 || lineage < 0 || stableCopy < 0 || latestCopy < 0 {
		t.Fatal("emergency workflow lost a required ordering boundary")
	}
	if !(stagingPush < gate && gate < lineage && lineage < stableCopy && stableCopy < latestCopy) {
		t.Fatal("the pulled-artifact gate must run after staging the candidate and before the lineage staging, the stable-tag copy, and any latest move")
	}

	digest := testDigest("staged")
	inputHash := testDigest("inputs")
	repository := "registry.example.invalid/plugins/demo"
	for _, tc := range []struct {
		name       string
		failStep   string
		toolStatus string
		digest     string
		version    string
		inputHash  string
		commit     string
		noTool     bool
		want       string
		wantError  bool
		wantVerify bool
	}{
		{name: "staged-candidate-loads", wantVerify: true},
		{name: "verifier-rejects-artifact", toolStatus: "1", want: "failed pulled-artifact verification; the stable tag was not modified", wantError: true, wantVerify: true},
		{name: "manifest-cannot-be-pulled", failStep: "fetch", want: "manifest cannot be pulled", wantError: true},
		{name: "layers-cannot-be-pulled", failStep: "pull", want: "layers cannot be pulled", wantError: true},
		{name: "malformed-digest", digest: "sha256:short", want: "invalid staged candidate digest", wantError: true},
		{name: "malformed-version", version: "1.2", want: "invalid stable plugin version", wantError: true},
		{name: "malformed-input-hash", inputHash: "not-a-hash", want: "invalid plugin input hash", wantError: true},
		{name: "malformed-source-commit", commit: strings.Repeat("z", 40), want: "invalid plugin source commit", wantError: true},
		{name: "tool-not-executable", noTool: true, want: "release tool is not executable", wantError: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			mustRun(t, root, "git", "init", "-q")
			mustRun(t, root, "git", "config", "user.name", "test")
			mustRun(t, root, "git", "config", "user.email", "test@example.com")
			mustWrite(t, filepath.Join(root, "main.go"), "package main\n")
			mustRun(t, root, "git", "add", ".")
			mustRun(t, root, "git", "commit", "-q", "-m", "fix")
			commit, err := resolveCommit(root, "HEAD")
			if err != nil {
				t.Fatal(err)
			}
			if tc.commit != "" {
				commit = tc.commit
			}
			bin := filepath.Join(root, "bin")
			if err := os.MkdirAll(bin, 0o755); err != nil {
				t.Fatal(err)
			}
			writeExecutableFixture(t, filepath.Join(bin, "oras"), `#!/usr/bin/env bash
set -uo pipefail
printf '%s\n' "$*" >> "$ORAS_LOG"
if [ "$1 $2" = "manifest fetch" ]; then
  if [ "${FAIL_STEP:-}" = fetch ]; then echo "registry unavailable" >&2; exit 1; fi
  test "$3" = "$WANT_REF" || { echo "unexpected manifest fetch reference: $3" >&2; exit 2; }
  test "${4:-}" = "--output" || { echo "manifest fetch did not write a file: $*" >&2; exit 2; }
  printf '%s' "$MANIFEST_BODY" > "$5"
  exit 0
fi
if [ "$1" = pull ]; then
  if [ "${FAIL_STEP:-}" = pull ]; then echo "blob unknown to registry" >&2; exit 1; fi
  test "$2" = "$WANT_REF" || { echo "unexpected pull reference: $2" >&2; exit 2; }
  test "${3:-}" = "--output" || { echo "pull did not write a directory: $*" >&2; exit 2; }
  printf '%s' '{}' > "$4/config.json"
  printf 'wasm-bytes' > "$4/plugin.wasm"
  exit 0
fi
echo "unsupported fake oras invocation: $*" >&2
exit 2
`)
			tool := filepath.Join(root, "plugin-release")
			if !tc.noTool {
				writeExecutableFixture(t, tool, "#!/usr/bin/env bash\nset -uo pipefail\nprintf '%s\\n' \"$*\" >> \"$TOOL_LOG\"\nexit "+tc.toolStatus+"\n")
			} else if err := os.WriteFile(tool, []byte("not executable\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			toolLog := filepath.Join(root, "tool.log")
			orasLog := filepath.Join(root, "oras.log")
			summary := filepath.Join(root, "summary.md")
			pullDir := filepath.Join(root, "pull")
			for _, path := range []string{toolLog, orasLog} {
				if err := os.WriteFile(path, nil, 0o600); err != nil {
					t.Fatal(err)
				}
			}
			version := "1.2.3"
			if tc.version != "" {
				version = tc.version
			}
			wantDigest := digest
			if tc.digest != "" {
				wantDigest = tc.digest
			}
			wantHash := inputHash
			if tc.inputHash != "" {
				wantHash = tc.inputHash
			}
			script := "set -euo pipefail\n" + contract +
				"\nverify_emergency_pull \"$TOOL\" \"$REPOSITORY\" \"$DIGEST\" \"$SOURCE_COMMIT\" \"$VERSION\" \"$INPUT_HASH\" \"$PULL_DIR\"\n"
			cmd := exec.Command("bash", "-c", script)
			cmd.Dir = root
			cmd.Env = append(os.Environ(),
				"PATH="+bin+":"+os.Getenv("PATH"),
				"ORAS_LOG="+orasLog,
				"TOOL_LOG="+toolLog,
				"TOOL="+tool,
				"FAIL_STEP="+tc.failStep,
				"WANT_REF="+repository+"@"+digest,
				"MANIFEST_BODY={}",
				"REPOSITORY="+repository,
				"DIGEST="+wantDigest,
				"SOURCE_COMMIT="+commit,
				"VERSION="+version,
				"INPUT_HASH="+wantHash,
				"PULL_DIR="+pullDir,
				"GITHUB_STEP_SUMMARY="+summary,
			)
			output, err := cmd.CombinedOutput()
			if tc.want != "" && !strings.Contains(string(output), tc.want) {
				t.Fatalf("gate output lacks %q:\n%s", tc.want, output)
			}
			if tc.wantError {
				if err == nil {
					t.Fatalf("an unverifiable staged candidate was accepted:\n%s", output)
				}
			} else if err != nil {
				t.Fatalf("a loadable staged candidate was rejected: %v\n%s", err, output)
			}
			verifyLog, readErr := os.ReadFile(toolLog)
			if readErr != nil {
				t.Fatal(readErr)
			}
			if tc.wantVerify != strings.Contains(string(verifyLog), "verify-pulled-plugin") {
				t.Fatalf("verifier invocation = %q, want invoked %v", verifyLog, tc.wantVerify)
			}
			if tc.wantVerify {
				for _, required := range []string{
					"--manifest " + pullDir + "/manifest.json",
					"--config " + pullDir + "/config.json",
					"--wasm " + pullDir + "/plugin.wasm",
					"--digest " + digest,
					"--source-commit " + commit,
					"--version " + version,
					"--input-hash " + inputHash,
				} {
					if !strings.Contains(string(verifyLog), required) {
						t.Fatalf("verifier was not bound to the pulled artifact (%q missing): %q", required, verifyLog)
					}
				}
				if !strings.Contains(string(verifyLog), "--source-created ") {
					t.Fatalf("verifier did not receive the source commit timestamp: %q", verifyLog)
				}
			}
			orasLogBytes, readErr := os.ReadFile(orasLog)
			if readErr != nil {
				t.Fatal(readErr)
			}
			if tc.wantVerify {
				for _, required := range []string{
					"manifest fetch " + repository + "@" + digest + " --output " + pullDir + "/manifest.json",
					"pull " + repository + "@" + digest + " --output " + pullDir,
				} {
					if !strings.Contains(string(orasLogBytes), required) {
						t.Fatalf("the staged candidate was not re-pulled by digest (%q missing): %q", required, orasLogBytes)
					}
				}
			}
			for _, unexpected := range []string{"cp ", "push ", "delete "} {
				if strings.Contains(string(orasLogBytes), unexpected) {
					t.Fatalf("the pull gate mutated the registry with %q: %q", unexpected, orasLogBytes)
				}
			}
			if _, statErr := os.Stat(pullDir); !os.IsNotExist(statErr) {
				t.Fatalf("the gate left its pull directory behind: %v", statErr)
			}
		})
	}
}

// TestEmergencyStagingTagIsRetainedAsProvenance proves the retention policy:
// the emergency staging tag is never deleted (registries implement tag deletion
// as manifest deletion, which would destroy the stable tag's manifest), the
// contract performs no registry mutation, and the run summary records the
// retained provenance reference.
func TestEmergencyStagingTagIsRetainedAsProvenance(t *testing.T) {
	contract := workflowShellContract(t, "emergency-overwrite-plugin-tag.yaml", "emergency-staging-cleanup-contract")
	repository := "registry.example.invalid/plugins/demo"
	digest := testDigest("published")
	inputHash := testDigest("inputs")
	sourceCommit := strings.Repeat("a", 40)
	stagingRef := repository + ":emergency-" + sourceCommit + "-" + strings.TrimPrefix(inputHash, "sha256:")
	root := t.TempDir()
	bin := filepath.Join(root, "bin")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	writeExecutableFixture(t, filepath.Join(bin, "oras"), "#!/usr/bin/env bash\necho \"oras $*\" >> \"$ORAS_LOG\"\nexit 0\n")
	summary := filepath.Join(root, "summary.md")
	cmd := exec.Command("bash", "-c", "set -euo pipefail\n"+contract)
	cmd.Dir = root
	cmd.Env = append(os.Environ(),
		"PATH="+bin+":"+os.Getenv("PATH"),
		"ORAS_LOG="+filepath.Join(root, "oras.log"),
		"REPOSITORY="+repository,
		"VERSION=1.2.3",
		"SOURCE_COMMIT="+sourceCommit,
		"INPUT_HASH="+inputHash,
		"DIGEST="+digest,
		"GITHUB_STEP_SUMMARY="+summary,
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("retention contract failed: %v\n%s", err, output)
	}
	if logBytes, readErr := os.ReadFile(filepath.Join(root, "oras.log")); readErr == nil {
		for _, forbidden := range []string{"delete", "push ", "cp "} {
			if strings.Contains(string(logBytes), forbidden) {
				t.Fatalf("retention contract mutated the registry with %q: %q", forbidden, logBytes)
			}
		}
	}
	summaryBytes, readErr := os.ReadFile(summary)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if !strings.Contains(string(summaryBytes), stagingRef) {
		t.Fatalf("run summary does not record the retained staging reference %q: %q", stagingRef, summaryBytes)
	}
}
// TestEmergencyPublishJobReaffirmsAuthorization closes the re-run bypass: the
// job that mutates the public registry repeats the maintain/admin check as its
// first step, so resuming it under a different triggering actor cannot complete
// an overwrite nobody authorized.
func TestEmergencyPublishJobReaffirmsAuthorization(t *testing.T) {
	emergency := mustWorkflow(t, "emergency-overwrite-plugin-tag.yaml")
	contracts := workflowShellContracts(t, "emergency-overwrite-plugin-tag.yaml", "emergency-authorization-contract")
	if len(contracts) != 2 {
		t.Fatalf("emergency workflow has %d authorization contracts, want the authorize job's and the publish job's", len(contracts))
	}
	if contracts[0] != contracts[1] {
		t.Fatal("the publish job's authorization gate drifted from the authorize job's")
	}
	publish := workflowJobSection(t, emergency, "publish")
	recheck := strings.Index(publish, "# BEGIN emergency-authorization-contract")
	appToken := strings.Index(publish, "actions/create-github-app-token@")
	checkout := strings.Index(publish, "uses: actions/checkout@")
	login := strings.Index(publish, "oras login")
	stagingPush := strings.Index(publish, `digest=$(publish_plugin_manifest "$emergency_candidate"`)
	stableCopy := strings.Index(publish, `oras cp "$candidate_repo@$desired" "$stable_ref"`)
	if recheck < 0 || appToken < 0 || checkout < 0 || login < 0 || stagingPush < 0 || stableCopy < 0 {
		t.Fatal("emergency publish job lost an expected step boundary")
	}
	if !(recheck < appToken && appToken < checkout && checkout < login && login < stagingPush && stagingPush < stableCopy) {
		t.Fatal("the publish job must re-require maintain or admin permission before App-token creation, checkout, registry login, and every mutation")
	}
	if strings.Contains(publish[:recheck], "oras ") || strings.Contains(publish[:recheck], "gh api") {
		t.Fatal("the publish job performs a registry or API call before re-affirming authorization")
	}
	if !strings.Contains(publish, "TRIGGERING_ACTOR: ${{ github.triggering_actor }}") {
		t.Fatal("the publish job must authorize github.triggering_actor, the actor who resumed the run")
	}

	// The duplicated contract must really execute as a gate in the publish job.
	for _, tc := range []struct {
		name      string
		actor     string
		roleName  string
		wantError bool
	}{
		{name: "maintainer-rerun", actor: "release-maintainer", roleName: "maintain"},
		{name: "administrator-rerun", actor: "release-admin", roleName: "admin"},
		{name: "write-collaborator-rerun", actor: "writer", roleName: "write", wantError: true},
		{name: "read-collaborator-rerun", actor: "reader", roleName: "read", wantError: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			bin := filepath.Join(root, "bin")
			if err := os.MkdirAll(bin, 0o755); err != nil {
				t.Fatal(err)
			}
			writeExecutableFixture(t, filepath.Join(bin, "gh"), `#!/usr/bin/env bash
set -euo pipefail
test "$1" = api
test "$2" = "repos/higress-group/higress/collaborators/$TRIGGERING_ACTOR/permission"
jq -cn --arg role_name "$FIXTURE_ROLE_NAME" '{permission:"write",role_name:$role_name}'
`)
			summary := filepath.Join(root, "summary.md")
			cmd := exec.Command("bash", "-c", "set -euo pipefail\n"+contracts[1])
			cmd.Env = append(os.Environ(),
				"PATH="+bin+":"+os.Getenv("PATH"),
				"GH_TOKEN=fixture-token",
				"FIXTURE_ROLE_NAME="+tc.roleName,
				"GITHUB_REPOSITORY=higress-group/higress",
				"GITHUB_STEP_SUMMARY="+summary,
				"TRIGGERING_ACTOR="+tc.actor,
			)
			output, err := cmd.CombinedOutput()
			if tc.wantError && err == nil {
				t.Fatalf("an unauthorized re-run actor reached the publishing job: %s", output)
			}
			if !tc.wantError && err != nil {
				t.Fatalf("an authorized re-run actor was rejected: %v\n%s", err, output)
			}
			summaryBytes, readErr := os.ReadFile(summary)
			if readErr != nil {
				t.Fatal(readErr)
			}
			if !strings.Contains(string(summaryBytes), "actor="+tc.actor+" role_name="+tc.roleName) {
				t.Fatalf("publish-job authorization did not record its decision: %s", summaryBytes)
			}
			if strings.Contains(string(output)+string(summaryBytes), "fixture-token") {
				t.Fatal("publish-job authorization leaked its GitHub token")
			}
		})
	}
}

const (
	// releaseAppLogin is the automation identity the real preparation PRs carry;
	// the workflow reads it from vars.RELEASE_PR_APP_LOGIN, never hard-codes it.
	releaseAppLogin     = "higress-release-automation[bot]"
	releasePRMaintainer = "maintainer-one"
	releasePRNumber     = 4700
)

// prepPR describes the GitHub preparation-PR payload promote may authorize.
// defaultPrepPR fills in the real automation's PR, so each case overrides
// exactly the one property it exercises.
type prepPR struct {
	number              int
	state               string
	baseRef             string
	headRef             string
	headSHA             string
	headRepo            string
	headFork            bool
	omitHeadRepo        bool
	title               string
	author              string
	authorType          string
	labels              []string
	mergedAt            string
	mergeCommit         string
	maintainerCanModify bool
}

func defaultPrepPR(gateway, mergeCommit, headSHA string) prepPR {
	return prepPR{
		number:      releasePRNumber,
		state:       "closed",
		baseRef:     "main",
		headRef:     "release/plugin-snapshot-" + gateway,
		headSHA:     headSHA,
		headRepo:    "higress-group/higress",
		title:       "chore: prepare plugin snapshot " + gateway,
		author:      releaseAppLogin,
		authorType:  "Bot",
		labels:      []string{"release/" + gateway},
		mergedAt:    "2026-09-02T10:00:21Z",
		mergeCommit: mergeCommit,
	}
}

func (p prepPR) payload() map[string]any {
	var mergedAt, mergeCommit any
	if p.mergedAt != "" {
		mergedAt = p.mergedAt
	}
	if p.mergeCommit != "" {
		mergeCommit = p.mergeCommit
	}
	labels := make([]map[string]any, 0, len(p.labels))
	for _, label := range p.labels {
		labels = append(labels, map[string]any{"name": label})
	}
	head := map[string]any{"ref": p.headRef, "sha": p.headSHA}
	if !p.omitHeadRepo {
		head["repo"] = map[string]any{"full_name": p.headRepo, "fork": p.headFork}
	}
	return map[string]any{
		"number":                p.number,
		"state":                 p.state,
		"merged_at":             mergedAt,
		"merge_commit_sha":      mergeCommit,
		"base":                  map[string]any{"ref": p.baseRef},
		"head":                  head,
		"title":                 p.title,
		"user":                  map[string]any{"login": p.author, "type": p.authorType},
		"labels":                labels,
		"maintainer_can_modify": p.maintainerCanModify,
	}
}

// prepReview describes one entry of the PR's review list.
type prepReview struct {
	user      string
	userType  string
	state     string
	submitted string
	commit    string
}

func (r prepReview) payload() map[string]any {
	return map[string]any{
		"user":         map[string]any{"login": r.user, "type": r.userType},
		"state":        r.state,
		"submitted_at": r.submitted,
		"commit_id":    r.commit,
	}
}

// approvedReviews is the review list of a properly reviewed preparation PR: one
// human approval of the final head commit, submitted before the merge.
func approvedReviews(headSHA string) []prepReview {
	return []prepReview{{
		user:      releasePRMaintainer,
		userType:  "User",
		state:     "APPROVED",
		submitted: "2026-09-02T10:00:02Z",
		commit:    headSHA,
	}}
}

// TestPromotionAuthorizationRequiresTheReviewedAutomationPreparationPR executes
// the workflow's authorization contract against a real Git repository whose
// preparation branch was SQUASH-merged, and a stubbed GitHub API. Labels, titles
// and branch names are mutable metadata any write-access collaborator can forge,
// so authorization must additionally rest on the App author, the canonical
// non-fork head, the merge commit, the frozen head content, and a human approval
// of that content submitted before the merge.
func TestPromotionAuthorizationRequiresTheReviewedAutomationPreparationPR(t *testing.T) {
	const gateway = "2.2.6"
	otherMerge := strings.Repeat("f", 40)
	staleHead := strings.Repeat("1", 40)

	for _, tc := range []struct {
		name               string
		mode               string
		pr                 func(pr *prepPR)
		pulls              func(payloads []map[string]any) []map[string]any
		reviews            func(headSHA string) []prepReview
		reviewerPermission string
		noAppLogin         bool
		apiFails           bool
		want               string
		wantError          bool
	}{
		{name: "squash-merged-reviewed-preparation-pr"},
		{
			// A collaborator with write access can create the branch, open a PR
			// with the exact title, and add the release label. Only the author
			// identity distinguishes this from the automation's PR.
			name: "forged-label-by-collaborator",
			pr:   func(pr *prepPR) { pr.author, pr.authorType = "write-collaborator", "User" },
			want: "not the release automation App",
		},
		{
			name: "other-app-author",
			pr:   func(pr *prepPR) { pr.author = "some-other-automation[bot]" },
			want: "not the release automation App",
		},
		{
			name: "app-author-with-user-type",
			pr:   func(pr *prepPR) { pr.authorType = "User" },
			want: "not the release automation App",
		},
		{
			name:    "approved-review-missing",
			reviews: func(string) []prepReview { return nil },
			want:    "has no review by someone other than its author",
		},
		{
			name: "self-approval",
			reviews: func(headSHA string) []prepReview {
				reviews := approvedReviews(headSHA)
				reviews[0].user = releaseAppLogin
				reviews[0].userType = "Bot"
				return reviews
			},
			want: "has no review by someone other than its author",
		},
		{
			name: "comment-only-review",
			reviews: func(headSHA string) []prepReview {
				reviews := approvedReviews(headSHA)
				reviews[0].state = "COMMENTED"
				return reviews
			},
			want: "has no review by someone other than its author",
		},
		{
			name: "changes-requested-review",
			reviews: func(headSHA string) []prepReview {
				reviews := approvedReviews(headSHA)
				reviews[0].state = "CHANGES_REQUESTED"
				return reviews
			},
			want: "has no review by someone other than its author",
		},
		{
			name: "approval-after-merge",
			reviews: func(headSHA string) []prepReview {
				reviews := approvedReviews(headSHA)
				reviews[0].submitted = "2026-09-02T10:05:00Z"
				return reviews
			},
			want: "has no review by someone other than its author",
		},
		{
			// The automation re-ran after the approval, so the approved content
			// is not the content that was merged.
			name: "approval-of-stale-head",
			reviews: func(string) []prepReview {
				reviews := approvedReviews(staleHead)
				return reviews
			},
			want: "has no review by someone other than its author",
		},
		{
			name: "fork-head",
			pr:   func(pr *prepPR) { pr.headRepo, pr.headFork = "attacker/higress", true },
			want: "is not the canonical higress-group/higress",
		},
		{
			name: "head-repository-absent",
			pr:   func(pr *prepPR) { pr.omitHeadRepo = true },
			want: "is not the canonical higress-group/higress",
		},
		{
			name: "maintainer-edits-allowed",
			pr:   func(pr *prepPR) { pr.maintainerCanModify = true },
			want: "still allows maintainer edits",
		},
		{
			name: "merge-commit-differs",
			pr:   func(pr *prepPR) { pr.mergeCommit = otherMerge },
			want: "expected exactly one PR merged into main at",
		},
		{
			name: "malformed-head-sha",
			pr:   func(pr *prepPR) { pr.headSHA = "0f64f58" },
			want: "reports no usable head commit",
		},
		{
			name: "missing-label",
			pr:   func(pr *prepPR) { pr.labels = nil },
			want: "does not carry the label release/" + gateway,
		},
		{
			name: "other-label",
			pr:   func(pr *prepPR) { pr.labels = []string{"release/2.2.5"} },
			want: "does not carry the label release/" + gateway,
		},
		{
			name: "not-merged",
			pr:   func(pr *prepPR) { pr.state, pr.mergedAt, pr.mergeCommit = "open", "", "" },
			want: "expected exactly one PR merged into main at",
		},
		{
			name: "other-branch",
			pr:   func(pr *prepPR) { pr.headRef = "some-feature" },
			want: "does not match the deterministic preparation branch",
		},
		{
			name: "other-title",
			pr:   func(pr *prepPR) { pr.title = "chore: prepare plugin snapshot 2.2.5" },
			want: "does not match the deterministic preparation branch",
		},
		{
			name: "other-release-preparation-pr",
			pr: func(pr *prepPR) {
				pr.headRef = "release/plugin-snapshot-2.2.5"
				pr.title = "chore: prepare plugin snapshot 2.2.5"
				pr.labels = []string{"release/2.2.5"}
			},
			want: "does not match the deterministic preparation branch",
		},
		{
			name: "merged-into-another-branch",
			pr:   func(pr *prepPR) { pr.baseRef = "release-2.2" },
			want: "expected exactly one PR merged into main at",
		},
		{
			name:  "no-associated-pr",
			pulls: func([]map[string]any) []map[string]any { return []map[string]any{} },
			want:  "expected exactly one PR merged into main at",
		},
		{
			name:  "duplicate-match",
			pulls: func(payloads []map[string]any) []map[string]any { return []map[string]any{payloads[0], payloads[0]} },
			want:  "expected exactly one PR merged into main at",
		},
		{
			// A write collaborator can approve the App-owned branch head
			// themselves; only a maintainer/admin approval authorizes promotion.
			name:               "write-collaborator-approval",
			reviewerPermission: "write",
			want:               "no pre-merge APPROVED review of head commit",
		},
		{
			name:               "reviewer-permission-lookup-unknown",
			reviewerPermission: "unknown",
			want:               "no pre-merge APPROVED review of head commit",
		},
		{name: "api-refusal", apiFails: true, wantError: true},
		{
			name:       "app-login-not-configured",
			noAppLogin: true,
			want:       "vars.RELEASE_PR_APP_LOGIN is not configured",
		},
		{
			name: "commit-not-on-main",
			mode: "off-main",
			want: "not reachable from main",
		},
		{
			name: "snapshot-from-unrelated-freeze",
			mode: "bogus-freeze",
			want: "does not descend from the code-freeze commit",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root, commit, headSHA := authorizationGitFixture(t, gateway, tc.mode)
			bin := filepath.Join(root, "bin")
			if err := os.MkdirAll(bin, 0o755); err != nil {
				t.Fatal(err)
			}
			spec := defaultPrepPR(gateway, commit, headSHA)
			if tc.pr != nil {
				tc.pr(&spec)
			}
			payloads := []map[string]any{spec.payload()}
			if tc.pulls != nil {
				payloads = tc.pulls(payloads)
			}
			reviews := approvedReviews(headSHA)
			if tc.reviews != nil {
				reviews = tc.reviews(headSHA)
			}
			reviewPayloads := make([]map[string]any, 0, len(reviews))
			for _, review := range reviews {
				reviewPayloads = append(reviewPayloads, review.payload())
			}
			status := "0"
			if tc.apiFails {
				status = "1"
			}
			writeExecutableFixture(t, filepath.Join(bin, "gh"), strings.ReplaceAll(`#!/usr/bin/env bash
set -uo pipefail
if [ "$1" != api ]; then echo "unexpected gh invocation: $*" >&2; exit 2; fi
endpoint=""
for arg in "$@"; do
  case "$arg" in
    repos/*) endpoint="$arg" ;;
  esac
done
if [ -z "$endpoint" ]; then echo "no endpoint in gh api invocation: $*" >&2; exit 2; fi
case "$endpoint" in
  */pulls) cat "$PULLS_FIXTURE" ;;
  */reviews) cat "$REVIEWS_FIXTURE" ;;
  */collaborators/*/permission) user=$(basename "$(dirname "$endpoint")"); cat "$PERMISSIONS_FIXTURE/$user.json" ;;
  *) echo "unexpected gh api endpoint: $endpoint" >&2; exit 2 ;;
esac
exit @@STATUS@@
`, "@@STATUS@@", status))
			pullsFixture := filepath.Join(root, "pulls.json")
			if err := os.WriteFile(pullsFixture, mustJSON(t, payloads), 0o600); err != nil {
				t.Fatal(err)
			}
			reviewsFixture := filepath.Join(root, "reviews.json")
			if err := os.WriteFile(reviewsFixture, mustJSON(t, reviewPayloads), 0o600); err != nil {
				t.Fatal(err)
			}
			permissionsDir := filepath.Join(root, "permissions")
			if err := os.MkdirAll(permissionsDir, 0o755); err != nil {
				t.Fatal(err)
			}
			reviewerPermission := "maintain"
			if tc.reviewerPermission != "" {
				reviewerPermission = tc.reviewerPermission
			}
			if err := os.WriteFile(filepath.Join(permissionsDir, releasePRMaintainer+".json"), mustJSON(t, map[string]string{"permission": reviewerPermission}), 0o600); err != nil {
				t.Fatal(err)
			}
			os.Setenv("PERMISSIONS_FIXTURE", permissionsDir)
			defer os.Unsetenv("PERMISSIONS_FIXTURE")
			summary := filepath.Join(root, "summary.md")
			contract := workflowShellContract(t, "promote-plugin-release.yaml", "promotion-authorization-contract")
			cmd := exec.Command("bash", "-c", "set -euo pipefail\n"+contract)
			cmd.Dir = root
			appLogin := releaseAppLogin
			if tc.noAppLogin {
				appLogin = ""
			}
			cmd.Env = append(os.Environ(),
				"PATH="+bin+":"+os.Getenv("PATH"),
				"PULLS_FIXTURE="+pullsFixture,
				"REVIEWS_FIXTURE="+reviewsFixture,
				"GH_TOKEN=fixture-token",
				"GITHUB_REPOSITORY=higress-group/higress",
				"GITHUB_STEP_SUMMARY="+summary,
				"RELEASE_PR_APP_LOGIN="+appLogin,
				"SOURCE_COMMIT="+commit,
				"SNAPSHOT_PATH=plugins/release/snapshots/"+gateway+".json",
			)
			output, err := cmd.CombinedOutput()
			if strings.Contains(string(output), "fixture-token") {
				t.Fatalf("authorization leaked its token:\n%s", output)
			}
			if tc.want != "" && !strings.Contains(string(output), tc.want) {
				t.Fatalf("authorization output lacks %q:\n%s", tc.want, output)
			}
			if tc.wantError || tc.want != "" {
				if err == nil {
					t.Fatalf("unauthorized promotion was accepted:\n%s", output)
				}
				return
			}
			if err != nil {
				t.Fatalf("authorized promotion was rejected: %v\n%s", err, output)
			}
			body, readErr := os.ReadFile(summary)
			if readErr != nil {
				t.Fatal(readErr)
			}
			for _, required := range []string{
				"Promotion of " + gateway + " authorized by merged preparation PR #" + strconv.Itoa(releasePRNumber),
				"author " + releaseAppLogin,
				"label release/" + gateway,
				"head " + headSHA,
				"at " + commit,
			} {
				if !strings.Contains(string(body), required) {
					t.Fatalf("step summary did not record %q: %s", required, body)
				}
			}
		})
	}
}

// TestPromotionAuthorizationReadsTheAppLoginFromRepositoryVariables keeps the
// authorizing identity configuration rather than a guess: the workflow must take
// it from vars.RELEASE_PR_APP_LOGIN, refuse to run without it, and never embed a
// bot login that a rename would silently invalidate.
func TestPromotionAuthorizationReadsTheAppLoginFromRepositoryVariables(t *testing.T) {
	promote := mustWorkflow(t, "promote-plugin-release.yaml")
	job := workflowJobSection(t, promote, "verify-and-promote")
	for _, required := range []string{
		"RELEASE_PR_APP_LOGIN: ${{ vars.RELEASE_PR_APP_LOGIN }}",
		`if [ -z "${RELEASE_PR_APP_LOGIN:-}" ]; then`,
		`if [ "$pr_author" != "$RELEASE_PR_APP_LOGIN" ] || [ "$pr_author_type" != Bot ]; then`,
		`.merge_commit_sha == $commit`,
		`if [ "$maintainer_can_modify" != false ]; then`,
		`(.commit_id // "") == $head`,
	} {
		if !strings.Contains(job, required) {
			t.Fatalf("promotion authorization lacks %q", required)
		}
	}
	if strings.Contains(promote, releaseAppLogin) {
		t.Fatalf("promote must not hard-code the App login %q; it is read from vars.RELEASE_PR_APP_LOGIN", releaseAppLogin)
	}
}

// TestPromotionAuthorizationRunsBeforeAnyRegistryMutation keeps the
// authorization decision ahead of the credential login and every write.
func TestPromotionAuthorizationRunsBeforeAnyRegistryMutation(t *testing.T) {
	promote := mustWorkflow(t, "promote-plugin-release.yaml")
	job := workflowJobSection(t, promote, "verify-and-promote")
	authorization := strings.Index(job, "# BEGIN promotion-authorization-contract")
	login := strings.Index(job, "oras login")
	verify := strings.Index(job, "      - name: Verify exact snapshot provenance\n")
	copyStep := strings.Index(job, "oras cp")
	if authorization < 0 || login < 0 || verify < 0 || copyStep < 0 {
		t.Fatal("promote's version phase lost an expected step boundary")
	}
	if !(authorization < verify && verify < login && login < copyStep) {
		t.Fatal("the merged preparation PR must authorize promotion before verification, registry login, and any copy")
	}
}

// TestPreparationSweepsRegistryBeforeOpeningThePR proves SPEC-4634004 wiring in
// the prepare workflow: the sweep runs after candidates exist and before the
// snapshot is rendered, its report is bound into the snapshot and the PR body,
// and the PR carries the release label promote requires.
func TestPreparationSweepsRegistryBeforeOpeningThePR(t *testing.T) {
	prepare := mustWorkflow(t, "prepare-plugin-release.yaml")
	candidates := strings.Index(prepare, "      - name: Build and publish content-addressed candidates\n")
	sweep := strings.Index(prepare, "      - name: Sweep public registry for planned tag conflicts\n")
	render := strings.Index(prepare, "      - name: Render canonical snapshot\n")
	prStep := strings.Index(prepare, "      - name: Create or update deterministic preparation PR\n")
	if candidates < 0 || sweep < 0 || render < 0 || prStep < 0 {
		t.Fatal("prepare lost an expected step")
	}
	if !(candidates < sweep && sweep < render && render < prStep) {
		t.Fatal("the registry sweep must run after candidates are built and before the snapshot is rendered and the PR is opened")
	}
	for _, required := range []string{
		"# BEGIN migration-preflight-contract",
		"args=(migration-preflight --root ../.. --catalog ../../plugins/release/catalog.json --plan /tmp/plan.json --output /tmp/migration-report.json --markdown /tmp/migration-report.md)",
		`if [ -n "$PREVIOUS" ]; then args+=(--previous "../../$PREVIOUS"); else args+=(--previous /tmp/bootstrap-snapshot.json); fi`,
		`if [ -f /tmp/candidates.json ]; then args+=(--candidate-evidence /tmp/candidates.json); fi`,
		"--migration-report /tmp/migration-report.json",
		`name: plugin-release-migration-report-${{ inputs.gateway_version }}`,
	} {
		if !strings.Contains(prepare, required) {
			t.Fatalf("prepare lacks migration preflight contract %q", required)
		}
	}
	// The sweep must not write to any registry: it is a read-only classification.
	sweepStep := prepare[sweep:render]
	for _, mutation := range []string{"oras login", "oras cp", "oras push"} {
		if strings.Contains(sweepStep, mutation) {
			t.Fatalf("migration sweep contains registry mutation %q", mutation)
		}
	}

	pr := prepare[prStep:]
	for _, required := range []string{
		"# BEGIN preparation-pr-contract",
		`label="release/$GATEWAY_VERSION"`,
		`label_ref=$(jq -rn --arg label "$label" '$label | @uri')`,
		`if ! gh api "repos/$GITHUB_REPOSITORY/labels/$label_ref" >/dev/null 2>&1; then`,
		`gh label create "$label"`,
		`test "$(gh api "repos/$GITHUB_REPOSITORY/labels/$label_ref" --jq .name)" = "$label"`,
		`gh pr create --base main --head "$branch" --title "$title" --body "$body" --label "$label" || gh pr edit "$branch" --title "$title" --body "$body" --add-label "$label"`,
		`if [ "$(jq -er '[.entries[] | select(.state == "blocked")] | length' /tmp/migration-report.json)" -gt 0 ]; then`,
		`$(cat /tmp/migration-report.md)`,
	} {
		if !strings.Contains(pr, required) {
			t.Fatalf("preparation PR publication lacks contract %q", required)
		}
	}
	labelCheck := strings.Index(pr, `gh api "repos/$GITHUB_REPOSITORY/labels/$label_ref"`)
	create := strings.Index(pr, "gh pr create")
	if labelCheck < 0 || create < 0 || labelCheck > create {
		t.Fatal("the release label must exist before the preparation PR is opened")
	}
}

func mustWorkflow(t *testing.T, name string) string {
	t.Helper()
	data, err := os.ReadFile("../../.github/workflows/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// workflowJobSection returns one top-level job's YAML text so a gate assertion
// cannot be satisfied by a different job in the same workflow.
func workflowJobSection(t *testing.T, workflow, job string) string {
	t.Helper()
	start := strings.Index(workflow, "\n  "+job+":\n")
	if start < 0 {
		t.Fatalf("workflow has no %s job", job)
	}
	rest := workflow[start+1:]
	end := len(rest)
	for _, candidate := range []string{"\n  latest:\n", "\n  prepare:\n", "\n  publish:\n", "\n  preflight:\n"} {
		if idx := strings.Index(rest, candidate); idx >= 0 && idx < end && candidate != "\n  "+job+":\n" {
			end = idx
		}
	}
	return rest[:end]
}

func mustJSON(t *testing.T, value any) []byte {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func gitOutput(t *testing.T, root string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = root
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v: %s", args, err, out)
	}
	return string(out)
}

func commitIsAncestor(t *testing.T, root, ancestor, descendant string) bool {
	t.Helper()
	cmd := exec.Command("git", "merge-base", "--is-ancestor", ancestor, descendant)
	cmd.Dir = root
	return cmd.Run() == nil
}

// authorizationGitFixture mirrors the real chain: a code-freeze commit on main, a
// preparation branch forking that exact commit and carrying the snapshot whose
// sourceCommit is the freeze, then that branch SQUASH-merged into main and
// checked out detached, exactly as promote sees it. It returns the repository,
// the merge commit handed to promote as source_commit, and the preparation
// branch head the PR reports as head.sha and a review must have approved.
func authorizationGitFixture(t *testing.T, gateway, mode string) (root, mergeCommit, headSHA string) {
	t.Helper()
	origin := t.TempDir()
	mustRun(t, origin, "git", "init", "-q", "--bare", "-b", "main")
	root = t.TempDir()
	mustRun(t, root, "git", "init", "-q", "-b", "main")
	mustRun(t, root, "git", "config", "user.name", "test")
	mustRun(t, root, "git", "config", "user.email", "test@example.com")
	mustRun(t, root, "git", "remote", "add", "origin", origin)
	mustWrite(t, filepath.Join(root, "plugins/wasm-go/extensions/demo/VERSION"), "1.0.0\n")
	mustRun(t, root, "git", "add", ".")
	mustRun(t, root, "git", "commit", "-q", "-m", "code freeze")
	mustRun(t, root, "git", "push", "-q", "origin", "HEAD:refs/heads/main")
	freeze, err := resolveCommit(root, "HEAD")
	if err != nil {
		t.Fatal(err)
	}
	recorded := freeze
	if mode == "bogus-freeze" {
		recorded = strings.Repeat("0", 40)
	}
	mustRun(t, root, "git", "checkout", "-q", "-B", "release/plugin-snapshot-"+gateway)
	snapshot := map[string]any{
		"schemaVersion": 1, "gatewayVersion": gateway, "sourceCommit": recorded,
		"provenanceMode": "candidate", "plugins": []any{},
	}
	snapshotPath := filepath.Join(root, "plugins", "release", "snapshots", gateway+".json")
	if err := os.MkdirAll(filepath.Dir(snapshotPath), 0o755); err != nil {
		t.Fatal(err)
	}
	data, err := json.MarshalIndent(snapshot, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(snapshotPath, append(data, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
	mustRun(t, root, "git", "add", ".")
	mustRun(t, root, "git", "commit", "-q", "-s", "-m", "chore: prepare plugin snapshot "+gateway)
	headSHA, err = resolveCommit(root, "HEAD")
	if err != nil {
		t.Fatal(err)
	}
	if mode == "off-main" {
		// The preparation branch was never merged: the commit exists and carries
		// the snapshot, but nothing on main authorizes it.
		return root, headSHA, headSHA
	}
	mustRun(t, root, "git", "checkout", "-q", "main")
	mustRun(t, root, "git", "merge", "-q", "--squash", headSHA)
	mustRun(t, root, "git", "commit", "-q", "-m", "chore: prepare plugin snapshot "+gateway+" (#"+strconv.Itoa(releasePRNumber)+")")
	mergeCommit, err = resolveCommit(root, "HEAD")
	if err != nil {
		t.Fatal(err)
	}
	mustRun(t, root, "git", "push", "-q", "origin", "HEAD:refs/heads/main")
	mustRun(t, root, "git", "checkout", "-q", mergeCommit)
	// The fixture must really have squash semantics: one parent (the code-freeze
	// commit) and a preparation head that is not an ancestor of the merge, so no
	// assertion below can be satisfied by a fast-forward shape production never
	// produces.
	parents := strings.Fields(gitOutput(t, root, "rev-list", "--parents", "-n", "1", mergeCommit))
	if len(parents) != 2 || parents[1] != freeze {
		t.Fatalf("fixture merge must be a squash of the preparation branch onto the code freeze %s, got parents %v", freeze, parents[1:])
	}
	if mergeCommit == headSHA || commitIsAncestor(t, root, headSHA, mergeCommit) {
		t.Fatal("fixture must not fast-forward: the preparation head cannot be an ancestor of its squash merge commit")
	}
	return root, mergeCommit, headSHA
}
