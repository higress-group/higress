// Copyright 2026 Higress Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import "encoding/json"

const (
	catalogSchemaVersion  = 1
	planSchemaVersion     = 1
	snapshotSchemaVersion = 1

	migrationReportSchemaVersion = 1
)

// Migration preflight vocabulary. A planned public version tag that is already
// occupied by a different artifact is classified at prepare time instead of
// hard-failing the promote batch later.
const (
	// migrationProbeCandidateDigest compares the public tag against the built
	// candidate digest and can block a plugin. migrationProbeExistenceOnly is
	// the dry-run rehearsal: it reports occupied planned tags without a
	// candidate digest to compare, so it can never block or enter a snapshot.
	migrationProbeCandidateDigest = "candidate-digest"
	migrationProbeExistenceOnly   = "existence-only"

	migrationStateBlocked   = "blocked"
	migrationStateSuspected = "suspected"

	migrationRecommendDeleteLegacy = "delete-legacy"
	migrationRecommendBumpVersion  = "bump-version"
	migrationRecommendAdoptPublic  = "adopt-public"

	migrationSourceMatch       = "match"
	migrationSourceMismatch    = "mismatch"
	migrationSourceUnannotated = "unannotated"
)

type Catalog struct {
	SchemaVersion       int                            `json:"schemaVersion"`
	Registry            string                         `json:"registry"`
	ConsoleMarketplace  *ConsoleMarketplacePolicy      `json:"consoleMarketplace,omitempty"`
	SharedInputGroups   map[string][]string            `json:"sharedInputGroups"`
	ConsumerInventories map[string][]ConsumerInventory `json:"consumerInventories"`
	Plugins             []Plugin                       `json:"plugins"`
}

type ConsumerInventory struct {
	Key            string `json:"key"`
	Classification string `json:"classification"`
	LogicalID      string `json:"logicalId,omitempty"`
	Reason         string `json:"reason,omitempty"`
}

type Plugin struct {
	LogicalID         string          `json:"logicalId"`
	Implementation    string          `json:"implementation"`
	SourceDir         string          `json:"sourceDir"`
	Image             string          `json:"image"`
	ReleaseEligible   bool            `json:"releaseEligible"`
	UnmanagedReason   string          `json:"unmanagedReason,omitempty"`
	ArtifactInputs    []string        `json:"artifactInputs"`
	SharedInputGroups []string        `json:"sharedInputGroups,omitempty"`
	Consumers         PluginConsumers `json:"consumers,omitempty"`
}

type PluginConsumers struct {
	Console      *ConsoleConsumer      `json:"console,omitempty"`
	PluginServer *PluginServerConsumer `json:"pluginServer,omitempty"`
}

type ConsoleConsumer struct {
	PropertyKey string                    `json:"propertyKey"`
	ResourceDir string                    `json:"resourceDir"`
	URLForm     string                    `json:"urlForm"`
	Marketplace *ConsoleMarketplaceBundle `json:"marketplace,omitempty"`
}

// ConsoleMarketplacePolicy opts a catalog into the strict marketplace
// coverage contract without making older schemaVersion=1 fixture catalogs
// invalid. Production catalogs set RequiredForStable=true.
type ConsoleMarketplacePolicy struct {
	RequiredForStable bool                                `json:"requiredForStable"`
	Bundles           map[string]ConsoleMarketplaceBundle `json:"bundles"`
}

// ConsoleMarketplaceBundle identifies reviewed Console classpath resources.
// Higress sources are bound to the exact dispatch commit; other repositories
// must name an immutable full source commit in the mapping itself.
type ConsoleMarketplaceBundle struct {
	Repository   string                         `json:"repository"`
	SourceCommit string                         `json:"sourceCommit,omitempty"`
	Files        []ConsoleMarketplaceBundleFile `json:"files"`
}

type ConsoleMarketplaceBundleFile struct {
	SourcePath string `json:"sourcePath"`
	TargetPath string `json:"targetPath"`
	SHA256     string `json:"sha256"`
}

type PluginServerConsumer struct {
	InventoryKey string   `json:"inventoryKey"`
	HTTPPath     string   `json:"httpPath"`
	Aliases      []string `json:"aliases,omitempty"`
}

type Plan struct {
	SchemaVersion   int              `json:"schemaVersion"`
	GatewayVersion  string           `json:"gatewayVersion"`
	SourceCommit    string           `json:"sourceCommit"`
	BaseCommit      string           `json:"baseCommit,omitempty"`
	PreviousRelease string           `json:"previousRelease,omitempty"`
	CatalogSHA256   string           `json:"catalogSha256"`
	PlanID          string           `json:"planId"`
	Plugins         []PlanEntry      `json:"plugins"`
	Deferred        []DeferredPlugin `json:"deferred,omitempty"`
}

