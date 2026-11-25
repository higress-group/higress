package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func activateCPPFixture(catalog *Catalog) {
	catalog.ConsoleMarketplace = &ConsoleMarketplacePolicy{RequiredForStable: true, Bundles: map[string]ConsoleMarketplaceBundle{}}
	for index := range catalog.Plugins {
		plugin := &catalog.Plugins[index]
		if plugin.Implementation != "cpp" {
			plugin.ReleaseEligible = false
			plugin.UnmanagedReason = "fixture"
			continue
		}
		plugin.ReleaseEligible, plugin.UnmanagedReason = true, ""
		plugin.Consumers = PluginConsumers{Console: &ConsoleConsumer{PropertyKey: plugin.LogicalID, ResourceDir: plugin.LogicalID, URLForm: "oci"}, PluginServer: &PluginServerConsumer{InventoryKey: plugin.LogicalID, HTTPPath: plugin.LogicalID}}
		bundle := ConsoleMarketplaceBundle{Repository: "higress-group/higress-console", SourceCommit: strings.Repeat("a", 40)}
		for _, target := range []string{"spec.yaml", "README.md", "README_EN.md"} {
			bundle.Files = append(bundle.Files, ConsoleMarketplaceBundleFile{SourcePath: "backend/sdk/src/main/resources/plugins/" + plugin.LogicalID + "/" + target, TargetPath: target, SHA256: strings.Repeat("b", 64)})
		}
		catalog.ConsoleMarketplace.Bundles[plugin.LogicalID] = bundle
	}
	for consumer, entries := range catalog.ConsumerInventories {
		for index := range entries {
			entries[index] = ConsumerInventory{Key: entries[index].Key, Classification: "managed", LogicalID: entries[index].Key}
		}
		catalog.ConsumerInventories[consumer] = entries
	}
}

func importFixture(t *testing.T) (string, string, string, string, string, string, CandidateEvidenceFile) {
	t.Helper()
	root, catalogPath, base, _ := prepareCPPHotfixFixture(t, cppHotfixVersion)
	var catalog Catalog
	if _, err := readJSON(catalogPath, &catalog); err != nil {
		t.Fatal(err)
	}
	trackedCatalog := filepath.Join(root, "plugins/release/catalog.json")
	mustCanonical(t, trackedCatalog, catalog)
	mustRun(t, root, "git", "add", ".")
	mustRun(t, root, "git", "commit", "-qm", "tracked source catalog")
	artifactSource, _ := resolveCommit(root, "HEAD")
	oldPlan, err := buildPlanWithMode(root, catalogPath, "", base, artifactSource, "2.0.1", "", releaseModeCPPHotfix)
	if err != nil {
		t.Fatal(err)
	}
	oldEvidence := CandidateEvidenceFile{Plugins: map[string]CandidateEvidence{}}
	for _, entry := range oldPlan.Plugins {
		oldEvidence.Plugins[entry.LogicalID] = CandidateEvidence{CandidateRef: "registry.example/candidates/" + entry.LogicalID + ":old-plan@sha256:" + strings.Repeat("c", 64), Digest: "sha256:" + strings.Repeat("c", 64), SourceCommit: artifactSource, InputHash: entry.InputHash}
	}
	imports := "plugins/release/imports/fixture"
	sourcePlan, sourceEvidence, sourceSnapshot := imports+"/source-plan.json", imports+"/source-evidence.json", imports+"/source-snapshot.json"
	mustCanonical(t, filepath.Join(root, sourcePlan), oldPlan)
	mustCanonical(t, filepath.Join(root, sourceEvidence), oldEvidence)
	oldSnapshot, err := renderSnapshotWithMode(catalogPath, filepath.Join(root, sourcePlan), "", filepath.Join(root, sourceEvidence), "", releaseModeCPPHotfix)
	if err != nil {
		t.Fatal(err)
	}
	mustCanonical(t, filepath.Join(root, sourceSnapshot), oldSnapshot)
	sourceCatalog := imports + "/source-catalog.json"
	mustWrite(t, filepath.Join(root, sourceCatalog), string(mustRead(t, trackedCatalog)))
	proof := func(relative string) ProofRef {
		return ProofRef{Path: relative, SHA256: sha256Hex(mustRead(t, filepath.Join(root, relative)))}
	}
	candidatePath, historyPath := imports+"/candidate-import.json", imports+"/history-import.json"
	mustCanonical(t, filepath.Join(root, candidatePath), CandidateImport{SchemaVersion: 1, Repository: "higress-group/higress", SourceCommit: artifactSource, Catalog: proof(sourceCatalog), Plan: proof(sourcePlan), Snapshot: proof(sourceSnapshot), Evidence: proof(sourceEvidence)})
	lock := legacyHistoryLock{SchemaVersion: 1, Plugins: map[string]legacyHistoryArtifact{}}
	for _, id := range sortedCPPIDs() {
		lock.Plugins[id] = legacyHistoryArtifact{LogicalKey: id, ServedPath: id, ServedVersion: "2.0.0", Repository: "registry.example/plugins/" + id, Tag: "2.0.0", Digest: "sha256:" + strings.Repeat("d", 64), WasmSHA256: strings.Repeat("e", 64)}
	}
	lockPath := imports + "/legacy-lock.json"
	mustCanonical(t, filepath.Join(root, lockPath), lock)
	mustCanonical(t, filepath.Join(root, historyPath), HistoryImport{SchemaVersion: 1, Repository: "higress-group/plugin-server", SourceCommit: strings.Repeat("f", 40), Lock: proof(lockPath), LogicalIDs: sortedCPPIDs()})
	activateCPPFixture(&catalog)
	mustCanonical(t, catalogPath, catalog)
	mustCanonical(t, trackedCatalog, catalog)
	mustRun(t, root, "git", "add", ".")
	mustRun(t, root, "git", "commit", "-qm", "F with reviewed import proofs")
	freeze, _ := resolveCommit(root, "HEAD")
	previousPath := filepath.Join(root, "previous.json")
	mustCanonical(t, previousPath, Snapshot{SchemaVersion: 1, GatewayVersion: "2.0.0", SourceCommit: artifactSource, ProvenanceMode: "candidate", Plugins: []SnapshotEntry{}})
	return root, catalogPath, previousPath, freeze, candidatePath, historyPath, oldEvidence
}

