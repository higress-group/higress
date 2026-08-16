// Copyright 2026 Higress Authors
// Licensed under the Apache License, Version 2.0.

package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBootstrapImportsResolvedPublicBaselineWithoutMutation(t *testing.T) {
	root, catalog, evidence, commit, digest := bootstrapFixture(t)
	output := filepath.Join(root, "plugins/release/snapshots/2.0.0.json")
	publicRef := "registry.example/plugins/demo:1.0.0"
	withManifestResolver(t, func(ref string) (ociManifest, error) {
		if ref != publicRef {
			t.Fatalf("resolved %q, want %q", ref, publicRef)
		}
		return ociManifest{Digest: digest}, nil
	})
	if err := commandBootstrap([]string{"--root", root, "--catalog", catalog, "--gateway-version", "2.0.0", "--source", commit, "--existing-evidence", evidence, "--output", output}); err != nil {
		t.Fatal(err)
	}
	var snapshot Snapshot
	data, err := readJSON(output, &snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.ProvenanceMode != "bootstrap-public" || len(snapshot.Plugins) != 1 || snapshot.Plugins[0].CandidateRef != "" || snapshot.Plugins[0].Digest != digest {
		t.Fatalf("unexpected bootstrap snapshot: %#v", snapshot)
	}
	want, err := json.MarshalIndent(snapshot, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	want = append(want, '\n')
	if !strings.HasSuffix(string(data), "\n") || string(data) != string(want) {
		t.Fatal("bootstrap output is not canonical")
	}
	version := mustRead(t, filepath.Join(root, "plugins/wasm-go/extensions/demo/VERSION"))
	if string(version) != "1.0.0\n" {
		t.Fatalf("bootstrap mutated VERSION: %q", version)
	}
	if _, err := os.Stat(filepath.Join(root, "plugins/wasm-go/extensions/demo/plugin.wasm")); !os.IsNotExist(err) {
		t.Fatal("bootstrap must not build an artifact")
	}
	// Bootstrap verification is the PR/readiness path: it re-resolves the
	// reviewed public tag/digest but deliberately does not require annotations
	// that predate the immutable-release protocol.
	if err := verifySnapshot(root, catalog, output, commit, commit, true, "public"); err != nil {
		t.Fatalf("bootstrap snapshot must pass public PR/readiness verification: %v", err)
	}
	if err := verifySnapshot(root, catalog, output, commit, commit, true, "candidate"); err != nil {
		t.Fatalf("bootstrap candidate validation must not resolve a candidate: %v", err)
	}
}

func TestCaptureBootstrapEvidenceIsCanonicalReadOnlyAndNotSelfReferential(t *testing.T) {
	root, catalog, _, commit, digest := bootstrapFixture(t)
	publicRef := "registry.example/plugins/demo:1.0.0"
	withManifestResolver(t, func(ref string) (ociManifest, error) {
		if ref != publicRef {
			t.Fatalf("resolved %q, want %q", ref, publicRef)
		}
		return ociManifest{Digest: digest}, nil
	})
	before := mustRead(t, filepath.Join(root, "plugins/wasm-go/extensions/demo/VERSION"))
	first, err := captureBootstrapEvidence(root, catalog, commit)
	if err != nil {
		t.Fatal(err)
	}
	second, err := captureBootstrapEvidence(root, catalog, commit)
	if err != nil {
		t.Fatal(err)
	}
	firstPath := filepath.Join(root, "first.json")
	secondPath := filepath.Join(root, "second.json")
	if err := writeCanonical(firstPath, first); err != nil {
		t.Fatal(err)
	}
	if err := writeCanonical(secondPath, second); err != nil {
		t.Fatal(err)
	}
	if string(mustRead(t, firstPath)) != string(mustRead(t, secondPath)) {
		t.Fatal("same exact source and public registry state must produce identical evidence bytes")
	}
	data := string(mustRead(t, firstPath))
	if strings.Contains(data, commit) || strings.Contains(data, "sourceCommit") || strings.Contains(data, "inputHash") {
		t.Fatalf("bootstrap evidence must not self-reference its target commit or derived input hash:\n%s", data)
	}
	entry := first.Plugins["demo"]
	if entry.Status != "public" || entry.Version != "1.0.0" || entry.PublicRef != publicRef || entry.Digest != digest {
		t.Fatalf("capture lost reviewed public provenance: %#v", entry)
	}
	if string(mustRead(t, filepath.Join(root, "plugins/wasm-go/extensions/demo/VERSION"))) != string(before) {
		t.Fatal("read-only evidence capture mutated the target tree")
	}
	if _, err := captureBootstrapEvidence(root, catalog, "HEAD"); err == nil || !strings.Contains(err.Error(), "40-character") {
		t.Fatalf("capture accepted mutable source ref: %v", err)
	}
}

func TestBootstrapRejectsMissingEvidenceAndPublicDigestMismatch(t *testing.T) {
	root, catalog, evidence, commit, digest := bootstrapFixture(t)
	var file BootstrapEvidenceFile
	if _, err := readJSON(evidence, &file); err != nil {
		t.Fatal(err)
	}
	file.Plugins = map[string]BootstrapEvidence{}
	if err := writeCanonical(evidence, file); err != nil {
		t.Fatal(err)
	}
	withManifestResolver(t, func(string) (ociManifest, error) { return ociManifest{Digest: digest}, nil })
	if err := commandBootstrap([]string{"--root", root, "--catalog", catalog, "--gateway-version", "2.0.0", "--source", commit, "--existing-evidence", evidence, "--output", filepath.Join(root, "out.json")}); err == nil {
		t.Fatal("missing bootstrap evidence must fail")
	}

	// Restore complete reviewed evidence, then prove that a mutable public tag
	// cannot be imported when it no longer resolves to that reviewed digest.
	root, catalog, evidence, commit, digest = bootstrapFixture(t)
	withManifestResolver(t, func(string) (ociManifest, error) {
		return ociManifest{Digest: "sha256:" + strings.Repeat("b", 64)}, nil
	})
	if err := commandBootstrap([]string{"--root", root, "--catalog", catalog, "--gateway-version", "2.0.0", "--source", commit, "--existing-evidence", evidence, "--output", filepath.Join(root, "out.json")}); err == nil || !strings.Contains(err.Error(), "resolved digest") {
		t.Fatalf("public digest mismatch must fail closed, got %v", err)
	}
	_ = digest
}

func TestBootstrapFailsWhenPublicResolutionFails(t *testing.T) {
	root, catalog, evidence, commit, _ := bootstrapFixture(t)
	withManifestResolver(t, func(string) (ociManifest, error) { return ociManifest{}, os.ErrNotExist })
	output := filepath.Join(root, "out.json")
	if err := commandBootstrap([]string{"--root", root, "--catalog", catalog, "--gateway-version", "2.0.0", "--source", commit, "--existing-evidence", evidence, "--output", output}); err == nil {
		t.Fatal("unresolvable public artifact must fail")
	}
	if _, err := os.Stat(output); !os.IsNotExist(err) {
		t.Fatal("failed bootstrap must not write a snapshot")
	}
}

func TestPlanRequiresBootstrapSnapshot(t *testing.T) {
	root, catalog, _, commit, _ := bootstrapFixture(t)
	if _, err := buildPlan(root, catalog, "", "", commit, "2.0.0", ""); err == nil || !strings.Contains(err.Error(), "bootstrap-snapshot") {
		t.Fatalf("unbootstrapped planning must fail closed, got %v", err)
	}
}

func TestBootstrapComparisonBaseIsOneTimeExactAndAnAncestor(t *testing.T) {
	root, catalog, _, _, digest := bootstrapFixture(t)
	base := addHistoricalCommitWithoutVersion(t, root)
	target, _ := resolveCommit(root, "HEAD")
	previous := bootstrapPreviousSnapshot(t, root, catalog, target, digest)

	plan, err := buildPlan(root, catalog, previous, base, target, "2.0.1", "")
	if err != nil {
		t.Fatal(err)
	}
	if plan.BaseCommit != base || len(plan.Plugins) != 1 || plan.Plugins[0].Version != "1.0.1" {
		t.Fatalf("unexpected bootstrap comparison plan: %#v", plan)
	}
	if _, err := buildPlan(root, catalog, previous, "HEAD~1", target, "2.0.1", ""); err == nil || !strings.Contains(err.Error(), "40-character") {
		t.Fatalf("abbreviated bootstrap base must fail, got %v", err)
	}
	if _, err := buildPlan(root, catalog, previous, "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", target, "2.0.1", ""); err == nil {
		t.Fatal("unknown/non-ancestor bootstrap base must fail")
	}
	if _, err := buildPlan(root, catalog, previous, "", target, "2.0.1", ""); err == nil || !strings.Contains(err.Error(), "requires one exact --base") {
		t.Fatalf("bootstrap planning without explicit comparison base must fail, got %v", err)
	}

	var ordinary Snapshot
	if _, err := readJSON(previous, &ordinary); err != nil {
		t.Fatal(err)
	}
	ordinary.ProvenanceMode = "candidate"
	ordinaryPath := filepath.Join(root, "ordinary.json")
	if err := writeCanonical(ordinaryPath, ordinary); err != nil {
		t.Fatal(err)
	}
	if _, err := buildPlan(root, catalog, ordinaryPath, base, target, "2.0.1", ""); err == nil || !strings.Contains(err.Error(), "only with a bootstrap-public") {
		t.Fatalf("ordinary snapshot accepted one-time comparison override: %v", err)
	}
}

func TestCatalogRejectsReleaseEligibleVersionThatIsNotTracked(t *testing.T) {
	root, catalog, _, _, _ := bootstrapFixture(t)
	if err := validateCatalog(root, catalog); err != nil {
		t.Fatalf("tracked VERSION should validate: %v", err)
	}
	mustRun(t, root, "git", "rm", "--cached", "plugins/wasm-go/extensions/demo/VERSION")
	if err := validateCatalog(root, catalog); err == nil || !strings.Contains(err.Error(), "must be tracked by Git") {
		t.Fatalf("untracked release VERSION must fail closed, got %v", err)
	}
}

func bootstrapFixture(t *testing.T) (root, catalog, evidence, commit, digest string) {
	t.Helper()
	root = t.TempDir()
	mustRun(t, root, "git", "init", "-q")
	mustRun(t, root, "git", "config", "user.name", "test")
	mustRun(t, root, "git", "config", "user.email", "test@example.com")
	mustWrite(t, filepath.Join(root, "plugins/wasm-go/extensions/demo/main.go"), "package main\n")
	mustWrite(t, filepath.Join(root, "plugins/wasm-go/extensions/demo/VERSION"), "1.0.0\n")
	mustWrite(t, filepath.Join(root, "plugins/wasm-rust/extensions/.keep"), "")
	mustRun(t, root, "git", "add", ".")
	mustRun(t, root, "git", "commit", "-q", "-m", "source")
	commit, _ = resolveCommit(root, "HEAD")
	c := Catalog{SchemaVersion: 1, Registry: "registry.example", Plugins: []Plugin{{LogicalID: "demo", Implementation: "go", SourceDir: "plugins/wasm-go/extensions/demo", Image: "plugins/demo", ReleaseEligible: true, ArtifactInputs: []string{"plugins/wasm-go/extensions/demo/**"}}}}
	catalog = filepath.Join(root, "catalog.json")
	if err := writeCanonical(catalog, c); err != nil {
		t.Fatal(err)
	}
	digest = "sha256:" + strings.Repeat("a", 64)
	evidence = filepath.Join(root, "evidence.json")
	if err := writeCanonical(evidence, BootstrapEvidenceFile{Plugins: map[string]BootstrapEvidence{"demo": {Status: "public", Version: "1.0.0", PublicRef: "registry.example/plugins/demo:1.0.0", Digest: digest}}}); err != nil {
		t.Fatal(err)
	}
	return root, catalog, evidence, commit, digest
}

func addHistoricalCommitWithoutVersion(t *testing.T, root string) string {
	t.Helper()
	target, _ := resolveCommit(root, "HEAD")
	mustRun(t, root, "git", "checkout", "--orphan", "historical")
	mustRun(t, root, "git", "rm", "-rf", ".")
	mustWrite(t, filepath.Join(root, "plugins/wasm-go/extensions/demo/main.go"), "package main\n\n// historical\n")
	mustWrite(t, filepath.Join(root, "plugins/wasm-rust/extensions/.keep"), "")
	mustRun(t, root, "git", "add", ".")
	mustRun(t, root, "git", "commit", "-q", "-m", "historical without VERSION")
	base, _ := resolveCommit(root, "HEAD")
	// Rebuild an ancestry chain historical -> target tree. The orphan source
	// commit itself remains available only as a tree donor.
	mustRun(t, root, "git", "read-tree", target)
	mustRun(t, root, "git", "checkout-index", "-a", "-f")
	mustRun(t, root, "git", "commit", "-q", "-m", "target tree")
	return base
}

func bootstrapPreviousSnapshot(t *testing.T, root, catalogPath, target, digest string) string {
	t.Helper()
	c, catalogData, err := loadCatalog(catalogPath)
	if err != nil {
		t.Fatal(err)
	}
	p := c.Plugins[0]
	hash, err := inputHash(root, target, "1.0.0", c, p)
	if err != nil {
		t.Fatal(err)
	}
	snapshot := Snapshot{SchemaVersion: 1, GatewayVersion: "2.0.0", SourceCommit: target, CatalogSHA256: sha256Hex(catalogData), ProvenanceMode: "bootstrap-public", Plugins: []SnapshotEntry{{LogicalID: p.LogicalID, Implementation: p.Implementation, SourceDir: p.SourceDir, Image: p.Image, Version: "1.0.0", OCIRef: "registry.example/plugins/demo:1.0.0", Digest: digest, InputHash: hash, SourceCommit: target, ProvenanceMode: "public"}}}
	path := filepath.Join(root, "bootstrap.json")
	if err := writeCanonical(path, snapshot); err != nil {
		t.Fatal(err)
	}
	return path
}

func withManifestResolver(t *testing.T, resolver func(string) (ociManifest, error)) {
	t.Helper()
	previous := ociManifestResolver
	ociManifestResolver = resolver
	t.Cleanup(func() { ociManifestResolver = previous })
}
