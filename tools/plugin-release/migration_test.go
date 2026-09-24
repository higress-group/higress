// Copyright 2026 Higress Authors
// Licensed under the Apache License, Version 2.0.

package main

import (
	"path/filepath"
	"strings"
	"testing"
)

type migrationFixture struct {
	root          string
	catalog       string
	plan          string
	previous      string
	evidence      string
	planValue     Plan
	snapshot      Snapshot
	controlRef    string
	controlDigest string
	demoRef       string
	otherRef      string
	demoDigest    string
	otherDigest   string
	demoInputHash string
	otherHash     string
	sourceCommit  string
}

// migrationFixture builds the pure-JSON inputs the sweep consumes: a catalog, a
// hashed plan for two plugins, candidate evidence for both, and a reviewed
// previous snapshot that supplies the known-good control tag. No Git tree is
// needed because the sweep classifies registry state, not source state.
func newMigrationFixture(t *testing.T) migrationFixture {
	t.Helper()
	root := t.TempDir()
	sourceCommit := strings.Repeat("a", 40)
	demoInputHash := "sha256:" + strings.Repeat("b", 64)
	otherHash := "sha256:" + strings.Repeat("c", 64)
	demoDigest := "sha256:" + strings.Repeat("d", 64)
	otherDigest := "sha256:" + strings.Repeat("e", 64)
	controlDigest := "sha256:" + strings.Repeat("f", 64)
	c := Catalog{SchemaVersion: 1, Registry: "registry.example", Plugins: []Plugin{
		{LogicalID: "demo", Implementation: "go", SourceDir: "plugins/wasm-go/extensions/demo", Image: "plugins/demo", ReleaseEligible: true, ArtifactInputs: []string{"plugins/wasm-go/extensions/demo/**"}},
		{LogicalID: "other", Implementation: "go", SourceDir: "plugins/wasm-go/extensions/other", Image: "plugins/other", ReleaseEligible: true, ArtifactInputs: []string{"plugins/wasm-go/extensions/other/**"}},
	}}
	catalogPath := filepath.Join(root, "catalog.json")
	if err := writeCanonical(catalogPath, c); err != nil {
		t.Fatal(err)
	}
	plan := Plan{SchemaVersion: planSchemaVersion, GatewayVersion: "2.0.1", SourceCommit: sourceCommit,
		PreviousRelease: "2.0.0", CatalogSHA256: sha256Hex(mustRead(t, catalogPath)), Plugins: []PlanEntry{
			{LogicalID: "demo", Implementation: "go", SourceDir: "plugins/wasm-go/extensions/demo", Image: "plugins/demo", PreviousVersion: "1.0.0", Version: "1.0.1", InputHash: demoInputHash, ChangedPaths: []string{}},
			{LogicalID: "other", Implementation: "go", SourceDir: "plugins/wasm-go/extensions/other", Image: "plugins/other", PreviousVersion: "1.9.9", Version: "2.0.0", InputHash: otherHash, ChangedPaths: []string{}},
		}}
	plan.PlanID = "sha256:" + canonicalObjectHash(plan, true)
	planPath := filepath.Join(root, "plan.json")
	if err := writeCanonical(planPath, plan); err != nil {
		t.Fatal(err)
	}
	previous := Snapshot{SchemaVersion: snapshotSchemaVersion, GatewayVersion: "2.0.0", SourceCommit: sourceCommit,
		CatalogSHA256: plan.CatalogSHA256, PlanID: plan.PlanID, ProvenanceMode: "candidate", Plugins: []SnapshotEntry{
			{LogicalID: "demo", Implementation: "go", SourceDir: "plugins/wasm-go/extensions/demo", Image: "plugins/demo", Version: "1.0.0", OCIRef: "registry.example/plugins/demo:1.0.0", Digest: controlDigest, InputHash: demoInputHash, SourceCommit: sourceCommit, CandidateRef: "registry.example/candidates/demo@" + controlDigest, ProvenanceMode: "candidate"},
		}}
	previousPath := filepath.Join(root, "previous.json")
	if err := writeCanonical(previousPath, previous); err != nil {
		t.Fatal(err)
	}
	evidence := CandidateEvidenceFile{Plugins: map[string]CandidateEvidence{
		"demo":  {CandidateRef: "registry.example/candidates/demo@" + demoDigest, Digest: demoDigest, SourceCommit: sourceCommit, InputHash: demoInputHash},
		"other": {CandidateRef: "registry.example/candidates/other@" + otherDigest, Digest: otherDigest, SourceCommit: sourceCommit, InputHash: otherHash},
	}}
	evidencePath := filepath.Join(root, "evidence.json")
	if err := writeCanonical(evidencePath, evidence); err != nil {
		t.Fatal(err)
	}
	snapshot, err := renderSnapshot(catalogPath, planPath, "", evidencePath, "")
	if err != nil {
		t.Fatal(err)
	}
	return migrationFixture{
		root: root, catalog: catalogPath, plan: planPath, previous: previousPath, evidence: evidencePath,
		planValue: plan, snapshot: snapshot,
		controlRef: "registry.example/plugins/demo:1.0.0", controlDigest: controlDigest,
		demoRef: "registry.example/plugins/demo:1.0.1", otherRef: "registry.example/plugins/other:2.0.0",
		demoDigest: demoDigest, otherDigest: otherDigest,
		demoInputHash: demoInputHash, otherHash: otherHash, sourceCommit: sourceCommit,
	}
}

