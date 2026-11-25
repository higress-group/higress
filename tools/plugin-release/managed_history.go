package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"sort"
)

type HistoryArtifact struct {
	Version         string    `json:"version"`
	OCIRef          string    `json:"ociRef"`
	Digest          string    `json:"digest"`
	WasmSHA256      string    `json:"wasmSha256"`
	ProvenanceMode  string    `json:"provenanceMode"`
	SourceCommit    string    `json:"sourceCommit,omitempty"`
	InputHash       string    `json:"inputHash,omitempty"`
	CandidateRef    string    `json:"candidateRef,omitempty"`
	CandidateImport *ProofRef `json:"candidateImport,omitempty"`
}

type HistoryEvidence struct {
	SchemaVersion int                             `json:"schemaVersion"`
	ControlRef    string                          `json:"controlRef"`
	ControlDigest string                          `json:"controlDigest"`
	Entries       map[string]HistoryEvidenceEntry `json:"entries"`
}

type HistoryEvidenceEntry struct {
	Version           string `json:"version"`
	Digest            string `json:"digest"`
	WasmSHA256        string `json:"wasmSha256,omitempty"`
	Publication       string `json:"publication"`
	ObservedDigest    string `json:"observedDigest,omitempty"`
	ObservedVersion   string `json:"observedVersion,omitempty"`
	ObservedRevision  string `json:"observedRevision,omitempty"`
	ObservedInputHash string `json:"observedInputHash,omitempty"`
}

type legacyHistoryLock struct {
	SchemaVersion int                              `json:"schemaVersion"`
	Plugins       map[string]legacyHistoryArtifact `json:"plugins"`
}

type legacyHistoryArtifact struct {
	LogicalKey    string                  `json:"logicalKey"`
	ServedPath    string                  `json:"servedPath"`
	ServedVersion string                  `json:"servedVersion"`
	Repository    string                  `json:"repository"`
	Tag           string                  `json:"tag"`
	Digest        string                  `json:"digest"`
	WasmSHA256    string                  `json:"wasmSha256"`
	History       []legacyHistoryArtifact `json:"history,omitempty"`
}

func (artifact *HistoryArtifact) UnmarshalJSON(data []byte) error {
	type plain HistoryArtifact
	var decoded plain
	if err := strictDecode(data, &decoded); err != nil {
		return err
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	if decoded.ProvenanceMode == "public" {
		for _, field := range []string{"sourceCommit", "inputHash", "candidateRef", "candidateImport"} {
			if _, present := fields[field]; present {
				return errors.New("public history must omit build provenance fields")
			}
		}
	}
	if raw, present := fields["candidateImport"]; present && bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return errors.New("candidateImport must be a proof object")
	}
	*artifact = HistoryArtifact(decoded)
	return nil
}

func (entry *SnapshotEntry) UnmarshalJSON(data []byte) error {
	type plain SnapshotEntry
	var decoded plain
	if err := strictDecode(data, &decoded); err != nil {
		return err
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	if raw, ok := fields["history"]; ok && bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return errors.New("history must be an array, not null")
	}
	if _, ok := fields["wasmSha256"]; ok && !wasmHashPattern.MatchString(decoded.WasmSHA256) {
		return errors.New("wasmSha256 must be a bare lowercase SHA256")
	}
	*entry = SnapshotEntry(decoded)
	return nil
}

func importedHistory(root string, proof ProofRef, commit string, catalog Catalog) (map[string][]HistoryArtifact, error) {
	var manifest HistoryImport
	if _, err := readProof(root, proof, commit, &manifest); err != nil {
		return nil, err
	}
	if manifest.SchemaVersion != 1 || manifest.Repository != "higress-group/plugin-server" || !commitPattern.MatchString(manifest.SourceCommit) || !reflect.DeepEqual(manifest.LogicalIDs, sortedCPPIDs()) {
		return nil, errors.New("invalid history import manifest")
	}
	var lock legacyHistoryLock
	if _, err := readProof(root, manifest.Lock, commit, &lock); err != nil {
		return nil, err
	}
	if lock.SchemaVersion != 1 {
		return nil, errors.New("unsupported legacy history lock")
	}
	result := map[string][]HistoryArtifact{}
	for _, plugin := range catalog.Plugins {
		if plugin.Implementation != "cpp" {
			continue
		}
		old, ok := lock.Plugins[plugin.LogicalID]
		mapping := plugin.Consumers.PluginServer
		if !ok || mapping == nil || old.LogicalKey != mapping.InventoryKey || old.ServedPath != mapping.HTTPPath || old.ServedVersion != "2.0.0" {
			return nil, fmt.Errorf("%s: legacy lock mapping mismatch", plugin.LogicalID)
		}
		artifacts := append([]legacyHistoryArtifact{old}, old.History...)
		for _, item := range artifacts {
			if item.Repository != catalog.Registry+"/"+plugin.Image || item.Tag != item.ServedVersion ||
				item.ServedPath != mapping.HTTPPath || item.LogicalKey != mapping.InventoryKey || len(item.History) != 0 && item.ServedVersion != old.ServedVersion {
				return nil, fmt.Errorf("%s: invalid legacy history tuple", plugin.LogicalID)
			}
			history := HistoryArtifact{Version: item.ServedVersion, OCIRef: item.Repository + ":" + item.Tag, Digest: item.Digest, WasmSHA256: item.WasmSHA256, ProvenanceMode: "public"}
			if err := validateHistoryArtifact(catalog.Registry, plugin.Image, history); err != nil {
				return nil, err
			}
			var err error
			result[plugin.LogicalID], err = appendHistory(result[plugin.LogicalID], history)
			if err != nil {
				return nil, err
			}
		}
	}
	if len(result) != len(managedCPPSources) {
		return nil, errors.New("history import must cover all nine C++ entries")
	}
	return result, nil
}

