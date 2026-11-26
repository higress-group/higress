package main

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func withWasmHashResolver(t *testing.T, resolver func(string, SnapshotEntry, string) (string, error)) {
	t.Helper()
	previous := ociWasmHashResolver
	ociWasmHashResolver = resolver
	t.Cleanup(func() { ociWasmHashResolver = previous })
}

func historyFixture(t *testing.T, existingHash bool) (string, Catalog, Plan, Snapshot, CandidateEvidenceFile) {
	return historyFixtureForImplementation(t, existingHash, "go")
}

func historyFixtureForImplementation(t *testing.T, existingHash bool, implementation string) (string, Catalog, Plan, Snapshot, CandidateEvidenceFile) {
	t.Helper()
	root := t.TempDir()
	mustRun(t, root, "git", "init", "-q")
	mustRun(t, root, "git", "config", "user.name", "test")
	mustRun(t, root, "git", "config", "user.email", "test@example.com")
	source := "plugins/wasm-" + implementation + "/extensions/demo"
	mustWrite(t, filepath.Join(root, source, "main.go"), "package main\n")
	mustWrite(t, filepath.Join(root, source, "VERSION"), "1.0.0\n")
	for _, language := range []string{"go", "rust"} {
		mustWrite(t, filepath.Join(root, "plugins/wasm-"+language+"/extensions/.keep"), "")
	}
	mustRun(t, root, "git", "add", ".")
	mustRun(t, root, "git", "commit", "-qm", "source")
	head, _ := resolveCommit(root, "HEAD")
	plugin := Plugin{LogicalID: "demo", Implementation: implementation, SourceDir: source, Image: "plugins/demo", ReleaseEligible: true, ArtifactInputs: []string{source + "/**"}, Consumers: PluginConsumers{PluginServer: &PluginServerConsumer{InventoryKey: "demo", HTTPPath: "demo"}}}
	catalog := Catalog{SchemaVersion: 1, Registry: "registry.example", Plugins: []Plugin{plugin}, ConsumerInventories: map[string][]ConsumerInventory{"pluginServer": {{Key: "demo", Classification: "managed", LogicalID: "demo"}}}}
	mustCanonical(t, filepath.Join(root, "catalog.json"), catalog)
	oldHash, err := inputHash(root, head, "1.0.0", catalog, plugin)
	if err != nil {
		t.Fatal(err)
	}
	newHash, err := inputHash(root, head, "1.0.1", catalog, plugin)
	if err != nil {
		t.Fatal(err)
	}
	old := SnapshotEntry{LogicalID: "demo", Implementation: implementation, SourceDir: source, Image: plugin.Image, Version: "1.0.0", OCIRef: "registry.example/plugins/demo:1.0.0", Digest: "sha256:" + strings.Repeat("a", 64), InputHash: oldHash, SourceCommit: head, CandidateRef: "registry.example/candidates/demo@sha256:" + strings.Repeat("a", 64), ProvenanceMode: "candidate", Consumers: plugin.Consumers,
		History: []HistoryArtifact{{Version: "0.9.0", OCIRef: "registry.example/plugins/demo:0.9.0", Digest: "sha256:" + strings.Repeat("c", 64), WasmSHA256: strings.Repeat("d", 64), ProvenanceMode: "public"}}}
	if existingHash {
		old.WasmSHA256 = strings.Repeat("e", 64)
	}
	previous := Snapshot{SchemaVersion: 1, GatewayVersion: "2.0.0", SourceCommit: head, ProvenanceMode: "candidate", Plugins: []SnapshotEntry{old, {LogicalID: "control", Version: "1.0.0", OCIRef: "registry.example/plugins/control:1.0.0", Digest: "sha256:" + strings.Repeat("f", 64), ProvenanceMode: "public"}}}
	plan := Plan{SchemaVersion: 1, GatewayVersion: "2.0.1", SourceCommit: head, BaseCommit: head, PreviousRelease: "2.0.0", CatalogSHA256: sha256Hex(mustRead(t, filepath.Join(root, "catalog.json"))), Plugins: []PlanEntry{{LogicalID: "demo", Implementation: implementation, SourceDir: source, Image: plugin.Image, PreviousVersion: "1.0.0", Version: "1.0.1", InputHash: newHash, ChangedPaths: []string{}}}}
	plan.PlanID = "sha256:" + canonicalObjectHash(plan, true)
	evidence := CandidateEvidenceFile{Plugins: map[string]CandidateEvidence{"demo": {CandidateRef: "registry.example/candidates/demo@sha256:" + strings.Repeat("b", 64), Digest: "sha256:" + strings.Repeat("b", 64), SourceCommit: head, InputHash: newHash, WasmSHA256: strings.Repeat("e", 64)}}}
	mustCanonical(t, filepath.Join(root, "plan.json"), plan)
	mustCanonical(t, filepath.Join(root, "previous.json"), previous)
	mustCanonical(t, filepath.Join(root, "built.json"), evidence)
	withWasmHashResolver(t, func(_ string, entry SnapshotEntry, _ string) (string, error) {
		if entry.Version == "0.9.0" {
			return strings.Repeat("d", 64), nil
		}
		return strings.Repeat("e", 64), nil
	})
	return root, catalog, plan, previous, evidence
}

