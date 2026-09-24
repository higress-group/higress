// Copyright 2026 Higress Authors
// Licensed under the Apache License, Version 2.0.

package main

import (
	"errors"
	"fmt"
	"reflect"
	"sort"
	"strings"
)

// migrationPreflight sweeps the public registry for every planned plugin tag
// before the preparation PR is created (SPEC-4634004). A planned tag that is
// already occupied by a different artifact becomes a migration-mode entry with
// a recommended disposition, so promote skips that one plugin instead of
// hard-failing the whole batch on an immutable tag conflict.
//
// The probe method is calibrated first: a known-good control tag from the
// reviewed previous snapshot must resolve to exactly its recorded digest. If it
// does not, no present/absent classification from this sweep is trustworthy and
// the preflight fails closed rather than reporting a clean registry.
//
// candidateEvidencePath may be empty for a dry-run rehearsal. Without candidate
// digests there is nothing to compare, so an occupied planned tag is reported as
// suspected and can never block a plugin or enter a snapshot.
func migrationPreflight(catalogPath, planPath, previousPath, candidateEvidencePath string) (MigrationReportFile, error) {
	c, catalogData, err := loadCatalog(catalogPath)
	if err != nil {
		return MigrationReportFile{}, err
	}
	var plan Plan
	if _, err := readJSON(planPath, &plan); err != nil {
		return MigrationReportFile{}, err
	}
	if err := validatePlanProvenance(plan, catalogData); err != nil {
		return MigrationReportFile{}, err
	}
	controlRef, controlDigest, err := migrationControlTag(previousPath)
	if err != nil {
		return MigrationReportFile{}, err
	}
	control, err := ociManifestResolver(controlRef)
	if err != nil {
		switch classifyOCIFailure(err, controlRef) {
		case ociFailureUnauthorized:
			return MigrationReportFile{}, fmt.Errorf("migration preflight control tag %s: registry authorization failed; authorization failure is never absence: %w", controlRef, err)
		default:
			return MigrationReportFile{}, fmt.Errorf("migration preflight cannot calibrate its probe against control tag %s: %w", controlRef, err)
		}
	}
	if control.Digest != controlDigest {
		return MigrationReportFile{}, fmt.Errorf("migration preflight probe is not trustworthy: control tag %s resolved to %s, expected the reviewed %s", controlRef, control.Digest, controlDigest)
	}
	probeMode := migrationProbeExistenceOnly
	var evidence CandidateEvidenceFile
	if candidateEvidencePath != "" {
		if _, err := readJSON(candidateEvidencePath, &evidence); err != nil {
			return MigrationReportFile{}, err
		}
		probeMode = migrationProbeCandidateDigest
	}
	report := MigrationReportFile{
		SchemaVersion:  migrationReportSchemaVersion,
		GatewayVersion: plan.GatewayVersion,
		SourceCommit:   plan.SourceCommit,
		PlanID:         plan.PlanID,
		Registry:       c.Registry,
		ProbeMode:      probeMode,
		ControlRef:     controlRef,
		ControlDigest:  controlDigest,
		Entries:        []MigrationReportEntry{},
	}
	for _, entry := range plan.Plugins {
		ociRef := c.Registry + "/" + entry.Image + ":" + entry.Version
		planned := ""
		if probeMode == migrationProbeCandidateDigest {
			proof, ok := evidence.Plugins[entry.LogicalID]
			if !ok || !digestPattern.MatchString(proof.Digest) || proof.InputHash != entry.InputHash {
				return MigrationReportFile{}, fmt.Errorf("migration preflight needs candidate evidence for every planned plugin; %s has no matching candidate digest", entry.LogicalID)
			}
			planned = proof.Digest
		}
		manifest, err := ociManifestResolver(ociRef)
		if err != nil {
			switch classifyOCIFailure(err, ociRef) {
			case ociFailureNotFound:
				continue
			case ociFailureUnauthorized:
				return MigrationReportFile{}, fmt.Errorf("migration preflight %s: registry authorization failed; authorization failure is never absence: %w", entry.LogicalID, err)
			default:
				return MigrationReportFile{}, fmt.Errorf("migration preflight cannot classify the planned public tag %s: %w", ociRef, err)
			}
		}
		if !digestPattern.MatchString(manifest.Digest) {
			return MigrationReportFile{}, fmt.Errorf("migration preflight %s returned invalid digest %q", entry.LogicalID, manifest.Digest)
		}
		if planned != "" && manifest.Digest == planned {
			continue
		}
		report.Entries = append(report.Entries, MigrationReportEntry{
			LogicalID:         entry.LogicalID,
			Version:           entry.Version,
			OCIRef:            ociRef,
			ExistingDigest:    manifest.Digest,
			PlannedDigest:     planned,
			InputHash:         entry.InputHash,
			ExistingVersion:   manifest.Annotations["org.opencontainers.image.version"],
			ExistingRevision:  manifest.Annotations["org.opencontainers.image.revision"],
			ExistingInputHash: manifest.Annotations["io.higress.plugin.input-hash"],
			SourceComparison:  migrationSourceComparison(manifest, plan.SourceCommit),
			State:             migrationEntryState(planned),
			Recommendation:    migrationEntryRecommendation(manifest, entry, plan.SourceCommit, planned),
		})
	}
	return report, nil
}