func validateHistoryArtifact(registry, image string, artifact HistoryArtifact) error {
	version, err := parseSemver(artifact.Version)
	if err != nil || version.prerelease != "" || artifact.Version == "latest" || artifact.OCIRef != registry+"/"+image+":"+artifact.Version ||
		!digestPattern.MatchString(artifact.Digest) || !wasmHashPattern.MatchString(artifact.WasmSHA256) {
		return errors.New("invalid immutable history identity or hash")
	}
	switch artifact.ProvenanceMode {
	case "public":
		if artifact.SourceCommit != "" || artifact.InputHash != "" || artifact.CandidateRef != "" || artifact.CandidateImport != nil {
			return errors.New("legacy public history must not invent build provenance")
		}
	case "candidate":
		if !commitPattern.MatchString(artifact.SourceCommit) || !digestPattern.MatchString(artifact.InputHash) ||
			!validPinnedCandidateReference(artifact.CandidateRef, artifact.Digest) {
			return errors.New("invalid candidate history provenance")
		}
		if artifact.CandidateImport != nil {
			return validateProof(*artifact.CandidateImport)
		}
	default:
		return errors.New("unsupported history provenanceMode")
	}
	return nil
}

func appendHistory(history []HistoryArtifact, artifact HistoryArtifact) ([]HistoryArtifact, error) {
	for _, old := range history {
		if old.Version == artifact.Version {
			if !reflect.DeepEqual(old, artifact) {
				return nil, errors.New("immutable history version collision")
			}
			return history, nil
		}
	}
	history = append(append([]HistoryArtifact(nil), history...), artifact)
	sort.Slice(history, func(left, right int) bool {
		first, _ := parseSemver(history[left].Version)
		second, _ := parseSemver(history[right].Version)
		return compareSemver(first, second) < 0
	})
	return history, nil
}

func historyFromCurrent(entry SnapshotEntry, hash string) HistoryArtifact {
	artifact := HistoryArtifact{Version: entry.Version, OCIRef: entry.OCIRef, Digest: entry.Digest, WasmSHA256: hash, ProvenanceMode: entryProvenance(entry)}
	if artifact.ProvenanceMode == "candidate" {
		artifact.SourceCommit, artifact.InputHash, artifact.CandidateRef, artifact.CandidateImport = entry.SourceCommit, entry.InputHash, entry.CandidateRef, entry.CandidateImport
	}
	return artifact
}

func historySubjects(plan Plan, previous Snapshot) map[string]SnapshotEntry {
	planned := map[string]bool{}
	for _, entry := range plan.Plugins {
		planned[entry.LogicalID] = true
	}
	subjects := map[string]SnapshotEntry{}
	for _, entry := range previous.Plugins {
		if planned[entry.LogicalID] && entry.Consumers.PluginServer != nil {
			subjects[entry.LogicalID] = entry
		}
	}
	return subjects
}

