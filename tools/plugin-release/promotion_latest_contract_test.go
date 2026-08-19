// Copyright 2026 Higress Authors
// Licensed under the Apache License, Version 2.0.

package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

type latestContractPlugin struct {
	ID              string
	Version         string
	Digest          string
	CurrentDigest   string
	CurrentVersion  string
	EvidenceStatus  string
	EvidenceVersion string
	EvidenceDigest  string
	EvidenceRef     string
	// Candidate marks the snapshot entry as candidate provenance, which the
	// promotion layout gate must verify before any latest alias moves.
	Candidate bool
}

type latestContractResult struct {
	err     error
	output  string
	log     string
	journal map[string]any
}

func TestPromotionLatestContractAllowsOnlyEvidenceBoundBootstrapReplacement(t *testing.T) {
	legacy := latestContractPlugin{
		ID: "legacy", Version: "2.0.1", Digest: testDigest("desired"),
		CurrentDigest: testDigest("mutable-legacy-latest"), EvidenceStatus: "public",
		EvidenceVersion: "2.0.0", EvidenceDigest: testDigest("legacy"),
	}

	t.Run("verified-bootstrap", func(t *testing.T) {
		result := runPromotionLatestContract(t, true, []latestContractPlugin{legacy})
		if result.err != nil {
			t.Fatalf("verified bootstrap latest replacement failed: %v\n%s", result.err, result.output)
		}
		if strings.Count(result.log, "cp ") != 1 {
			t.Fatalf("bootstrap replacement writes = %q, want exactly one copy", result.log)
		}
		entry := latestJournalEntries(t, result.journal)[0]
		if entry["preflight"] != "bootstrap-replace-unclassified" || entry["oldVersion"] != "" {
			t.Fatalf("bootstrap journal did not retain its evidence-bound migration state: %#v", entry)
		}
	})

	t.Run("verified-bootstrap-missing", func(t *testing.T) {
		missing := latestContractPlugin{
			ID: "new-to-version-tags", Version: "1.0.0", Digest: testDigest("new-desired"),
			CurrentDigest: testDigest("legacy-latest-only"), EvidenceStatus: "missing", EvidenceVersion: "1.0.0",
		}
		result := runPromotionLatestContract(t, true, []latestContractPlugin{missing})
		if result.err != nil {
			t.Fatalf("verified missing bootstrap entry did not authorize its legacy latest migration: %v\n%s", result.err, result.output)
		}
		entry := latestJournalEntries(t, result.journal)[0]
		if entry["preflight"] != "bootstrap-replace-unclassified" || entry["oldVersion"] != "" {
			t.Fatalf("missing bootstrap migration attributed a version to an unclassified alias: %#v", entry)
		}
	})

	t.Run("ordinary-snapshot", func(t *testing.T) {
		result := runPromotionLatestContract(t, false, []latestContractPlugin{legacy})
		if result.err == nil || !strings.Contains(result.output, "outside a verified bootstrap snapshot") {
			t.Fatalf("ordinary snapshot accepted an unannotated latest: err=%v\n%s", result.err, result.output)
		}
		assertNoLatestMutation(t, result.log)
	})

	for _, tc := range []struct {
		name   string
		mutate func(*latestContractPlugin)
		want   string
	}{
		{
			name: "invalid-evidence-status",
			mutate: func(plugin *latestContractPlugin) {
				plugin.EvidenceStatus = "invalid"
			},
			want: "does not authorize unclassified legacy latest replacement",
		},
		{
			name: "missing-evidence",
			mutate: func(plugin *latestContractPlugin) {
				plugin.EvidenceStatus = ""
			},
			want: "does not authorize unclassified legacy latest replacement",
		},
		{
			name: "wrong-public-reference",
			mutate: func(plugin *latestContractPlugin) {
				plugin.EvidenceRef = "registry.example.invalid/plugins/other:2.0.0"
			},
			want: "public reference does not match latest image",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			plugin := legacy
			tc.mutate(&plugin)
			result := runPromotionLatestContract(t, true, []latestContractPlugin{plugin})
			if result.err == nil || !strings.Contains(result.output, tc.want) {
				t.Fatalf("invalid bootstrap binding was accepted: err=%v\n%s", result.err, result.output)
			}
			assertNoLatestMutation(t, result.log)
		})
	}

	t.Run("invalid-evidence-hash", func(t *testing.T) {
		result := runPromotionLatestContractFixture(t, true, true, []latestContractPlugin{legacy})
		if result.err == nil {
			t.Fatalf("bootstrap snapshot with an invalid evidence hash was accepted:\n%s", result.output)
		}
		assertNoLatestMutation(t, result.log)
	})

	t.Run("public-evidence-same-version", func(t *testing.T) {
		plugin := legacy
		plugin.Version = plugin.EvidenceVersion
		result := runPromotionLatestContract(t, true, []latestContractPlugin{plugin})
		if result.err == nil || !strings.Contains(result.output, "does not authorize replacing the same stable version") {
			t.Fatalf("bootstrap evidence allowed a same-version different-digest replacement: err=%v\n%s", result.err, result.output)
		}
		assertNoLatestMutation(t, result.log)
	})

	t.Run("missing-evidence-version-mismatch", func(t *testing.T) {
		plugin := latestContractPlugin{
			ID: "missing-version", Version: "1.0.1", Digest: testDigest("missing-desired"),
			CurrentDigest: testDigest("missing-legacy"), EvidenceStatus: "missing", EvidenceVersion: "1.0.0",
		}
		result := runPromotionLatestContract(t, true, []latestContractPlugin{plugin})
		if result.err == nil || !strings.Contains(result.output, "missing bootstrap entry version does not match") {
			t.Fatalf("missing evidence authorized a different selected version: err=%v\n%s", result.err, result.output)
		}
		assertNoLatestMutation(t, result.log)
	})
}