func migrationEntryState(plannedDigest string) string {
	if plannedDigest == "" {
		return migrationStateSuspected
	}
	return migrationStateBlocked
}

func migrationEntryRecommendation(manifest ociManifest, planned PlanEntry, sourceCommit, plannedDigest string) string {
	if plannedDigest == "" {
		return ""
	}
	return migrationRecommendation(manifest, planned, sourceCommit)
}

// migrationControlTag selects the known-good public artifact the probe is
// validated against: a pipeline-published tag of the previous snapshot that this
// release's batch is not excluding, preferring an entry this pipeline built over
// one adopted from a pre-existing public artifact. Those tags are immutable
// published artifacts, so a probe that cannot reproduce the reviewed digest is
// measuring the registry incorrectly.
//
// An entry carrying a migration exclusion is never eligible: its public tag
// still serves the legacy artifact that caused the exclusion, not the digest its
// snapshot records, so calibrating against it would fail every later prepare.
func migrationControlTag(previousPath string) (string, string, error) {
	if previousPath == "" {
		return "", "", errors.New("migration preflight requires the reviewed previous snapshot to calibrate its registry probe")
	}
	var previous Snapshot
	if _, err := readJSON(previousPath, &previous); err != nil {
		return "", "", err
	}
	adoptedRef, adoptedDigest := "", ""
	for _, entry := range previous.Plugins {
		if entry.Migration != nil {
			continue
		}
		if entry.OCIRef == "" || !digestPattern.MatchString(entry.Digest) {
			continue
		}
		switch entryProvenance(entry) {
		case "candidate":
			return entry.OCIRef, entry.Digest, nil
		case "public":
			if adoptedRef == "" {
				adoptedRef, adoptedDigest = entry.OCIRef, entry.Digest
			}
		}
	}
	if adoptedRef != "" {
		return adoptedRef, adoptedDigest, nil
	}
	return "", "", errors.New("previous snapshot carries no published, non-excluded public control tag; migration preflight cannot calibrate its probe")
}

// validatePlanProvenance re-checks the immutable plan identity the sweep binds
// its report to, so a report can never be attached to a plan that was edited
// after it was hashed.
func validatePlanProvenance(plan Plan, catalogData []byte) error {
	if plan.SchemaVersion != planSchemaVersion || !commitPattern.MatchString(plan.SourceCommit) || !digestPattern.MatchString(plan.PlanID) {
		return errors.New("plan has an unsupported schema or invalid immutable provenance")
	}
	if plan.CatalogSHA256 != sha256Hex(catalogData) {
		return errors.New("plan catalogSha256 does not match catalog")
	}
	if want := "sha256:" + canonicalObjectHash(plan, true); plan.PlanID != want {
		return fmt.Errorf("planId does not match canonical plan content: got %s, want %s", plan.PlanID, want)
	}
	gatewayVersion, err := parseSemver(plan.GatewayVersion)
	if err != nil || gatewayVersion.prerelease != "" {
		return errors.New("plan gatewayVersion must be stable SemVer")
	}
	return nil
}