func TestManagedCPPExplicitImportPreservesArtifactSourceAndTuple(t *testing.T) {
	root, catalogPath, previousPath, freeze, candidatePath, historyPath, original := importFixture(t)
	plan, err := buildPlanWithImports(root, catalogPath, previousPath, "", freeze, "2.1.0", "", "", candidatePath, historyPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Plugins) != 9 {
		t.Fatalf("selected %d instead of fixed nine", len(plan.Plugins))
	}
	for _, selected := range plan.Plugins {
		if selected.ArtifactSourceCommit != original.Plugins[selected.LogicalID].SourceCommit || selected.ArtifactSourceCommit == freeze || selected.CandidateImport == nil {
			t.Fatal("artifact source H was replaced by freeze F")
		}
		entry := original.Plugins[selected.LogicalID]
		entry.WasmSHA256 = strings.Repeat("a", 64)
		original.Plugins[selected.LogicalID] = entry
	}
	planPath, evidencePath := filepath.Join(root, "plan.json"), filepath.Join(root, "merged.json")
	mustCanonical(t, planPath, plan)
	mustCanonical(t, evidencePath, original)
	snapshot, err := renderSnapshotAtRoot(root, catalogPath, planPath, previousPath, evidencePath, "", "")
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range snapshot.Plugins {
		old := original.Plugins[entry.LogicalID]
		if entry.SourceCommit != old.SourceCommit || entry.Digest != old.Digest || entry.InputHash != old.InputHash || entry.CandidateRef != old.CandidateRef || len(entry.History) != 1 || entry.History[0].Version != "2.0.0" || entry.History[0].SourceCommit != "" {
			t.Fatal("import changed tuple or invented legacy provenance")
		}
	}
}