func notFoundError(ref string) error {
	return &migrationProbeError{message: "Error response from registry: " + ref + ": not found"}
}

type migrationProbeError struct {
	message string
}

func (e *migrationProbeError) Error() string { return e.message }

// migrationResolver serves a fixed registry view: every reference in present is
// resolved from the map, and every other reference is reported absent with the
// provider-structured registry error the classifier requires.
func migrationResolver(t *testing.T, present map[string]ociManifest) func(string) (ociManifest, error) {
	t.Helper()
	return func(ref string) (ociManifest, error) {
		if manifest, ok := present[ref]; ok {
			return manifest, nil
		}
		return ociManifest{}, notFoundError(ref)
	}
}

func TestMigrationPreflightBlocksOnlyOccupiedPlannedTags(t *testing.T) {
	f := newMigrationFixture(t)
	legacy := "sha256:" + strings.Repeat("1", 64)
	withManifestResolver(t, migrationResolver(t, map[string]ociManifest{
		f.controlRef: {Digest: f.controlDigest},
		f.demoRef:    {Digest: legacy},
	}))
	report, err := migrationPreflight(f.catalog, f.plan, f.previous, f.evidence)
	if err != nil {
		t.Fatal(err)
	}
	if report.ProbeMode != migrationProbeCandidateDigest || report.ControlRef != f.controlRef || report.ControlDigest != f.controlDigest {
		t.Fatalf("report lost its probe calibration: %#v", report)
	}
	if report.GatewayVersion != "2.0.1" || report.SourceCommit != f.sourceCommit || report.PlanID != f.planValue.PlanID || report.Registry != "registry.example" {
		t.Fatalf("report is not bound to the exact plan: %#v", report)
	}
	if len(report.Entries) != 1 {
		t.Fatalf("absent planned tag must not be reported: %#v", report.Entries)
	}
	entry := report.Entries[0]
	if entry.LogicalID != "demo" || entry.State != migrationStateBlocked || entry.Recommendation != migrationRecommendDeleteLegacy {
		t.Fatalf("unannotated legacy artifact was not classified as delete-legacy: %#v", entry)
	}
	if entry.OCIRef != f.demoRef || entry.ExistingDigest != legacy || entry.PlannedDigest != f.demoDigest || entry.InputHash != f.demoInputHash {
		t.Fatalf("report entry does not carry the reviewed conflict: %#v", entry)
	}
	if entry.SourceComparison != migrationSourceUnannotated || entry.ExistingVersion != "" || entry.ExistingRevision != "" || entry.ExistingInputHash != "" {
		t.Fatalf("unannotated artifact recorded annotations: %#v", entry)
	}
}

func TestMigrationPreflightAcceptsEmergencyDigestEqualToCandidateEvidence(t *testing.T) {
	f := newMigrationFixture(t)
	// The unified publication contract makes an emergency overwrite with the
	// candidate inputs byte-identical, so this is already present, not blocked.
	withManifestResolver(t, migrationResolver(t, map[string]ociManifest{
		f.controlRef: {Digest: f.controlDigest},
		f.demoRef:    {Digest: f.demoDigest, Annotations: map[string]string{"org.opencontainers.image.version": "1.0.1", "org.opencontainers.image.revision": f.sourceCommit, "io.higress.plugin.input-hash": f.demoInputHash}},
		f.otherRef:   {Digest: f.otherDigest},
	}))
	report, err := migrationPreflight(f.catalog, f.plan, f.previous, f.evidence)
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Entries) != 0 {
		t.Fatalf("a planned tag already serving the planned digest is not a conflict: %#v", report.Entries)
	}
}

