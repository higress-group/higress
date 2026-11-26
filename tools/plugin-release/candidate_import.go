package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strings"
)

var wasmHashPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

func validPinnedCandidateReference(reference, digest string) bool {
	if !digestPattern.MatchString(digest) || !strings.HasSuffix(reference, "@"+digest) {
		return false
	}
	unpinned := strings.TrimSuffix(reference, "@"+digest)
	if strings.HasSuffix(unpinned, ":latest") {
		return false
	}
	return registryQualifiedReferencePattern.MatchString(reference) || registryQualifiedReferencePattern.MatchString(unpinned)
}

type ProofRef struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
}

type CandidateImport struct {
	SchemaVersion int      `json:"schemaVersion"`
	Repository    string   `json:"repository"`
	SourceCommit  string   `json:"sourceCommit"`
	Catalog       ProofRef `json:"catalog"`
	Plan          ProofRef `json:"plan"`
	Snapshot      ProofRef `json:"snapshot"`
	Evidence      ProofRef `json:"evidence"`
}

type HistoryImport struct {
	SchemaVersion int      `json:"schemaVersion"`
	Repository    string   `json:"repository"`
	SourceCommit  string   `json:"sourceCommit"`
	Lock          ProofRef `json:"lock"`
	LogicalIDs    []string `json:"logicalIds"`
}

func strictDecode(data []byte, value any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(value); err != nil {
		return err
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return errors.New("expected exactly one JSON object")
	}
	return nil
}

func validateProof(proof ProofRef) error {
	if !safeRepositoryPath(proof.Path) || strings.ContainsRune(proof.Path, 0) || !wasmHashPattern.MatchString(proof.SHA256) {
		return errors.New("invalid hash-bound repository proof")
	}
	return nil
}

func readProof(root string, proof ProofRef, commit string, value any) ([]byte, error) {
	if err := validateProof(proof); err != nil {
		return nil, err
	}
	data, err := readRepositoryFileWithoutSymlinks(root, proof.Path)
	if err != nil {
		return nil, err
	}
	if sha256Hex(data) != proof.SHA256 {
		return nil, fmt.Errorf("proof %s hash mismatch", proof.Path)
	}
	if commit != "" {
		tracked, err := repositoryBytesAtCommit(root, commit, proof.Path)
		if err != nil || !bytes.Equal(tracked, data) {
			return nil, fmt.Errorf("proof %s differs from tracked bytes at %s", proof.Path, commit)
		}
		mode, err := runGit(root, "ls-tree", commit, "--", proof.Path)
		if err != nil || !strings.HasPrefix(mode, "100644 ") && !strings.HasPrefix(mode, "100755 ") {
			return nil, fmt.Errorf("proof %s is not a tracked regular file", proof.Path)
		}
	}
	if value != nil {
		if err := strictDecode(data, value); err != nil {
			return nil, fmt.Errorf("proof %s: %w", proof.Path, err)
		}
	}
	return data, nil
}

func proofForPath(root, path, commit string) (*ProofRef, error) {
	if !safeRepositoryPath(path) || !strings.HasPrefix(path, "plugins/release/imports/") {
		return nil, errors.New("import manifests must be safe repository-relative paths below plugins/release/imports")
	}
	data, err := readRepositoryFileWithoutSymlinks(root, path)
	if err != nil {
		return nil, err
	}
	proof := &ProofRef{Path: path, SHA256: sha256Hex(data)}
	_, err = readProof(root, *proof, commit, nil)
	return proof, err
}

func effectiveArtifactSource(plan Plan, entry PlanEntry) (string, error) {
	if entry.CandidateImport == nil {
		if entry.ArtifactSourceCommit != "" {
			return "", errors.New("artifactSourceCommit requires candidateImport")
		}
		return plan.SourceCommit, nil
	}
	if err := validateProof(*entry.CandidateImport); err != nil {
		return "", err
	}
	if entry.Implementation != "cpp" || !commitPattern.MatchString(entry.ArtifactSourceCommit) {
		return "", errors.New("invalid imported artifact source")
	}
	return entry.ArtifactSourceCommit, nil
}