func mustCanonical(t *testing.T, path string, value any) {
	t.Helper()
	if err := writeCanonical(path, value); err != nil {
		t.Fatal(err)
	}
}

func historyRegistry(t *testing.T, previous Snapshot, behavior string) {
	t.Helper()
	old := previous.Plugins[0]
	withManifestResolver(t, func(ref string) (ociManifest, error) {
		if ref == previous.Plugins[1].OCIRef {
			return ociManifest{Digest: previous.Plugins[1].Digest}, nil
		}
		if ref == old.OCIRef {
			switch behavior {
			case "404":
				return ociManifest{}, errors.New("HTTP 404 Not Found")
			case "401":
				return ociManifest{}, errors.New("HTTP 401 Unauthorized")
			case "403":
				return ociManifest{}, errors.New("HTTP 403 Forbidden")
			case "network":
				return ociManifest{}, errors.New("connection reset by peer")
			case "unknown":
				return ociManifest{}, errors.New("unclassified registry failure")
			case "different":
				return ociManifest{Digest: "sha256:" + strings.Repeat("9", 64)}, nil
			case "annotations":
				return ociManifest{Digest: old.Digest}, nil
			}
			return ociManifest{Digest: old.Digest, Annotations: map[string]string{"org.opencontainers.image.version": old.Version, "org.opencontainers.image.revision": old.SourceCommit, "io.higress.plugin.input-hash": old.InputHash}}, nil
		}
		if ref == "registry.example/plugins/demo:0.9.0" {
			return ociManifest{Digest: old.History[0].Digest}, nil
		}
		if strings.Contains(ref, "/candidates/demo@") {
			return ociManifest{Digest: "sha256:" + strings.Repeat("b", 64), Annotations: map[string]string{"org.opencontainers.image.version": "1.0.1", "org.opencontainers.image.revision": old.SourceCommit, "io.higress.plugin.input-hash": ""}}, nil
		}
		return ociManifest{}, errors.New("unexpected registry reference " + ref)
	})
}

func TestManagedHistoryPublicationMatrix(t *testing.T) {
	for _, implementation := range []string{"go", "rust"} {
		t.Run(implementation, func(t *testing.T) { testManagedHistoryPublicationMatrix(t, implementation) })
	}
}

