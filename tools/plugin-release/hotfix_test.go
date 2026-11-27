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
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
)

func TestCPPHotfixPlanRenderVerifyAndOrdinaryIsolation(t *testing.T) {
	root, catalogPath, base, target := prepareCPPHotfixFixture(t, cppHotfixVersion)
	previousPath := filepath.Join(root, "previous.json")
	previous := Snapshot{SchemaVersion: snapshotSchemaVersion, GatewayVersion: "2.2.6", SourceCommit: base, ProvenanceMode: "candidate", Plugins: []SnapshotEntry{{LogicalID: "demo", Version: "1.0.0"}}}
	if err := writeCanonical(previousPath, previous); err != nil {
		t.Fatal(err)
	}
	ordinary, err := buildPlan(root, catalogPath, previousPath, "", target, "2.2.7", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(ordinary.Plugins) != 0 {
		t.Fatalf("ordinary planner selected hotfix-only entries: %+v", ordinary.Plugins)
	}

	plan, err := buildPlanWithMode(root, catalogPath, "", base, target, "2.2.7", "", releaseModeCPPHotfix)
	if err != nil {
		t.Fatal(err)
	}
	assertCPPHotfixIDs(t, plan.Plugins)
	planPath := filepath.Join(root, "plan.json")
	if err := writeCanonical(planPath, plan); err != nil {
		t.Fatal(err)
	}
	evidence := CandidateEvidenceFile{Plugins: map[string]CandidateEvidence{}}
	for _, entry := range plan.Plugins {
		digest := "sha256:" + sha256Hex([]byte(entry.LogicalID))
		evidence.Plugins[entry.LogicalID] = CandidateEvidence{CandidateRef: "registry.example/candidates/" + entry.LogicalID + "@" + digest, Digest: digest, SourceCommit: target, InputHash: entry.InputHash}
	}
	evidencePath := filepath.Join(root, "evidence.json")
	if err := writeCanonical(evidencePath, evidence); err != nil {
		t.Fatal(err)
	}
	snapshot, err := renderSnapshotWithMode(catalogPath, planPath, "", evidencePath, "", releaseModeCPPHotfix)
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Plugins) != len(cppHotfixSources) {
		t.Fatalf("snapshot contains %d plugins", len(snapshot.Plugins))
	}
	for _, entry := range snapshot.Plugins {
		if entry.Version != cppHotfixVersion || !reflect.DeepEqual(entry.Consumers, PluginConsumers{}) {
			t.Fatalf("invalid hotfix snapshot entry: %+v", entry)
		}
	}
	snapshotPath := filepath.Join(root, "snapshot.json")
	if err := writeCanonical(snapshotPath, snapshot); err != nil {
		t.Fatal(err)
	}
	if err := verifySnapshotBindingsWithMode(root, catalogPath, snapshotPath, planPath, "", target, target, false, "candidate", releaseModeCPPHotfix); err != nil {
		t.Fatal(err)
	}

	snapshot.Plugins[0].Consumers = PluginConsumers{PluginServer: &PluginServerConsumer{InventoryKey: "basic-auth", HTTPPath: "basic-auth"}}
	if err := writeCanonical(snapshotPath, snapshot); err != nil {
		t.Fatal(err)
	}
	if err := verifySnapshotBindingsWithMode(root, catalogPath, snapshotPath, planPath, "", target, target, false, "candidate", releaseModeCPPHotfix); err == nil || !strings.Contains(err.Error(), "consumer") {
		t.Fatalf("consumer injection was not rejected: %v", err)
	}
}

func TestCPPHotfixRejectsWrongVersionExtraPluginAndMissingEvidence(t *testing.T) {
	root, catalogPath, base, target := prepareCPPHotfixFixture(t, "2.0.2")
	if _, err := buildPlanWithMode(root, catalogPath, "", base, target, "2.2.7", "", releaseModeCPPHotfix); err == nil || !strings.Contains(err.Error(), "VERSION must be 2.0.1") {
		t.Fatalf("wrong VERSION was not rejected: %v", err)
	}

	root, catalogPath, base, target = prepareCPPHotfixFixture(t, cppHotfixVersion)
	plan, err := buildPlanWithMode(root, catalogPath, "", base, target, "2.2.7", "", releaseModeCPPHotfix)
	if err != nil {
		t.Fatal(err)
	}
	plan.Plugins = append(plan.Plugins, PlanEntry{LogicalID: "model-mapper", Implementation: "cpp", Version: cppHotfixVersion})
	if err := validateCPPHotfixPlan(plan); err == nil {
		t.Fatal("out-of-scope model-mapper was accepted")
	}
	plan.Plugins = plan.Plugins[:len(cppHotfixSources)]
	planPath := filepath.Join(root, "plan.json")
	if err := writeCanonical(planPath, plan); err != nil {
		t.Fatal(err)
	}
	evidencePath := filepath.Join(root, "evidence.json")
	if err := writeCanonical(evidencePath, CandidateEvidenceFile{Plugins: map[string]CandidateEvidence{}}); err != nil {
		t.Fatal(err)
	}
	if _, err := renderSnapshotWithMode(catalogPath, planPath, "", evidencePath, "", releaseModeCPPHotfix); err == nil || !strings.Contains(err.Error(), "missing candidate evidence") {
		t.Fatalf("missing candidate evidence was not rejected: %v", err)
	}
}

