// Copyright 2026 Higress Authors
// Licensed under the Apache License, Version 2.0.

package main

import (
	"encoding/json"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
)

const v223Commit = "39ec41aab6eb1d40499bed2847085696de0ebb96"

// The historical 44/17/1 fixture was established by this catalog revision.
// Freeze its checkout as well as its target: catalog validation reads local
// directories and VERSION files, which legitimately change on current HEAD.
const firstManagedReleaseFixtureCommit = "d3c0721dc8f67f5f126d6d374abca676622e330a"

func TestFirstManagedReleasePlansExactlyFromV223DespiteMissingVersions(t *testing.T) {
	repository, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	// A local shared clone reuses Git objects without registering a worktree
	// or changing the caller's checkout; TempDir owns all temporary metadata.
	mustRun(t, repository, "git", "clone", "--quiet", "--shared", "--no-checkout", repository, root)
	mustRun(t, root, "git", "checkout", "--quiet", "--detach", firstManagedReleaseFixtureCommit)
	target, err := resolveCommit(root, firstManagedReleaseFixtureCommit)
	if err != nil {
		t.Fatal(err)
	}
	if err := requireAncestor(root, v223Commit, target); err != nil {
		t.Fatal(err)
	}
	catalogPath := filepath.Join(root, "plugins/release/catalog.json")
	c, catalogData, err := loadCatalog(catalogPath)
	if err != nil {
		t.Fatal(err)
	}
	plugins := append([]Plugin(nil), c.Plugins...)
	sort.Slice(plugins, func(i, j int) bool { return plugins[i].LogicalID < plugins[j].LogicalID })
	dummyDigest := "sha256:" + strings.Repeat("a", 64)
	previous := Snapshot{SchemaVersion: 1, GatewayVersion: "2.2.3", SourceCommit: target, CatalogSHA256: sha256Hex(catalogData), ProvenanceMode: "bootstrap-public"}
	missingHistoricalVersions := 0
	deferredAlpha := 0
	eligible := map[string]Plugin{}
	for _, p := range plugins {
		if !p.ReleaseEligible {
			continue
		}
		eligible[p.LogicalID] = p
		versionRaw, err := fileAtCommit(root, target, p.SourceDir+"/VERSION")
		if err != nil {
			t.Fatal(err)
		}
		version := strings.TrimSpace(versionRaw)
		parsed, err := parseSemver(version)
		if err != nil {
			t.Fatal(err)
		}
		// A deferred alpha VERSION has no public artifact and no bootstrap
		// snapshot entry; the plan must defer it instead of importing it.
		if isAlphaPrerelease(parsed.prerelease) {
			deferredAlpha++
			continue
		}
		if _, err := fileAtCommit(root, v223Commit, p.SourceDir+"/VERSION"); err != nil {
			missingHistoricalVersions++
		}
		previous.Plugins = append(previous.Plugins, SnapshotEntry{LogicalID: p.LogicalID, Implementation: p.Implementation, SourceDir: p.SourceDir, Image: p.Image, Version: version, OCIRef: c.Registry + "/" + p.Image + ":" + version, Digest: dummyDigest, InputHash: dummyDigest, SourceCommit: target, ProvenanceMode: "public", Consumers: cloneConsumers(p.Consumers)})
	}
	if missingHistoricalVersions != 17 || len(previous.Plugins) != 44 || deferredAlpha != 1 {
		t.Fatalf("v2.2.3 fixture drift: eligible=%d missing VERSION=%d deferred=%d, want 44/17/1", len(previous.Plugins), missingHistoricalVersions, deferredAlpha)
	}
	previousPath := filepath.Join(t.TempDir(), "bootstrap-v2.2.3.json")
	if err := writeCanonical(previousPath, previous); err != nil {
		t.Fatal(err)
	}

	first, err := buildPlan(root, catalogPath, previousPath, v223Commit, target, "2.2.4", "")
	if err != nil {
		t.Fatalf("plan from v2.2.3 with missing historical VERSION files: %v", err)
	}
	second, err := buildPlan(root, catalogPath, previousPath, v223Commit, target, "2.2.4", "")
	if err != nil {
		t.Fatal(err)
	}
	firstBytes, _ := json.Marshal(first)
	secondBytes, _ := json.Marshal(second)
	if string(firstBytes) != string(secondBytes) {
		t.Fatal("same exact bootstrap comparison produced non-deterministic plans")
	}
	if first.BaseCommit != v223Commit || first.SourceCommit != target {
		t.Fatalf("plan lost exact comparison provenance: base=%s target=%s", first.BaseCommit, first.SourceCommit)
	}

	paths, err := changedPaths(root, v223Commit, target)
	if err != nil {
		t.Fatal(err)
	}
	expected := map[string]bool{}
	for id, p := range eligible {
		previousVersionRaw, err := fileAtCommit(root, target, p.SourceDir+"/VERSION")
		if err != nil {
			t.Fatal(err)
		}
		previousVersion, err := parseSemver(strings.TrimSpace(previousVersionRaw))
		if err != nil {
			t.Fatal(err)
		}
		if isAlphaPrerelease(previousVersion.prerelease) {
			continue
		}
		if previousVersion.prerelease != "" {
			expected[id] = true
			continue
		}
		for _, path := range paths {
			if pluginInputMatches(c, p, path) || path == p.SourceDir+"/VERSION" {
				expected[id] = true
				break
			}
		}
	}
	if len(first.Deferred) != 1 || first.Deferred[0].LogicalID != "replay-protection" ||
		first.Deferred[0].Version != "1.0.0-alpha" || first.Deferred[0].Reason != "alpha-prerelease" {
		t.Fatalf("first plan must defer exactly the alpha prereleases: %#v", first.Deferred)
	}
	if len(first.Plugins) != len(expected) {
		t.Fatalf("first plan has %d entries, exact artifact diff affects %d", len(first.Plugins), len(expected))
	}
	for _, entry := range first.Plugins {
		p, ok := eligible[entry.LogicalID]
		if !ok || !expected[entry.LogicalID] {
			t.Fatalf("plan included excluded or unaffected plugin %q", entry.LogicalID)
		}
		previousVersion, _ := parseSemver(entry.PreviousVersion)
		if entry.Implementation != p.Implementation || entry.SourceDir != p.SourceDir || entry.Image != p.Image || !digestPattern.MatchString(entry.InputHash) || (len(entry.ChangedPaths) == 0 && previousVersion.prerelease == "") {
			t.Fatalf("affected plugin lacks deterministic build evidence: %#v", entry)
		}
		if entry.Backfill {
			t.Fatalf("plugin %q present in the bootstrap baseline must not be a backfill", entry.LogicalID)
		}
	}
}