func migrationSourceComparison(manifest ociManifest, sourceCommit string) string {
	revision := manifest.Annotations["org.opencontainers.image.revision"]
	if revision == "" &&
		manifest.Annotations["io.higress.plugin.input-hash"] == "" &&
		manifest.Annotations["org.opencontainers.image.version"] == "" {
		return migrationSourceUnannotated
	}
	if revision == sourceCommit {
		return migrationSourceMatch
	}
	return migrationSourceMismatch
}

// migrationRecommendation is a deterministic, mutually exclusive disposition.
// Deleting a public tag is recommended only for an artifact carrying no
// pipeline annotation at all, which is how a pre-pipeline manual artifact
// presents itself. Anything the pipeline published but that cannot be matched
// to this plan must be resolved by bumping the planned version: an unproven
// artifact is never deleted automatically.
func migrationRecommendation(manifest ociManifest, planned PlanEntry, sourceCommit string) string {
	version := manifest.Annotations["org.opencontainers.image.version"]
	revision := manifest.Annotations["org.opencontainers.image.revision"]
	inputHash := manifest.Annotations["io.higress.plugin.input-hash"]
	if version == "" && revision == "" && inputHash == "" {
		return migrationRecommendDeleteLegacy
	}
	if inputHash == planned.InputHash && revision == sourceCommit && version == planned.Version {
		return migrationRecommendAdoptPublic
	}
	return migrationRecommendBumpVersion
}

// migrationRecommendationFromRecord re-derives the disposition from the fields
// recorded in a report or snapshot marker. Binding and verification both use it,
// so a hand-edited recommendation can never survive validation.
func migrationRecommendationFromRecord(existingVersion, existingRevision, existingInputHash, plannedInputHash, plannedVersion, sourceCommit string) string {
	manifest := ociManifest{Annotations: map[string]string{
		"org.opencontainers.image.version":  existingVersion,
		"org.opencontainers.image.revision": existingRevision,
		"io.higress.plugin.input-hash":      existingInputHash,
	}}
	return migrationRecommendation(manifest, PlanEntry{InputHash: plannedInputHash, Version: plannedVersion}, sourceCommit)
}

func migrationSourceComparisonFromRecord(existingVersion, existingRevision, existingInputHash, sourceCommit string) string {
	manifest := ociManifest{Annotations: map[string]string{
		"org.opencontainers.image.version":  existingVersion,
		"org.opencontainers.image.revision": existingRevision,
		"io.higress.plugin.input-hash":      existingInputHash,
	}}
	return migrationSourceComparison(manifest, sourceCommit)
}

