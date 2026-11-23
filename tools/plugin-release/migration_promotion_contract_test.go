// Copyright 2026 Higress Authors
// Licensed under the Apache License, Version 2.0.

package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

type versionContractPlugin struct {
	ID             string
	Version        string
	Digest         string
	CandidateRef   string
	Provenance     string
	Blocked        bool
	ExistingDigest string
}

type versionContractResult struct {
	err     error
	output  string
	log     string
	summary string
	journal map[string]any
	state   map[string]any
}

// TestPromotionVersionContractSkipsMigrationExcludedPlugins proves the prepare
// time exclusion is what keeps a legacy occupied tag from failing the batch: the
// excluded plugin is neither probed nor copied, its divergent public tag is left
// untouched, and the remaining plugins promote normally.
func TestPromotionVersionContractSkipsMigrationExcludedPlugins(t *testing.T) {
	excluded := versionContractPlugin{
		ID: "ai-context-limit", Version: "1.0.0", Digest: testDigest("planned"),
		CandidateRef: "registry.example.invalid/candidates/ai-context-limit@" + testDigest("planned"),
		Provenance:   "candidate", Blocked: true, ExistingDigest: testDigest("legacy-manual"),
	}
	promoted := versionContractPlugin{
		ID: "ai-agent", Version: "2.0.2", Digest: testDigest("agent"),
		CandidateRef: "registry.example.invalid/candidates/ai-agent@" + testDigest("agent"),
		Provenance:   "candidate",
	}
	result := runPromotionVersionContract(t, []versionContractPlugin{excluded, promoted})
	if result.err != nil {
		t.Fatalf("migration exclusion failed the promote batch: %v\n%s", result.err, result.output)
	}
	if strings.Contains(result.output, "immutable tag conflict") {
		t.Fatalf("excluded plugin still triggered the immutable tag conflict:\n%s", result.output)
	}
	if !strings.Contains(result.log, "cp registry.example.invalid/candidates/ai-agent@") {
		t.Fatalf("the non-excluded plugin was not promoted:\n%s", result.log)
	}
	if strings.Contains(result.log, "ai-context-limit") {
		t.Fatalf("the excluded plugin was probed or copied:\n%s", result.log)
	}
	entries := versionJournalEntries(t, result.journal)
	if len(entries) != 2 {
		t.Fatalf("version journal lost an entry: %#v", entries)
	}
	if entries[0]["preflight"] != "migration-excluded" || entries[1]["preflight"] != "absent" {
		t.Fatalf("unexpected version journal states: %#v", entries)
	}
	migration, ok := entries[0]["migration"].(map[string]any)
	if !ok || migration["state"] != "blocked" || migration["recommendation"] != "delete-legacy" || migration["existingDigest"] != excluded.ExistingDigest {
		t.Fatalf("version journal did not document the exclusion: %#v", entries[0])
	}
	if !strings.Contains(result.summary, "Migration preflight excluded from this promote batch: ai-context-limit") {
		t.Fatalf("step summary did not document the exclusion: %q", result.summary)
	}
	if got := result.state[excludedRef(excluded)]; digestOf(t, got) != excluded.ExistingDigest {
		t.Fatalf("the excluded legacy tag was mutated: %#v", got)
	}
}

// TestPromotionVersionContractFailsClosedWhenEverythingIsExcluded keeps an empty
// batch from publishing a degenerate completion marker.
func TestPromotionVersionContractFailsClosedWhenEverythingIsExcluded(t *testing.T) {
	result := runPromotionVersionContract(t, []versionContractPlugin{{
		ID: "only", Version: "1.0.0", Digest: testDigest("planned"),
		CandidateRef: "registry.example.invalid/candidates/only@" + testDigest("planned"),
		Provenance:   "candidate", Blocked: true, ExistingDigest: testDigest("legacy"),
	}})
	if result.err == nil || !strings.Contains(result.output, "every snapshot plugin is excluded by migration preflight") {
		t.Fatalf("an entirely excluded batch was promoted: err=%v\n%s", result.err, result.output)
	}
	assertNoLatestMutation(t, result.log)
}

