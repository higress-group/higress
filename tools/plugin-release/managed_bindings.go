package main

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"reflect"
	"time"
)

func validatePlanImports(root string, catalog Catalog, plan Plan, previous Snapshot, evidence CandidateEvidenceFile) error {
	count := 0
	var candidateProof *ProofRef
	for _, planned := range plan.Plugins {
		if _, err := effectiveArtifactSource(plan, planned); err != nil {
			return err
		}
		if planned.Implementation == "cpp" && planned.PreviousVersion == "" && planned.CandidateImport == nil {
			return errors.New("new managed C++ entries require explicit imports; build fallback is forbidden")
		}
		if planned.CandidateImport == nil {
			continue
		}
		if candidateProof != nil && !reflect.DeepEqual(candidateProof, planned.CandidateImport) {
			return errors.New("mixed candidate import manifests")
		}
		candidateProof = planned.CandidateImport
		count++
	}
	if count == 0 {
		if plan.HistoryImport != nil {
			return errors.New("historyImport requires the complete first candidate import")
		}
		return nil
	}
	if count != len(managedCPPSources) || plan.HistoryImport == nil {
		return errors.New("candidate/history imports must cover the complete nine-entry migration")
	}
	for _, entry := range previous.Plugins {
		if entry.Implementation == "cpp" {
			return errors.New("C++ imports cannot override a previous current")
		}
	}
	manifest, original, err := loadCandidateImport(root, *candidateProof, plan.SourceCommit, catalog)
	if err != nil {
		return err
	}
	for _, planned := range plan.Plugins {
		if planned.CandidateImport == nil {
			continue
		}
		old, ok := original.Plugins[planned.LogicalID]
		actual, present := evidence.Plugins[planned.LogicalID]
		if !ok || !present || planned.PreviousVersion != "" || planned.Version != cppHotfixVersion || planned.ArtifactSourceCommit != manifest.SourceCommit || planned.InputHash != old.InputHash ||
			actual.CandidateRef != old.CandidateRef || actual.Digest != old.Digest || actual.InputHash != old.InputHash || actual.SourceCommit != old.SourceCommit || !reflect.DeepEqual(actual.Lineage, old.Lineage) || old.WasmSHA256 != "" && old.WasmSHA256 != actual.WasmSHA256 {
			return fmt.Errorf("%s: imported candidate tuple was replaced", planned.LogicalID)
		}
	}
	return nil
}

func renderManagedBindings(root string, catalog Catalog, plan Plan, previous Snapshot, evidence CandidateEvidenceFile, snapshot *Snapshot) error {
	if plan.HistoryImport != nil || evidence.HistoryEvidence != nil {
		if plan.PlanID != "sha256:"+canonicalObjectHash(plan, true) {
			return errors.New("managed planId does not bind its canonical content")
		}
	}
	if err := validatePlanImports(root, catalog, plan, previous, evidence); err != nil {
		return err
	}
	if err := validateHistoryEvidence(plan, previous, evidence.HistoryEvidence); err != nil {
		return err
	}
	var seed map[string][]HistoryArtifact
	if plan.HistoryImport != nil {
		var err error
		seed, err = importedHistory(root, *plan.HistoryImport, plan.SourceCommit, catalog)
		if err != nil {
			return err
		}
		snapshot.HistoryImport = plan.HistoryImport
	}
	planned := map[string]PlanEntry{}
	for _, entry := range plan.Plugins {
		planned[entry.LogicalID] = entry
	}
	old := map[string]SnapshotEntry{}
	for _, entry := range previous.Plugins {
		old[entry.LogicalID] = entry
	}
	for index := range snapshot.Plugins {
		entry := &snapshot.Plugins[index]
		if _, changed := planned[entry.LogicalID]; !changed {
			continue
		}
		if entry.WasmSHA256 != "" && !wasmHashPattern.MatchString(entry.WasmSHA256) {
			return errors.New("candidate wasmSha256 must be a bare hash, not an OCI digest")
		}
		if entry.Consumers.PluginServer != nil && !wasmHashPattern.MatchString(entry.WasmSHA256) {
			return fmt.Errorf("%s: new pluginServer current requires verified Wasm bytes", entry.LogicalID)
		}
		if history, imported := seed[entry.LogicalID]; imported {
			entry.History = history
		} else if previousEntry, existed := old[entry.LogicalID]; existed {
			entry.History = append([]HistoryArtifact(nil), previousEntry.History...)
			if previousEntry.Consumers.PluginServer != nil {
				history, err := deriveHistory(previousEntry, evidence.HistoryEvidence.Entries[entry.LogicalID])
				if err != nil {
					return err
				}
				entry.History = history
			}
		}
	}
	return validateSnapshotHistory(catalog, *snapshot)
}