// applyMigrationReport binds a definitive sweep into a rendered snapshot. A
// snapshot that excludes nothing — neither newly blocked by this sweep nor
// carried from an earlier release — is left byte-identical to the no-conflict
// path.
func applyMigrationReport(snapshot Snapshot, reportPath string) (Snapshot, error) {
	var report MigrationReportFile
	if _, err := readJSON(reportPath, &report); err != nil {
		return Snapshot{}, err
	}
	if err := validateMigrationReport(report, snapshot); err != nil {
		return Snapshot{}, err
	}
	// renderSnapshot carries an earlier release's exclusion forward verbatim, so
	// the marker that binds those entries has to be rebuilt here even when this
	// sweep found nothing: dropping it would make an unrelated later release fail
	// verification wholesale instead of excluding only the still-blocked plugin.
	carried := make([]string, 0)
	for _, entry := range snapshot.Plugins {
		if entry.Migration == nil {
			continue
		}
		if entry.Migration.State != migrationStateBlocked {
			return Snapshot{}, fmt.Errorf("%s carries migration state %q from the previous snapshot; only %q exclusions are carried forward", entry.LogicalID, entry.Migration.State, migrationStateBlocked)
		}
		carried = append(carried, entry.LogicalID)
	}
	if len(report.Entries) == 0 && len(carried) == 0 {
		return snapshot, nil
	}
	// Copy on write: the caller's snapshot must not change through a shared
	// backing array when markers are attached.
	snapshot.Plugins = append([]SnapshotEntry(nil), snapshot.Plugins...)
	excluded := append([]string(nil), carried...)
	if len(report.Entries) > 0 {
		entries := make(map[string]MigrationReportEntry, len(report.Entries))
		for _, entry := range report.Entries {
			entries[entry.LogicalID] = entry
		}
		for i := range snapshot.Plugins {
			reported, ok := entries[snapshot.Plugins[i].LogicalID]
			if !ok {
				continue
			}
			if snapshot.Plugins[i].Migration != nil {
				return Snapshot{}, fmt.Errorf("%s is carried as an existing migration exclusion and cannot be blocked again by this sweep; re-plan it after its disposition completes", reported.LogicalID)
			}
			snapshot.Plugins[i].Migration = &SnapshotMigration{
				State:             reported.State,
				SourceCommit:      report.SourceCommit,
				ExistingDigest:    reported.ExistingDigest,
				PlannedDigest:     reported.PlannedDigest,
				ExistingVersion:   reported.ExistingVersion,
				ExistingRevision:  reported.ExistingRevision,
				ExistingInputHash: reported.ExistingInputHash,
				SourceComparison:  reported.SourceComparison,
				Recommendation:    reported.Recommendation,
			}
			excluded = append(excluded, reported.LogicalID)
			delete(entries, reported.LogicalID)
		}
		if len(entries) > 0 {
			missing := make([]string, 0, len(entries))
			for id := range entries {
				missing = append(missing, id)
			}
			sort.Strings(missing)
			return Snapshot{}, fmt.Errorf("migration report names %s, which the snapshot does not carry", strings.Join(missing, ", "))
		}
	}
	sort.Strings(excluded)
	snapshot.MigrationPreflight = &SnapshotMigrationPreflight{
		ControlRef:    report.ControlRef,
		ControlDigest: report.ControlDigest,
		Excluded:      excluded,
	}
	if err := validateSnapshotMigration(snapshot); err != nil {
		return Snapshot{}, err
	}
	return snapshot, nil
}

// requireNoCarriedMigration keeps `render-snapshot` from silently writing a
// snapshot that carries earlier exclusions but no sweep to rebind their marker.
func requireNoCarriedMigration(snapshot Snapshot) error {
	carried := make([]string, 0)
	for _, entry := range snapshot.Plugins {
		if entry.Migration != nil {
			carried = append(carried, entry.LogicalID)
		}
	}
	if len(carried) == 0 {
		return nil
	}
	return fmt.Errorf("the previous snapshot carries migration exclusions for %s, so --migration-report is required to rebind them", strings.Join(carried, ", "))
}