func testManagedHistoryPublicationMatrix(t *testing.T, implementation string) {
	for _, hasHash := range []bool{false, true} {
		for _, behavior := range []string{"published", "404", "excluded", "401", "403", "network", "unknown", "different", "annotations"} {
			t.Run(map[bool]string{false: "missing-hash", true: "existing-hash"}[hasHash]+"/"+behavior, func(t *testing.T) {
				root, _, plan, previous, evidence := historyFixtureForImplementation(t, hasHash, implementation)
				if behavior == "excluded" {
					previous.Plugins[0].Migration = &SnapshotMigration{State: migrationStateBlocked, SourceCommit: plan.SourceCommit, ExistingDigest: "sha256:" + strings.Repeat("9", 64), PlannedDigest: previous.Plugins[0].Digest, SourceComparison: migrationSourceUnannotated, Recommendation: migrationRecommendDeleteLegacy}
					mustCanonical(t, filepath.Join(root, "previous.json"), previous)
				}
				historyRegistry(t, previous, behavior)
				output := filepath.Join(root, "merged.json")
				err := commandCaptureHistoryEvidence([]string{"--root", root, "--catalog", filepath.Join(root, "catalog.json"), "--plan", filepath.Join(root, "plan.json"), "--previous", filepath.Join(root, "previous.json"), "--candidate-evidence", filepath.Join(root, "built.json"), "--output", output})
				valid := behavior == "published" || behavior == "404" || behavior == "excluded"
				if !valid {
					if err == nil {
						t.Fatal("registry failure accepted as publication/absence")
					}
					if _, statErr := os.Stat(output); !os.IsNotExist(statErr) {
						t.Fatal("failed capture wrote output")
					}
					return
				}
				if err != nil {
					t.Fatal(err)
				}
				if _, err := readJSON(output, &evidence); err != nil {
					t.Fatal(err)
				}
				if err := replayHistoryEvidence(root, plan, previous, evidence.HistoryEvidence); err != nil {
					t.Fatal(err)
				}
				snapshot, err := renderSnapshotAtRoot(root, filepath.Join(root, "catalog.json"), filepath.Join(root, "plan.json"), filepath.Join(root, "previous.json"), output, "", "")
				if err != nil {
					t.Fatal(err)
				}
				want := 1
				if behavior == "published" {
					want = 2
				}
				if len(snapshot.Plugins[0].History) != want || snapshot.Plugins[0].History[0] != previous.Plugins[0].History[0] {
					t.Fatal("publication did not preserve/derive history")
				}
				if behavior != "published" && !reflect.DeepEqual(snapshot.Plugins[0].History, previous.Plugins[0].History) {
					t.Fatal("unpublished current changed previous history")
				}
				if behavior == "404" || behavior == "excluded" {
					overrides := filepath.Join(root, "overrides.json")
					mustCanonical(t, overrides, map[string]string{"demo": "1.0.1"})
					if _, err := buildPlan(root, filepath.Join(root, "catalog.json"), filepath.Join(root, "previous.json"), "", plan.SourceCommit, "2.0.1", overrides); err != nil {
						t.Fatalf("unpublished/excluded current blocks bump recovery: %v", err)
					}
				}
			})
		}
	}
}

func TestManagedHistoryRejectsForgedPublished404InResolve(t *testing.T) {
	for _, hasHash := range []bool{false, true} {
		t.Run(map[bool]string{false: "missing-hash", true: "existing-hash"}[hasHash], func(t *testing.T) {
			root, catalog, plan, previous, evidence := historyFixture(t, hasHash)
			historyRegistry(t, previous, "404")
			captured, err := captureHistoryEvidence(root, filepath.Join(root, "previous.json"), plan, previous)
			if err != nil || captured.Entries["demo"].Publication != "absent" {
				t.Fatalf("genuine 404 capture: %v", err)
			}
			old := previous.Plugins[0]
			captured.Entries["demo"] = HistoryEvidenceEntry{Version: old.Version, Digest: old.Digest, WasmSHA256: strings.Repeat("e", 64), Publication: "published", ObservedDigest: old.Digest, ObservedVersion: old.Version, ObservedRevision: old.SourceCommit, ObservedInputHash: old.InputHash}
			evidence.HistoryEvidence = captured
			previousResolver := ociManifestResolver
			withManifestResolver(t, func(ref string) (ociManifest, error) {
				if ref == evidence.Plugins["demo"].CandidateRef {
					return ociManifest{Digest: evidence.Plugins["demo"].Digest, Annotations: map[string]string{"org.opencontainers.image.version": "1.0.1", "org.opencontainers.image.revision": plan.SourceCommit, "io.higress.plugin.input-hash": plan.Plugins[0].InputHash}}, nil
				}
				return previousResolver(ref)
			})
			evidencePath := releaseEvidencePath(root, Snapshot{GatewayVersion: plan.GatewayVersion})
			mustCanonical(t, evidencePath, evidence)
			snapshot, err := renderSnapshotAtRoot(root, filepath.Join(root, "catalog.json"), filepath.Join(root, "plan.json"), filepath.Join(root, "previous.json"), evidencePath, "", "")
			if err != nil {
				t.Fatalf("offline structural rendering must not guess registry: %v", err)
			}
			snapshotPath := filepath.Join(root, "snapshot.json")
			mustCanonical(t, snapshotPath, snapshot)
			if err := applyPlan(root, plan); err != nil {
				t.Fatal(err)
			}
			mustRun(t, root, "git", "add", ".")
			mustRun(t, root, "git", "commit", "-qm", "Q")
			committed, _ := resolveCommit(root, "HEAD")
			if err := verifySnapshotBindings(root, filepath.Join(root, "catalog.json"), snapshotPath, filepath.Join(root, "plan.json"), filepath.Join(root, "previous.json"), plan.SourceCommit, committed, false, "candidate"); err != nil {
				t.Fatalf("structural verification failed before live replay: %v", err)
			}
			if err := verifyManagedBindings(root, catalog, snapshot, filepath.Join(root, "plan.json"), filepath.Join(root, "previous.json"), committed, true); err == nil || !strings.Contains(err.Error(), "classification does not replay") {
				t.Fatalf("forged genuine 404 was not rejected by registry replay: %v", err)
			}
			if err := verifySnapshotBindings(root, filepath.Join(root, "catalog.json"), snapshotPath, filepath.Join(root, "plan.json"), filepath.Join(root, "previous.json"), plan.SourceCommit, committed, true, "candidate"); err == nil || !strings.Contains(err.Error(), "classification does not replay") {
				t.Fatalf("verify-snapshot --resolve accepted forged publication: %v", err)
			}
		})
	}
}