func TestFirstManagedReleaseDefersNewAlphaUntilStable(t *testing.T) {
	root, catalogPath, base := backfillRepo(t, map[string]string{
		"mcp-server": "2.0.2", "replay-protection": "1.0.0-alpha", "stable": "1.0.0",
	})
	c, catalogData, err := loadCatalog(catalogPath)
	if err != nil {
		t.Fatal(err)
	}
	digest := "sha256:" + strings.Repeat("a", 64)
	publicVersions := map[string]string{"mcp-server": "2.0.2", "stable": "1.0.0"}
	previous := Snapshot{SchemaVersion: 1, GatewayVersion: "2.2.3", SourceCommit: base,
		CatalogSHA256: sha256Hex(catalogData), ProvenanceMode: "bootstrap-public"}
	for _, p := range c.Plugins {
		version := publicVersions[p.LogicalID]
		if version == "" {
			continue
		}
		previous.Plugins = append(previous.Plugins, SnapshotEntry{LogicalID: p.LogicalID, Implementation: p.Implementation,
			SourceDir: p.SourceDir, Image: p.Image, Version: version, OCIRef: c.Registry + "/" + p.Image + ":" + version,
			Digest: digest, InputHash: digest, SourceCommit: base, ProvenanceMode: "public"})
	}
	previousPath := filepath.Join(t.TempDir(), "bootstrap.json")
	if err := writeCanonical(previousPath, previous); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Join(root, "plugins/wasm-go/extensions/stable/main.go"), "package main\n// changed stable artifact\n")

	for _, tc := range []struct {
		version  string
		planned  map[string]string
		deferred []DeferredPlugin
	}{
		{
			version: "2.0.3-alpha",
			planned: map[string]string{"stable": "1.0.1"},
			deferred: []DeferredPlugin{
				{LogicalID: "mcp-server", Version: "2.0.3-alpha", Reason: "alpha-prerelease"},
				{LogicalID: "replay-protection", Version: "1.0.0-alpha", Reason: "alpha-prerelease"},
			},
		},
		{
			version:  "2.0.3",
			planned:  map[string]string{"mcp-server": "2.0.3", "stable": "1.0.1"},
			deferred: []DeferredPlugin{{LogicalID: "replay-protection", Version: "1.0.0-alpha", Reason: "alpha-prerelease"}},
		},
	} {
		t.Run(tc.version, func(t *testing.T) {
			mustWrite(t, filepath.Join(root, "plugins/wasm-go/extensions/mcp-server/VERSION"), tc.version+"\n")
			mustRun(t, root, "git", "add", ".")
			mustRun(t, root, "git", "commit", "-q", "-m", "candidate "+tc.version)
			target, err := resolveCommit(root, "HEAD")
			if err != nil {
				t.Fatal(err)
			}
			plan, err := buildPlan(root, catalogPath, previousPath, base, target, "2.2.4", "")
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(plan.Deferred, tc.deferred) {
				t.Fatalf("deferred plugins = %#v, want %#v", plan.Deferred, tc.deferred)
			}
			planned := map[string]string{}
			for _, entry := range plan.Plugins {
				if _, duplicate := planned[entry.LogicalID]; duplicate {
					t.Fatalf("duplicate planned plugin %q", entry.LogicalID)
				}
				planned[entry.LogicalID] = entry.Version
				if entry.Backfill || entry.PreviousVersion != publicVersions[entry.LogicalID] || len(entry.ChangedPaths) == 0 || !digestPattern.MatchString(entry.InputHash) {
					t.Fatalf("existing public plugin lost its artifact/provenance evidence: %#v", entry)
				}
			}
			if !reflect.DeepEqual(planned, tc.planned) {
				t.Fatalf("planned versions = %#v, want %#v", planned, tc.planned)
			}
		})
	}
}