// TestPromotionLatestContractSkipsMigrationExcludedPlugins proves the latest
// alias of an excluded plugin never moves and that the exclusion is documented
// on the completion marker journal.
func TestPromotionLatestContractSkipsMigrationExcludedPlugins(t *testing.T) {
	result := runPromotionLatestContract(t, false, []latestContractPlugin{
		{
			ID: "excluded", Version: "1.0.0", Digest: testDigest("planned"),
			CurrentDigest: testDigest("legacy-latest"),
			Blocked:       true, BlockedExistingDigest: testDigest("legacy-manual"),
		},
		{ID: "promoted", Version: "2.0.2", Digest: testDigest("agent")},
	})
	if result.err != nil {
		t.Fatalf("latest phase rejected a migration exclusion: %v\n%s", result.err, result.output)
	}
	if strings.Contains(result.log, "plugins/excluded") {
		t.Fatalf("excluded plugin was probed or its latest alias moved:\n%s", result.log)
	}
	if strings.Count(result.log, "cp ") != 1 || !strings.Contains(result.log, "plugins/promoted:latest") {
		t.Fatalf("only the promoted plugin's alias should move:\n%s", result.log)
	}
	entries := latestJournalEntries(t, result.journal)
	if len(entries) != 1 || entries[0]["ref"] != "registry.example.invalid/plugins/promoted:2.0.2" {
		t.Fatalf("excluded plugin joined the latest preflight entries: %#v", entries)
	}
	excludedList, ok := result.journal["migrationExcluded"].([]any)
	if !ok || len(excludedList) != 1 {
		t.Fatalf("latest journal did not document the exclusion: %#v", result.journal["migrationExcluded"])
	}
	documented, ok := excludedList[0].(map[string]any)
	if !ok || documented["logicalId"] != "excluded" || documented["recommendation"] != "delete-legacy" || documented["existingDigest"] != testDigest("legacy-manual") {
		t.Fatalf("latest journal exclusion record is incomplete: %#v", excludedList[0])
	}
}

// TestPromotionLatestContractFailsClosedWhenEverythingIsExcluded keeps an empty
// latest batch from pushing a marker with no entries.
func TestPromotionLatestContractFailsClosedWhenEverythingIsExcluded(t *testing.T) {
	result := runPromotionLatestContract(t, false, []latestContractPlugin{{
		ID: "only", Version: "1.0.0", Digest: testDigest("planned"),
		Blocked: true, BlockedExistingDigest: testDigest("legacy"),
	}})
	if result.err == nil || !strings.Contains(result.output, "no latest alias can move") {
		t.Fatalf("an entirely excluded batch moved latest: err=%v\n%s", result.err, result.output)
	}
	assertNoLatestMutation(t, result.log)
}

func excludedRef(plugin versionContractPlugin) string {
	return "registry.example.invalid/plugins/" + plugin.ID + ":" + plugin.Version
}

func digestOf(t *testing.T, value any) string {
	t.Helper()
	record, ok := value.(map[string]any)
	if !ok {
		t.Fatalf("registry state entry has unexpected type: %#v", value)
	}
	digest, _ := record["digest"].(string)
	return digest
}

func versionJournalEntries(t *testing.T, journal map[string]any) []map[string]any {
	t.Helper()
	if journal["phase"] != "version-complete" {
		t.Fatalf("version journal is not complete: %#v", journal)
	}
	raw, ok := journal["entries"].([]any)
	if !ok {
		t.Fatalf("version journal entries have unexpected type: %#v", journal)
	}
	entries := make([]map[string]any, 0, len(raw))
	for _, entry := range raw {
		entries = append(entries, entry.(map[string]any))
	}
	return entries
}