// validateMigrationReport binds the sweep to the exact snapshot it qualifies.
// Only a definitive candidate-digest comparison may block a plugin, and every
// recorded disposition must be re-derivable from the recorded annotations.
func validateMigrationReport(report MigrationReportFile, snapshot Snapshot) error {
	if report.SchemaVersion != migrationReportSchemaVersion {
		return fmt.Errorf("migration report schemaVersion must be %d", migrationReportSchemaVersion)
	}
	if report.ProbeMode != migrationProbeCandidateDigest {
		return fmt.Errorf("a snapshot binds only a %s migration sweep, got %q", migrationProbeCandidateDigest, report.ProbeMode)
	}
	if report.GatewayVersion != snapshot.GatewayVersion || report.SourceCommit != snapshot.SourceCommit || report.PlanID != snapshot.PlanID {
		return errors.New("migration report and snapshot immutable provenance differ")
	}
	if report.ControlRef == "" || !digestPattern.MatchString(report.ControlDigest) {
		return errors.New("migration report lacks a calibrated control tag")
	}
	if report.Registry == "" || strings.Contains(report.Registry, "://") {
		return errors.New("migration report registry must be a host without a URL scheme")
	}
	snapshotEntries := make(map[string]SnapshotEntry, len(snapshot.Plugins))
	for _, entry := range snapshot.Plugins {
		snapshotEntries[entry.LogicalID] = entry
	}
	last := ""
	for _, entry := range report.Entries {
		if last != "" && entry.LogicalID <= last {
			return fmt.Errorf("migration report entries are not sorted and unique by logicalId at %q", entry.LogicalID)
		}
		last = entry.LogicalID
		if entry.State != migrationStateBlocked {
			return fmt.Errorf("migration report entry %s has state %q; only %s entries may be bound into a snapshot", entry.LogicalID, entry.State, migrationStateBlocked)
		}
		if err := validateMigrationFields(entry.LogicalID, entry.ExistingDigest, entry.PlannedDigest, entry.SourceComparison, entry.Recommendation); err != nil {
			return err
		}
		snapshotEntry, ok := snapshotEntries[entry.LogicalID]
		if !ok {
			return fmt.Errorf("migration report names %s, which the snapshot does not carry", entry.LogicalID)
		}
		if entry.OCIRef != snapshotEntry.OCIRef || entry.OCIRef != report.Registry+"/"+snapshotEntry.Image+":"+entry.Version {
			return fmt.Errorf("migration report reference for %s does not match its planned snapshot reference", entry.LogicalID)
		}
		if entry.Version != snapshotEntry.Version || entry.InputHash != snapshotEntry.InputHash || entry.PlannedDigest != snapshotEntry.Digest {
			return fmt.Errorf("migration report for %s does not match the planned snapshot digest and inputs", entry.LogicalID)
		}
		if entry.ExistingDigest == entry.PlannedDigest {
			return fmt.Errorf("migration report blocks %s although the public tag already serves the planned digest", entry.LogicalID)
		}
		if want := migrationRecommendationFromRecord(entry.ExistingVersion, entry.ExistingRevision, entry.ExistingInputHash, entry.InputHash, entry.Version, report.SourceCommit); entry.Recommendation != want {
			return fmt.Errorf("migration report recommendation for %s is %q, but its recorded annotations require %q", entry.LogicalID, entry.Recommendation, want)
		}
		if want := migrationSourceComparisonFromRecord(entry.ExistingVersion, entry.ExistingRevision, entry.ExistingInputHash, report.SourceCommit); entry.SourceComparison != want {
			return fmt.Errorf("migration report source comparison for %s is %q, but its recorded annotations require %q", entry.LogicalID, entry.SourceComparison, want)
		}
	}
	return nil
}

func validateMigrationFields(logicalID, existingDigest, plannedDigest, sourceComparison, recommendation string) error {
	if !digestPattern.MatchString(existingDigest) || !digestPattern.MatchString(plannedDigest) {
		return fmt.Errorf("%s migration state must record the existing and planned digests", logicalID)
	}
	if existingDigest == plannedDigest {
		return fmt.Errorf("%s migration state records an identical existing and planned digest", logicalID)
	}
	switch sourceComparison {
	case migrationSourceMatch, migrationSourceMismatch, migrationSourceUnannotated:
	default:
		return fmt.Errorf("%s migration state has unsupported source comparison %q", logicalID, sourceComparison)
	}
	switch recommendation {
	case migrationRecommendDeleteLegacy, migrationRecommendBumpVersion, migrationRecommendAdoptPublic:
	default:
		return fmt.Errorf("%s migration state has unsupported recommendation %q", logicalID, recommendation)
	}
	return nil
}