func TestMigrationPreflightRecommendationsAreDeterministic(t *testing.T) {
	f := newMigrationFixture(t)
	annotated := func(version, revision, inputHash string) ociManifest {
		annotations := map[string]string{}
		if version != "" {
			annotations["org.opencontainers.image.version"] = version
		}
		if revision != "" {
			annotations["org.opencontainers.image.revision"] = revision
		}
		if inputHash != "" {
			annotations["io.higress.plugin.input-hash"] = inputHash
		}
		return ociManifest{Digest: "sha256:" + strings.Repeat("9", 64), Annotations: annotations}
	}
	for _, tc := range []struct {
		name             string
		manifest         ociManifest
		recommendation   string
		sourceComparison string
	}{
		{name: "unannotated-legacy", manifest: annotated("", "", ""), recommendation: migrationRecommendDeleteLegacy, sourceComparison: migrationSourceUnannotated},
		{name: "same-inputs-managed", manifest: annotated("1.0.1", f.sourceCommit, f.demoInputHash), recommendation: migrationRecommendAdoptPublic, sourceComparison: migrationSourceMatch},
		{name: "other-managed-build", manifest: annotated("1.0.1", strings.Repeat("7", 40), "sha256:"+strings.Repeat("8", 64)), recommendation: migrationRecommendBumpVersion, sourceComparison: migrationSourceMismatch},
		{name: "same-source-different-version", manifest: annotated("9.9.9", f.sourceCommit, f.demoInputHash), recommendation: migrationRecommendBumpVersion, sourceComparison: migrationSourceMatch},
		{name: "partial-annotation", manifest: annotated("1.0.1", "", ""), recommendation: migrationRecommendBumpVersion, sourceComparison: migrationSourceMismatch},
	} {
		t.Run(tc.name, func(t *testing.T) {
			withManifestResolver(t, migrationResolver(t, map[string]ociManifest{
				f.controlRef: {Digest: f.controlDigest},
				f.demoRef:    tc.manifest,
			}))
			report, err := migrationPreflight(f.catalog, f.plan, f.previous, f.evidence)
			if err != nil {
				t.Fatal(err)
			}
			if len(report.Entries) != 1 {
				t.Fatalf("expected exactly one conflict: %#v", report.Entries)
			}
			entry := report.Entries[0]
			if entry.Recommendation != tc.recommendation || entry.SourceComparison != tc.sourceComparison {
				t.Fatalf("recommendation=%q sourceComparison=%q, want %q/%q", entry.Recommendation, entry.SourceComparison, tc.recommendation, tc.sourceComparison)
			}
			if want := migrationRecommendationFromRecord(entry.ExistingVersion, entry.ExistingRevision, entry.ExistingInputHash, entry.InputHash, entry.Version, report.SourceCommit); want != entry.Recommendation {
				t.Fatalf("recorded disposition is not re-derivable: %q != %q", entry.Recommendation, want)
			}
		})
	}
}

func TestMigrationPreflightFailsClosedOnUncalibratedProbe(t *testing.T) {
	f := newMigrationFixture(t)
	for _, tc := range []struct {
		name     string
		control  ociManifest
		err      error
		contains string
	}{
		{name: "digest-mismatch", control: ociManifest{Digest: "sha256:" + strings.Repeat("2", 64)}, contains: "probe is not trustworthy"},
		{name: "control-absent", err: notFoundError(f.controlRef), contains: "cannot calibrate"},
		{name: "control-unauthorized", err: &migrationProbeError{message: "unauthorized: authentication required"}, contains: "authorization failure is never absence"},
		{name: "control-unclassified", err: &migrationProbeError{message: "dial tcp: connection refused"}, contains: "cannot calibrate"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			withManifestResolver(t, func(ref string) (ociManifest, error) {
				if ref != f.controlRef {
					t.Fatalf("probe continued past an uncalibrated control tag: %s", ref)
				}
				if tc.err != nil {
					return ociManifest{}, tc.err
				}
				return tc.control, nil
			})
			_, err := migrationPreflight(f.catalog, f.plan, f.previous, f.evidence)
			if err == nil || !strings.Contains(err.Error(), tc.contains) {
				t.Fatalf("uncalibrated probe was accepted: %v", err)
			}
		})
	}
}