func hasManagedFields(snapshot Snapshot) bool {
	if snapshot.HistoryImport != nil {
		return true
	}
	for _, entry := range snapshot.Plugins {
		if entry.CandidateImport != nil || entry.WasmSHA256 != "" || len(entry.History) != 0 {
			return true
		}
	}
	return false
}

func verifyManagedBindings(root string, catalog Catalog, snapshot Snapshot, planPath, previousPath, committed string, resolve bool) error {
	var previous Snapshot
	if previousPath != "" {
		if _, err := readJSON(previousPath, &previous); err != nil {
			return err
		}
	}
	managed := hasManagedFields(snapshot) || hasManagedFields(previous)
	for _, plugin := range catalog.Plugins {
		managed = managed || plugin.Implementation == "cpp" && plugin.ReleaseEligible
	}
	var evidence CandidateEvidenceFile
	evidencePath := releaseEvidencePath(root, snapshot)
	evidenceData, evidenceErr := readJSON(evidencePath, &evidence)
	if evidenceErr == nil {
		managed = managed || evidence.HistoryEvidence != nil
		for _, proof := range evidence.Plugins {
			managed = managed || proof.WasmSHA256 != ""
		}
	} else if !errors.Is(evidenceErr, os.ErrNotExist) {
		return fmt.Errorf("committed merged candidate evidence: %w", evidenceErr)
	}
	if !managed {
		return nil
	}
	if planPath == "" || previousPath == "" || committed == "" {
		return errors.New("snapshots with import/history/Wasm evidence require --plan --previous --committed-source")
	}
	if err := requireAncestor(root, snapshot.SourceCommit, committed); err != nil {
		return err
	}
	var plan Plan
	if _, err := readJSON(planPath, &plan); err != nil {
		return err
	}
	if evidenceErr != nil {
		return fmt.Errorf("committed merged candidate evidence: %w", evidenceErr)
	}
	tracked, err := repositoryBytesAtCommit(root, committed, "plugins/release/evidence/"+snapshot.GatewayVersion+".json")
	if err != nil || !bytes.Equal(tracked, evidenceData) {
		return errors.New("merged candidate evidence differs from committed-source")
	}
	expected := snapshot
	expected.Plugins = append([]SnapshotEntry(nil), snapshot.Plugins...)
	expected.HistoryImport = nil
	if err := renderManagedBindings(root, catalog, plan, previous, evidence, &expected); err != nil {
		return err
	}
	if !reflect.DeepEqual(expected.HistoryImport, snapshot.HistoryImport) {
		return errors.New("snapshot historyImport differs from plan")
	}
	if resolve {
		if err := replayHistoryEvidence(root, plan, previous, evidence.HistoryEvidence); err != nil {
			return err
		}
	}
	planned := map[string]PlanEntry{}
	for _, entry := range plan.Plugins {
		planned[entry.LogicalID] = entry
	}
	for index, entry := range snapshot.Plugins {
		if !reflect.DeepEqual(entry.History, expected.Plugins[index].History) {
			return fmt.Errorf("%s: history cannot be derived from previous/import/publication evidence", entry.LogicalID)
		}
		if selected, ok := planned[entry.LogicalID]; ok {
			proof, present := evidence.Plugins[entry.LogicalID]
			source, err := effectiveArtifactSource(plan, selected)
			if err != nil {
				return err
			}
			if !present || entry.Digest != proof.Digest || entry.CandidateRef != proof.CandidateRef || entry.SourceCommit != source || proof.SourceCommit != source || entry.InputHash != proof.InputHash || entry.WasmSHA256 != proof.WasmSHA256 {
				return fmt.Errorf("%s: merged evidence differs from current", entry.LogicalID)
			}
		}
		if entry.CandidateImport != nil {
			if err := verifyRetainedCandidateImport(root, catalog, entry, committed); err != nil {
				return err
			}
		}
		for _, history := range entry.History {
			historical := SnapshotEntry{LogicalID: entry.LogicalID, Implementation: entry.Implementation, SourceDir: entry.SourceDir, Image: entry.Image, Version: history.Version, OCIRef: history.OCIRef, Digest: history.Digest, WasmSHA256: history.WasmSHA256, SourceCommit: history.SourceCommit, InputHash: history.InputHash, CandidateRef: history.CandidateRef, CandidateImport: history.CandidateImport, ProvenanceMode: history.ProvenanceMode}
			if history.CandidateImport != nil {
				if err := verifyRetainedCandidateImport(root, catalog, historical, committed); err != nil {
					return err
				}
			}
			if resolve {
				if err := verifyOCI(historical, history.ProvenanceMode, "mixed", "public"); err != nil {
					return err
				}
				if _, err := ociWasmHashResolver(root, historical, history.OCIRef); err != nil {
					return err
				}
			}
		}
	}
	if resolve {
		if snapshot.HistoryImport != nil {
			return verifyHistoryImportOrigin(root, *snapshot.HistoryImport)
		}
	}
	return nil
}