func TestManagedCPPImportFailsClosed(t *testing.T) {
	for _, mutation := range []string{"missing-both", "missing-history", "missing-candidate", "proof-bytes", "source", "version", "input", "reference", "previous", "hotfix", "untracked"} {
		t.Run(mutation, func(t *testing.T) {
			root, catalog, previous, freeze, candidate, history, _ := importFixture(t)
			switch mutation {
			case "missing-both":
				candidate, history = "", ""
			case "missing-history":
				history = ""
			case "missing-candidate":
				candidate = ""
			case "proof-bytes":
				mustWrite(t, filepath.Join(root, "plugins/release/imports/fixture/source-evidence.json"), "{}\n")
			case "source":
				var manifest CandidateImport
				_, _ = readJSON(filepath.Join(root, candidate), &manifest)
				manifest.SourceCommit = freeze
				mustCanonical(t, filepath.Join(root, candidate), manifest)
			case "version":
				mustWrite(t, filepath.Join(root, "plugins/wasm-cpp/extensions/basic_auth/VERSION"), "2.0.2\n")
			case "input":
				mustWrite(t, filepath.Join(root, "plugins/wasm-cpp/common/route_rule_matcher.h"), "changed\n")
			case "reference":
				var evidence CandidateEvidenceFile
				path := filepath.Join(root, "plugins/release/imports/fixture/source-evidence.json")
				_, _ = readJSON(path, &evidence)
				entry := evidence.Plugins["basic-auth"]
				entry.CandidateRef = "registry.example/candidates/other@" + entry.Digest
				evidence.Plugins["basic-auth"] = entry
				mustCanonical(t, path, evidence)
			case "previous":
				mustCanonical(t, previous, Snapshot{SchemaVersion: 1, GatewayVersion: "2.0.0", SourceCommit: freeze, ProvenanceMode: "candidate", Plugins: []SnapshotEntry{{LogicalID: "basic-auth", Implementation: "cpp", Version: "2.0.1"}}})
			case "hotfix":
				if _, err := buildPlanWithImports(root, catalog, "", freeze, freeze, "2.1.0", "", releaseModeCPPHotfix, candidate, history); err == nil {
					t.Fatal("hotfix accepted imports")
				}
				return
			case "untracked":
				data := mustRead(t, filepath.Join(root, candidate))
				candidate = "plugins/release/imports/untracked.json"
				mustWrite(t, filepath.Join(root, candidate), string(data))
			}
			if mutation == "version" || mutation == "input" {
				mustRun(t, root, "git", "add", ".")
				mustRun(t, root, "git", "commit", "-qm", "drift")
				freeze, _ = resolveCommit(root, "HEAD")
			}
			if _, err := buildPlanWithImports(root, catalog, previous, "", freeze, "2.1.0", "", "", candidate, history); err == nil {
				t.Fatal("invalid candidate/history import accepted")
			}
		})
	}
}

func TestManagedCPPFixedCatalogStatesAndDirectorySet(t *testing.T) {
	for _, mutation := range []string{"valid-unmanaged", "valid-managed", "partial", "new-directory", "missing-excluded", "excluded-id", "source", "image", "consumer", "inventory", "bundle"} {
		t.Run(mutation, func(t *testing.T) {
			root, catalogPath, _, _ := prepareCPPHotfixFixture(t, cppHotfixVersion)
			var catalog Catalog
			_, _ = readJSON(catalogPath, &catalog)
			if mutation != "valid-unmanaged" {
				activateCPPFixture(&catalog)
			}
			index := 1
			switch mutation {
			case "partial":
				catalog.Plugins[index].ReleaseEligible = false
			case "new-directory":
				mustWrite(t, filepath.Join(root, "plugins/wasm-cpp/extensions/new_plugin/.keep"), "")
			case "missing-excluded":
				if err := os.RemoveAll(filepath.Join(root, "plugins/wasm-cpp/extensions/model_router")); err != nil {
					t.Fatal(err)
				}
			case "excluded-id":
				catalog.Plugins[index].LogicalID = "model-router"
			case "source":
				catalog.Plugins[index].SourceDir = "plugins/wasm-cpp/extensions/model_router"
			case "image":
				catalog.Plugins[index].Image = "plugins/other"
			case "consumer":
				catalog.Plugins[index].Consumers.PluginServer.HTTPPath = "other"
			case "inventory":
				catalog.ConsumerInventories["console"][0].Classification = "unmanaged"
			case "bundle":
				delete(catalog.ConsoleMarketplace.Bundles, catalog.Plugins[index].LogicalID)
			}
			mustCanonical(t, catalogPath, catalog)
			err := validateCatalog(root, catalogPath)
			if mutation == "valid-unmanaged" || mutation == "valid-managed" {
				if err != nil {
					t.Fatal(err)
				}
			} else if err == nil {
				t.Fatal("invalid fixed C++ state accepted")
			}
		})
	}
}

