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

// TestMigrationExclusionSurvivesAnUnrelatedLaterRelease proves a blocked plugin
// stays excluded, verifiable, and re-derivable in a later release that does not
// plan it. Release 2.0.1 blocks two of three plugins at commit A; release 2.0.2
// plans only the third plugin at commit B. The carried exclusions must survive
// verbatim, the top-level preflight marker must be rebound to the new sweep,
// verification must still pass at the new commit, and promote must still skip
// both blocked plugins while promoting the third.
func TestMigrationExclusionSurvivesAnUnrelatedLaterRelease(t *testing.T) {
	root, catalogPath, commitA := backfillRepo(t, map[string]string{"demo": "1.0.1", "other": "2.0.0", "third": "1.5.0"})
	c, _, err := loadCatalog(catalogPath)
	if err != nil {
		t.Fatal(err)
	}
	byID := map[string]Plugin{}
	for _, p := range c.Plugins {
		byID[p.LogicalID] = p
	}
	catalogSHA := sha256Hex(mustRead(t, catalogPath))
	digest := func(fill string) string { return "sha256:" + strings.Repeat(fill, 64) }
	hash := func(commit, version, id string) string {
		t.Helper()
		out, err := inputHash(root, commit, version, c, byID[id])
		if err != nil {
			t.Fatal(err)
		}
		return out
	}
	demoHash := hash(commitA, "1.0.1", "demo")
	otherHash := hash(commitA, "2.0.0", "other")
	thirdHashA := hash(commitA, "1.5.0", "third")

	// Release 2.0.2 touches only third, so the two excluded plugins keep the
	// exact inputs their carried entries record.
	mustWrite(t, filepath.Join(root, byID["third"].SourceDir, "VERSION"), "1.5.1\n")
	mustRun(t, root, "git", "add", ".")
	mustRun(t, root, "git", "commit", "-q", "-m", "release third 1.5.1")
	commitB, err := resolveCommit(root, "HEAD")
	if err != nil {
		t.Fatal(err)
	}
	if demoHash != hash(commitB, "1.0.1", "demo") || otherHash != hash(commitB, "2.0.0", "other") {
		t.Fatal("fixture changed an excluded plugin's inputs between the two releases")
	}
	thirdHashB := hash(commitB, "1.5.1", "third")
	if thirdHashA == thirdHashB {
		t.Fatal("fixture did not change the planned plugin's inputs between the two releases")
	}

	baseDigest := digest("0")
	demoDigest := digest("5")
	otherDigest := digest("6")
	thirdDigestA := digest("7")
	thirdDigestB := digest("8")
	demoLegacy := digest("d")
	otherLegacy := digest("e")
	publicRef := func(id, version string) string { return c.Registry + "/" + byID[id].Image + ":" + version }
	candidateRef := func(id, dgst string) string { return c.Registry + "/candidates/" + id + "@" + dgst }
	annotated := func(dgst, commit, inputHashValue, version string) ociManifest {
		return ociManifest{Digest: dgst, Annotations: map[string]string{
			"org.opencontainers.image.revision": commit,
			"io.higress.plugin.input-hash":      inputHashValue,
			"org.opencontainers.image.version":  version,
		}}
	}
	entry := func(id, version, inputHashValue, dgst, commit string) SnapshotEntry {
		p := byID[id]
		return SnapshotEntry{LogicalID: id, Implementation: p.Implementation, SourceDir: p.SourceDir, Image: p.Image,
			Version: version, OCIRef: publicRef(id, version), Digest: dgst, InputHash: inputHashValue, SourceCommit: commit,
			CandidateRef: candidateRef(id, dgst), ProvenanceMode: "candidate", Consumers: catalogConsumers(c, p)}
	}
	planEntry := func(id, previousVersion, version, inputHashValue string) PlanEntry {
		p := byID[id]
		return PlanEntry{LogicalID: id, Implementation: p.Implementation, SourceDir: p.SourceDir, Image: p.Image,
			PreviousVersion: previousVersion, Version: version, InputHash: inputHashValue, ChangedPaths: []string{}}
	}
	write := func(name string, value any) string {
		path := filepath.Join(root, name)
		if err := writeCanonical(path, value); err != nil {
			t.Fatal(err)
		}
		return path
	}

	// The reviewed 2.0.0 baseline supplies release A's control tag.
	baselinePath := write("2.0.0.json", Snapshot{SchemaVersion: snapshotSchemaVersion, GatewayVersion: "2.0.0",
		SourceCommit: commitA, CatalogSHA256: catalogSHA, PlanID: digest("b"), ProvenanceMode: "candidate",
		Plugins: []SnapshotEntry{entry("third", "1.4.9", hash(commitA, "1.4.9", "third"), baseDigest, commitA)}})

	// Release 2.0.1: demo's planned tag holds an unannotated legacy artifact and
	// other's holds a differently built artifact of the very same source, so both
	// are blocked with different dispositions.
	planA := Plan{SchemaVersion: planSchemaVersion, GatewayVersion: "2.0.1", SourceCommit: commitA, PreviousRelease: "2.0.0",
		CatalogSHA256: catalogSHA, Plugins: []PlanEntry{
			planEntry("demo", "", "1.0.1", demoHash),
			planEntry("other", "", "2.0.0", otherHash),
			planEntry("third", "1.4.9", "1.5.0", thirdHashA),
		}}
	planA.PlanID = "sha256:" + canonicalObjectHash(planA, true)
	planAPath := write("plan-2.0.1.json", planA)
	evidenceAPath := write("evidence-2.0.1.json", CandidateEvidenceFile{Plugins: map[string]CandidateEvidence{
		"demo":  {CandidateRef: candidateRef("demo", demoDigest), Digest: demoDigest, SourceCommit: commitA, InputHash: demoHash},
		"other": {CandidateRef: candidateRef("other", otherDigest), Digest: otherDigest, SourceCommit: commitA, InputHash: otherHash},
		"third": {CandidateRef: candidateRef("third", thirdDigestA), Digest: thirdDigestA, SourceCommit: commitA, InputHash: thirdHashA},
	}})
	withManifestResolver(t, migrationResolver(t, map[string]ociManifest{
		publicRef("third", "1.4.9"): {Digest: baseDigest},
		publicRef("demo", "1.0.1"):  {Digest: demoLegacy},
		publicRef("other", "2.0.0"): annotated(otherLegacy, commitA, otherHash, "2.0.0"),
	}))
	reportA, err := migrationPreflight(catalogPath, planAPath, baselinePath, evidenceAPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(reportA.Entries) != 2 || reportA.Entries[0].LogicalID != "demo" || reportA.Entries[1].LogicalID != "other" {
		t.Fatalf("release A did not block both conflicting plugins: %#v", reportA.Entries)
	}
	if reportA.Entries[0].Recommendation != migrationRecommendDeleteLegacy || reportA.Entries[1].Recommendation != migrationRecommendAdoptPublic {
		t.Fatalf("release A recorded unexpected dispositions: %#v", reportA.Entries)
	}
	if reportA.Entries[1].SourceComparison != migrationSourceMatch {
		t.Fatalf("release A mis-compared the annotated artifact's source: %#v", reportA.Entries[1])
	}
	reportAPath := write("report-2.0.1.json", reportA)
	renderedA, err := renderSnapshot(catalogPath, planAPath, baselinePath, evidenceAPath, "")
	if err != nil {
		t.Fatal(err)
	}
	snapshotA, err := applyMigrationReport(renderedA, reportAPath)
	if err != nil {
		t.Fatal(err)
	}
	snapshotAPath := write("2.0.1.json", snapshotA)
	if snapshotA.MigrationPreflight == nil || snapshotA.MigrationPreflight.ControlRef != publicRef("third", "1.4.9") ||
		snapshotA.MigrationPreflight.ControlDigest != baseDigest || strings.Join(snapshotA.MigrationPreflight.Excluded, ",") != "demo,other" {
		t.Fatalf("release A lost its preflight marker: %#v", snapshotA.MigrationPreflight)
	}

	// Release 2.0.2 plans only third, at a different source commit.
	planB := Plan{SchemaVersion: planSchemaVersion, GatewayVersion: "2.0.2", SourceCommit: commitB, PreviousRelease: "2.0.1",
		CatalogSHA256: catalogSHA, Plugins: []PlanEntry{planEntry("third", "1.5.0", "1.5.1", thirdHashB)}}
	planB.PlanID = "sha256:" + canonicalObjectHash(planB, true)
	planBPath := write("plan-2.0.2.json", planB)
	evidenceBPath := write("evidence-2.0.2.json", CandidateEvidenceFile{Plugins: map[string]CandidateEvidence{
		"third": {CandidateRef: candidateRef("third", thirdDigestB), Digest: thirdDigestB, SourceCommit: commitB, InputHash: thirdHashB},
	}})

	// The sweep calibrates against third's 2.0.1 tag: the two excluded entries
	// sort ahead of it and their public tags still serve the legacy artifacts.
	withManifestResolver(t, migrationResolver(t, map[string]ociManifest{
		publicRef("third", "1.5.0"): {Digest: thirdDigestA},
	}))
	reportB, err := migrationPreflight(catalogPath, planBPath, snapshotAPath, evidenceBPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(reportB.Entries) != 0 {
		t.Fatalf("release B unexpectedly found a conflict: %#v", reportB.Entries)
	}
	if reportB.ControlRef != publicRef("third", "1.5.0") || reportB.ControlDigest != thirdDigestA {
		t.Fatalf("release B calibrated against an excluded tag: %#v", reportB)
	}
	reportBPath := write("report-2.0.2.json", reportB)

	renderedB, err := renderSnapshot(catalogPath, planBPath, snapshotAPath, evidenceBPath, "")
	if err != nil {
		t.Fatal(err)
	}
	if renderedB.MigrationPreflight != nil {
		t.Fatalf("rendering alone must not invent a preflight marker: %#v", renderedB.MigrationPreflight)
	}
	if err := requireNoCarriedMigration(renderedB); err == nil || !strings.Contains(err.Error(), "--migration-report is required to rebind them") {
		t.Fatalf("rendering carried exclusions without a sweep must fail closed: %v", err)
	}
	snapshotB, err := applyMigrationReport(renderedB, reportBPath)
	if err != nil {
		t.Fatal(err)
	}
	snapshotBPath := write("2.0.2.json", snapshotB)
	carried := map[string]SnapshotEntry{}
	for _, snapshotEntry := range snapshotB.Plugins {
		carried[snapshotEntry.LogicalID] = snapshotEntry
	}
	previous := map[string]SnapshotEntry{}
	for _, snapshotEntry := range snapshotA.Plugins {
		previous[snapshotEntry.LogicalID] = snapshotEntry
	}
	for _, id := range []string{"demo", "other"} {
		if !reflect.DeepEqual(previous[id], carried[id]) {
			t.Fatalf("%s was not carried forward verbatim:\n previous %#v\n carried  %#v", id, previous[id], carried[id])
		}
		if carried[id].Migration.SourceCommit != commitA {
			t.Fatalf("%s lost the source commit of the sweep that decided its disposition: %#v", id, carried[id].Migration)
		}
	}
	// The carried adopt-public disposition is only re-derivable from the commit
	// that decided it: release B was computed at a different source commit.
	if carried["other"].Migration.Recommendation != migrationRecommendAdoptPublic || carried["other"].Migration.SourceComparison != migrationSourceMatch {
		t.Fatalf("carried adopt-public disposition did not survive the later release: %#v", carried["other"].Migration)
	}
	if carried["third"].Migration != nil || carried["third"].Version != "1.5.1" || carried["third"].Digest != thirdDigestB {
		t.Fatalf("the planned plugin was not re-planned by release B: %#v", carried["third"])
	}
	if snapshotB.MigrationPreflight == nil || snapshotB.MigrationPreflight.ControlRef != publicRef("third", "1.5.0") ||
		snapshotB.MigrationPreflight.ControlDigest != thirdDigestA || strings.Join(snapshotB.MigrationPreflight.Excluded, ",") != "demo,other" {
		t.Fatalf("release B did not rebind the preflight marker to its own sweep: %#v", snapshotB.MigrationPreflight)
	}
	if err := validateSnapshotMigration(snapshotB); err != nil {
		t.Fatalf("carried exclusions did not validate in the later release: %v", err)
	}

	// Verification of the later release must tolerate both carried exclusions at
	// the new source commit, in either OCI source mode.
	untouched := map[string]string{publicRef("demo", "1.0.1"): demoLegacy, publicRef("other", "2.0.0"): otherLegacy}
	forbidden := make([]string, 0, len(untouched))
	for ref := range untouched {
		forbidden = append(forbidden, ref)
	}
	sort.Strings(forbidden)
	candidatesB := map[string]ociManifest{
		candidateRef("demo", demoDigest):    annotated(demoDigest, commitA, demoHash, "1.0.1"),
		candidateRef("other", otherDigest):  annotated(otherDigest, commitA, otherHash, "2.0.0"),
		candidateRef("third", thirdDigestB): annotated(thirdDigestB, commitB, thirdHashB, "1.5.1"),
	}
	withManifestResolver(t, neverResolve(t, forbidden, candidatesB))
	if err := verifySnapshotBindings(root, catalogPath, snapshotBPath, planBPath, snapshotAPath, commitB, commitB, true, "candidate"); err != nil {
		t.Fatalf("release B verification rejected the carried exclusions: %v", err)
	}
	publicB := map[string]ociManifest{publicRef("third", "1.5.1"): annotated(thirdDigestB, commitB, thirdHashB, "1.5.1")}
	withManifestResolver(t, neverResolve(t, forbidden, publicB))
	if err := verifySnapshotBindings(root, catalogPath, snapshotBPath, planBPath, snapshotAPath, commitB, commitB, true, "public"); err != nil {
		t.Fatalf("public verification of release B rejected the carried exclusions: %v", err)
	}

	// Promote of the later release still skips both blocked plugins and leaves
	// their divergent public tags untouched.
	result := runPromotionVersionContractState(t, snapshotDocument(t, snapshotB), map[string]any{
		publicRef("demo", "1.0.1"):  map[string]any{"digest": demoLegacy, "version": ""},
		publicRef("other", "2.0.0"): map[string]any{"digest": otherLegacy, "version": ""},
		publicRef("third", "1.5.0"): map[string]any{"digest": thirdDigestA, "version": ""},
	})
	if result.err != nil {
		t.Fatalf("release B promote failed on carried exclusions: %v\n%s", result.err, result.output)
	}
	entries := versionJournalEntries(t, result.journal)
	if len(entries) != 3 {
		t.Fatalf("release B journal lost an entry: %#v", entries)
	}
	if entries[0]["preflight"] != "migration-excluded" || entries[1]["preflight"] != "migration-excluded" || entries[2]["preflight"] != "absent" {
		t.Fatalf("release B journal has unexpected states: %#v", entries)
	}
	if migration, ok := entries[1]["migration"].(map[string]any); !ok || migration["recommendation"] != migrationRecommendAdoptPublic {
		t.Fatalf("release B journal did not document the carried disposition: %#v", entries[1])
	}
	if !strings.Contains(result.log, "cp "+candidateRef("third", thirdDigestB)+" "+publicRef("third", "1.5.1")) {
		t.Fatalf("release B did not promote the planned plugin:\n%s", result.log)
	}
	for _, ref := range forbidden {
		if strings.Contains(result.log, ref) {
			t.Fatalf("release B touched an excluded plugin's public tag %s:\n%s", ref, result.log)
		}
		if got := digestOf(t, result.state[ref]); got != untouched[ref] {
			t.Fatalf("excluded tag %s was mutated from %s to %s", ref, untouched[ref], got)
		}
	}
	if got := digestOf(t, result.state[publicRef("third", "1.5.1")]); got != thirdDigestB {
		t.Fatalf("release B published the wrong digest for third: %s", got)
	}
	if !strings.Contains(result.summary, "Migration preflight excluded from this promote batch: demo, other") {
		t.Fatalf("release B step summary did not document the carried exclusions: %q", result.summary)
	}
}

// neverResolve serves present references and fails the test on any reference an
// excluded plugin's divergent public tag would require, so a verification that
// resolves too much cannot pass silently.
func neverResolve(t *testing.T, forbidden []string, present map[string]ociManifest) func(string) (ociManifest, error) {
	t.Helper()
	return func(ref string) (ociManifest, error) {
		for _, banned := range forbidden {
			if ref == banned {
				t.Fatalf("verification resolved an excluded plugin's divergent public tag: %s", ref)
			}
		}
		if manifest, ok := present[ref]; ok {
			return manifest, nil
		}
		return ociManifest{}, notFoundError(ref)
	}
}

// snapshotDocument converts a rendered snapshot into the JSON document the
// workflow shell contracts consume.
func snapshotDocument(t *testing.T, snapshot Snapshot) map[string]any {
	t.Helper()
	var document map[string]any
	if err := json.Unmarshal(mustJSON(t, snapshot), &document); err != nil {
		t.Fatal(err)
	}
	return document
}