func TestManagedHistoryEvidenceRejectsIncompleteOrUnboundRecords(t *testing.T) {
	for _, mutation := range []string{"missing-control", "wrong-control", "missing", "extra", "version", "digest", "hash", "publication", "observed", "annotation"} {
		t.Run(mutation, func(t *testing.T) {
			root, _, plan, previous, _ := historyFixture(t, true)
			historyRegistry(t, previous, "published")
			evidence, err := captureHistoryEvidence(root, filepath.Join(root, "previous.json"), plan, previous)
			if err != nil {
				t.Fatal(err)
			}
			record := evidence.Entries["demo"]
			switch mutation {
			case "missing-control":
				evidence.ControlDigest = ""
			case "wrong-control":
				evidence.ControlRef = "registry.example/plugins/unknown:1.0.0"
			case "missing":
				delete(evidence.Entries, "demo")
			case "extra":
				evidence.Entries["unknown"] = record
			case "version":
				record.Version = "1.0.9"
			case "digest":
				record.Digest = "sha256:" + strings.Repeat("0", 64)
			case "hash":
				record.WasmSHA256 = "sha256:" + strings.Repeat("e", 64)
			case "publication":
				record.Publication = "maybe"
			case "observed":
				record.ObservedDigest = ""
			case "annotation":
				record.ObservedInputHash = ""
			}
			if mutation != "missing" {
				evidence.Entries["demo"] = record
			}
			if err := validateHistoryEvidence(plan, previous, evidence); err == nil {
				t.Fatal("malformed history evidence accepted")
			}
		})
	}
}