func TestManagedProofRejectsUnsafePathsHashesAndSymlinks(t *testing.T) {
	root := t.TempDir()
	for _, path := range []string{"/tmp/proof", "../proof", `a\b`, "a/../b", "a\x00b"} {
		if err := validateProof(ProofRef{Path: path, SHA256: strings.Repeat("a", 64)}); err == nil {
			t.Fatalf("accepted unsafe proof %q", path)
		}
	}
	mustWrite(t, filepath.Join(root, "target.json"), "{}\n")
	if err := os.Symlink("target.json", filepath.Join(root, "proof.json")); err != nil {
		t.Fatal(err)
	}
	if _, err := readProof(root, ProofRef{Path: "proof.json", SHA256: sha256Hex([]byte("{}\n"))}, "", nil); err == nil {
		t.Fatal("symlink proof accepted")
	}
}

func TestManagedContractFailurePropagation(t *testing.T) {
	if os.Getenv("PLUGIN_RELEASE_NEGATIVE_PROBE") == "1" {
		t.Fatal("intentional runner-only propagation probe")
	}
	if err := validateProof(ProofRef{Path: "../escape", SHA256: strings.Repeat("a", 64)}); err == nil {
		t.Fatal("invalid proof accepted")
	}
}

func TestManagedPinnedCandidateReferenceRetainsOriginalTag(t *testing.T) {
	digest := "sha256:" + strings.Repeat("a", 64)
	for reference, expected := range map[string]bool{"registry.example/candidates/demo:old-plan@" + digest: true, "registry.example/candidates/demo@" + digest: true, "registry.example/candidates/demo:latest@" + digest: false, "registry.example/candidates/demo:old-plan": false} {
		if validPinnedCandidateReference(reference, digest) != expected {
			t.Fatalf("pinned reference contract: %s", reference)
		}
	}
}

func TestManagedCPPFirstImportRejectsEachOccupiedDifferentTag(t *testing.T) {
	for _, id := range sortedCPPIDs() {
		t.Run(id, func(t *testing.T) {
			root, catalogPath, previousPath, freeze, candidatePath, historyPath, evidence := importFixture(t)
			plan, err := buildPlanWithImports(root, catalogPath, previousPath, "", freeze, "2.1.0", "", "", candidatePath, historyPath)
			if err != nil {
				t.Fatal(err)
			}
			planPath, evidencePath := filepath.Join(root, "plan.json"), filepath.Join(root, "merged.json")
			mustCanonical(t, planPath, plan)
			mustCanonical(t, evidencePath, evidence)
			controlDigest := "sha256:" + strings.Repeat("8", 64)
			mustCanonical(t, previousPath, Snapshot{Plugins: []SnapshotEntry{{LogicalID: "control", OCIRef: "registry.example/plugins/control:1.0.0", Digest: controlDigest, ProvenanceMode: "public"}}})
			withManifestResolver(t, func(ref string) (ociManifest, error) {
				if ref == "registry.example/plugins/control:1.0.0" {
					return ociManifest{Digest: controlDigest}, nil
				}
				if ref == "registry.example/plugins/"+id+":2.0.1" {
					return ociManifest{Digest: "sha256:" + strings.Repeat("9", 64)}, nil
				}
				return ociManifest{}, fmt.Errorf("HTTP 404 Not Found")
			})
			report, err := migrationPreflight(catalogPath, planPath, previousPath, evidencePath)
			if err == nil || len(report.Entries) != 1 || report.Entries[0].LogicalID != id || report.Entries[0].ArtifactSourceCommit != evidence.Plugins[id].SourceCommit {
				t.Fatalf("occupied imported tag did not block complete migration: %+v %v", report, err)
			}
		})
	}
}

