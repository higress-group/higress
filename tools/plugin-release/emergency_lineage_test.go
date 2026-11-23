// Copyright 2026 Higress Authors
// Licensed under the Apache License, Version 2.0.

package main

import (
	"reflect"
	"strings"
	"testing"
)

func TestAppendEmergencyLineagePreservesCandidateEvidenceAndIsIdempotent(t *testing.T) {
	evidencePath, snapshotPath, original, record := emergencyLineageFixture(t)
	updated, err := appendEmergencyLineage(evidencePath, snapshotPath, "2.2.6", "demo", "1.2.3", "plugins/demo", record)
	if err != nil {
		t.Fatal(err)
	}
	candidate := updated.Plugins["demo"]
	if candidate.CandidateRef != original.CandidateRef || candidate.Digest != original.Digest ||
		candidate.SourceCommit != original.SourceCommit || candidate.InputHash != original.InputHash {
		t.Fatalf("append changed immutable candidate evidence: before=%#v after=%#v", original, candidate)
	}
	if len(candidate.Lineage) != 1 || candidate.Lineage[0] != record {
		t.Fatalf("emergency lineage was not appended exactly: %#v", candidate.Lineage)
	}
	if !reflect.DeepEqual(updated.Plugins["other"], CandidateEvidence{
		CandidateRef: "registry.example/candidates/other@" + testDigest("other"),
		Digest:       testDigest("other"), SourceCommit: strings.Repeat("c", 40), InputHash: testDigest("other-input"),
	}) {
		t.Fatalf("append changed unrelated candidate evidence: %#v", updated.Plugins["other"])
	}

	if err := writeCanonical(evidencePath, updated); err != nil {
		t.Fatal(err)
	}
	retried, err := appendEmergencyLineage(evidencePath, snapshotPath, "2.2.6", "demo", "1.2.3", "plugins/demo", record)
	if err != nil {
		t.Fatalf("exact retry was not idempotent: %v", err)
	}
	if !reflect.DeepEqual(retried, updated) {
		t.Fatalf("exact retry changed evidence:\nfirst=%#v\nretry=%#v", updated, retried)
	}
}

func TestAppendEmergencyLineageRejectsMalformedCollisionAndBindingDrift(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(t *testing.T, evidencePath, snapshotPath string, record *EmergencyLineage)
		want   string
	}{
		{
			name: "malformed-digest",
			mutate: func(_ *testing.T, _, _ string, record *EmergencyLineage) {
				record.Digest = "sha256:nope"
			},
			want: "digest must",
		},
		{
			name: "malformed-run-id",
			mutate: func(_ *testing.T, _, _ string, record *EmergencyLineage) {
				record.WorkflowRunID = "run-42"
			},
			want: "workflowRunId",
		},
		{
			name: "run-id-collision",
			mutate: func(t *testing.T, evidencePath, _ string, record *EmergencyLineage) {
				var evidence CandidateEvidenceFile
				if _, err := readJSON(evidencePath, &evidence); err != nil {
					t.Fatal(err)
				}
				candidate := evidence.Plugins["demo"]
				candidate.Lineage = []EmergencyLineage{{
					Digest: testDigest("other-emergency"), InputHash: record.InputHash,
					SourceCommit: record.SourceCommit, WorkflowRunID: record.WorkflowRunID,
				}}
				evidence.Plugins["demo"] = candidate
				if err := writeCanonical(evidencePath, evidence); err != nil {
					t.Fatal(err)
				}
			},
			want: "already records different",
		},
		{
			name: "snapshot-version-drift",
			mutate: func(t *testing.T, _, snapshotPath string, _ *EmergencyLineage) {
				var snapshot Snapshot
				if _, err := readJSON(snapshotPath, &snapshot); err != nil {
					t.Fatal(err)
				}
				snapshot.Plugins[0].Version = "1.2.4"
				if err := writeCanonical(snapshotPath, snapshot); err != nil {
					t.Fatal(err)
				}
			},
			want: "public image binding",
		},
		{
			name: "candidate-digest-drift",
			mutate: func(t *testing.T, evidencePath, _ string, _ *EmergencyLineage) {
				var evidence CandidateEvidenceFile
				if _, err := readJSON(evidencePath, &evidence); err != nil {
					t.Fatal(err)
				}
				candidate := evidence.Plugins["demo"]
				candidate.Digest = testDigest("drift")
				candidate.CandidateRef = "registry.example/candidates/demo@" + candidate.Digest
				evidence.Plugins["demo"] = candidate
				if err := writeCanonical(evidencePath, evidence); err != nil {
					t.Fatal(err)
				}
			},
			want: "public image binding",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			evidencePath, snapshotPath, _, record := emergencyLineageFixture(t)
			tc.mutate(t, evidencePath, snapshotPath, &record)
			_, err := appendEmergencyLineage(evidencePath, snapshotPath, "2.2.6", "demo", "1.2.3", "plugins/demo", record)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("invalid lineage or binding was accepted, err=%v", err)
			}
		})
	}
}

func emergencyLineageFixture(t *testing.T) (evidencePath, snapshotPath string, candidate CandidateEvidence, record EmergencyLineage) {
	t.Helper()
	root := t.TempDir()
	digest := testDigest("candidate")
	candidate = CandidateEvidence{
		CandidateRef: "registry.example/candidates/demo@" + digest,
		Digest:       digest,
		SourceCommit: strings.Repeat("a", 40),
		InputHash:    testDigest("candidate-input"),
	}
	evidence := CandidateEvidenceFile{Plugins: map[string]CandidateEvidence{
		"demo": candidate,
		"other": {
			CandidateRef: "registry.example/candidates/other@" + testDigest("other"),
			Digest:       testDigest("other"), SourceCommit: strings.Repeat("c", 40), InputHash: testDigest("other-input"),
		},
	}}
	snapshot := Snapshot{
		SchemaVersion: snapshotSchemaVersion, GatewayVersion: "2.2.6",
		Plugins: []SnapshotEntry{{
			LogicalID: "demo", Image: "plugins/demo", Version: "1.2.3",
			OCIRef: "registry.example/plugins/demo:1.2.3", Digest: candidate.Digest,
			InputHash: candidate.InputHash, SourceCommit: candidate.SourceCommit, CandidateRef: candidate.CandidateRef,
		}},
	}
	evidencePath = root + "/evidence.json"
	snapshotPath = root + "/snapshot.json"
	if err := writeCanonical(evidencePath, evidence); err != nil {
		t.Fatal(err)
	}
	if err := writeCanonical(snapshotPath, snapshot); err != nil {
		t.Fatal(err)
	}
	record = EmergencyLineage{
		Digest: testDigest("emergency"), InputHash: testDigest("emergency-input"),
		SourceCommit: strings.Repeat("b", 40), WorkflowRunID: "123456789",
	}
	return evidencePath, snapshotPath, candidate, record
}