func TestPromotionLatestContractKeepsAnnotatedMonotonicityAndConflicts(t *testing.T) {
	for _, tc := range []struct {
		name           string
		currentVersion string
		candidate      string
		want           string
	}{
		{name: "downgrade", currentVersion: "2.1.0", candidate: "2.0.1", want: "would move latest backwards"},
		{name: "same-version-conflict", currentVersion: "2.0.1", candidate: "2.0.1", want: "latest alias conflict"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			plugin := latestContractPlugin{
				ID: "annotated", Version: tc.candidate, Digest: testDigest("desired"),
				CurrentDigest: testDigest("current"), CurrentVersion: tc.currentVersion,
			}
			result := runPromotionLatestContract(t, false, []latestContractPlugin{plugin})
			if result.err == nil || !strings.Contains(result.output, tc.want) {
				t.Fatalf("annotated latest guard did not fail as expected: err=%v\n%s", result.err, result.output)
			}
			assertNoLatestMutation(t, result.log)
		})
	}
}

func TestPromotionLatestContractCreatesAbsentAndSkipsIdentical(t *testing.T) {
	desired := testDigest("desired")
	result := runPromotionLatestContract(t, false, []latestContractPlugin{
		{ID: "absent", Version: "1.0.0", Digest: testDigest("created")},
		{ID: "identical", Version: "2.0.1", Digest: desired, CurrentDigest: desired},
	})
	if result.err != nil {
		t.Fatalf("absent/identical latest batch failed: %v\n%s", result.err, result.output)
	}
	if strings.Count(result.log, "cp ") != 1 || !strings.Contains(result.log, "plugins/absent:latest") {
		t.Fatalf("only the absent alias should be created:\n%s", result.log)
	}
	entries := latestJournalEntries(t, result.journal)
	if entries[0]["preflight"] != "create" || entries[1]["preflight"] != "identical" {
		t.Fatalf("unexpected absence/identity journal: %#v", entries)
	}
}

func TestPromotionLatestContractDoesNotMutateBeforeCompletePreflight(t *testing.T) {
	result := runPromotionLatestContract(t, false, []latestContractPlugin{
		{ID: "would-create", Version: "1.0.0", Digest: testDigest("created")},
		{ID: "reject-later", Version: "2.0.1", Digest: testDigest("desired"), CurrentDigest: testDigest("legacy")},
	})
	if result.err == nil || !strings.Contains(result.output, "outside a verified bootstrap snapshot") {
		t.Fatalf("incomplete preflight unexpectedly succeeded: err=%v\n%s", result.err, result.output)
	}
	assertNoLatestMutation(t, result.log)
}

