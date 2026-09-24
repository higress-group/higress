// Copyright 2026 Higress Authors
// Licensed under the Apache License, Version 2.0.

package main

import (
	"errors"
	"flag"
	"fmt"
	"regexp"
	"strings"
)

var workflowRunIDPattern = regexp.MustCompile(`^[1-9][0-9]*$`)

func commandAppendEmergencyLineage(args []string) error {
	fs := flag.NewFlagSet("append-emergency-lineage", flag.ContinueOnError)
	evidencePath := fs.String("evidence", "", "candidate evidence JSON to update")
	snapshotPath := fs.String("snapshot", "", "release snapshot that binds the candidate evidence")
	outputPath := fs.String("output", "", "canonical updated evidence output")
	gatewayVersion := fs.String("gateway-version", "", "stable gateway release version")
	logicalID := fs.String("id", "", "logical plugin ID")
	version := fs.String("version", "", "stable plugin version")
	image := fs.String("image", "", "catalog public image path")
	digest := fs.String("digest", "", "emergency manifest digest")
	inputHash := fs.String("input-hash", "", "emergency source input hash")
	sourceCommit := fs.String("source-commit", "", "emergency source commit")
	workflowRunID := fs.String("workflow-run-id", "", "GitHub Actions workflow run ID")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *evidencePath == "" || *snapshotPath == "" || *outputPath == "" {
		return errors.New("--evidence, --snapshot, and --output are required")
	}
	record := EmergencyLineage{
		Digest:        *digest,
		InputHash:     *inputHash,
		SourceCommit:  *sourceCommit,
		WorkflowRunID: *workflowRunID,
	}
	evidence, err := appendEmergencyLineage(*evidencePath, *snapshotPath, *gatewayVersion, *logicalID, *version, *image, record)
	if err != nil {
		return err
	}
	return writeCanonical(*outputPath, evidence)
}

func appendEmergencyLineage(evidencePath, snapshotPath, gatewayVersion, logicalID, version, image string, record EmergencyLineage) (CandidateEvidenceFile, error) {
	gateway, err := parseSemver(gatewayVersion)
	if err != nil || gateway.prerelease != "" {
		return CandidateEvidenceFile{}, errors.New("gateway version must be stable SemVer")
	}
	pluginVersion, err := parseSemver(version)
	if err != nil || pluginVersion.prerelease != "" {
		return CandidateEvidenceFile{}, errors.New("plugin version must be stable SemVer")
	}
	if !safeIDPattern.MatchString(logicalID) {
		return CandidateEvidenceFile{}, errors.New("logical plugin ID is invalid")
	}
	if !strings.HasPrefix(image, "plugins/") || !safeIDPattern.MatchString(strings.TrimPrefix(image, "plugins/")) {
		return CandidateEvidenceFile{}, errors.New("plugin image must be a safe plugins/<name> path")
	}
	if err := validateEmergencyLineageRecord(record); err != nil {
		return CandidateEvidenceFile{}, fmt.Errorf("new emergency lineage: %w", err)
	}

	var evidence CandidateEvidenceFile
	if _, err := readJSON(evidencePath, &evidence); err != nil {
		return CandidateEvidenceFile{}, err
	}
	candidate, ok := evidence.Plugins[logicalID]
	if !ok {
		return CandidateEvidenceFile{}, fmt.Errorf("candidate evidence does not contain requested plugin %s", logicalID)
	}
	if !digestPattern.MatchString(candidate.Digest) || !commitPattern.MatchString(candidate.SourceCommit) ||
		!digestPattern.MatchString(candidate.InputHash) || !strings.HasSuffix(candidate.CandidateRef, "@"+candidate.Digest) {
		return CandidateEvidenceFile{}, fmt.Errorf("candidate evidence for %s has invalid immutable provenance", logicalID)
	}
	seenRuns := map[string]EmergencyLineage{}
	for i, existing := range candidate.Lineage {
		if err := validateEmergencyLineageRecord(existing); err != nil {
			return CandidateEvidenceFile{}, fmt.Errorf("candidate evidence lineage %d for %s: %w", i, logicalID, err)
		}
		if prior, duplicate := seenRuns[existing.WorkflowRunID]; duplicate {
			if prior != existing {
				return CandidateEvidenceFile{}, fmt.Errorf("candidate evidence contains conflicting records for workflow run ID %s", existing.WorkflowRunID)
			}
			return CandidateEvidenceFile{}, fmt.Errorf("candidate evidence contains duplicate records for workflow run ID %s", existing.WorkflowRunID)
		}
		seenRuns[existing.WorkflowRunID] = existing
	}

	var snapshot Snapshot
	if _, err := readJSON(snapshotPath, &snapshot); err != nil {
		return CandidateEvidenceFile{}, err
	}
	if snapshot.SchemaVersion != snapshotSchemaVersion || snapshot.GatewayVersion != gatewayVersion {
		return CandidateEvidenceFile{}, errors.New("snapshot does not match the requested gateway version")
	}
	var binding *SnapshotEntry
	for i := range snapshot.Plugins {
		if snapshot.Plugins[i].LogicalID != logicalID {
			continue
		}
		if binding != nil {
			return CandidateEvidenceFile{}, fmt.Errorf("snapshot contains duplicate plugin %s", logicalID)
		}
		binding = &snapshot.Plugins[i]
	}
	if binding == nil {
		return CandidateEvidenceFile{}, fmt.Errorf("snapshot does not contain requested plugin %s", logicalID)
	}
	expectedSuffix := "/" + image + ":" + version
	if binding.Version != version || binding.Image != image || !strings.HasSuffix(binding.OCIRef, expectedSuffix) ||
		binding.Digest != candidate.Digest || binding.InputHash != candidate.InputHash ||
		binding.SourceCommit != candidate.SourceCommit || binding.CandidateRef != candidate.CandidateRef {
		return CandidateEvidenceFile{}, fmt.Errorf("candidate evidence and snapshot do not contain the requested %s/%s:%s public image binding", gatewayVersion, logicalID, version)
	}

	if existing, exists := seenRuns[record.WorkflowRunID]; exists {
		if existing != record {
			return CandidateEvidenceFile{}, fmt.Errorf("workflow run ID %s already records different emergency lineage", record.WorkflowRunID)
		}
		return evidence, nil
	}
	candidate.Lineage = append(candidate.Lineage, record)
	evidence.Plugins[logicalID] = candidate
	return evidence, nil
}

func validateEmergencyLineageRecord(record EmergencyLineage) error {
	if !digestPattern.MatchString(record.Digest) {
		return errors.New("digest must be a lowercase sha256 digest")
	}
	if !digestPattern.MatchString(record.InputHash) {
		return errors.New("inputHash must be a lowercase sha256 digest")
	}
	if !commitPattern.MatchString(record.SourceCommit) {
		return errors.New("sourceCommit must be a full lowercase Git commit")
	}
	if !workflowRunIDPattern.MatchString(record.WorkflowRunID) {
		return errors.New("workflowRunId must be a positive decimal GitHub Actions run ID")
	}
	return nil
}