type PlanEntry struct {
	LogicalID       string   `json:"logicalId"`
	Implementation  string   `json:"implementation"`
	SourceDir       string   `json:"sourceDir"`
	Image           string   `json:"image"`
	PreviousVersion string   `json:"previousVersion,omitempty"`
	Version         string   `json:"version"`
	InputHash       string   `json:"inputHash"`
	ChangedPaths    []string `json:"changedPaths"`
	// Backfill marks a bootstrap release entry whose stable public tag was
	// absent from the reviewed baseline: promotion creates the tag from this
	// candidate. The marker is bootstrap-only provenance and migration state;
	// after the complete version batch verifies, the entry participates in the
	// same serialized monotonic latest policy as every selected stable plugin.
	Backfill bool `json:"backfill,omitempty"`
}

// DeferredPlugin records a release-eligible catalog entry that was ignored
// for release selection. The only supported reason is an alpha prerelease
// VERSION, which denotes a development build without a public artifact.
type DeferredPlugin struct {
	LogicalID string `json:"logicalId"`
	Version   string `json:"version"`
	Reason    string `json:"reason"`
}

type CandidateEvidenceFile struct {
	Plugins map[string]CandidateEvidence `json:"plugins"`
}

type CandidateEvidence struct {
	CandidateRef string             `json:"candidateRef"`
	Digest       string             `json:"digest"`
	SourceCommit string             `json:"sourceCommit"`
	InputHash    string             `json:"inputHash"`
	Lineage      []EmergencyLineage `json:"lineage,omitempty"`
}

// EmergencyLineage records a successful same-version emergency publication
// without replacing the immutable candidate evidence that authorized the
// original release.
type EmergencyLineage struct {
	Digest        string `json:"digest"`
	InputHash     string `json:"inputHash"`
	SourceCommit  string `json:"sourceCommit"`
	WorkflowRunID string `json:"workflowRunId"`
}

// BootstrapEvidenceFile is deliberately separate from candidate evidence. A
// bootstrap imports already-public immutable artifacts and must not pretend
// that historical images were built by the candidate workflow.
type BootstrapEvidenceFile struct {
	Plugins map[string]BootstrapEvidence `json:"plugins"`
}

// BootstrapEvidence classifies one release-eligible plugin's public artifact:
// "public" resolves to a reviewed digest, "missing" is a stable VERSION whose
// public tag is genuinely absent and needs a candidate backfill, and
// "deferred" is an alpha prerelease VERSION excluded from release selection.
// An authorization failure is never recorded here; it aborts the capture.
type BootstrapEvidence struct {
	Status    string `json:"status"`
	Version   string `json:"version"`
	PublicRef string `json:"publicRef,omitempty"`
	Digest    string `json:"digest,omitempty"`
}

// MigrationReportFile is the prepare-time public registry sweep. It records the
// control tag the probe was calibrated against, so a reviewer can see that the
// present/absent classifications came from a method proven against a known-good
// public artifact rather than from an unvalidated lookup.
type MigrationReportFile struct {
	SchemaVersion  int                    `json:"schemaVersion"`
	GatewayVersion string                 `json:"gatewayVersion"`
	SourceCommit   string                 `json:"sourceCommit"`
	PlanID         string                 `json:"planId"`
	Registry       string                 `json:"registry"`
	ProbeMode      string                 `json:"probeMode"`
	ControlRef     string                 `json:"controlRef"`
	ControlDigest  string                 `json:"controlDigest"`
	Entries        []MigrationReportEntry `json:"entries"`
}

// MigrationReportEntry describes one planned public version tag that is already
// occupied. PlannedDigest is empty in existence-only mode, which is also the
// only mode that reports State "suspected"; a suspected entry never blocks a
// plugin and never enters a snapshot.
type MigrationReportEntry struct {
	LogicalID         string `json:"logicalId"`
	Version           string `json:"version"`
	OCIRef            string `json:"ociRef"`
	ExistingDigest    string `json:"existingDigest"`
	PlannedDigest     string `json:"plannedDigest,omitempty"`
	InputHash         string `json:"inputHash"`
	ExistingVersion   string `json:"existingVersion,omitempty"`
	ExistingRevision  string `json:"existingRevision,omitempty"`
	ExistingInputHash string `json:"existingInputHash,omitempty"`
	SourceComparison  string `json:"sourceComparison"`
	State             string `json:"state"`
	Recommendation    string `json:"recommendation,omitempty"`
}

type Snapshot struct {
	SchemaVersion   int    `json:"schemaVersion"`
	GatewayVersion  string `json:"gatewayVersion"`
	SourceCommit    string `json:"sourceCommit"`
	PreviousRelease string `json:"previousRelease,omitempty"`
	CatalogSHA256   string `json:"catalogSha256"`
	PlanID          string `json:"planId"`
	ProvenanceMode  string `json:"provenanceMode"`
	// BootstrapEvidence declares that this snapshot is the first managed
	// release rendered from the one-time bootstrap baseline and names the
	// committed bootstrap evidence the preparation PR must carry. Validation
	// never infers bootstrap mode from a missing previous-snapshot file.
	BootstrapEvidence *SnapshotBootstrapEvidence `json:"bootstrapEvidence,omitempty"`
	// MigrationPreflight records the prepare-time public registry sweep that
	// excluded one or more planned plugins from this release batch. It is
	// present exactly when some entry carries a migration marker, and its
	// control tag proves the sweep's probe method was validated against a
	// known-good public artifact before any classification was trusted.
	MigrationPreflight *SnapshotMigrationPreflight `json:"migrationPreflight,omitempty"`
	Plugins            []SnapshotEntry             `json:"plugins"`
}

