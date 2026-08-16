// Copyright 2026 Higress Authors
// Licensed under the Apache License, Version 2.0.

package main

import (
	"encoding/json"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

const v223Commit = "39ec41aab6eb1d40499bed2847085696de0ebb96"

func TestFirstManagedReleasePlansExactlyFromV223DespiteMissingVersions(t *testing.T) {
	root := filepath.Clean("../..")
	target, err := resolveCommit(root, "HEAD")
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
	if missingHistoricalVersions != 16 || len(previous.Plugins) != 43 || deferredAlpha != 1 {
		t.Fatalf("v2.2.3 fixture drift: eligible=%d missing VERSION=%d deferred=%d, want 43/16/1", len(previous.Plugins), missingHistoricalVersions, deferredAlpha)
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
		t.Fatalf("first plan must defer exactly the alpha prerelease: %#v", first.Deferred)
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