func loadCandidateImport(root string, proof ProofRef, freeze string, catalog Catalog) (CandidateImport, CandidateEvidenceFile, error) {
	var manifest CandidateImport
	var originalCatalog Catalog
	var originalPlan Plan
	var originalSnapshot Snapshot
	var evidence CandidateEvidenceFile
	if _, err := readProof(root, proof, freeze, &manifest); err != nil {
		return manifest, evidence, err
	}
	if manifest.SchemaVersion != 1 || manifest.Repository != "higress-group/higress" || !commitPattern.MatchString(manifest.SourceCommit) {
		return manifest, evidence, errors.New("invalid candidate import manifest identity")
	}
	if freeze != "" {
		if err := requireAncestor(root, manifest.SourceCommit, freeze); err != nil {
			return manifest, evidence, err
		}
	}
	catalogData, err := readProof(root, manifest.Catalog, freeze, &originalCatalog)
	if err != nil {
		return manifest, evidence, err
	}
	originalBytes, err := repositoryBytesAtCommit(root, manifest.SourceCommit, "plugins/release/catalog.json")
	if err != nil || !bytes.Equal(catalogData, originalBytes) {
		return manifest, evidence, errors.New("import catalog is not the artifact source's tracked catalog")
	}
	for _, item := range []struct {
		proof ProofRef
		value any
	}{{manifest.Plan, &originalPlan}, {manifest.Snapshot, &originalSnapshot}, {manifest.Evidence, &evidence}} {
		if _, err := readProof(root, item.proof, freeze, item.value); err != nil {
			return manifest, evidence, err
		}
	}
	if err := validateCPPHotfixCatalog(originalCatalog); err != nil {
		return manifest, evidence, err
	}
	if err := validatePlanProvenance(originalPlan, catalogData); err != nil {
		return manifest, evidence, err
	}
	if err := validateCPPHotfixPlan(originalPlan); err != nil {
		return manifest, evidence, err
	}
	if err := validateCPPHotfixSnapshot(originalSnapshot); err != nil {
		return manifest, evidence, err
	}
	if originalSnapshot.SchemaVersion != 1 || originalSnapshot.CatalogSHA256 != sha256Hex(catalogData) ||
		originalSnapshot.PlanID != originalPlan.PlanID || originalSnapshot.GatewayVersion != originalPlan.GatewayVersion ||
		originalPlan.SourceCommit != manifest.SourceCommit || originalSnapshot.SourceCommit != manifest.SourceCommit ||
		len(evidence.Plugins) != len(managedCPPSources) || evidence.HistoryEvidence != nil || originalPlan.HistoryImport != nil || originalSnapshot.HistoryImport != nil {
		return manifest, evidence, errors.New("original import evidence has inconsistent provenance")
	}
	for _, planned := range originalPlan.Plugins {
		var originalPlugin, currentPlugin Plugin
		var snapshotEntry SnapshotEntry
		for _, plugin := range originalCatalog.Plugins {
			if plugin.LogicalID == planned.LogicalID {
				originalPlugin = plugin
			}
		}
		for _, plugin := range catalog.Plugins {
			if plugin.LogicalID == planned.LogicalID {
				currentPlugin = plugin
			}
		}
		for _, entry := range originalSnapshot.Plugins {
			if entry.LogicalID == planned.LogicalID {
				snapshotEntry = entry
			}
		}
		proofEntry, ok := evidence.Plugins[planned.LogicalID]
		if !ok || planned.CandidateImport != nil || planned.ArtifactSourceCommit != "" || snapshotEntry.CandidateImport != nil || len(snapshotEntry.History) != 0 ||
			originalCatalog.Registry != catalog.Registry || currentPlugin.SourceDir != originalPlugin.SourceDir || currentPlugin.Image != originalPlugin.Image ||
			!reflect.DeepEqual(currentPlugin.ArtifactInputs, originalPlugin.ArtifactInputs) || !reflect.DeepEqual(currentPlugin.SharedInputGroups, originalPlugin.SharedInputGroups) {
			return manifest, evidence, fmt.Errorf("%s imported identity or input selection changed", planned.LogicalID)
		}
		for _, group := range originalPlugin.SharedInputGroups {
			if !reflect.DeepEqual(originalCatalog.SharedInputGroups[group], catalog.SharedInputGroups[group]) {
				return manifest, evidence, errors.New("imported shared inputs changed")
			}
		}
		if !digestPattern.MatchString(proofEntry.Digest) || proofEntry.SourceCommit != manifest.SourceCommit || proofEntry.InputHash != planned.InputHash ||
			!validPinnedCandidateReference(proofEntry.CandidateRef, proofEntry.Digest) || snapshotEntry.CandidateRef != proofEntry.CandidateRef || snapshotEntry.Digest != proofEntry.Digest ||
			snapshotEntry.Implementation != planned.Implementation || snapshotEntry.SourceDir != planned.SourceDir || snapshotEntry.Image != planned.Image || snapshotEntry.Version != planned.Version ||
			snapshotEntry.SourceCommit != manifest.SourceCommit || snapshotEntry.InputHash != proofEntry.InputHash || snapshotEntry.OCIRef != catalog.Registry+"/"+planned.Image+":"+planned.Version {
			return manifest, evidence, fmt.Errorf("%s original candidate tuple mismatch", planned.LogicalID)
		}
		for _, commit := range []string{manifest.SourceCommit, freeze} {
			if commit == "" {
				continue
			}
			version, err := fileAtCommit(root, commit, currentPlugin.SourceDir+"/VERSION")
			hash, hashErr := inputHash(root, commit, planned.Version, catalog, currentPlugin)
			if err != nil || strings.TrimSpace(version) != cppHotfixVersion || hashErr != nil || hash != planned.InputHash {
				return manifest, evidence, fmt.Errorf("%s imported VERSION or inputHash drift at %s", planned.LogicalID, commit)
			}
		}
	}
	return manifest, evidence, nil
}