// validateSnapshotMigration checks the snapshot-internal migration binding: the
// markers and the preflight marker must exist together, the excluded set must be
// exactly the marked entries, and every recorded disposition must still be
// re-derivable from the recorded annotations.
func validateSnapshotMigration(snapshot Snapshot) error {
	excluded := make([]string, 0)
	for _, entry := range snapshot.Plugins {
		if entry.Migration == nil {
			continue
		}
		migration := *entry.Migration
		if migration.State != migrationStateBlocked {
			return fmt.Errorf("%s has unsupported migration state %q", entry.LogicalID, migration.State)
		}
		if err := validateMigrationFields(entry.LogicalID, migration.ExistingDigest, migration.PlannedDigest, migration.SourceComparison, migration.Recommendation); err != nil {
			return err
		}
		if migration.PlannedDigest != entry.Digest {
			return fmt.Errorf("%s migration state does not record its own snapshot digest as the planned digest", entry.LogicalID)
		}
		if !commitPattern.MatchString(migration.SourceCommit) {
			return fmt.Errorf("%s migration state does not record the source commit of the sweep that decided it", entry.LogicalID)
		}
		// The disposition is re-derived against the commit of the sweep that
		// decided it, which for a carried exclusion belongs to an earlier
		// release. Deriving it against this snapshot's source commit instead
		// would invalidate every carried exclusion.
		if want := migrationRecommendationFromRecord(migration.ExistingVersion, migration.ExistingRevision, migration.ExistingInputHash, entry.InputHash, entry.Version, migration.SourceCommit); migration.Recommendation != want {
			return fmt.Errorf("%s migration recommendation %q is not re-derivable from its recorded annotations, want %q", entry.LogicalID, migration.Recommendation, want)
		}
		if want := migrationSourceComparisonFromRecord(migration.ExistingVersion, migration.ExistingRevision, migration.ExistingInputHash, migration.SourceCommit); migration.SourceComparison != want {
			return fmt.Errorf("%s migration source comparison %q is not re-derivable from its recorded annotations, want %q", entry.LogicalID, migration.SourceComparison, want)
		}
		excluded = append(excluded, entry.LogicalID)
	}
	sort.Strings(excluded)
	if snapshot.MigrationPreflight == nil {
		if len(excluded) > 0 {
			return fmt.Errorf("snapshot excludes %s by migration state but carries no migrationPreflight marker", strings.Join(excluded, ", "))
		}
		return nil
	}
	preflight := *snapshot.MigrationPreflight
	if len(excluded) == 0 {
		return errors.New("snapshot carries a migrationPreflight marker but excludes no plugin")
	}
	if preflight.ControlRef == "" || !digestPattern.MatchString(preflight.ControlDigest) {
		return errors.New("snapshot migrationPreflight marker lacks a calibrated control tag")
	}
	if !reflect.DeepEqual(preflight.Excluded, excluded) {
		return fmt.Errorf("snapshot migrationPreflight excludes %v, but the marked entries are %v", preflight.Excluded, excluded)
	}
	return nil
}