func verifyRetainedCandidateImport(root string, catalog Catalog, entry SnapshotEntry, committed string) error {
	manifest, original, err := loadCandidateImport(root, *entry.CandidateImport, "", catalog)
	if err != nil {
		return err
	}
	for _, proof := range []ProofRef{*entry.CandidateImport, manifest.Catalog, manifest.Plan, manifest.Snapshot, manifest.Evidence} {
		if _, err := readProof(root, proof, committed, nil); err != nil {
			return err
		}
	}
	old, ok := original.Plugins[entry.LogicalID]
	if !ok || entry.Version != cppHotfixVersion || entry.SourceCommit != manifest.SourceCommit || entry.Digest != old.Digest || entry.CandidateRef != old.CandidateRef || entry.InputHash != old.InputHash {
		return errors.New("retained imported candidate tuple drift")
	}
	return nil
}

func verifyHistoryImportOrigin(root string, proof ProofRef) error {
	var manifest HistoryImport
	if _, err := readProof(root, proof, "", &manifest); err != nil {
		return err
	}
	archived, err := readProof(root, manifest.Lock, "", nil)
	if err != nil {
		return err
	}
	if manifest.Repository != "higress-group/plugin-server" || !commitPattern.MatchString(manifest.SourceCommit) {
		return errors.New("invalid pinned history origin identity")
	}
	client := &http.Client{Timeout: 30 * time.Second}
	response, err := client.Get("https://raw.githubusercontent.com/" + manifest.Repository + "/" + manifest.SourceCommit + "/unmanaged-plugins.lock.json")
	if err != nil {
		return errors.New("cannot independently read the pinned history lock origin")
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("pinned history origin returned HTTP %d", response.StatusCode)
	}
	remote, err := io.ReadAll(io.LimitReader(response.Body, int64(len(archived))+1))
	if err != nil {
		return err
	}
	if !bytes.Equal(archived, remote) {
		return errors.New("history lock archive differs from pinned origin bytes")
	}
	return nil
}

func commandCaptureHistoryEvidence(args []string) error {
	fs, root, catalogPath := parseCommon("capture-history-evidence", args)
	planPath := fs.String("plan", "", "exact release plan")
	previousPath := fs.String("previous", "", "reviewed previous snapshot")
	evidencePath := fs.String("candidate-evidence", "", "built candidate evidence; imported candidates are read only")
	output := fs.String("output", "-", "merged evidence output; written only after all checks succeed")
	if err := fs.Parse(args); err != nil {
		return err
	}
	catalog, catalogData, err := loadCatalog(*catalogPath)
	if err != nil {
		return err
	}
	var plan Plan
	var previous Snapshot
	var evidence CandidateEvidenceFile
	if _, err := readJSON(*planPath, &plan); err != nil {
		return err
	}
	if err := validatePlanProvenance(plan, catalogData); err != nil {
		return err
	}
	if _, err := readJSON(*previousPath, &previous); err != nil {
		return err
	}
	if _, err := readJSON(*evidencePath, &evidence); err != nil {
		return err
	}
	if evidence.Plugins == nil {
		evidence.Plugins = map[string]CandidateEvidence{}
	}
	for _, selected := range plan.Plugins {
		if selected.CandidateImport != nil {
			_, original, err := loadCandidateImport(*root, *selected.CandidateImport, plan.SourceCommit, catalog)
			if err != nil {
				return err
			}
			if _, exists := evidence.Plugins[selected.LogicalID]; exists {
				return errors.New("built evidence cannot override an imported candidate")
			}
			evidence.Plugins[selected.LogicalID] = original.Plugins[selected.LogicalID]
		}
		proof, ok := evidence.Plugins[selected.LogicalID]
		source, err := effectiveArtifactSource(plan, selected)
		if err != nil {
			return err
		}
		if !ok || proof.SourceCommit != source || proof.InputHash != selected.InputHash || !validPinnedCandidateReference(proof.CandidateRef, proof.Digest) {
			return errors.New("candidate evidence is incomplete or mismatched")
		}
		entry := SnapshotEntry{LogicalID: selected.LogicalID, Version: selected.Version, Digest: proof.Digest, SourceCommit: source, InputHash: selected.InputHash, CandidateRef: proof.CandidateRef, WasmSHA256: proof.WasmSHA256, ProvenanceMode: "candidate"}
		hash, err := ociWasmHashResolver(*root, entry, proof.CandidateRef)
		if err != nil {
			return err
		}
		proof.WasmSHA256 = hash
		evidence.Plugins[selected.LogicalID] = proof
	}
	evidence.HistoryEvidence, err = captureHistoryEvidence(*root, *previousPath, plan, previous)
	if err != nil {
		return err
	}
	if err := validatePlanImports(*root, catalog, plan, previous, evidence); err != nil {
		return err
	}
	return writeCanonical(*output, evidence)
}