// TestMigrationControlTagSkipsExcludedEntries proves the calibration probe never
// selects an excluded entry's public tag. That tag still serves the legacy
// artifact that caused the exclusion, not the digest its snapshot records, so
// calibrating against it would fail every later prepare — including a release
// that has nothing to do with the blocked plugin.
func TestMigrationControlTagSkipsExcludedEntries(t *testing.T) {
	f := newMigrationFixture(t)
	entry := func(id, fill, provenance string, blocked bool) SnapshotEntry {
		digest := "sha256:" + strings.Repeat(fill, 64)
		out := SnapshotEntry{LogicalID: id, Implementation: "go", SourceDir: "plugins/wasm-go/extensions/" + id,
			Image: "plugins/" + id, Version: "1.0.0", OCIRef: "registry.example/plugins/" + id + ":1.0.0", Digest: digest,
			InputHash: digest, SourceCommit: f.sourceCommit, ProvenanceMode: provenance}
		if provenance == "candidate" {
			out.CandidateRef = "registry.example/candidates/" + id + "@" + digest
		}
		if blocked {
			out.Migration = &SnapshotMigration{State: migrationStateBlocked, SourceCommit: f.sourceCommit,
				ExistingDigest: "sha256:" + strings.Repeat("1", 64), PlannedDigest: digest,
				SourceComparison: migrationSourceUnannotated, Recommendation: migrationRecommendDeleteLegacy}
		}
		return out
	}
	previousPath := func(name string, entries ...SnapshotEntry) string {
		path := filepath.Join(f.root, name+".json")
		previous := Snapshot{SchemaVersion: snapshotSchemaVersion, GatewayVersion: "2.0.0", SourceCommit: f.sourceCommit,
			CatalogSHA256: f.planValue.CatalogSHA256, PlanID: f.planValue.PlanID, ProvenanceMode: "candidate", Plugins: entries}
		if err := writeCanonical(path, previous); err != nil {
			t.Fatal(err)
		}
		return path
	}
	for _, tc := range []struct {
		name       string
		previous   string
		wantRef    string
		wantDigest string
		contains   string
	}{
		{
			// Both excluded entries sort ahead of the only usable control.
			name:       "skips-excluded-entries",
			previous:   previousPath("control-skips", entry("aaa", "2", "candidate", true), entry("bbb", "3", "public", true), entry("demo", "f", "candidate", false)),
			wantRef:    f.controlRef,
			wantDigest: f.controlDigest,
		},
		{
			name:       "prefers-pipeline-published-tag",
			previous:   previousPath("control-prefers", entry("adopted", "4", "public", false), entry("built", "5", "candidate", false)),
			wantRef:    "registry.example/plugins/built:1.0.0",
			wantDigest: "sha256:" + strings.Repeat("5", 64),
		},
		{
			name:       "falls-back-to-adopted-public-tag",
			previous:   previousPath("control-adopted", entry("adopted", "4", "public", false)),
			wantRef:    "registry.example/plugins/adopted:1.0.0",
			wantDigest: "sha256:" + strings.Repeat("4", 64),
		},
		{
			name:     "every-entry-excluded",
			previous: previousPath("control-all-blocked", entry("aaa", "2", "candidate", true), entry("bbb", "3", "public", true)),
			contains: "cannot calibrate its probe",
		},
		{
			name:     "no-published-provenance",
			previous: previousPath("control-unpublished", entry("local", "6", "local", false)),
			contains: "cannot calibrate its probe",
		},
		{
			name:     "no-previous-snapshot",
			contains: "requires the reviewed previous snapshot",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ref, digest, err := migrationControlTag(tc.previous)
			if tc.contains != "" {
				if err == nil || !strings.Contains(err.Error(), tc.contains) {
					t.Fatalf("control tag selection did not fail closed: %v", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if ref != tc.wantRef || digest != tc.wantDigest {
				t.Fatalf("control tag = %s @ %s, want %s @ %s", ref, digest, tc.wantRef, tc.wantDigest)
			}
		})
	}

	// The sweep must calibrate against the surviving control and never probe an
	// excluded entry's tag, while still classifying a real conflict.
	sweep := previousPath("control-sweep", entry("aaa", "2", "candidate", true), entry("bbb", "3", "public", true), entry("demo", "f", "candidate", false))
	legacy := "sha256:" + strings.Repeat("1", 64)
	var probed []string
	withManifestResolver(t, func(ref string) (ociManifest, error) {
		probed = append(probed, ref)
		if ref == f.controlRef {
			return ociManifest{Digest: f.controlDigest}, nil
		}
		if ref == f.demoRef {
			return ociManifest{Digest: legacy}, nil
		}
		return ociManifest{}, notFoundError(ref)
	})
	report, err := migrationPreflight(f.catalog, f.plan, sweep, f.evidence)
	if err != nil {
		t.Fatal(err)
	}
	if report.ControlRef != f.controlRef || report.ControlDigest != f.controlDigest {
		t.Fatalf("sweep calibrated against the wrong control: %#v", report)
	}
	if len(report.Entries) != 1 || report.Entries[0].LogicalID != "demo" || report.Entries[0].Recommendation != migrationRecommendDeleteLegacy {
		t.Fatalf("sweep lost the conflict it still had to classify: %#v", report.Entries)
	}
	for _, ref := range probed {
		if strings.Contains(ref, "/aaa:") || strings.Contains(ref, "/bbb:") {
			t.Fatalf("sweep probed an excluded entry's public tag: %s", ref)
		}
	}
}

func TestMigrationPreflightAuthorizationFailureIsNeverAbsence(t *testing.T) {
	f := newMigrationFixture(t)
	withManifestResolver(t, func(ref string) (ociManifest, error) {
		if ref == f.controlRef {
			return ociManifest{Digest: f.controlDigest}, nil
		}
		return ociManifest{}, &migrationProbeError{message: "denied: requested access to the resource is denied"}
	})
	_, err := migrationPreflight(f.catalog, f.plan, f.previous, f.evidence)
	if err == nil || !strings.Contains(err.Error(), "authorization failure is never absence") {
		t.Fatalf("authorization failure was treated as absence: %v", err)
	}
}

func TestMigrationPreflightWithoutCandidatesOnlySuspects(t *testing.T) {
	f := newMigrationFixture(t)
	withManifestResolver(t, migrationResolver(t, map[string]ociManifest{
		f.controlRef: {Digest: f.controlDigest},
		f.demoRef:    {Digest: "sha256:" + strings.Repeat("1", 64)},
	}))
	report, err := migrationPreflight(f.catalog, f.plan, f.previous, "")
	if err != nil {
		t.Fatal(err)
	}
	if report.ProbeMode != migrationProbeExistenceOnly {
		t.Fatalf("dry-run sweep reported probe mode %q", report.ProbeMode)
	}
	if len(report.Entries) != 1 {
		t.Fatalf("expected the occupied planned tag to be reported: %#v", report.Entries)
	}
	entry := report.Entries[0]
	if entry.State != migrationStateSuspected || entry.Recommendation != "" || entry.PlannedDigest != "" {
		t.Fatalf("existence-only sweep blocked a plugin: %#v", entry)
	}
	// A dry-run report can never enter a snapshot.
	reportPath := filepath.Join(f.root, "report.json")
	if err := writeCanonical(reportPath, report); err != nil {
		t.Fatal(err)
	}
	if _, err := applyMigrationReport(f.snapshot, reportPath); err == nil || !strings.Contains(err.Error(), migrationProbeCandidateDigest) {
		t.Fatalf("existence-only report was bound into a snapshot: %v", err)
	}
	if markdown := renderMigrationMarkdown(report); !strings.Contains(markdown, "Dry-run existence sweep") || strings.Contains(markdown, "**delete-legacy**") {
		t.Fatalf("dry-run markdown misreported the sweep:\n%s", markdown)
	}
}

func TestMigrationPreflightRejectsIncompleteCandidateEvidence(t *testing.T) {
	f := newMigrationFixture(t)
	evidence := CandidateEvidenceFile{Plugins: map[string]CandidateEvidence{
		"demo": {CandidateRef: "registry.example/candidates/demo@" + f.demoDigest, Digest: f.demoDigest, SourceCommit: f.sourceCommit, InputHash: f.demoInputHash},
	}}
	path := filepath.Join(f.root, "partial-evidence.json")
	if err := writeCanonical(path, evidence); err != nil {
		t.Fatal(err)
	}
	withManifestResolver(t, migrationResolver(t, map[string]ociManifest{f.controlRef: {Digest: f.controlDigest}}))
	if _, err := migrationPreflight(f.catalog, f.plan, f.previous, path); err == nil || !strings.Contains(err.Error(), "no matching candidate digest") {
		t.Fatalf("sweep accepted a plan entry without candidate evidence: %v", err)
	}
}

func TestMigrationPreflightRejectsUnhashedOrStalePlan(t *testing.T) {
	f := newMigrationFixture(t)
	withManifestResolver(t, migrationResolver(t, map[string]ociManifest{f.controlRef: {Digest: f.controlDigest}}))
	edited := f.planValue
	edited.Plugins[0].Version = "1.0.2"
	path := filepath.Join(f.root, "edited-plan.json")
	if err := writeCanonical(path, edited); err != nil {
		t.Fatal(err)
	}
	if _, err := migrationPreflight(f.catalog, path, f.previous, f.evidence); err == nil || !strings.Contains(err.Error(), "planId does not match canonical plan content") {
		t.Fatalf("sweep accepted a plan edited after it was hashed: %v", err)
	}
	if _, err := migrationPreflight(f.catalog, f.plan, "", f.evidence); err == nil || !strings.Contains(err.Error(), "calibrate") {
		t.Fatalf("sweep ran without a control tag: %v", err)
	}
}

func TestApplyMigrationReportBindsExclusionIntoSnapshot(t *testing.T) {
	f := newMigrationFixture(t)
	legacy := "sha256:" + strings.Repeat("1", 64)
	withManifestResolver(t, migrationResolver(t, map[string]ociManifest{
		f.controlRef: {Digest: f.controlDigest},
		f.demoRef:    {Digest: legacy},
	}))
	report, err := migrationPreflight(f.catalog, f.plan, f.previous, f.evidence)
	if err != nil {
		t.Fatal(err)
	}
	reportPath := filepath.Join(f.root, "report.json")
	if err := writeCanonical(reportPath, report); err != nil {
		t.Fatal(err)
	}
	snapshot, err := applyMigrationReport(f.snapshot, reportPath)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.MigrationPreflight == nil || len(snapshot.MigrationPreflight.Excluded) != 1 || snapshot.MigrationPreflight.Excluded[0] != "demo" {
		t.Fatalf("snapshot lost its migration preflight marker: %#v", snapshot.MigrationPreflight)
	}
	if snapshot.MigrationPreflight.ControlRef != f.controlRef || snapshot.MigrationPreflight.ControlDigest != f.controlDigest {
		t.Fatalf("snapshot marker dropped the probe calibration: %#v", snapshot.MigrationPreflight)
	}
	byID := map[string]SnapshotEntry{}
	for _, entry := range snapshot.Plugins {
		byID[entry.LogicalID] = entry
	}
	if byID["demo"].Migration == nil || byID["demo"].Migration.Recommendation != migrationRecommendDeleteLegacy || byID["demo"].Migration.ExistingDigest != legacy || byID["demo"].Migration.PlannedDigest != f.demoDigest {
		t.Fatalf("blocked entry lost its migration state: %#v", byID["demo"].Migration)
	}
	if byID["other"].Migration != nil {
		t.Fatalf("clear entry was marked: %#v", byID["other"].Migration)
	}
	if err := validateSnapshotMigration(snapshot); err != nil {
		t.Fatalf("rendered migration binding did not validate: %v", err)
	}
	// An empty report leaves the snapshot byte-identical to the all-green path.
	empty := report
	empty.Entries = []MigrationReportEntry{}
	emptyPath := filepath.Join(f.root, "empty-report.json")
	if err := writeCanonical(emptyPath, empty); err != nil {
		t.Fatal(err)
	}
	unchanged, err := applyMigrationReport(f.snapshot, emptyPath)
	if err != nil {
		t.Fatal(err)
	}
	if unchanged.MigrationPreflight != nil {
		t.Fatalf("empty report added a marker: %#v", unchanged.MigrationPreflight)
	}
	wantPath := filepath.Join(f.root, "want.json")
	gotPath := filepath.Join(f.root, "got.json")
	if err := writeCanonical(wantPath, f.snapshot); err != nil {
		t.Fatal(err)
	}
	if err := writeCanonical(gotPath, unchanged); err != nil {
		t.Fatal(err)
	}
	if string(mustRead(t, wantPath)) != string(mustRead(t, gotPath)) {
		t.Fatal("an all-green sweep changed the snapshot bytes")
	}
	if markdown := renderMigrationMarkdown(report); !strings.Contains(markdown, "**delete-legacy**") || !strings.Contains(markdown, legacy) || !strings.Contains(markdown, f.demoDigest) {
		t.Fatalf("migration report markdown lost the conflict detail:\n%s", markdown)
	}
	if markdown := renderMigrationMarkdown(empty); !strings.Contains(markdown, "No planned version tag is occupied") {
		t.Fatalf("all-green markdown misreported the sweep:\n%s", markdown)
	}
}

func TestApplyMigrationReportRejectsInconsistentReports(t *testing.T) {
	f := newMigrationFixture(t)
	legacy := "sha256:" + strings.Repeat("1", 64)
	withManifestResolver(t, migrationResolver(t, map[string]ociManifest{
		f.controlRef: {Digest: f.controlDigest},
		f.demoRef:    {Digest: legacy},
	}))
	report, err := migrationPreflight(f.catalog, f.plan, f.previous, f.evidence)
	if err != nil {
		t.Fatal(err)
	}
	base := report.Entries[0]
	for _, tc := range []struct {
		name     string
		mutate   func(*MigrationReportFile)
		contains string
	}{
		{name: "wrong-gateway", mutate: func(r *MigrationReportFile) { r.GatewayVersion = "2.0.2" }, contains: "immutable provenance differ"},
		{name: "wrong-plan-id", mutate: func(r *MigrationReportFile) { r.PlanID = "sha256:" + strings.Repeat("0", 64) }, contains: "immutable provenance differ"},
		{name: "no-control", mutate: func(r *MigrationReportFile) { r.ControlDigest = "" }, contains: "calibrated control tag"},
		{name: "unknown-plugin", mutate: func(r *MigrationReportFile) {
			r.Entries = []MigrationReportEntry{reportEntryWith(base, func(e *MigrationReportEntry) { e.LogicalID = "ghost" })}
		}, contains: "does not carry"},
		{name: "unsorted", mutate: func(r *MigrationReportFile) {
			other := reportEntryWith(base, func(e *MigrationReportEntry) { e.LogicalID = "aaa" })
			r.Entries = []MigrationReportEntry{base, other}
		}, contains: "not sorted and unique"},
		{name: "hand-edited-recommendation", mutate: func(r *MigrationReportFile) {
			r.Entries = []MigrationReportEntry{reportEntryWith(base, func(e *MigrationReportEntry) { e.Recommendation = migrationRecommendBumpVersion })}
		}, contains: "recorded annotations require"},
		{name: "planned-digest-drift", mutate: func(r *MigrationReportFile) {
			r.Entries = []MigrationReportEntry{reportEntryWith(base, func(e *MigrationReportEntry) { e.PlannedDigest = legacy })}
		}, contains: "identical existing and planned digest"},
		{name: "reference-drift", mutate: func(r *MigrationReportFile) {
			r.Entries = []MigrationReportEntry{reportEntryWith(base, func(e *MigrationReportEntry) { e.OCIRef = "registry.example/plugins/demo:9.9.9" })}
		}, contains: "planned snapshot reference"},
		{name: "suspected-state", mutate: func(r *MigrationReportFile) {
			r.Entries = []MigrationReportEntry{reportEntryWith(base, func(e *MigrationReportEntry) { e.State = migrationStateSuspected; e.Recommendation = "" })}
		}, contains: "only blocked entries may be bound"},
		{name: "unsupported-recommendation", mutate: func(r *MigrationReportFile) {
			r.Entries = []MigrationReportEntry{reportEntryWith(base, func(e *MigrationReportEntry) { e.Recommendation = "delete-everything" })}
		}, contains: "unsupported recommendation"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mutated := report
			mutated.Entries = append([]MigrationReportEntry(nil), report.Entries...)
			tc.mutate(&mutated)
			path := filepath.Join(f.root, "mutated.json")
			if err := writeCanonical(path, mutated); err != nil {
				t.Fatal(err)
			}
			if _, err := applyMigrationReport(f.snapshot, path); err == nil || !strings.Contains(err.Error(), tc.contains) {
				t.Fatalf("inconsistent migration report was accepted: %v", err)
			}
		})
	}
}

func reportEntryWith(entry MigrationReportEntry, mutate func(*MigrationReportEntry)) MigrationReportEntry {
	out := entry
	mutate(&out)
	return out
}

func TestValidateSnapshotMigrationRequiresConsistentMarkers(t *testing.T) {
	f := newMigrationFixture(t)
	legacy := "sha256:" + strings.Repeat("1", 64)
	withManifestResolver(t, migrationResolver(t, map[string]ociManifest{
		f.controlRef: {Digest: f.controlDigest},
		f.demoRef:    {Digest: legacy},
	}))
	report, err := migrationPreflight(f.catalog, f.plan, f.previous, f.evidence)
	if err != nil {
		t.Fatal(err)
	}
	reportPath := filepath.Join(f.root, "report.json")
	if err := writeCanonical(reportPath, report); err != nil {
		t.Fatal(err)
	}
	snapshot, err := applyMigrationReport(f.snapshot, reportPath)
	if err != nil {
		t.Fatal(err)
	}
	mutate := func(fn func(*Snapshot)) string {
		changed := snapshot
		changed.Plugins = append([]SnapshotEntry(nil), snapshot.Plugins...)
		for i := range changed.Plugins {
			if changed.Plugins[i].Migration != nil {
				migration := *changed.Plugins[i].Migration
				changed.Plugins[i].Migration = &migration
			}
		}
		if changed.MigrationPreflight != nil {
			preflight := *changed.MigrationPreflight
			changed.MigrationPreflight = &preflight
		}
		fn(&changed)
		path := filepath.Join(f.root, "mutated-snapshot.json")
		if err := writeCanonical(path, changed); err != nil {
			t.Fatal(err)
		}
		return path
	}
	for _, tc := range []struct {
		name     string
		mutate   func(*Snapshot)
		contains string
	}{
		{name: "dropped-preflight-marker", mutate: func(s *Snapshot) { s.MigrationPreflight = nil }, contains: "carries no migrationPreflight marker"},
		{name: "dropped-entry-marker", mutate: func(s *Snapshot) { s.Plugins[0].Migration = nil }, contains: "excludes no plugin"},
		{name: "excluded-list-drift", mutate: func(s *Snapshot) { s.MigrationPreflight.Excluded = []string{"demo", "other"} }, contains: "but the marked entries are"},
		{name: "planned-digest-drift", mutate: func(s *Snapshot) { s.Plugins[0].Migration.PlannedDigest = legacy }, contains: "identical existing and planned digest"},
		{name: "own-digest-drift", mutate: func(s *Snapshot) { s.Plugins[0].Migration.PlannedDigest = "sha256:" + strings.Repeat("3", 64) }, contains: "its own snapshot digest"},
		{name: "hand-edited-disposition", mutate: func(s *Snapshot) { s.Plugins[0].Migration.Recommendation = migrationRecommendAdoptPublic }, contains: "not re-derivable"},
		{name: "unsupported-state", mutate: func(s *Snapshot) { s.Plugins[0].Migration.State = "pending" }, contains: "unsupported migration state"},
		{name: "dropped-control", mutate: func(s *Snapshot) { s.MigrationPreflight.ControlRef = "" }, contains: "calibrated control tag"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := mutate(tc.mutate)
			var changed Snapshot
			if _, err := readJSON(path, &changed); err != nil {
				t.Fatal(err)
			}
			if err := validateSnapshotMigration(changed); err == nil || !strings.Contains(err.Error(), tc.contains) {
				t.Fatalf("inconsistent migration binding was accepted: %v", err)
			}
		})
	}
	if err := validateSnapshotMigration(f.snapshot); err != nil {
		t.Fatalf("all-green snapshot failed migration validation: %v", err)
	}
}

