package main

import (
	"path/filepath"
	"strings"
	"testing"
)

// TestVersionRestatementCarriesForward proves the re-preparation semantics: a
// managed previous snapshot whose sourceCommit predates its own preparation
// PR's VERSION edits must not re-bump plugins whose only change is the VERSION
// file restating the previously recorded version.
func TestVersionRestatementCarriesForward(t *testing.T) {
	root, catalog, base := backfillRepo(t, map[string]string{"demo": "1.0.0"})
	c, _, err := loadCatalog(catalog)
	if err != nil {
		t.Fatal(err)
	}
	p := c.Plugins[0]
	digest := "sha256:" + strings.Repeat("a", 64)
	stableHash, err := inputHash(root, base, "1.0.0", c, p)
	if err != nil {
		t.Fatal(err)
	}
	previous := Snapshot{SchemaVersion: 1, GatewayVersion: "2.0.0", SourceCommit: base, CatalogSHA256: sha256Hex(mustRead(t, catalog)), PlanID: digest, ProvenanceMode: "candidate",
		Plugins: []SnapshotEntry{{LogicalID: p.LogicalID, Implementation: p.Implementation, SourceDir: p.SourceDir, Image: p.Image, Version: "1.0.0",
			OCIRef: "registry.example/plugins/demo:1.0.0", Digest: digest, InputHash: stableHash, SourceCommit: base,
			CandidateRef: "registry.example/candidates/demo@" + digest, ProvenanceMode: "candidate"}}}
	previousPath := filepath.Join(root, "previous.json")
	if err := writeCanonical(previousPath, previous); err != nil {
		t.Fatal(err)
	}

	// The preparation PR's own bookkeeping edit: VERSION changes 1.0.0 ->
	// 1.0.1 while the previous snapshot already records 1.0.1.
	mustWrite(t, filepath.Join(root, p.SourceDir, "VERSION"), "1.0.1\n")
	mustRun(t, root, "git", "add", ".")
	mustRun(t, root, "git", "commit", "-q", "-m", "bookkeeping")
	target, _ := resolveCommit(root, "HEAD")
	prev := previous
	prev.Plugins[0].Version = "1.0.1"
	prev.Plugins[0].OCIRef = "registry.example/plugins/demo:1.0.1"
	bumpHash, err := inputHash(root, target, "1.0.1", c, p)
	if err != nil {
		t.Fatal(err)
	}
	prev.Plugins[0].InputHash = bumpHash
	if err := writeCanonical(previousPath, prev); err != nil {
		t.Fatal(err)
	}

	plan, err := buildPlan(root, catalog, previousPath, "", target, "2.0.1", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Plugins) != 0 {
		t.Fatalf("VERSION-only restatement must carry forward, got %#v", plan.Plugins)
	}

	// A hand-bumped VERSION differing from the recorded version stays a
	// release signal.
	mustWrite(t, filepath.Join(root, p.SourceDir, "VERSION"), "2.0.0\n")
	mustRun(t, root, "git", "add", ".")
	mustRun(t, root, "git", "commit", "-q", "-m", "manual bump")
	target2, _ := resolveCommit(root, "HEAD")
	plan, err = buildPlan(root, catalog, previousPath, "", target2, "2.0.1", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Plugins) != 1 || plan.Plugins[0].Version != "2.0.0" {
		t.Fatalf("hand-bumped VERSION must plan its exact version, got %#v", plan.Plugins)
	}

	// A code change alongside the bookkeeping VERSION edit still plans.
	mustWrite(t, filepath.Join(root, p.SourceDir, "VERSION"), "1.0.1\n")
	mustWrite(t, filepath.Join(root, p.SourceDir, "main.go"), "package main // changed\n")
	mustRun(t, root, "git", "add", ".")
	mustRun(t, root, "git", "commit", "-q", "-m", "code change")
	target3, _ := resolveCommit(root, "HEAD")
	plan, err = buildPlan(root, catalog, previousPath, "", target3, "2.0.1", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Plugins) != 1 || plan.Plugins[0].Version != "1.0.2" {
		t.Fatalf("code change must plan a patch bump, got %#v", plan.Plugins)
	}
}