// renderMigrationMarkdown renders the reviewer-facing section embedded in the
// preparation PR body. It is deterministic: no timestamps, no run identifiers.
func renderMigrationMarkdown(report MigrationReportFile) string {
	var out strings.Builder
	fmt.Fprintf(&out, "## Plugin migration preflight\n\n")
	fmt.Fprintf(&out, "Read-only public registry sweep of every planned version tag, calibrated against control tag `%s` = `%s` (probe mode `%s`).\n\n", report.ControlRef, report.ControlDigest, report.ProbeMode)
	blocked := make([]MigrationReportEntry, 0, len(report.Entries))
	for _, entry := range report.Entries {
		if entry.State == migrationStateBlocked {
			blocked = append(blocked, entry)
		}
	}
	if len(report.Entries) == 0 {
		out.WriteString("No planned version tag is occupied by a different artifact: the promote batch includes every planned plugin.\n")
		return out.String()
	}
	if len(blocked) == 0 {
		out.WriteString("Dry-run existence sweep: the following planned version tags already exist publicly. Their digests cannot be compared before candidates are built, so no plugin is excluded yet; the non-dry-run sweep classifies each one.\n\n")
	} else {
		fmt.Fprintf(&out, "%d planned plugin(s) are in migration mode and are **excluded from this promote batch** (`migration.state=blocked`) until their disposition completes. Every other planned plugin promotes normally.\n\n", len(blocked))
	}
	out.WriteString("| Plugin | Planned tag | Existing digest | Planned digest | Source | Recommendation |\n")
	out.WriteString("| --- | --- | --- | --- | --- | --- |\n")
	for _, entry := range report.Entries {
		planned := entry.PlannedDigest
		if planned == "" {
			planned = "_(no candidate digest in dry run)_"
		}
		recommendation := entry.Recommendation
		if recommendation == "" {
			recommendation = "_(classified by the non-dry-run sweep)_"
		}
		fmt.Fprintf(&out, "| `%s` | `%s` | `%s` | `%s` | %s | %s |\n", entry.LogicalID, entry.OCIRef, entry.ExistingDigest, planned, entry.SourceComparison, recommendation)
	}
	for _, entry := range blocked {
		fmt.Fprintf(&out, "\n### %s\n\n", entry.LogicalID)
		fmt.Fprintf(&out, "- existing digest: `%s`\n", entry.ExistingDigest)
		fmt.Fprintf(&out, "- planned digest: `%s`\n", entry.PlannedDigest)
		fmt.Fprintf(&out, "- planned input hash: `%s`\n", entry.InputHash)
		fmt.Fprintf(&out, "- existing annotations: version=%s revision=%s input-hash=%s\n",
			migrationMarkdownValue(entry.ExistingVersion), migrationMarkdownValue(entry.ExistingRevision), migrationMarkdownValue(entry.ExistingInputHash))
		fmt.Fprintf(&out, "- source comparison: %s\n", entry.SourceComparison)
		fmt.Fprintf(&out, "- recommendation: **%s** — %s\n", entry.Recommendation, migrationRemediation(entry))
	}
	return out.String()
}

func migrationMarkdownValue(value string) string {
	if value == "" {
		return "_(none)_"
	}
	return "`" + value + "`"
}

func migrationRemediation(entry MigrationReportEntry) string {
	switch entry.Recommendation {
	case migrationRecommendDeleteLegacy:
		return fmt.Sprintf("the public tag carries no pipeline annotation, which is how a pre-pipeline manual artifact presents itself. Missing annotations are evidence, not proof of provenance: a maintainer must first confirm by hand that nothing still depends on the artifact behind `%s`. Only after that confirmation may a registry administrator delete the tag, and the next `prepare-plugin-release` run then plans and promotes this plugin normally.", entry.OCIRef)
	case migrationRecommendBumpVersion:
		return fmt.Sprintf("a different managed build already occupies this exact version. Re-run prepare with `version_overrides` setting `%s` to a reviewed stable version greater than `%s`.", entry.LogicalID, entry.Version)
	case migrationRecommendAdoptPublic:
		return fmt.Sprintf("the public artifact was built from the same source commit and input hash as this plan, so the published tag already serves these inputs. Leave it published; the next release re-plans `%s` from its input hash.", entry.LogicalID)
	default:
		return "unclassified conflict; a maintainer must review the existing artifact before this plugin promotes."
	}
}

func commandMigrationPreflight(args []string) error {
	fs, _, catalog := parseCommon("migration-preflight", args)
	plan := fs.String("plan", "", "plan path")
	previous := fs.String("previous", "", "reviewed previous snapshot supplying the control tag")
	candidateEvidence := fs.String("candidate-evidence", "", "candidate evidence path (omit for a dry-run existence sweep)")
	output := fs.String("output", "-", "canonical migration report output")
	markdown := fs.String("markdown", "", "reviewer-facing Markdown report output")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *plan == "" {
		return errors.New("--plan is required")
	}
	report, err := migrationPreflight(*catalog, *plan, *previous, *candidateEvidence)
	if err != nil {
		return err
	}
	if *markdown != "" {
		if err := writeText(*markdown, renderMigrationMarkdown(report)); err != nil {
			return err
		}
	}
	return writeCanonical(*output, report)
}