func TestManagedHistoryStrictSchemaAndCollisions(t *testing.T) {
	for _, data := range []string{`{"history":null}`, `{"history":{}}`, `{"history":[{"unknown":true}]}`, `{"wasmSha256":"sha256:aa"}`, `{"unknown":true}`} {
		var entry SnapshotEntry
		if err := strictDecode([]byte(data), &entry); err == nil {
			t.Fatalf("accepted malformed snapshot entry %s", data)
		}
	}
	_, catalog, _, previous, _ := historyFixture(t, true)
	for _, mutation := range []string{"latest", "duplicate", "current", "path", "digest", "hash", "public-provenance", "global-path", "no-consumer"} {
		t.Run(mutation, func(t *testing.T) {
			entry := previous.Plugins[0]
			entry.History = append([]HistoryArtifact(nil), entry.History...)
			snapshot := Snapshot{Plugins: []SnapshotEntry{entry}}
			switch mutation {
			case "latest":
				snapshot.Plugins[0].History[0].Version = "latest"
			case "duplicate":
				snapshot.Plugins[0].History = append(entry.History, entry.History[0])
			case "current":
				snapshot.Plugins[0].History[0].Version = entry.Version
			case "path":
				snapshot.Plugins[0].History[0].OCIRef = "registry.example/plugins/other:0.9.0"
			case "digest":
				snapshot.Plugins[0].History[0].Digest = "bad"
			case "hash":
				snapshot.Plugins[0].History[0].WasmSHA256 = "sha256:" + strings.Repeat("a", 64)
			case "public-provenance":
				snapshot.Plugins[0].History[0].SourceCommit = entry.SourceCommit
			case "global-path":
				duplicate := entry
				duplicate.LogicalID = "other"
				snapshot.Plugins = append(snapshot.Plugins, duplicate)
			case "no-consumer":
				snapshot.Plugins[0].Consumers.PluginServer = nil
			}
			if err := validateSnapshotHistory(catalog, snapshot); err == nil {
				t.Fatal("history collision/schema mutation accepted")
			}
		})
	}
}

func TestManagedHistoryReplansWhenOnlyPreviousHistoryIsPublic(t *testing.T) {
	for _, implementation := range []string{"go", "rust"} {
		t.Run(implementation, func(t *testing.T) {
			root, _, plan, previous, _ := historyFixtureForImplementation(t, false, implementation)
			previous.Plugins = previous.Plugins[:1]
			mustCanonical(t, filepath.Join(root, "previous.json"), previous)
			old := previous.Plugins[0]
			withManifestResolver(t, func(ref string) (ociManifest, error) {
				if ref == old.History[0].OCIRef {
					return ociManifest{Digest: old.History[0].Digest}, nil
				}
				return ociManifest{}, errors.New("HTTP 404 Not Found")
			})
			evidence, err := captureHistoryEvidence(root, filepath.Join(root, "previous.json"), plan, previous)
			if err != nil || evidence.ControlRef != old.History[0].OCIRef || evidence.Entries["demo"].Publication != "absent" {
				t.Fatalf("cannot calibrate against retained published history: %+v %v", evidence, err)
			}
			if err := replayHistoryEvidence(root, plan, previous, evidence); err != nil {
				t.Fatal(err)
			}
			if _, err := migrationPreflight(filepath.Join(root, "catalog.json"), filepath.Join(root, "plan.json"), filepath.Join(root, "previous.json"), filepath.Join(root, "built.json")); err != nil {
				t.Fatalf("history-only control blocks migration recovery: %v", err)
			}
			record := evidence.Entries["demo"]
			record.WasmSHA256 = strings.Repeat("0", 64)
			evidence.Entries["demo"] = record
			if err := replayHistoryEvidence(root, plan, previous, evidence); err == nil {
				t.Fatal("unbound optional Wasm hash accepted for unpublished previous current")
			}
		})
	}
}

func TestManagedHistoryCannotDowngradeToLegacyByDeletingAllNewFields(t *testing.T) {
	t.Setenv("GIT_AUTHOR_DATE", "2026-01-02T03:04:05+00:00")
	t.Setenv("GIT_COMMITTER_DATE", "2026-01-02T03:04:05+00:00")
	root, _, plan, previous, evidence := historyFixture(t, true)
	historyRegistry(t, previous, "published")
	var err error
	evidence.HistoryEvidence, err = captureHistoryEvidence(root, filepath.Join(root, "previous.json"), plan, previous)
	if err != nil {
		t.Fatal(err)
	}
	evidencePath := releaseEvidencePath(root, Snapshot{GatewayVersion: plan.GatewayVersion})
	mustCanonical(t, evidencePath, evidence)
	snapshot, err := renderSnapshotAtRoot(root, filepath.Join(root, "catalog.json"), filepath.Join(root, "plan.json"), filepath.Join(root, "previous.json"), evidencePath, "", "")
	if err != nil {
		t.Fatal(err)
	}
	snapshotPath := filepath.Join(root, "snapshot.json")
	mustCanonical(t, snapshotPath, snapshot)
	if err := applyPlan(root, plan); err != nil {
		t.Fatal(err)
	}
	mustRun(t, root, "git", "add", ".")
	mustRun(t, root, "git", "commit", "-qm", "preparation with managed history")
	committed, _ := resolveCommit(root, "HEAD")
	if err := verifySnapshotBindings(root, filepath.Join(root, "catalog.json"), snapshotPath, filepath.Join(root, "plan.json"), filepath.Join(root, "previous.json"), plan.SourceCommit, committed, false, "candidate"); err != nil {
		t.Fatal(err)
	}
	for index := range snapshot.Plugins {
		snapshot.Plugins[index].History = nil
		snapshot.Plugins[index].WasmSHA256 = ""
		snapshot.Plugins[index].CandidateImport = nil
	}
	snapshot.HistoryImport = nil
	mustCanonical(t, snapshotPath, snapshot)
	if err := verifySnapshotBindings(root, filepath.Join(root, "catalog.json"), snapshotPath, filepath.Join(root, "plan.json"), filepath.Join(root, "previous.json"), plan.SourceCommit, committed, false, "candidate"); err == nil {
		t.Fatal("deleting all new fields bypassed previous-history binding through legacy compatibility")
	}
}