// runPromotionVersionContract executes the workflow's version-phase shell
// contract against a fake ORAS CLI and a file-backed registry, the same way the
// latest-phase contract is exercised.
func runPromotionVersionContract(t *testing.T, plugins []versionContractPlugin) versionContractResult {
	t.Helper()
	const gateway = "2.2.6"
	snapshot := map[string]any{"gatewayVersion": gateway, "plugins": []any{}}
	registry := map[string]any{}
	for _, plugin := range plugins {
		ref := excludedRef(plugin)
		entry := map[string]any{
			"logicalId": plugin.ID, "ociRef": ref, "digest": plugin.Digest, "version": plugin.Version,
			"candidateRef": plugin.CandidateRef, "provenanceMode": plugin.Provenance,
		}
		if plugin.Blocked {
			entry["migration"] = map[string]any{
				"state": "blocked", "existingDigest": plugin.ExistingDigest, "plannedDigest": plugin.Digest,
				"sourceComparison": "unannotated", "recommendation": "delete-legacy",
			}
			registry[ref] = map[string]any{"digest": plugin.ExistingDigest, "version": ""}
		}
		snapshot["plugins"] = append(snapshot["plugins"].([]any), entry)
	}
	return runPromotionVersionContractState(t, snapshot, registry)
}

// runPromotionVersionContractState executes the same contract against an
// arbitrary snapshot document, so a snapshot the real render pipeline produced
// can be promoted without re-describing it by hand.
func runPromotionVersionContractState(t *testing.T, snapshot, registry map[string]any) versionContractResult {
	t.Helper()
	root := t.TempDir()
	bin := filepath.Join(root, "bin")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	logPath := filepath.Join(root, "oras.log")
	statePath := filepath.Join(root, "registry.json")
	summaryPath := filepath.Join(root, "summary.md")

	gateway, _ := snapshot["gatewayVersion"].(string)
	if gateway == "" {
		t.Fatal("snapshot fixture has no gatewayVersion")
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
  jq -cn --arg version "$(jq -r '.version // empty' <<<"$record")" '{annotations:{"org.opencontainers.image.version":$version}}'
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
	versionContract := workflowShellContract(t, "promote-plugin-release.yaml", "promotion-version-contract")
	script := "set -euo pipefail\n" + descriptorContracts[0] + "\n" + versionContract
	snapshotSHA := sha256.Sum256([]byte("version-" + root))
	snapshotSHAHex := hex.EncodeToString(snapshotSHA[:])
	journalPath := filepath.Join("/tmp", "plugin-release-version-"+snapshotSHAHex+".json")
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
		"REGISTRY=registry.example.invalid",
		"SNAPSHOT_PATH="+snapshotPath,
		"SNAPSHOT_SHA256="+snapshotSHAHex,
		"GITHUB_STEP_SUMMARY="+summaryPath,
	)
	output, runErr := cmd.CombinedOutput()
	result := versionContractResult{err: runErr, output: string(output)}
	if logBytes, err := os.ReadFile(logPath); err == nil {
		result.log = string(logBytes)
	} else if !os.IsNotExist(err) {
		t.Fatal(err)
	}
	if summary, err := os.ReadFile(summaryPath); err == nil {
		result.summary = string(summary)
	} else if !os.IsNotExist(err) {
		t.Fatal(err)
	}
	if journalBytes, err := os.ReadFile(journalPath); err == nil {
		if err := json.Unmarshal(journalBytes, &result.journal); err != nil {
			t.Fatalf("decode version journal: %v", err)
		}
	} else if runErr == nil {
		t.Fatalf("successful version contract did not write its journal: %v", err)
	}
	if stateBytes, err := os.ReadFile(statePath); err == nil {
		if err := json.Unmarshal(stateBytes, &result.state); err != nil {
			t.Fatalf("decode registry state: %v", err)
		}
	} else if !os.IsNotExist(err) {
		t.Fatal(err)
	}
	return result
}