// TestVerifySnapshotToleratesMigrationExclusion proves the exclusion survives
// verification: the known-divergent public tag is never resolved, the candidate
// for the same entry still is, and dropping either half of the marker fails
// closed.
func TestVerifySnapshotToleratesMigrationExclusion(t *testing.T) {
	root := t.TempDir()
	mustRun(t, root, "git", "init", "-q")
	mustRun(t, root, "git", "config", "user.name", "test")
	mustRun(t, root, "git", "config", "user.email", "test@example.com")
	mustWrite(t, filepath.Join(root, "plugins/wasm-go/extensions/demo/main.go"), "package main\n")
	mustWrite(t, filepath.Join(root, "plugins/wasm-go/extensions/demo/VERSION"), "1.0.0\n")
	mustWrite(t, filepath.Join(root, "plugins/wasm-rust/extensions/.keep"), "")
	mustRun(t, root, "git", "add", ".")
	mustRun(t, root, "git", "commit", "-q", "-m", "source")
	commit, _ := resolveCommit(root, "HEAD")
	c := Catalog{SchemaVersion: 1, Registry: "registry.example", Plugins: []Plugin{{LogicalID: "demo", Implementation: "go", SourceDir: "plugins/wasm-go/extensions/demo", Image: "plugins/demo", ReleaseEligible: true, ArtifactInputs: []string{"plugins/wasm-go/extensions/demo/**"}}}}
	catalogPath := filepath.Join(root, "catalog.json")
	if err := writeCanonical(catalogPath, c); err != nil {
		t.Fatal(err)
	}
	hash, err := inputHash(root, commit, "1.0.0", c, c.Plugins[0])
	if err != nil {
		t.Fatal(err)
	}
	planned := "sha256:" + strings.Repeat("d", 64)
	legacy := "sha256:" + strings.Repeat("1", 64)
	publicRef := "registry.example/plugins/demo:1.0.0"
	snapshot := Snapshot{SchemaVersion: snapshotSchemaVersion, GatewayVersion: "2.0.0", SourceCommit: commit,
		CatalogSHA256: sha256Hex(mustRead(t, catalogPath)), PlanID: "sha256:" + strings.Repeat("0", 64), ProvenanceMode: "candidate",
		MigrationPreflight: &SnapshotMigrationPreflight{ControlRef: "registry.example/plugins/carried:1.0.0", ControlDigest: legacy, Excluded: []string{"demo"}},
		Plugins: []SnapshotEntry{{LogicalID: "demo", Implementation: "go", SourceDir: c.Plugins[0].SourceDir, Image: c.Plugins[0].Image,
			Version: "1.0.0", OCIRef: publicRef, Digest: planned, InputHash: hash, SourceCommit: commit,
			CandidateRef: "registry.example/candidates/demo@" + planned, ProvenanceMode: "candidate",
			Migration: &SnapshotMigration{State: migrationStateBlocked, SourceCommit: commit, ExistingDigest: legacy, PlannedDigest: planned, SourceComparison: migrationSourceUnannotated, Recommendation: migrationRecommendDeleteLegacy}}}}
	snapshotPath := filepath.Join(root, "snapshot.json")
	if err := writeCanonical(snapshotPath, snapshot); err != nil {
		t.Fatal(err)
	}
	candidateRef := snapshot.Plugins[0].CandidateRef
	withManifestResolver(t, func(ref string) (ociManifest, error) {
		if ref == publicRef {
			t.Fatalf("verification resolved the known-divergent public tag of an excluded plugin: %s", ref)
		}
		if ref != candidateRef {
			t.Fatalf("verification resolved an unexpected reference: %s", ref)
		}
		return ociManifest{Digest: planned, Annotations: map[string]string{"org.opencontainers.image.revision": commit, "io.higress.plugin.input-hash": hash, "org.opencontainers.image.version": "1.0.0"}}, nil
	})
	if err := verifySnapshot(root, catalogPath, snapshotPath, commit, commit, true, "candidate"); err != nil {
		t.Fatalf("candidate verification rejected a migration exclusion: %v", err)
	}
	withManifestResolver(t, func(ref string) (ociManifest, error) {
		if ref == publicRef {
			t.Fatalf("public verification resolved the excluded plugin's divergent tag: %s", ref)
		}
		t.Fatalf("public verification resolved an unexpected reference: %s", ref)
		return ociManifest{}, nil
	})
	if err := verifySnapshot(root, catalogPath, snapshotPath, commit, commit, true, "public"); err != nil {
		t.Fatalf("public verification did not tolerate the recorded exclusion: %v", err)
	}
	for _, tc := range []struct {
		name     string
		mutate   func(*Snapshot)
		contains string
	}{
		{name: "dropped-marker", mutate: func(s *Snapshot) { s.MigrationPreflight = nil }, contains: "carries no migrationPreflight marker"},
		{name: "dropped-exclusion", mutate: func(s *Snapshot) { s.Plugins[0].Migration = nil }, contains: "excludes no plugin"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			changed := snapshot
			changed.Plugins = append([]SnapshotEntry(nil), snapshot.Plugins...)
			migration := *snapshot.Plugins[0].Migration
			changed.Plugins[0].Migration = &migration
			preflight := *snapshot.MigrationPreflight
			changed.MigrationPreflight = &preflight
			tc.mutate(&changed)
			path := filepath.Join(root, "tampered.json")
			if err := writeCanonical(path, changed); err != nil {
				t.Fatal(err)
			}
			if err := verifySnapshot(root, catalogPath, path, commit, commit, false, "candidate"); err == nil || !strings.Contains(err.Error(), tc.contains) {
				t.Fatalf("verification accepted an inconsistent migration binding: %v", err)
			}
		})
	}
}