func prepareCPPHotfixFixture(t *testing.T, targetVersion string) (string, string, string, string) {
	t.Helper()
	root := t.TempDir()
	mustRun(t, root, "git", "init", "-q")
	mustRun(t, root, "git", "config", "user.name", "test")
	mustRun(t, root, "git", "config", "user.email", "test@example.com")
	mustWrite(t, filepath.Join(root, "plugins/wasm-go/extensions/demo/main.go"), "package main\n")
	mustWrite(t, filepath.Join(root, "plugins/wasm-go/extensions/demo/VERSION"), "1.0.0\n")
	mustWrite(t, filepath.Join(root, "plugins/wasm-rust/extensions/.gitkeep"), "")
	for _, path := range []string{".bazelrc", ".bazelversion", "BUILD", "Dockerfile", "Makefile", "WORKSPACE", "bazel/wasm.bzl", "common/route_rule_matcher.h"} {
		mustWrite(t, filepath.Join(root, "plugins/wasm-cpp", path), "fixture\n")
	}
	catalog := Catalog{SchemaVersion: catalogSchemaVersion, Registry: "registry.example", SharedInputGroups: map[string][]string{"cpp-workspace": {
		"plugins/wasm-cpp/.bazelrc", "plugins/wasm-cpp/.bazelversion", "plugins/wasm-cpp/BUILD", "plugins/wasm-cpp/Dockerfile",
		"plugins/wasm-cpp/Makefile", "plugins/wasm-cpp/WORKSPACE", "plugins/wasm-cpp/bazel/**", "plugins/wasm-cpp/common/**",
	}}, ConsumerInventories: map[string][]ConsumerInventory{}, Plugins: []Plugin{{LogicalID: "demo", Implementation: "go", SourceDir: "plugins/wasm-go/extensions/demo", Image: "plugins/demo", ReleaseEligible: true, ArtifactInputs: []string{"plugins/wasm-go/extensions/demo/**"}}}}
	ids := make([]string, 0, len(cppHotfixSources))
	for id := range cppHotfixSources {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		source := "plugins/wasm-cpp/extensions/" + cppHotfixSources[id]
		mustWrite(t, filepath.Join(root, source, "plugin.cc"), "// fixture\n")
		mustWrite(t, filepath.Join(root, source, "VERSION"), "2.0.0\n")
		catalog.Plugins = append(catalog.Plugins, Plugin{LogicalID: id, Implementation: "cpp", SourceDir: source, Image: "plugins/" + id, ReleaseEligible: false, UnmanagedReason: "hotfix-only", ArtifactInputs: []string{source + "/**"}, SharedInputGroups: []string{"cpp-workspace"}})
		for _, consumer := range []string{"console", "pluginServer"} {
			catalog.ConsumerInventories[consumer] = append(catalog.ConsumerInventories[consumer], ConsumerInventory{Key: id, Classification: "unmanaged", Reason: "hotfix-only"})
		}
	}
	for _, source := range excludedCPPSources {
		mustWrite(t, filepath.Join(root, "plugins/wasm-cpp/extensions", source, ".keep"), "")
	}
	catalogPath := filepath.Join(root, "catalog.json")
	if err := writeCanonical(catalogPath, catalog); err != nil {
		t.Fatal(err)
	}
	mustRun(t, root, "git", "add", ".")
	mustRun(t, root, "git", "commit", "-q", "-m", "base")
	base, err := resolveCommit(root, "HEAD")
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range ids {
		path := filepath.Join(root, "plugins/wasm-cpp/extensions", cppHotfixSources[id], "VERSION")
		if err := os.WriteFile(path, []byte(targetVersion+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	mustRun(t, root, "git", "add", ".")
	mustRun(t, root, "git", "commit", "-q", "-m", "hotfix")
	target, err := resolveCommit(root, "HEAD")
	if err != nil {
		t.Fatal(err)
	}
	return root, catalogPath, base, target
}

func assertCPPHotfixIDs(t *testing.T, entries []PlanEntry) {
	t.Helper()
	ids := []string{}
	for _, entry := range entries {
		ids = append(ids, entry.LogicalID)
		if entry.Version != cppHotfixVersion {
			t.Fatalf("%s has version %s", entry.LogicalID, entry.Version)
		}
	}
	sort.Strings(ids)
	expected := make([]string, 0, len(cppHotfixSources))
	for id := range cppHotfixSources {
		expected = append(expected, id)
	}
	sort.Strings(expected)
	if !reflect.DeepEqual(ids, expected) {
		t.Fatalf("hotfix IDs = %v, want %v", ids, expected)
	}
}
