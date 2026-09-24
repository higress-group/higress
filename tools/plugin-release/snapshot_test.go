// Copyright 2026 Higress Authors
// Licensed under the Apache License, Version 2.0.

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRenderSnapshotRejectsCandidateProvenanceMismatch(t *testing.T) {
	root := t.TempDir()
	catalog := filepath.Join(root, "catalog.json")
	plan := filepath.Join(root, "plan.json")
	evidence := filepath.Join(root, "evidence.json")
	commit := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	digest := "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	c := Catalog{SchemaVersion: 1, Registry: "registry.example", Plugins: []Plugin{{
		LogicalID: "demo", Implementation: "go", SourceDir: "plugins/wasm-go/extensions/demo", Image: "plugins/demo", ReleaseEligible: true,
		ArtifactInputs: []string{"plugins/wasm-go/extensions/demo/**"},
	}}}
	if err := writeCanonical(catalog, c); err != nil {
		t.Fatal(err)
	}
	planValue := Plan{SchemaVersion: 1, GatewayVersion: "2.0.0", SourceCommit: commit, CatalogSHA256: sha256Hex(mustRead(t, catalog)), PlanID: digest,
		Plugins: []PlanEntry{{LogicalID: "demo", Implementation: "go", SourceDir: "plugins/wasm-go/extensions/demo", Image: "plugins/demo", Version: "1.0.0", InputHash: digest}}}
	if err := writeCanonical(plan, planValue); err != nil {
		t.Fatal(err)
	}
	e := CandidateEvidenceFile{Plugins: map[string]CandidateEvidence{"demo": {CandidateRef: "registry.example/candidate/demo@" + digest, Digest: digest, SourceCommit: commit, InputHash: "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"}}}
	if err := writeCanonical(evidence, e); err != nil {
		t.Fatal(err)
	}
	if _, err := renderSnapshot(catalog, plan, "", evidence, ""); err == nil {
		t.Fatal("expected mismatched candidate input hash to fail")
	}
}

func TestVerifySnapshotUsesPreMergeInputsAndMergedVersions(t *testing.T) {
	root := t.TempDir()
	mustRun(t, root, "git", "init", "-q")
	mustRun(t, root, "git", "config", "user.name", "test")
	mustRun(t, root, "git", "config", "user.email", "test@example.com")
	mustWrite(t, filepath.Join(root, "plugins/wasm-go/extensions/demo/main.go"), "package main\n")
	// A non-alpha prerelease keeps this plugin in release selection; an alpha
	// VERSION would now be deferred and skip the committed-source checks below.
	mustWrite(t, filepath.Join(root, "plugins/wasm-go/extensions/demo/VERSION"), "1.0.0-beta.1\n")
	mustWrite(t, filepath.Join(root, "plugins/wasm-rust/extensions/.keep"), "")
	mustRun(t, root, "git", "add", ".")
	mustRun(t, root, "git", "commit", "-q", "-m", "source")
	preMerge, _ := resolveCommit(root, "HEAD")
	c := Catalog{SchemaVersion: 1, Registry: "registry.example", Plugins: []Plugin{{LogicalID: "demo", Implementation: "go", SourceDir: "plugins/wasm-go/extensions/demo", Image: "plugins/demo", ReleaseEligible: true, ArtifactInputs: []string{"plugins/wasm-go/extensions/demo/**"}}}}
	catalog := filepath.Join(root, "catalog.json")
	if err := writeCanonical(catalog, c); err != nil {
		t.Fatal(err)
	}
	hash, err := inputHash(root, preMerge, "1.0.0", c, c.Plugins[0])
	if err != nil {
		t.Fatal(err)
	}
	snapshot := Snapshot{SchemaVersion: 1, GatewayVersion: "2.0.0", SourceCommit: preMerge, CatalogSHA256: sha256Hex(mustRead(t, catalog)), ProvenanceMode: "candidate", Plugins: []SnapshotEntry{{LogicalID: "demo", Implementation: "go", SourceDir: c.Plugins[0].SourceDir, Image: c.Plugins[0].Image, Version: "1.0.0", OCIRef: "registry.example/plugins/demo:1.0.0", Digest: "sha256:" + strings.Repeat("a", 64), InputHash: hash, SourceCommit: preMerge, CandidateRef: "registry.example/candidate/demo@sha256:" + strings.Repeat("a", 64)}}}
	snapshotPath := filepath.Join(root, "snapshot.json")
	if err := writeCanonical(snapshotPath, snapshot); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Join(root, "plugins/wasm-go/extensions/demo/VERSION"), "1.0.0\n")
	mustRun(t, root, "git", "add", ".")
	mustRun(t, root, "git", "commit", "-q", "-m", "merge")
	merged, _ := resolveCommit(root, "HEAD")
	recomputed, recomputeErr := inputHash(root, preMerge, "1.0.0", c, c.Plugins[0])
	if recomputeErr != nil || recomputed != hash {
		t.Fatalf("premerge hash %s recomputed %s: %v", hash, recomputed, recomputeErr)
	}
	if err := verifySnapshot(root, catalog, snapshotPath, preMerge, merged, false, "candidate"); err != nil {
		t.Fatal(err)
	}
	withManifestResolver(t, func(ref string) (ociManifest, error) {
		if ref != snapshot.Plugins[0].CandidateRef {
			t.Fatalf("candidate verification resolved %q", ref)
		}
		return ociManifest{Digest: snapshot.Plugins[0].Digest, Annotations: map[string]string{"org.opencontainers.image.revision": preMerge, "io.higress.plugin.input-hash": hash, "org.opencontainers.image.version": "1.0.0"}}, nil
	})
	if err := verifySnapshot(root, catalog, snapshotPath, preMerge, merged, true, "candidate"); err != nil {
		t.Fatal(err)
	}
	withManifestResolver(t, func(ref string) (ociManifest, error) {
		if ref != snapshot.Plugins[0].OCIRef {
			t.Fatalf("public verification resolved %q", ref)
		}
		return ociManifest{Digest: snapshot.Plugins[0].Digest, Annotations: map[string]string{"org.opencontainers.image.revision": preMerge, "io.higress.plugin.input-hash": hash, "org.opencontainers.image.version": "1.0.0"}}, nil
	})
	if err := verifySnapshot(root, catalog, snapshotPath, preMerge, merged, true, "public"); err != nil {
		t.Fatal(err)
	}
}

func mustRead(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}