func runPromotionLatestContract(t *testing.T, bootstrap bool, plugins []latestContractPlugin) latestContractResult {
	t.Helper()
	return runPromotionLatestContractFixture(t, bootstrap, false, plugins)
}

func runPromotionLatestContractFixture(t *testing.T, bootstrap, corruptEvidenceSHA bool, plugins []latestContractPlugin) latestContractResult {
	t.Helper()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "tools"), 0o755); err != nil {
		t.Fatal(err)
	}
	toolDir, err := filepath.Abs(".")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(toolDir, filepath.Join(root, "tools", "plugin-release")); err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(root, "bin")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	logPath := filepath.Join(root, "oras.log")
	statePath := filepath.Join(root, "registry.json")

	const gateway = "2.2.4"
	snapshot := map[string]any{"gatewayVersion": gateway, "plugins": []any{}}
	registry := map[string]any{}
	evidencePlugins := map[string]any{}
	for _, plugin := range plugins {
		ref := "registry.example.invalid/plugins/" + plugin.ID + ":" + plugin.Version
		latest := "registry.example.invalid/plugins/" + plugin.ID + ":latest"
		snapshotEntry := map[string]any{
			"logicalId": plugin.ID, "ociRef": ref, "digest": plugin.Digest, "version": plugin.Version, "candidateRef": "",
		}
		if plugin.Candidate {
			snapshotEntry["provenanceMode"] = "candidate"
			snapshotEntry["candidateRef"] = "registry.example.invalid/candidates/" + plugin.ID + "@" + plugin.Digest
		}
		snapshot["plugins"] = append(snapshot["plugins"].([]any), snapshotEntry)
		registry[ref] = map[string]any{"digest": plugin.Digest, "version": plugin.Version}
		if plugin.CurrentDigest != "" {
			registry[latest] = map[string]any{"digest": plugin.CurrentDigest, "version": plugin.CurrentVersion}
		}
		if plugin.EvidenceStatus != "" {
			publicRef := plugin.EvidenceRef
			if publicRef == "" {
				publicRef = "registry.example.invalid/plugins/" + plugin.ID + ":" + plugin.EvidenceVersion
			}
			evidencePlugins[plugin.ID] = map[string]any{
				"status": plugin.EvidenceStatus, "version": plugin.EvidenceVersion, "digest": plugin.EvidenceDigest,
				"publicRef": publicRef,
			}
		}
	}
	if bootstrap {
		evidencePath := filepath.Join(root, "plugins", "release", "bootstrap-evidence", gateway+".json")
		evidenceBytes := marshalLatestFixture(t, map[string]any{"plugins": evidencePlugins})
		if err := os.MkdirAll(filepath.Dir(evidencePath), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(evidencePath, evidenceBytes, 0o600); err != nil {
			t.Fatal(err)
		}
		evidenceSHA := sha256.Sum256(evidenceBytes)
		evidenceSHAHex := hex.EncodeToString(evidenceSHA[:])
		if corruptEvidenceSHA {
			evidenceSHAHex = strings.Repeat("0", 64)
		}
		snapshot["bootstrapEvidence"] = map[string]any{
			"path":   "plugins/release/bootstrap-evidence/" + gateway + ".json",
			"sha256": evidenceSHAHex,
		}
	}
	snapshotPath := filepath.Join(root, "plugins", "release", "snapshots", gateway+".json")
	if err := os.MkdirAll(filepath.Dir(snapshotPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(snapshotPath, marshalLatestFixture(t, snapshot), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(statePath, marshalLatestFixture(t, registry), 0o600); err != nil {
		t.Fatal(err)
	}
	writeExecutableFixture(t, filepath.Join(bin, "oras"), `#!/usr/bin/env bash
set -euo pipefail
printf '%s\n' "$*" >> "$ORAS_LOG"
if [ "$1 $2" = "manifest fetch" ]; then
  ref=$3
  if ! record=$(jq -cer --arg ref "$ref" '.[$ref]' "$ORAS_STATE" 2>/dev/null); then
    echo "response status code 404: Not Found" >&2
    exit 1
  fi
  if [ "${4:-}" = "--descriptor" ]; then
    jq -cn --arg digest "$(jq -er .digest <<<"$record")" '{digest:$digest}'
    exit 0
  fi
  if [ "${4:-}" = "--format" ]; then
    jq -cn --arg version "$(jq -r '.version // empty' <<<"$record")" '{annotations:{"org.opencontainers.image.version":$version}}'
    exit 0
  fi
  # A plain manifest fetch resolves the full manifest for the layout gate.
  if [ "${ORAS_LAYOUT_MODE:-}" = incident ]; then
    layers='[{"mediaType":"application/vnd.module.wasm.content.layer.v1+wasm","digest":"sha256:1111111111111111111111111111111111111111111111111111111111111111"}]'
  else
    layers='[{"mediaType":"application/vnd.module.wasm.config.v1+json","digest":"sha256:1111111111111111111111111111111111111111111111111111111111111111"},{"mediaType":"application/vnd.module.wasm.content.layer.v1+wasm","digest":"sha256:2222222222222222222222222222222222222222222222222222222222222222"}]'
  fi
  jq -cn --arg version "$(jq -r '.version // empty' <<<"$record")" --argjson layers "$layers" '{schemaVersion:2,mediaType:"application/vnd.oci.image.manifest.v1+json",annotations:{"org.opencontainers.image.version":$version},layers:$layers}'
  exit 0
fi
if [ "$1" = cp ]; then
  source=$2
  destination=$3
  digest=${source##*@}
  jq --arg ref "$destination" --arg digest "$digest" '.[$ref]={digest:$digest,version:""}' "$ORAS_STATE" > "$ORAS_STATE.next"
  mv "$ORAS_STATE.next" "$ORAS_STATE"
  exit 0
fi
echo "unsupported fake oras invocation: $*" >&2
exit 2
`)

	descriptorContracts := workflowShellContracts(t, "promote-plugin-release.yaml", "promotion-descriptor-contract")
	if len(descriptorContracts) != 2 {
		t.Fatalf("promotion workflow has %d descriptor contracts, want 2", len(descriptorContracts))
	}
	layoutContracts := workflowShellContracts(t, "promote-plugin-release.yaml", "promotion-layout-contract")
	if len(layoutContracts) != 3 {
		t.Fatalf("promotion workflow has %d layout contracts, want 3 (version copy, smoke gate, latest)", len(layoutContracts))
	}
	for i, contract := range layoutContracts {
		if contract != layoutContracts[0] {
			t.Fatalf("promotion layout contract %d is not canonical", i)
		}
	}
	latestContract := workflowShellContract(t, "promote-plugin-release.yaml", "promotion-latest-contract")
	script := "set -euo pipefail\n" + descriptorContracts[1] + "\n" + layoutContracts[2] + "\n" + latestContract
	snapshotSHA := sha256.Sum256([]byte(root))
	snapshotSHAHex := hex.EncodeToString(snapshotSHA[:])
	journalPath := filepath.Join("/tmp", "plugin-release-latest-"+snapshotSHAHex+".json")
	t.Cleanup(func() {
		_ = os.Remove(journalPath)
		_ = os.Remove("/tmp/journal.next")
	})
	cmd := exec.Command("bash", "-c", script)
	cmd.Dir = root
	cmd.Env = append(os.Environ(),
		"PATH="+bin+":"+os.Getenv("PATH"),
		"ORAS_LOG="+logPath,
		"ORAS_STATE="+statePath,
		"PLUGIN_RELEASE_BIN="+pluginReleaseVerifyBin(t),
		"SNAPSHOT_PATH=plugins/release/snapshots/"+gateway+".json",
		"SNAPSHOT_SHA256="+snapshotSHAHex,
	)
	output, runErr := cmd.CombinedOutput()
	logBytes, readErr := os.ReadFile(logPath)
	if readErr != nil && !os.IsNotExist(readErr) {
		t.Fatal(readErr)
	}
	result := latestContractResult{err: runErr, output: string(output), log: string(logBytes)}
	if journalBytes, err := os.ReadFile(journalPath); err == nil {
		if err := json.Unmarshal(journalBytes, &result.journal); err != nil {
			t.Fatalf("decode latest journal: %v", err)
		}
	} else if runErr == nil {
		t.Fatalf("successful latest contract did not write its journal: %v", err)
	}
	return result
}

func latestJournalEntries(t *testing.T, journal map[string]any) []map[string]any {
	t.Helper()
	if journal["phase"] != "latest-complete" {
		t.Fatalf("latest journal is not complete: %#v", journal)
	}
	raw, ok := journal["entries"].([]any)
	if !ok {
		t.Fatalf("latest journal entries have unexpected type: %#v", journal)
	}
	entries := make([]map[string]any, 0, len(raw))
	for _, entry := range raw {
		entries = append(entries, entry.(map[string]any))
	}
	return entries
}

func assertNoLatestMutation(t *testing.T, log string) {
	t.Helper()
	if strings.Contains(log, "cp ") || strings.Contains(log, "push ") {
		t.Fatalf("latest registry was mutated before complete preflight:\n%s", log)
	}
}

func marshalLatestFixture(t *testing.T, value any) []byte {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func testDigest(seed string) string {
	sum := sha256.Sum256([]byte(seed))
	return "sha256:" + hex.EncodeToString(sum[:])
}

var (
	pluginReleaseVerifyBuild    sync.Once
	pluginReleaseVerifyPath     string
	pluginReleaseVerifyBuildErr error
)

// pluginReleaseVerifyBin builds the real plugin-release tool once so the
// extracted promotion contracts exercise the same verify-oci-layout gate the
// workflow runs.
func pluginReleaseVerifyBin(t *testing.T) string {
	t.Helper()
	pluginReleaseVerifyBuild.Do(func() {
		dir, err := os.MkdirTemp("", "plugin-release-verify-")
		if err != nil {
			pluginReleaseVerifyBuildErr = err
			return
		}
		path := filepath.Join(dir, "plugin-release-verify")
		output, err := exec.Command("go", "build", "-o", path, ".").CombinedOutput()
		if err != nil {
			pluginReleaseVerifyBuildErr = fmt.Errorf("build plugin-release: %v: %s", err, output)
			return
		}
		pluginReleaseVerifyPath = path
	})
	if pluginReleaseVerifyBuildErr != nil {
		t.Fatal(pluginReleaseVerifyBuildErr)
	}
	return pluginReleaseVerifyPath
}

// TestPromotionLayoutContractRunsTheSharedPredicate proves the extracted
// promotion-layout-contract shell function surfaces the shared predicate's
// layout verdict and fails with the offending reference on rejection.
func TestPromotionLayoutContractRunsTheSharedPredicate(t *testing.T) {
	contract := workflowShellContract(t, "promote-plugin-release.yaml", "promotion-layout-contract")
	tmp := t.TempDir()
	bin := filepath.Join(tmp, "bin")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	writeExecutableFixture(t, filepath.Join(bin, "layout-shim"), `#!/usr/bin/env bash
set -euo pipefail
ref=""
prev=""
for arg in "$@"; do
  if [ "$prev" = "--ref" ]; then ref=$arg; fi
  prev=$arg
done
case "$ORAS_LAYOUT_MODE" in
  oci) printf 'oci\n'; exit 0 ;;
  compat) printf 'compat\n'; exit 0 ;;
  *) echo "$ref is not loadable by Envoy as a Wasm OCI image" >&2; exit 1 ;;
esac
`)
	const ref = "registry.example.invalid/plugins/demo:1.0.0"
	run := func(mode string) (string, error) {
		script := "set -euo pipefail\n" + contract + "\nverify_envoy_loadable_layout " + ref
		cmd := exec.Command("bash", "-c", script)
		cmd.Env = append(os.Environ(), "PLUGIN_RELEASE_BIN="+filepath.Join(bin, "layout-shim"), "ORAS_LAYOUT_MODE="+mode)
		output, err := cmd.CombinedOutput()
		return string(output), err
	}
	for _, mode := range []string{"oci", "compat"} {
		output, err := run(mode)
		if err != nil || strings.TrimSpace(output) != mode {
			t.Fatalf("layout contract did not surface the %s verdict: err=%v output=%q", mode, err, output)
		}
	}
	output, err := run("incident")
	if err == nil || !strings.Contains(output, "promoted artifact is not loadable by Envoy as a Wasm OCI image: "+ref) {
		t.Fatalf("layout contract did not fail naming the offending ref: err=%v output=%q", err, output)
	}
}

// TestPromotionWorkflowGatesCandidateArtifactsOnEnvoyLayout statically proves
// the promotion workflow checks every candidate-provenance public ref with the
// shared predicate after copy, records the verdict in the version journal, and
// does all of it before the latest phase can move any alias.
func TestPromotionWorkflowGatesCandidateArtifactsOnEnvoyLayout(t *testing.T) {
	data, err := os.ReadFile("../../.github/workflows/promote-plugin-release.yaml")
	if err != nil {
		t.Fatal(err)
	}
	workflow := string(data)
	for _, required := range []string{
		`verify-oci-layout --ref "$ref"`,
		`if [ "$(jq -r '.provenanceMode // "candidate"' <<<"$entry")" = candidate ]; then
              verify_envoy_loadable_layout "$ref" >/dev/null
            fi`,
		`layout=$(verify_envoy_loadable_layout "$ref")`,
		`(.entries[] | select(.ref == $ref)) |= . + {layoutGate: $layoutGate}`,
		`test "$candidate_total" = "$verified_total"`,
		`Gateway layout smoke gate:`,
	} {
		if !strings.Contains(workflow, required) {
			t.Fatalf("promotion workflow lacks the Envoy layout gate contract %q", strings.Split(required, "\n")[0])
		}
	}
	smokeGate := strings.Index(workflow, "Gateway layout smoke gate before latest movement")
	journalUpload := strings.Index(workflow, "Persist complete version batch journal")
	latestPhase := strings.Index(workflow, "Promote latest only after complete verified batch")
	if smokeGate < 0 || journalUpload < 0 || latestPhase < 0 || !(smokeGate < journalUpload && journalUpload < latestPhase) {
		t.Fatal("the layout smoke gate must run after the version batch and before its journal and the latest phase")
	}
}

func TestPromotionLatestContractGatesCandidateLayoutBeforeLatestMovement(t *testing.T) {
	t.Run("envoy-loadable-candidate-moves-latest", func(t *testing.T) {
		t.Setenv("ORAS_LAYOUT_MODE", "two-layer")
		result := runPromotionLatestContract(t, false, []latestContractPlugin{
			{ID: "gated", Version: "1.0.0", Digest: testDigest("gated-desired"), Candidate: true},
		})
		if result.err != nil {
			t.Fatalf("Envoy-loadable candidate was blocked from latest: %v\n%s", result.err, result.output)
		}
		if !strings.Contains(result.log, "plugins/gated:latest") {
			t.Fatalf("gated candidate never created its latest alias:\n%s", result.log)
		}
	})

	t.Run("issue-4528-single-layer-candidate-blocks-latest", func(t *testing.T) {
		t.Setenv("ORAS_LAYOUT_MODE", "incident")
		result := runPromotionLatestContract(t, false, []latestContractPlugin{
			{ID: "incident", Version: "1.0.0", Digest: testDigest("incident-desired"), Candidate: true},
		})
		if result.err == nil || !strings.Contains(result.output, "not loadable by Envoy as a Wasm OCI image") {
			t.Fatalf("incident-layout candidate was not blocked: err=%v\n%s", result.err, result.output)
		}
		assertNoLatestMutation(t, result.log)
	})

	t.Run("historical-public-import-is-not-gated-on-layout", func(t *testing.T) {
		t.Setenv("ORAS_LAYOUT_MODE", "incident")
		result := runPromotionLatestContract(t, false, []latestContractPlugin{
			{ID: "legacy-public", Version: "2.0.1", Digest: testDigest("legacy-desired")},
		})
		if result.err != nil {
			t.Fatalf("historical public import was blocked on layout: %v\n%s", result.err, result.output)
		}
	})
}