// SnapshotMigrationPreflight binds the excluded set to the calibrated sweep.
// Excluded lists every entry this snapshot keeps out of the promote batch,
// whether this release's sweep blocked it or an earlier release did and this
// snapshot carries the exclusion forward.
type SnapshotMigrationPreflight struct {
	ControlRef    string   `json:"controlRef"`
	ControlDigest string   `json:"controlDigest"`
	Excluded      []string `json:"excluded"`
}

// SnapshotMigration marks an entry that the promote batch must skip: the
// planned public version tag is already occupied by a different artifact, so
// publishing this entry would either fail the immutable-tag preflight or
// overwrite an artifact nobody reviewed. The entry stays in the snapshot (it is
// still the authoritative record of the planned version and candidate digest)
// and is carried forward verbatim until a later release re-plans the plugin
// after its disposition completes.
type SnapshotMigration struct {
	State string `json:"state"`
	// SourceCommit is the plan source commit of the sweep that decided this
	// disposition. The recommendation compares the occupant's annotations with
	// that commit, so recording it keeps a carried exclusion re-derivable — and
	// therefore still valid — in every later release.
	SourceCommit      string `json:"sourceCommit"`
	ExistingDigest    string `json:"existingDigest"`
	PlannedDigest     string `json:"plannedDigest"`
	ExistingVersion   string `json:"existingVersion,omitempty"`
	ExistingRevision  string `json:"existingRevision,omitempty"`
	ExistingInputHash string `json:"existingInputHash,omitempty"`
	SourceComparison  string `json:"sourceComparison"`
	Recommendation    string `json:"recommendation"`
}

// SnapshotBootstrapEvidence binds the first managed release to the exact
// committed bootstrap evidence file it was reviewed against. Path is the
// deterministic repository-relative committed location and SHA256 is the
// lowercase hex SHA-256 of its canonical bytes.
type SnapshotBootstrapEvidence struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
}

type SnapshotEntry struct {
	LogicalID      string `json:"logicalId"`
	Implementation string `json:"implementation"`
	SourceDir      string `json:"sourceDir"`
	Image          string `json:"image"`
	Version        string `json:"version"`
	OCIRef         string `json:"ociRef"`
	Digest         string `json:"digest"`
	InputHash      string `json:"inputHash"`
	SourceCommit   string `json:"sourceCommit"`
	CandidateRef   string `json:"candidateRef"`
	ProvenanceMode string `json:"provenanceMode"`
	// Backfill marks an entry imported by building a candidate for a stable
	// VERSION whose public tag was absent at bootstrap. Promotion creates the
	// version tag from the candidate; the marker records bootstrap provenance
	// and migration state and is not an exclusion from the serialized
	// monotonic latest policy.
	Backfill bool `json:"backfill,omitempty"`
	// Migration marks an entry excluded from this release's promote batch by
	// the prepare-time registry sweep. Promotion skips it in both the version
	// and latest phases and journals the exclusion; verification tolerates the
	// divergent public tag it records.
	Migration *SnapshotMigration `json:"migration,omitempty"`
	Consumers PluginConsumers    `json:"consumers,omitempty"`
}

func cloneConsumers(in PluginConsumers) PluginConsumers {
	data, _ := json.Marshal(in)
	var out PluginConsumers
	_ = json.Unmarshal(data, &out)
	return out
}

func catalogConsumers(c Catalog, p Plugin) PluginConsumers {
	out := cloneConsumers(p.Consumers)
	if out.Console != nil && out.Console.Marketplace == nil && c.ConsoleMarketplace != nil {
		if bundle, ok := c.ConsoleMarketplace.Bundles[p.LogicalID]; ok {
			copy := bundle
			out.Console.Marketplace = &copy
		}
	}
	return out
}

type ConsoleRecoveryManifest struct {
	SchemaVersion         int                     `json:"schemaVersion"`
	GatewayVersion        string                  `json:"gatewayVersion"`
	SnapshotPath          string                  `json:"snapshotPath"`
	SnapshotSHA256        string                  `json:"snapshotSha256"`
	ImageRepository       string                  `json:"imageRepository"`
	OriginalConsoleCommit string                  `json:"originalConsoleCommit"`
	OriginalImageDigest   string                  `json:"originalImageDigest"`
	RequiredSourceBranch  string                  `json:"requiredSourceBranch"`
	Plugins               []ConsoleRecoveryPlugin `json:"plugins"`
}

type ConsoleRecoveryPlugin struct {
	LogicalID string          `json:"logicalId"`
	Version   string          `json:"version"`
	OCIRef    string          `json:"ociRef"`
	Digest    string          `json:"digest"`
	Console   ConsoleConsumer `json:"console"`
}