func validateHistoryEvidence(plan Plan, previous Snapshot, evidence *HistoryEvidence) error {
	seenPrevious := map[string]bool{}
	for _, entry := range previous.Plugins {
		if seenPrevious[entry.LogicalID] {
			return errors.New("duplicate previous current in history evidence")
		}
		seenPrevious[entry.LogicalID] = true
	}
	subjects := historySubjects(plan, previous)
	if len(subjects) == 0 {
		if evidence != nil {
			return errors.New("unexpected historyEvidence with no upgraded pluginServer current")
		}
		return nil
	}
	if evidence == nil || evidence.SchemaVersion != 1 || !registryQualifiedReferencePattern.MatchString(evidence.ControlRef) || !digestPattern.MatchString(evidence.ControlDigest) || len(evidence.Entries) != len(subjects) {
		return errors.New("historyEvidence requires a complete calibrated control and exact upgraded entry set")
	}
	if !matchesHistoryControl(previous, evidence) {
		return errors.New("historyEvidence control is not a non-excluded previous public reference")
	}
	for id, record := range evidence.Entries {
		entry, ok := subjects[id]
		if !ok || record.Version != entry.Version || record.Digest != entry.Digest || record.WasmSHA256 != "" && (!wasmHashPattern.MatchString(record.WasmSHA256) || entry.WasmSHA256 != "" && entry.WasmSHA256 != record.WasmSHA256) {
			return fmt.Errorf("%s: historyEvidence previous tuple mismatch", id)
		}
		switch record.Publication {
		case "published":
			if entry.Migration != nil || !wasmHashPattern.MatchString(record.WasmSHA256) || record.ObservedDigest != entry.Digest {
				return fmt.Errorf("%s: incomplete published history evidence", id)
			}
			if entryProvenance(entry) == "candidate" && (record.ObservedVersion != entry.Version || record.ObservedRevision != entry.SourceCommit || record.ObservedInputHash != entry.InputHash) {
				return fmt.Errorf("%s: published history annotations mismatch", id)
			}
		case "absent":
			if entry.Migration != nil || entryProvenance(entry) != "candidate" || record.ObservedDigest != "" || record.ObservedVersion != "" || record.ObservedRevision != "" || record.ObservedInputHash != "" {
				return fmt.Errorf("%s: invalid absence evidence", id)
			}
		case "excluded":
			marker := entry.Migration
			if marker == nil || marker.State != migrationStateBlocked || marker.PlannedDigest != entry.Digest || record.ObservedDigest != marker.ExistingDigest ||
				record.ObservedVersion != marker.ExistingVersion || record.ObservedRevision != marker.ExistingRevision || record.ObservedInputHash != marker.ExistingInputHash ||
				marker.SourceComparison != migrationSourceComparisonFromRecord(marker.ExistingVersion, marker.ExistingRevision, marker.ExistingInputHash, marker.SourceCommit) {
				return fmt.Errorf("%s: history exclusion does not bind migration marker", id)
			}
		default:
			return fmt.Errorf("%s: unknown history publication classification", id)
		}
	}
	return nil
}

func matchesHistoryControl(previous Snapshot, evidence *HistoryEvidence) bool {
	for _, entry := range previous.Plugins {
		if entry.Migration == nil && entry.OCIRef == evidence.ControlRef && entry.Digest == evidence.ControlDigest {
			return true
		}
		for _, artifact := range entry.History {
			if artifact.OCIRef == evidence.ControlRef && artifact.Digest == evidence.ControlDigest {
				return true
			}
		}
	}
	return false
}

func deriveHistory(previous SnapshotEntry, record HistoryEvidenceEntry) ([]HistoryArtifact, error) {
	history := append([]HistoryArtifact(nil), previous.History...)
	if record.Publication == "published" {
		return appendHistory(history, historyFromCurrent(previous, record.WasmSHA256))
	}
	return history, nil
}

func validateSnapshotHistory(catalog Catalog, snapshot Snapshot) error {
	claims := map[string]string{}
	for _, entry := range snapshot.Plugins {
		if entry.WasmSHA256 != "" && !wasmHashPattern.MatchString(entry.WasmSHA256) {
			return errors.New("current wasmSha256 is not a bare SHA256")
		}
		mapping := entry.Consumers.PluginServer
		if len(entry.History) != 0 && mapping == nil {
			return errors.New("history requires a pluginServer consumer")
		}
		seen := map[string]bool{entry.Version: true}
		versions := []string{entry.Version}
		last := ""
		for _, artifact := range entry.History {
			if err := validateHistoryArtifact(catalog.Registry, entry.Image, artifact); err != nil {
				return fmt.Errorf("%s: %w", entry.LogicalID, err)
			}
			if seen[artifact.Version] {
				return errors.New("history duplicates current or another history version")
			}
			if last != "" {
				previousVersion, _ := parseSemver(last)
				version, _ := parseSemver(artifact.Version)
				if compareSemver(previousVersion, version) >= 0 {
					return errors.New("history must be sorted by SemVer")
				}
			}
			last, seen[artifact.Version] = artifact.Version, true
			versions = append(versions, artifact.Version)
		}
		if mapping == nil {
			continue
		}
		for _, path := range append([]string{mapping.HTTPPath, mapping.InventoryKey}, mapping.Aliases...) {
			if !safeIDPattern.MatchString(path) {
				return errors.New("unsafe pluginServer history path")
			}
			for _, version := range versions {
				key := path + "/" + version
				if owner := claims[key]; owner != "" && owner != entry.LogicalID {
					return fmt.Errorf("history path/version collision %s", key)
				}
				claims[key] = entry.LogicalID
			}
		}
	}
	return nil
}