func bindPlanImports(root string, catalog Catalog, plan *Plan, previous Snapshot, candidatePath, historyPath string) error {
	previousCPP := 0
	for _, entry := range previous.Plugins {
		if entry.Implementation == "cpp" {
			previousCPP++
		}
	}
	previousIDs := map[string]bool{}
	for _, entry := range previous.Plugins {
		previousIDs[entry.LogicalID] = true
	}
	first := false
	for _, entry := range catalog.Plugins {
		first = first || entry.Implementation == "cpp" && entry.ReleaseEligible && !previousIDs[entry.LogicalID]
	}
	if !first {
		if candidatePath != "" || historyPath != "" {
			return errors.New("imports are permitted only for the first complete managed C++ migration")
		}
		return nil
	}
	if previousCPP != 0 || candidatePath == "" || historyPath == "" {
		return errors.New("first managed C++ migration requires both imports and no previous C++ entries")
	}
	candidateProof, err := proofForPath(root, candidatePath, plan.SourceCommit)
	if err != nil {
		return err
	}
	historyProof, err := proofForPath(root, historyPath, plan.SourceCommit)
	if err != nil {
		return err
	}
	manifest, evidence, err := loadCandidateImport(root, *candidateProof, plan.SourceCommit, catalog)
	if err != nil {
		return err
	}
	count := 0
	for index := range plan.Plugins {
		entry := &plan.Plugins[index]
		if entry.Implementation != "cpp" {
			continue
		}
		original, ok := evidence.Plugins[entry.LogicalID]
		if !ok || entry.Version != cppHotfixVersion || original.InputHash != entry.InputHash || entry.PreviousVersion != "" {
			return fmt.Errorf("%s does not match the imported candidate", entry.LogicalID)
		}
		entry.CandidateImport, entry.ArtifactSourceCommit = candidateProof, manifest.SourceCommit
		count++
	}
	if count != len(managedCPPSources) {
		return errors.New("candidate import must cover all nine C++ entries")
	}
	plan.HistoryImport = historyProof
	_, err = importedHistory(root, *historyProof, plan.SourceCommit, catalog)
	return err
}

func sortedCPPIDs() []string {
	ids := make([]string, 0, len(managedCPPSources))
	for id := range managedCPPSources {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

func releaseEvidencePath(root string, snapshot Snapshot) string {
	return filepath.Join(root, "plugins/release/evidence", snapshot.GatewayVersion+".json")
}

func repositoryBytesAtCommit(root, commit, path string) ([]byte, error) {
	return exec.Command("git", "-C", root, "show", commit+":"+path).Output()
}