func TestManagedBindingsCannotStripFirstAdoptionSignals(t *testing.T) {
	t.Setenv("GIT_AUTHOR_DATE", "2026-01-02T03:04:05+00:00")
	t.Setenv("GIT_COMMITTER_DATE", "2026-01-02T03:04:05+00:00")
	t.Run("structured-evidence", func(t *testing.T) {
		root, _, plan, previous, evidence := historyFixture(t, false)
		previous.Plugins[0].History = nil
		mustCanonical(t, filepath.Join(root, "previous.json"), previous)
		historyRegistry(t, previous, "published")
		var err error
		evidence.HistoryEvidence, err = captureHistoryEvidence(root, filepath.Join(root, "previous.json"), plan, previous)
		if err != nil {
			t.Fatal(err)
		}
		evidencePath := releaseEvidencePath(root, Snapshot{GatewayVersion: plan.GatewayVersion})
		mustCanonical(t, evidencePath, evidence)
		snapshot, err := renderSnapshotAtRoot(root, filepath.Join(root, "catalog.json"), filepath.Join(root, "plan.json"), filepath.Join(root, "previous.json"), evidencePath, "", "")
		if err != nil {
			t.Fatal(err)
		}
		snapshotPath := filepath.Join(root, "snapshot.json")
		mustCanonical(t, snapshotPath, snapshot)
		if err := applyPlan(root, plan); err != nil {
			t.Fatal(err)
		}
		mustRun(t, root, "git", "add", ".")
		mustRun(t, root, "git", "commit", "-qm", "first structured preparation")
		committed, _ := resolveCommit(root, "HEAD")
		if err := verifySnapshotBindings(root, filepath.Join(root, "catalog.json"), snapshotPath, filepath.Join(root, "plan.json"), filepath.Join(root, "previous.json"), plan.SourceCommit, committed, false, "candidate"); err != nil {
			t.Fatal(err)
		}
		for index := range snapshot.Plugins {
			snapshot.Plugins[index].History = nil
			snapshot.Plugins[index].WasmSHA256 = ""
		}
		mustCanonical(t, snapshotPath, snapshot)
		if err := verifySnapshotBindings(root, filepath.Join(root, "catalog.json"), snapshotPath, filepath.Join(root, "plan.json"), filepath.Join(root, "previous.json"), plan.SourceCommit, committed, false, "candidate"); err == nil {
			t.Fatal("stripped first structured evidence entered legacy compatibility")
		}
	})
	t.Run("active-cpp-catalog", func(t *testing.T) {
		root, catalogPath, previousPath, _, _, _, _ := importFixture(t)
		var catalog Catalog
		if _, err := readJSON(catalogPath, &catalog); err != nil {
			t.Fatal(err)
		}
		if err := verifyManagedBindings(root, catalog, Snapshot{GatewayVersion: "2.1.0"}, "", previousPath, "", false); err == nil {
			t.Fatal("active C++ catalog entered legacy compatibility without import bindings")
		}
	})
}
