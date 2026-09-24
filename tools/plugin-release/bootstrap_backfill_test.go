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

import (
	"errors"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

func TestClassifyOCIFailureDistinguishesUnauthorizedFromAbsent(t *testing.T) {
	const aiCacheRef = "higress-registry.cn-hangzhou.cr.aliyuncs.com/candidates/ai-cache:5a150abf7a1a76129eef4e2d61927116a59c9141f87cdfb1d4a0b0f58e049a544bf98709658a92d40b7bda75b5f458387526143e09b321a401ff7a258fdea47f"
	const authWordRef = "registry.example/plugins/unauthorized-401-forbidden-403:1.0.0"
	cases := []struct {
		msg         string
		expectedRef string
		want        ociFailureClass
	}{
		{"401 Unauthorized", "", ociFailureUnauthorized},
		{"403 Forbidden", "", ociFailureUnauthorized},
		{"denied: requested access to the resource is denied", "", ociFailureUnauthorized},
		{"UNAUTHORIZED: authentication required", "", ociFailureUnauthorized},
		{"authorization required", "", ociFailureUnauthorized},
		{"response status code 401", "", ociFailureUnauthorized},
		{"HTTP/1.1 403", "", ociFailureUnauthorized},
		{"status: 403", "", ociFailureUnauthorized},
		{"registry error 401", "", ociFailureOther},
		{"backend code 403", "", ociFailureOther},
		{"404 Not Found", "", ociFailureNotFound},
		{"manifest unknown", "", ociFailureNotFound},
		{"name unknown", "", ociFailureNotFound},
		{"repository does not exist", "", ociFailureNotFound},
		{`Error response from registry: failed to find "higress-registry.cn-hangzhou.cr.aliyuncs.com/plugins/missing:1.0.0": higress-registry.cn-hangzhou.cr.aliyuncs.com/plugins/missing:1.0.0: not found`, "higress-registry.cn-hangzhou.cr.aliyuncs.com/plugins/missing:1.0.0", ociFailureNotFound},
		{`Error response from registry: higress-registry.cn-hangzhou.cr.aliyuncs.com/plugins/other:1.0.0: not found`, "higress-registry.cn-hangzhou.cr.aliyuncs.com/plugins/missing:1.0.0", ociFailureOther},
		{"Error response from registry: " + aiCacheRef + ": not found", aiCacheRef, ociFailureNotFound},
		{"transport error while resolving " + aiCacheRef + ": dial tcp: i/o timeout", aiCacheRef, ociFailureOther},
		{"Error response from registry: " + authWordRef + ": not found", authWordRef, ociFailureNotFound},
		{"transport error while resolving " + authWordRef, authWordRef, ociFailureOther},
		{"response status code 403 while resolving " + aiCacheRef, aiCacheRef, ociFailureUnauthorized},
		{"authentication required while resolving " + authWordRef, authWordRef, ociFailureUnauthorized},
		{"connection refused", "", ociFailureOther},
		{"i/o timeout", "", ociFailureOther},
		// A local executable/file failure or generic "not found" text is never
		// absence evidence: only explicit registry 404-class markers qualify.
		{"Error response from daemon: not found", "", ociFailureOther},
		{`exec: "oras": executable file not found in $PATH`, "", ociFailureOther},
		{"open /tmp/descriptor.json: no such file or directory", "", ociFailureOther},
		{"not found", "", ociFailureOther},
		{"Error response from registry: not found", "", ociFailureOther},
		{"Error response from registry: /tmp/plugins/missing:1.0.0: not found", "/tmp/plugins/missing:1.0.0", ociFailureOther},
		// Structured authorization always wins over absence, even when the
		// registry also says "unknown".
		{"HTTP 403: manifest unknown", "", ociFailureUnauthorized},
		{"response status code 401: not found", "", ociFailureUnauthorized},
		{`Error response from registry: 403 Forbidden: higress-registry.cn-hangzhou.cr.aliyuncs.com/plugins/missing:1.0.0: not found`, "higress-registry.cn-hangzhou.cr.aliyuncs.com/plugins/missing:1.0.0", ociFailureUnauthorized},
	}
	for _, tc := range cases {
		if got := classifyOCIFailure(errors.New(tc.msg), tc.expectedRef); got != tc.want {
			t.Fatalf("classifyOCIFailure(%q) = %d, want %d", tc.msg, got, tc.want)
		}
	}
}

func TestCaptureBootstrapEvidenceDefersAlphaWithoutResolution(t *testing.T) {
	root, catalog, commit := backfillRepo(t, map[string]string{"alpha": "1.0.0-alpha.1", "demo": "1.0.0"})
	digest := "sha256:" + strings.Repeat("a", 64)
	withManifestResolver(t, func(ref string) (ociManifest, error) {
		if strings.HasPrefix(ref, "registry.example/plugins/alpha:") {
			t.Fatalf("deferred alpha prerelease must never be resolved, got %q", ref)
		}
		if ref != "registry.example/plugins/demo:1.0.0" {
			t.Fatalf("resolved %q, want the stable demo ref", ref)
		}
		return ociManifest{Digest: digest}, nil
	})
	evidence, err := captureBootstrapEvidence(root, catalog, commit)
	if err != nil {
		t.Fatal(err)
	}
	if got := evidence.Plugins["alpha"]; got.Status != "deferred" || got.Version != "1.0.0-alpha.1" || got.PublicRef != "" || got.Digest != "" {
		t.Fatalf("alpha prerelease must be deferred without resolution: %#v", got)
	}
	if got := evidence.Plugins["demo"]; got.Status != "public" || got.Version != "1.0.0" || got.PublicRef != "registry.example/plugins/demo:1.0.0" || got.Digest != digest {
		t.Fatalf("stable public artifact lost reviewed provenance: %#v", got)
	}
}

func TestCaptureBootstrapEvidenceMarksStableAbsentForBackfill(t *testing.T) {
	root, catalog, commit := backfillRepo(t, map[string]string{"demo": "1.0.0"})
	withManifestResolver(t, func(string) (ociManifest, error) {
		return ociManifest{}, errors.New("404: manifest unknown")
	})
	evidence, err := captureBootstrapEvidence(root, catalog, commit)
	if err != nil {
		t.Fatal(err)
	}
	got := evidence.Plugins["demo"]
	if got.Status != "missing" || got.Version != "1.0.0" || got.PublicRef != "registry.example/plugins/demo:1.0.0" || got.Digest != "" {
		t.Fatalf("stable absent artifact must be classified missing for backfill: %#v", got)
	}
}

func TestCaptureBootstrapEvidenceFailsClosedForNonAlphaPrereleaseAbsent(t *testing.T) {
	root, catalog, commit := backfillRepo(t, map[string]string{"demo": "1.0.0-beta.1"})
	withManifestResolver(t, func(string) (ociManifest, error) {
		return ociManifest{}, errors.New("manifest unknown")
	})
	if _, err := captureBootstrapEvidence(root, catalog, commit); err == nil || !strings.Contains(err.Error(), "only stable versions may be backfilled") {
		t.Fatalf("absent non-alpha prerelease must fail closed, got %v", err)
	}
}

func TestCaptureBootstrapEvidenceTreatsUnauthorizedAsConfigurationError(t *testing.T) {
	for _, msg := range []string{"401 Unauthorized", "denied: requested access to the resource is denied"} {
		root, catalog, commit := backfillRepo(t, map[string]string{"demo": "1.0.0"})
		withManifestResolver(t, func(string) (ociManifest, error) {
			return ociManifest{}, errors.New(msg)
		})
		if _, err := captureBootstrapEvidence(root, catalog, commit); err == nil || !strings.Contains(err.Error(), "never an absent artifact") {
			t.Fatalf("%q must abort capture as an authorization/configuration error, got %v", msg, err)
		}
	}
}

func TestBootstrapBackfillProducesMixedSnapshotVerifiedFromBothSources(t *testing.T) {
	root, catalog, base := backfillRepo(t, map[string]string{"alpha": "1.0.0-alpha", "miss": "1.0.0", "pub": "1.0.0"})
	mustWrite(t, filepath.Join(root, "release-notes", "notes.md"), "notes\n")
	mustRun(t, root, "git", "add", ".")
	mustRun(t, root, "git", "commit", "-q", "-m", "unrelated")
	target, _ := resolveCommit(root, "HEAD")
	pubDigest := "sha256:" + strings.Repeat("a", 64)
	candidateDigest := "sha256:" + strings.Repeat("c", 64)
	candidateRef := "registry.example/candidates/miss@" + candidateDigest

	withManifestResolver(t, func(ref string) (ociManifest, error) {
		switch ref {
		case "registry.example/plugins/pub:1.0.0":
			return ociManifest{Digest: pubDigest}, nil
		case "registry.example/plugins/miss:1.0.0":
			return ociManifest{}, errors.New("404: manifest unknown")
		default:
			t.Fatalf("unexpected bootstrap resolve %q", ref)
			return ociManifest{}, nil
		}
	})
	evidence, err := captureBootstrapEvidence(root, catalog, target)
	if err != nil {
		t.Fatal(err)
	}
	evidencePath := filepath.Join(root, "bootstrap-evidence.json")
	if err := writeCanonical(evidencePath, evidence); err != nil {
		t.Fatal(err)
	}
	bootstrapPath := filepath.Join(root, "bootstrap.json")
	if err := commandBootstrap([]string{"--root", root, "--catalog", catalog, "--gateway-version", "2.0.0", "--source", target, "--existing-evidence", evidencePath, "--output", bootstrapPath}); err != nil {
		t.Fatal(err)
	}
	var bootstrap Snapshot
	if _, err := readJSON(bootstrapPath, &bootstrap); err != nil {
		t.Fatal(err)
	}
	if bootstrap.ProvenanceMode != "bootstrap-public" || len(bootstrap.Plugins) != 1 || bootstrap.Plugins[0].LogicalID != "pub" {
		t.Fatalf("bootstrap must import only the resolvable public artifact: %#v", bootstrap.Plugins)
	}

	plan, err := buildPlan(root, catalog, bootstrapPath, base, target, "2.0.1", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Plugins) != 1 || plan.Plugins[0].LogicalID != "miss" || !plan.Plugins[0].Backfill ||
		plan.Plugins[0].Version != "1.0.0" || plan.Plugins[0].PreviousVersion != "" {
		t.Fatalf("stable absent plugin must be planned exactly once as a backfill: %#v", plan.Plugins)
	}
	if len(plan.Deferred) != 1 || plan.Deferred[0].LogicalID != "alpha" || plan.Deferred[0].Reason != "alpha-prerelease" {
		t.Fatalf("alpha plugin must be deferred, not planned: %#v", plan.Deferred)
	}
	planPath := filepath.Join(root, "plan.json")
	if err := writeCanonical(planPath, plan); err != nil {
		t.Fatal(err)
	}
	candidatePath := filepath.Join(root, "candidates.json")
	if err := writeCanonical(candidatePath, CandidateEvidenceFile{Plugins: map[string]CandidateEvidence{
		"miss": {CandidateRef: candidateRef, Digest: candidateDigest, SourceCommit: target, InputHash: plan.Plugins[0].InputHash},
	}}); err != nil {
		t.Fatal(err)
	}
	mixed, err := renderSnapshot(catalog, planPath, bootstrapPath, candidatePath, evidencePath)
	if err != nil {
		t.Fatal(err)
	}
	if mixed.ProvenanceMode != "mixed" || len(mixed.Plugins) != 2 {
		t.Fatalf("backfill snapshot must mix historical public and candidate provenance: %#v", mixed)
	}
	backfill, carried := mixed.Plugins[0], mixed.Plugins[1]
	if backfill.LogicalID != "miss" || !backfill.Backfill || backfill.ProvenanceMode != "candidate" ||
		backfill.CandidateRef != candidateRef || backfill.Digest != candidateDigest || backfill.OCIRef != "registry.example/plugins/miss:1.0.0" {
		t.Fatalf("backfill entry lost candidate provenance: %#v", backfill)
	}
	if carried.LogicalID != "pub" || carried.Backfill || carried.ProvenanceMode != "public" || carried.Digest != pubDigest || carried.CandidateRef != "" {
		t.Fatalf("unchanged public artifact must carry forward without candidate provenance: %#v", carried)
	}
	// The first managed release must name the exact committed bootstrap
	// evidence the preparation PR carries; validation never infers bootstrap
	// mode from a missing previous-snapshot file.
	if mixed.BootstrapEvidence == nil || mixed.BootstrapEvidence.Path != "plugins/release/bootstrap-evidence/2.0.1.json" ||
		mixed.BootstrapEvidence.SHA256 != sha256Hex(mustRead(t, evidencePath)) {
		t.Fatalf("first managed release must carry the deterministic committed bootstrap evidence marker: %#v", mixed.BootstrapEvidence)
	}
	committedEvidence := filepath.Join(root, "plugins/release/bootstrap-evidence/2.0.1.json")
	if err := writeCanonical(committedEvidence, evidence); err != nil {
		t.Fatal(err)
	}
	mixedPath := filepath.Join(root, "mixed.json")
	if err := writeCanonical(mixedPath, mixed); err != nil {
		t.Fatal(err)
	}
	annotations := map[string]string{
		"org.opencontainers.image.revision": target,
		"io.higress.plugin.input-hash":      backfill.InputHash,
		"org.opencontainers.image.version":  "1.0.0",
	}

	// PR validation resolves only candidate references; the historical public
	// entry is deliberately not re-resolved or annotated there. The bootstrap
	// baseline itself is not committed, so the carried public entry is bound by
	// the committed evidence marker rather than a previous snapshot.
	withManifestResolver(t, func(ref string) (ociManifest, error) {
		if ref != candidateRef {
			t.Fatalf("candidate validation resolved %q, want only the backfill candidate", ref)
		}
		return ociManifest{Digest: candidateDigest, Annotations: annotations}, nil
	})
	if err := verifySnapshotBindings(root, catalog, mixedPath, planPath, "", target, target, true, "candidate"); err != nil {
		t.Fatalf("mixed snapshot must pass candidate PR validation: %v", err)
	}

	// Post-promotion verification resolves every public tag; the backfilled
	// tag serves the copied candidate content with its provenance annotations.
	withManifestResolver(t, func(ref string) (ociManifest, error) {
		switch ref {
		case "registry.example/plugins/pub:1.0.0":
			return ociManifest{Digest: pubDigest}, nil
		case "registry.example/plugins/miss:1.0.0":
			return ociManifest{Digest: candidateDigest, Annotations: annotations}, nil
		default:
			t.Fatalf("unexpected public resolve %q", ref)
			return ociManifest{}, nil
		}
	})
	if err := verifySnapshotBindings(root, catalog, mixedPath, planPath, "", target, target, true, "public"); err != nil {
		t.Fatalf("mixed snapshot must pass post-promotion public verification: %v", err)
	}

	withManifestResolver(t, func(string) (ociManifest, error) {
		return ociManifest{Digest: "sha256:" + strings.Repeat("d", 64)}, nil
	})
	if err := verifySnapshotBindings(root, catalog, mixedPath, planPath, "", target, target, true, "public"); err == nil {
		t.Fatal("a public tag drifting from the reviewed digest must fail closed")
	}
}

// mixedBackfillFixture renders the first managed release over a bootstrap
// baseline with one backfilled ("miss") and one carried public ("pub") plugin
// and commits the bootstrap evidence at its deterministic marker path.
func mixedBackfillFixture(t *testing.T) (root, catalog, planPath, snapshotPath string, snapshot Snapshot) {
	t.Helper()
	root, catalog, base := backfillRepo(t, map[string]string{"miss": "1.0.0", "pub": "1.0.0"})
	mustWrite(t, filepath.Join(root, "release-notes", "notes.md"), "notes\n")
	mustRun(t, root, "git", "add", ".")
	mustRun(t, root, "git", "commit", "-q", "-m", "unrelated")
	target, _ := resolveCommit(root, "HEAD")
	pubDigest := "sha256:" + strings.Repeat("a", 64)
	candidateDigest := "sha256:" + strings.Repeat("c", 64)
	candidateRef := "registry.example/candidates/miss@" + candidateDigest
	withManifestResolver(t, func(ref string) (ociManifest, error) {
		switch ref {
		case "registry.example/plugins/pub:1.0.0":
			return ociManifest{Digest: pubDigest}, nil
		case "registry.example/plugins/miss:1.0.0":
			return ociManifest{}, errors.New("404: manifest unknown")
		default:
			t.Fatalf("unexpected bootstrap resolve %q", ref)
			return ociManifest{}, nil
		}
	})
	evidence, err := captureBootstrapEvidence(root, catalog, target)
	if err != nil {
		t.Fatal(err)
	}
	evidencePath := filepath.Join(root, "bootstrap-evidence.json")
	if err := writeCanonical(evidencePath, evidence); err != nil {
		t.Fatal(err)
	}
	bootstrapPath := filepath.Join(root, "bootstrap.json")
	if err := commandBootstrap([]string{"--root", root, "--catalog", catalog, "--gateway-version", "2.0.0", "--source", target, "--existing-evidence", evidencePath, "--output", bootstrapPath}); err != nil {
		t.Fatal(err)
	}
	plan, err := buildPlan(root, catalog, bootstrapPath, base, target, "2.0.1", "")
	if err != nil {
		t.Fatal(err)
	}
	planPath = filepath.Join(root, "plan.json")
	if err := writeCanonical(planPath, plan); err != nil {
		t.Fatal(err)
	}
	candidatePath := filepath.Join(root, "candidates.json")
	if err := writeCanonical(candidatePath, CandidateEvidenceFile{Plugins: map[string]CandidateEvidence{
		"miss": {CandidateRef: candidateRef, Digest: candidateDigest, SourceCommit: target, InputHash: plan.Plugins[0].InputHash},
	}}); err != nil {
		t.Fatal(err)
	}
	mixed, err := renderSnapshot(catalog, planPath, bootstrapPath, candidatePath, evidencePath)
	if err != nil {
		t.Fatal(err)
	}
	if err := writeCanonical(filepath.Join(root, mixed.BootstrapEvidence.Path), evidence); err != nil {
		t.Fatal(err)
	}
	snapshotPath = filepath.Join(root, "mixed.json")
	if err := writeCanonical(snapshotPath, mixed); err != nil {
		t.Fatal(err)
	}
	return root, catalog, planPath, snapshotPath, mixed
}

func TestVerifySnapshotBindsBackfillExactlyBetweenPlanAndSnapshot(t *testing.T) {
	root, catalog, planPath, snapshotPath, mixed := mixedBackfillFixture(t)
	target := mixed.SourceCommit

	// Positive: exact plan binding plus the committed evidence marker verifies.
	if err := verifySnapshotBindings(root, catalog, snapshotPath, planPath, "", target, target, false, "candidate"); err != nil {
		t.Fatalf("exact plan/snapshot backfill binding must verify: %v", err)
	}

	writeSnapshot := func(mutate func(*Snapshot)) string {
		t.Helper()
		dup := mixed
		dup.Plugins = append([]SnapshotEntry(nil), mixed.Plugins...)
		if mixed.BootstrapEvidence != nil {
			marker := *mixed.BootstrapEvidence
			dup.BootstrapEvidence = &marker
		}
		mutate(&dup)
		path := filepath.Join(t.TempDir(), "snapshot.json")
		if err := writeCanonical(path, dup); err != nil {
			t.Fatal(err)
		}
		return path
	}
	writePlan := func(mutate func(*Plan)) string {
		t.Helper()
		var plan Plan
		if _, err := readJSON(planPath, &plan); err != nil {
			t.Fatal(err)
		}
		mutate(&plan)
		path := filepath.Join(t.TempDir(), "plan.json")
		if err := writeCanonical(path, plan); err != nil {
			t.Fatal(err)
		}
		return path
	}

	// Snapshot drops the backfill flag the plan recorded.
	tampered := writeSnapshot(func(s *Snapshot) { s.Plugins[0].Backfill = false })
	if err := verifySnapshotBindings(root, catalog, tampered, planPath, "", target, target, false, "candidate"); err == nil || !strings.Contains(err.Error(), "differs from its plan entry") {
		t.Fatalf("snapshot dropping a planned backfill must fail: %v", err)
	}
	// Plan drops the backfill flag the snapshot recorded.
	tamperedPlan := writePlan(func(p *Plan) { p.Plugins[0].Backfill = false })
	if err := verifySnapshotBindings(root, catalog, snapshotPath, tamperedPlan, "", target, target, false, "candidate"); err == nil || !strings.Contains(err.Error(), "planId does not match canonical plan content") {
		t.Fatalf("plan dropping a snapshotted backfill must fail: %v", err)
	}
	// A planned backfill without the committed evidence marker fails closed.
	unmarked := writeSnapshot(func(s *Snapshot) { s.BootstrapEvidence = nil })
	if err := verifySnapshotBindings(root, catalog, unmarked, planPath, "", target, target, false, "candidate"); err == nil || !strings.Contains(err.Error(), "no committed bootstrap evidence marker") {
		t.Fatalf("planned backfill without the marker must fail: %v", err)
	}
	// The marker must name the deterministic committed path.
	badPath := writeSnapshot(func(s *Snapshot) { s.BootstrapEvidence.Path = "plugins/release/bootstrap-evidence/other.json" })
	if err := verifySnapshotBindings(root, catalog, badPath, planPath, "", target, target, false, "candidate"); err == nil || !strings.Contains(err.Error(), "deterministic committed path") {
		t.Fatalf("a non-deterministic evidence path must fail: %v", err)
	}
	// The committed evidence bytes must match the marker digest exactly.
	badSHA := writeSnapshot(func(s *Snapshot) { s.BootstrapEvidence.SHA256 = "0" + s.BootstrapEvidence.SHA256[1:] })
	if err := verifySnapshotBindings(root, catalog, badSHA, planPath, "", target, target, false, "candidate"); err == nil || !strings.Contains(err.Error(), "does not match the snapshot marker sha256") {
		t.Fatalf("evidence bytes differing from the marker digest must fail: %v", err)
	}
	// The bootstrap baseline itself never carries the marker.
	baseline := writeSnapshot(func(s *Snapshot) { s.ProvenanceMode = "bootstrap-public" })
	if err := verifySnapshotBindings(root, catalog, baseline, planPath, "", target, target, false, "candidate"); err == nil || !strings.Contains(err.Error(), "never carries a bootstrap evidence marker") {
		t.Fatalf("a marked bootstrap-public snapshot must fail: %v", err)
	}
	// The marker is allowed only on the first managed release rendered from
	// the bootstrap baseline, never on a release with a managed predecessor.
	managedPrevious := writeSnapshot(func(s *Snapshot) {
		s.GatewayVersion = mixed.PreviousRelease
		s.PreviousRelease = ""
		s.BootstrapEvidence = nil
		s.ProvenanceMode = "mixed"
	})
	if err := verifySnapshotBindings(root, catalog, snapshotPath, planPath, managedPrevious, target, target, false, "candidate"); err == nil || !strings.Contains(err.Error(), "only on the first managed release") {
		t.Fatalf("marker with a managed previous snapshot must fail: %v", err)
	}
}

func TestVerifySnapshotBindsCarriedBackfillToPreviousRelease(t *testing.T) {
	root, catalog, _, snapshotPath, mixed := mixedBackfillFixture(t)
	target := mixed.SourceCommit

	// The next managed release plans nothing: both plugins carry forward, and
	// the historical backfill marker persists as provenance/migration state
	// without a new bootstrap evidence marker.
	_, catalogData, err := loadCatalog(catalog)
	if err != nil {
		t.Fatal(err)
	}
	plan := Plan{SchemaVersion: 1, GatewayVersion: "2.0.2", SourceCommit: target, PreviousRelease: mixed.GatewayVersion, CatalogSHA256: sha256Hex(catalogData)}
	plan.PlanID = "sha256:" + canonicalObjectHash(plan, true)
	planPath := filepath.Join(root, "plan2.json")
	if err := writeCanonical(planPath, plan); err != nil {
		t.Fatal(err)
	}
	evidence := filepath.Join(root, "candidates2.json")
	if err := writeCanonical(evidence, CandidateEvidenceFile{Plugins: map[string]CandidateEvidence{}}); err != nil {
		t.Fatal(err)
	}
	next, err := renderSnapshot(catalog, planPath, snapshotPath, evidence, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(next.Plugins) != 2 || !next.Plugins[0].Backfill || next.BootstrapEvidence != nil {
		t.Fatalf("carried backfill must persist as migration state without a new marker: %#v", next)
	}
	nextPath := filepath.Join(root, "next.json")
	if err := writeCanonical(nextPath, next); err != nil {
		t.Fatal(err)
	}
	if err := verifySnapshotBindings(root, catalog, nextPath, planPath, snapshotPath, target, target, false, "candidate"); err != nil {
		t.Fatalf("carried backfill bound to the previous release must verify: %v", err)
	}
	wrongPrevious := mixed
	wrongPrevious.GatewayVersion = "1.9.9"
	wrongPreviousPath := filepath.Join(t.TempDir(), "previous.json")
	if err := writeCanonical(wrongPreviousPath, wrongPrevious); err != nil {
		t.Fatal(err)
	}
	if err := verifySnapshotBindings(root, catalog, nextPath, planPath, wrongPreviousPath, target, target, false, "candidate"); err == nil || !strings.Contains(err.Error(), "exact stable previousRelease") {
		t.Fatalf("a different previous snapshot must not satisfy exact carry binding: %v", err)
	}

	// A snapshot-only backfill switch that neither plan nor previous release
	// recorded is rejected: backfill is not a mutable per-release flag.
	flipped := next
	flipped.Plugins = append([]SnapshotEntry(nil), next.Plugins...)
	flipped.Plugins[1].Backfill = true
	flippedPath := filepath.Join(root, "flipped.json")
	if err := writeCanonical(flippedPath, flipped); err != nil {
		t.Fatal(err)
	}
	if err := verifySnapshotBindings(root, catalog, flippedPath, planPath, snapshotPath, target, target, false, "candidate"); err == nil || !strings.Contains(err.Error(), "stable candidate provenance") {
		t.Fatalf("a snapshot-only backfill injection must fail: %v", err)
	}
	// Dropping the historical marker on carry is equally rejected.
	dropped := next
	dropped.Plugins = append([]SnapshotEntry(nil), next.Plugins...)
	dropped.Plugins[0].Backfill = false
	droppedPath := filepath.Join(root, "dropped.json")
	if err := writeCanonical(droppedPath, dropped); err != nil {
		t.Fatal(err)
	}
	if err := verifySnapshotBindings(root, catalog, droppedPath, planPath, snapshotPath, target, target, false, "candidate"); err == nil || !strings.Contains(err.Error(), "differs from the previous snapshot") {
		t.Fatalf("dropping carried backfill state must fail: %v", err)
	}
}

func TestBootstrapSnapshotRejectsStaleMissingEvidence(t *testing.T) {
	root, catalog, commit := backfillRepo(t, map[string]string{"demo": "1.0.0"})
	evidence := filepath.Join(root, "evidence.json")
	if err := writeCanonical(evidence, BootstrapEvidenceFile{Plugins: map[string]BootstrapEvidence{
		"demo": {Status: "missing", Version: "1.0.0", PublicRef: "registry.example/plugins/demo:1.0.0"},
	}}); err != nil {
		t.Fatal(err)
	}
	args := []string{"--root", root, "--catalog", catalog, "--gateway-version", "2.0.0", "--source", commit, "--existing-evidence", evidence, "--output", filepath.Join(root, "out.json")}
	withManifestResolver(t, func(string) (ociManifest, error) {
		return ociManifest{Digest: "sha256:" + strings.Repeat("a", 64)}, nil
	})
	if err := commandBootstrap(args); err == nil || !strings.Contains(err.Error(), "stale") {
		t.Fatalf("a public tag appearing after review must reject the stale evidence, got %v", err)
	}
	withManifestResolver(t, func(string) (ociManifest, error) {
		return ociManifest{}, errors.New("403 Forbidden")
	})
	if err := commandBootstrap(args); err == nil || strings.Contains(err.Error(), "stale") {
		t.Fatalf("an authorization failure must abort, never confirm absence: %v", err)
	}
}

func TestRenderSnapshotRejectsDeferredPlanAbuse(t *testing.T) {
	root, catalog, commit := backfillRepo(t, map[string]string{"demo": "1.0.0"})
	digest := "sha256:" + strings.Repeat("a", 64)
	evidence := filepath.Join(root, "evidence.json")
	if err := writeCanonical(evidence, CandidateEvidenceFile{Plugins: map[string]CandidateEvidence{}}); err != nil {
		t.Fatal(err)
	}
	newPlan := func(mutate func(*Plan)) string {
		plan := Plan{SchemaVersion: 1, GatewayVersion: "2.0.0", SourceCommit: commit, CatalogSHA256: sha256Hex(mustRead(t, catalog)), PlanID: digest}
		mutate(&plan)
		path := filepath.Join(t.TempDir(), "plan.json")
		if err := writeCanonical(path, plan); err != nil {
			t.Fatal(err)
		}
		return path
	}
	entry := PlanEntry{LogicalID: "demo", Implementation: "go", SourceDir: "plugins/wasm-go/extensions/demo", Image: "plugins/demo", Version: "1.0.0", InputHash: digest}

	plan := newPlan(func(p *Plan) {
		p.Plugins = []PlanEntry{entry}
		p.Deferred = []DeferredPlugin{{LogicalID: "demo", Version: "1.0.0", Reason: "alpha-prerelease"}}
	})
	if _, err := renderSnapshot(catalog, plan, "", evidence, ""); err == nil || !strings.Contains(err.Error(), "both plans and defers") {
		t.Fatalf("a plugin must not be both planned and deferred: %v", err)
	}
	plan = newPlan(func(p *Plan) {
		p.Deferred = []DeferredPlugin{{LogicalID: "demo", Version: "1.0.0", Reason: "manual"}}
	})
	if _, err := renderSnapshot(catalog, plan, "", evidence, ""); err == nil || !strings.Contains(err.Error(), "unsupported reason") {
		t.Fatalf("only alpha-prerelease deferral is supported: %v", err)
	}
	plan = newPlan(func(p *Plan) {
		p.Deferred = []DeferredPlugin{{LogicalID: "demo", Version: "1.0.0", Reason: "alpha-prerelease"}, {LogicalID: "ghost", Version: "1.0.0", Reason: "alpha-prerelease"}}
	})
	if _, err := renderSnapshot(catalog, plan, "", evidence, ""); err == nil || !strings.Contains(err.Error(), "unknown or release-ineligible") {
		t.Fatalf("deferral must reference release-eligible catalog plugins: %v", err)
	}
}

func TestDeferredAlphaCarriesForwardOnlyStablePreviousEntry(t *testing.T) {
	root, catalog, base := backfillRepo(t, map[string]string{"demo": "1.0.0"})
	digest := "sha256:" + strings.Repeat("a", 64)
	c, _, err := loadCatalog(catalog)
	if err != nil {
		t.Fatal(err)
	}
	p := c.Plugins[0]
	stableHash, err := inputHash(root, base, "1.0.0", c, p)
	if err != nil {
		t.Fatal(err)
	}
	previous := Snapshot{SchemaVersion: 1, GatewayVersion: "2.0.0", SourceCommit: base, CatalogSHA256: sha256Hex(mustRead(t, catalog)), PlanID: digest, ProvenanceMode: "candidate",
		Plugins: []SnapshotEntry{{LogicalID: p.LogicalID, Implementation: p.Implementation, SourceDir: p.SourceDir, Image: p.Image, Version: "1.0.0",
			OCIRef: "registry.example/plugins/demo:1.0.0", Digest: digest, InputHash: stableHash, SourceCommit: base,
			CandidateRef: "registry.example/candidates/demo@" + digest, ProvenanceMode: "candidate"}}}
	previousPath := filepath.Join(root, "previous.json")
	if err := writeCanonical(previousPath, previous); err != nil {
		t.Fatal(err)
	}

	// The plugin moves to an alpha VERSION: its inputs changed, yet no
	// candidate is planned and no new snapshot entry is created.
	mustWrite(t, filepath.Join(root, p.SourceDir, "VERSION"), "1.1.0-alpha\n")
	mustRun(t, root, "git", "add", ".")
	mustRun(t, root, "git", "commit", "-q", "-m", "alpha development")
	target, _ := resolveCommit(root, "HEAD")
	plan, err := buildPlan(root, catalog, previousPath, "", target, "2.0.1", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Plugins) != 0 || len(plan.Deferred) != 1 || plan.Deferred[0].LogicalID != "demo" || plan.Deferred[0].Version != "1.1.0-alpha" {
		t.Fatalf("changed alpha plugin must be deferred without a candidate: %#v %#v", plan.Plugins, plan.Deferred)
	}
	planPath := filepath.Join(root, "plan.json")
	if err := writeCanonical(planPath, plan); err != nil {
		t.Fatal(err)
	}
	evidence := filepath.Join(root, "evidence.json")
	if err := writeCanonical(evidence, CandidateEvidenceFile{Plugins: map[string]CandidateEvidence{}}); err != nil {
		t.Fatal(err)
	}
	snapshot, err := renderSnapshot(catalog, planPath, previousPath, evidence, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Plugins) != 1 || snapshot.Plugins[0].Version != "1.0.0" || snapshot.Plugins[0].InputHash != stableHash {
		t.Fatalf("deferred plugin must carry only its earlier stable release forward: %#v", snapshot.Plugins)
	}
	snapshotPath := filepath.Join(root, "snapshot.json")
	if err := writeCanonical(snapshotPath, snapshot); err != nil {
		t.Fatal(err)
	}
	if err := verifySnapshotBindings(root, catalog, snapshotPath, planPath, previousPath, target, target, false, "candidate"); err != nil {
		t.Fatalf("stable carry-forward for a deferred plugin must verify: %v", err)
	}

	omitted := snapshot
	omitted.Plugins = nil
	omittedPath := filepath.Join(root, "omitted.json")
	if err := writeCanonical(omittedPath, omitted); err != nil {
		t.Fatal(err)
	}
	if err := verifySnapshotBindings(root, catalog, omittedPath, planPath, previousPath, target, target, false, "candidate"); err == nil || !strings.Contains(err.Error(), "dropped its previous stable snapshot entry") {
		t.Fatalf("a deferred alpha plugin must carry its previous stable entry: %v", err)
	}

	verifyDeferredMutation := func(name string, mutate func(*Plan), want string) {
		t.Helper()
		mutatedPlan := plan
		mutatedPlan.Deferred = append([]DeferredPlugin(nil), plan.Deferred...)
		mutate(&mutatedPlan)
		mutatedPlan.PlanID = "sha256:" + canonicalObjectHash(mutatedPlan, true)
		mutatedPlanPath := filepath.Join(t.TempDir(), "plan.json")
		if err := writeCanonical(mutatedPlanPath, mutatedPlan); err != nil {
			t.Fatal(err)
		}
		mutatedSnapshot := snapshot
		mutatedSnapshot.PlanID = mutatedPlan.PlanID
		mutatedSnapshotPath := filepath.Join(t.TempDir(), "snapshot.json")
		if err := writeCanonical(mutatedSnapshotPath, mutatedSnapshot); err != nil {
			t.Fatal(err)
		}
		if err := verifySnapshotBindings(root, catalog, mutatedSnapshotPath, mutatedPlanPath, previousPath, target, target, false, "candidate"); err == nil || !strings.Contains(err.Error(), want) {
			t.Fatalf("%s must fail exact deferred binding: %v", name, err)
		}
	}
	verifyDeferredMutation("missing deferral", func(p *Plan) { p.Deferred = nil }, "omits the exact alpha deferral")
	verifyDeferredMutation("duplicate deferral", func(p *Plan) { p.Deferred = append(p.Deferred, p.Deferred[0]) }, "more than once")
	verifyDeferredMutation("wrong deferral version", func(p *Plan) { p.Deferred[0].Version = "1.1.0-alpha.2" }, "does not match the alpha prerelease")
	verifyDeferredMutation("wrong deferral reason", func(p *Plan) { p.Deferred[0].Reason = "manual" }, "does not match the alpha prerelease")

	tampered := snapshot
	tampered.Plugins[0].Version = "1.1.0-alpha"
	tampered.Plugins[0].OCIRef = "registry.example/plugins/demo:1.1.0-alpha"
	tamperedPath := filepath.Join(root, "tampered.json")
	if err := writeCanonical(tamperedPath, tampered); err != nil {
		t.Fatal(err)
	}
	if err := verifySnapshot(root, catalog, tamperedPath, target, target, false, "candidate"); err == nil || !strings.Contains(err.Error(), "deferred") {
		t.Fatalf("an alpha build must never become a snapshot entry: %v", err)
	}
}

func TestVerifySnapshotAllowsNewDeferredAlphaWithoutPreviousEntry(t *testing.T) {
	root, catalog, target := backfillRepo(t, map[string]string{"new-alpha": "1.0.0-alpha.1"})
	plan := Plan{
		SchemaVersion:  1,
		GatewayVersion: "2.0.0",
		SourceCommit:   target,
		CatalogSHA256:  sha256Hex(mustRead(t, catalog)),
		Deferred:       []DeferredPlugin{{LogicalID: "new-alpha", Version: "1.0.0-alpha.1", Reason: "alpha-prerelease"}},
	}
	plan.PlanID = "sha256:" + canonicalObjectHash(plan, true)
	planPath := filepath.Join(root, "plan.json")
	if err := writeCanonical(planPath, plan); err != nil {
		t.Fatal(err)
	}
	snapshot := Snapshot{SchemaVersion: 1, GatewayVersion: plan.GatewayVersion, SourceCommit: target, CatalogSHA256: plan.CatalogSHA256, PlanID: plan.PlanID, ProvenanceMode: "candidate"}
	snapshotPath := filepath.Join(root, "snapshot.json")
	if err := writeCanonical(snapshotPath, snapshot); err != nil {
		t.Fatal(err)
	}
	if err := verifySnapshotBindings(root, catalog, snapshotPath, planPath, "", target, target, false, "candidate"); err != nil {
		t.Fatalf("a new alpha plugin with no prior stable entry may remain absent: %v", err)
	}
}

func TestVerifySnapshotRejectsTamperedPlanAndPlannedEntryIdentity(t *testing.T) {
	root, catalog, planPath, snapshotPath, snapshot := mixedBackfillFixture(t)
	var originalPlan Plan
	if _, err := readJSON(planPath, &originalPlan); err != nil {
		t.Fatal(err)
	}
	writePair := func(mutatePlan func(*Plan), mutateSnapshot func(*Snapshot)) (string, string) {
		t.Helper()
		plan := originalPlan
		plan.Plugins = append([]PlanEntry(nil), originalPlan.Plugins...)
		plan.Deferred = append([]DeferredPlugin(nil), originalPlan.Deferred...)
		mutatePlan(&plan)
		plan.PlanID = "sha256:" + canonicalObjectHash(plan, true)
		planFile := filepath.Join(t.TempDir(), "plan.json")
		if err := writeCanonical(planFile, plan); err != nil {
			t.Fatal(err)
		}
		candidate := snapshot
		candidate.Plugins = append([]SnapshotEntry(nil), snapshot.Plugins...)
		candidate.PlanID = plan.PlanID
		mutateSnapshot(&candidate)
		snapshotFile := filepath.Join(t.TempDir(), "snapshot.json")
		if err := writeCanonical(snapshotFile, candidate); err != nil {
			t.Fatal(err)
		}
		return planFile, snapshotFile
	}
	assertFailure := func(name, planFile, snapshotFile, want string) {
		t.Helper()
		if err := verifySnapshotBindings(root, catalog, snapshotFile, planFile, "", snapshot.SourceCommit, snapshot.SourceCommit, false, "candidate"); err == nil || !strings.Contains(err.Error(), want) {
			t.Fatalf("%s must fail: %v", name, err)
		}
	}

	// The target differs from its parent only by release notes, so the artifact
	// hash still recomputes. Exact plan binding must nevertheless reject the
	// stale per-entry source commit.
	parent, err := resolveCommit(root, "HEAD^")
	if err != nil {
		t.Fatal(err)
	}
	planFile, snapshotFile := writePair(func(*Plan) {}, func(s *Snapshot) { s.Plugins[0].SourceCommit = parent })
	assertFailure("stale planned entry source commit", planFile, snapshotFile, "differs from its plan entry")

	planFile, snapshotFile = writePair(func(p *Plan) { p.Plugins = append(p.Plugins, p.Plugins[0]) }, func(*Snapshot) {})
	assertFailure("duplicate plan plugin", planFile, snapshotFile, "duplicate plugin")

	planFile, snapshotFile = writePair(func(p *Plan) { p.SchemaVersion = 2 }, func(*Snapshot) {})
	assertFailure("unsupported plan schema", planFile, snapshotFile, "unsupported schema")

	planFile, snapshotFile = writePair(func(p *Plan) { p.Plugins[0].Image = "plugins/other" }, func(*Snapshot) {})
	assertFailure("plan identity drift", planFile, snapshotFile, "differs from its plan entry")

	// A content edit without updating planId must be rejected before any weaker
	// field comparison can accept it.
	tampered := originalPlan
	tampered.Plugins = append([]PlanEntry(nil), originalPlan.Plugins...)
	tampered.Plugins[0].ChangedPaths = []string{"unreviewed/path"}
	tamperedPlanPath := filepath.Join(t.TempDir(), "plan.json")
	if err := writeCanonical(tamperedPlanPath, tampered); err != nil {
		t.Fatal(err)
	}
	assertFailure("tampered canonical plan", tamperedPlanPath, snapshotPath, "planId does not match canonical plan content")
}

func TestManagedReleasePlansNewCatalogPluginWithoutBackfill(t *testing.T) {
	root, catalog, base := backfillRepo(t, map[string]string{"demo": "1.0.0"})
	digest := "sha256:" + strings.Repeat("a", 64)
	c, _, err := loadCatalog(catalog)
	if err != nil {
		t.Fatal(err)
	}
	p := c.Plugins[0]
	stableHash, err := inputHash(root, base, "1.0.0", c, p)
	if err != nil {
		t.Fatal(err)
	}
	previous := Snapshot{SchemaVersion: 1, GatewayVersion: "2.0.0", SourceCommit: base, CatalogSHA256: sha256Hex(mustRead(t, catalog)), PlanID: digest, ProvenanceMode: "candidate",
		Plugins: []SnapshotEntry{{LogicalID: p.LogicalID, Implementation: p.Implementation, SourceDir: p.SourceDir, Image: p.Image, Version: "1.0.0",
			OCIRef: "registry.example/plugins/demo:1.0.0", Digest: digest, InputHash: stableHash, SourceCommit: base,
			CandidateRef: "registry.example/candidates/demo@" + digest, ProvenanceMode: "candidate"}}}
	previousPath := filepath.Join(root, "previous.json")
	if err := writeCanonical(previousPath, previous); err != nil {
		t.Fatal(err)
	}

	newDir := "plugins/wasm-go/extensions/newbie"
	mustWrite(t, filepath.Join(root, newDir, "main.go"), "package main\n")
	mustWrite(t, filepath.Join(root, newDir, "VERSION"), "1.0.0\n")
	mustRun(t, root, "git", "add", ".")
	mustRun(t, root, "git", "commit", "-q", "-m", "new catalog plugin")
	target, _ := resolveCommit(root, "HEAD")
	c.Plugins = append(c.Plugins, Plugin{LogicalID: "newbie", Implementation: "go", SourceDir: newDir, Image: "plugins/newbie", ReleaseEligible: true, ArtifactInputs: []string{newDir + "/**"}})
	if err := writeCanonical(catalog, c); err != nil {
		t.Fatal(err)
	}
	plan, err := buildPlan(root, catalog, previousPath, "", target, "2.0.1", "")
	if err != nil {
		t.Fatal(err)
	}
	// A new catalog plugin in a managed release is a genuine new release: it
	// is planned through the same candidate path but is not imported history,
	// so latest advances normally for it at promotion.
	if len(plan.Plugins) != 1 || plan.Plugins[0].LogicalID != "newbie" || plan.Plugins[0].Backfill {
		t.Fatalf("new managed-release plugin must be planned without backfill marking: %#v", plan.Plugins)
	}
	if len(plan.Deferred) != 0 {
		t.Fatalf("no plugin is deferred here: %#v", plan.Deferred)
	}
}

func backfillRepo(t *testing.T, versions map[string]string) (root, catalogPath, commit string) {
	t.Helper()
	root = t.TempDir()
	mustRun(t, root, "git", "init", "-q")
	mustRun(t, root, "git", "config", "user.name", "test")
	mustRun(t, root, "git", "config", "user.email", "test@example.com")
	names := make([]string, 0, len(versions))
	for name := range versions {
		names = append(names, name)
	}
	sort.Strings(names)
	plugins := make([]Plugin, 0, len(names))
	for _, name := range names {
		dir := "plugins/wasm-go/extensions/" + name
		mustWrite(t, filepath.Join(root, dir, "main.go"), "package main\n")
		mustWrite(t, filepath.Join(root, dir, "VERSION"), versions[name]+"\n")
		plugins = append(plugins, Plugin{LogicalID: name, Implementation: "go", SourceDir: dir, Image: "plugins/" + name, ReleaseEligible: true, ArtifactInputs: []string{dir + "/**"}})
	}
	mustWrite(t, filepath.Join(root, "plugins/wasm-rust/extensions/.keep"), "")
	mustRun(t, root, "git", "add", ".")
	mustRun(t, root, "git", "commit", "-q", "-m", "source")
	commit, _ = resolveCommit(root, "HEAD")
	catalogPath = filepath.Join(root, "catalog.json")
	c := Catalog{SchemaVersion: 1, Registry: "registry.example", Plugins: plugins}
	if err := writeCanonical(catalogPath, c); err != nil {
		t.Fatal(err)
	}
	return root, catalogPath, commit
}