func TestManagedCPPImportCaptureIsReadOnlyAndHasNoBuildFallback(t *testing.T) {
	for _, missing := range []bool{false, true} {
		t.Run(map[bool]string{false: "reuse-all-nine", true: "candidate-404-zero-write"}[missing], func(t *testing.T) {
			root, catalogPath, previousPath, freeze, candidatePath, historyPath, original := importFixture(t)
			plan, err := buildPlanWithImports(root, catalogPath, previousPath, "", freeze, "2.1.0", "", "", candidatePath, historyPath)
			if err != nil {
				t.Fatal(err)
			}
			planPath, builtPath, outputPath := filepath.Join(root, "plan.json"), filepath.Join(root, "built.json"), filepath.Join(root, "output.json")
			mustCanonical(t, planPath, plan)
			mustCanonical(t, builtPath, CandidateEvidenceFile{Plugins: map[string]CandidateEvidence{}})
			calls := map[string]int{}
			withWasmHashResolver(t, func(_ string, entry SnapshotEntry, ref string) (string, error) {
				old := original.Plugins[entry.LogicalID]
				if ref != old.CandidateRef || entry.SourceCommit != old.SourceCommit || entry.Digest != old.Digest || entry.InputHash != old.InputHash {
					t.Fatal("candidate import changed immutable artifact source/ref/bytes")
				}
				calls[entry.LogicalID]++
				if missing {
					return "", fmt.Errorf("HTTP 404 Not Found")
				}
				return strings.Repeat("a", 64), nil
			})
			err = commandCaptureHistoryEvidence([]string{"--root", root, "--catalog", catalogPath, "--plan", planPath, "--previous", previousPath, "--candidate-evidence", builtPath, "--output", outputPath})
			if missing {
				if err == nil {
					t.Fatal("missing candidate accepted")
				}
				if _, err := os.Stat(outputPath); !os.IsNotExist(err) {
					t.Fatal("candidate failure wrote fallback evidence")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if len(calls) != 9 {
				t.Fatalf("read only %d imported artifacts", len(calls))
			}
			for id, count := range calls {
				if count != 1 {
					t.Fatalf("imported candidate %s was resolved %d times", id, count)
				}
			}
		})
	}
}

func TestManagedCPPImportedSnapshotVerifiesPreparationAndMergedHeads(t *testing.T) {
	root, catalogPath, previousPath, freeze, candidatePath, historyPath, evidence := importFixture(t)
	plan, err := buildPlanWithImports(root, catalogPath, previousPath, "", freeze, "2.1.0", "", "", candidatePath, historyPath)
	if err != nil {
		t.Fatal(err)
	}
	for id, proof := range evidence.Plugins {
		proof.WasmSHA256 = strings.Repeat("a", 64)
		evidence.Plugins[id] = proof
	}
	planPath, evidencePath, snapshotPath := filepath.Join(root, "plan.json"), releaseEvidencePath(root, Snapshot{GatewayVersion: plan.GatewayVersion}), filepath.Join(root, "snapshot.json")
	mustCanonical(t, planPath, plan)
	mustCanonical(t, evidencePath, evidence)
	snapshot, err := renderSnapshotAtRoot(root, catalogPath, planPath, previousPath, evidencePath, "", "")
	if err != nil {
		t.Fatal(err)
	}
	mustCanonical(t, snapshotPath, snapshot)
	if err := applyPlan(root, plan); err != nil {
		t.Fatal(err)
	}
	mustRun(t, root, "git", "add", ".")
	mustRun(t, root, "git", "commit", "-qm", "preparation fixture Q")
	preparation, _ := resolveCommit(root, "HEAD")
	if err := verifySnapshotBindings(root, catalogPath, snapshotPath, planPath, previousPath, freeze, preparation, false, "candidate"); err != nil {
		t.Fatalf("import verification at Q: %v", err)
	}
	mustRun(t, root, "git", "commit", "--allow-empty", "-qm", "merged fixture M")
	merged, _ := resolveCommit(root, "HEAD")
	if err := verifySnapshotBindings(root, catalogPath, snapshotPath, planPath, previousPath, freeze, merged, false, "candidate"); err != nil {
		t.Fatalf("import verification at M: %v", err)
	}
	mustWrite(t, filepath.Join(root, "plugins/wasm-cpp/common/route_rule_matcher.h"), "changed matcher\n")
	mustRun(t, root, "git", "add", ".")
	mustRun(t, root, "git", "commit", "-qm", "shared matcher drift")
	drifted, _ := resolveCommit(root, "HEAD")
	if err := verifySnapshotBindings(root, catalogPath, snapshotPath, planPath, previousPath, freeze, drifted, false, "candidate"); err == nil {
		t.Fatal("committed-source artifact drift accepted")
	}
	next, err := buildPlan(root, catalogPath, snapshotPath, "", drifted, "2.2.0", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(next.Plugins) != 9 {
		t.Fatalf("shared matcher pruned normal managed plan to %d", len(next.Plugins))
	}
	for _, entry := range next.Plugins {
		if entry.Version != "2.0.2" || entry.CandidateImport != nil || entry.ArtifactSourceCommit != "" {
			t.Fatal("later managed planning reimported a candidate or failed to bump")
		}
	}
}
