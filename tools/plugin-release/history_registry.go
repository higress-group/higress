package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
)

var ociWasmHashResolver = resolveOCIWasmHash

func resolveOCIWasmHash(root string, entry SnapshotEntry, ref string) (string, error) {
	if !digestPattern.MatchString(entry.Digest) || !registryQualifiedReferencePattern.MatchString(ref) && !validPinnedCandidateReference(ref, entry.Digest) {
		return "", errors.New("invalid artifact read reference")
	}
	repository := strings.Split(ref, "@")[0]
	if strings.LastIndex(repository, ":") > strings.LastIndex(repository, "/") {
		repository = repository[:strings.LastIndex(repository, ":")]
	}
	pinned := repository + "@" + entry.Digest
	directory, err := os.MkdirTemp("", "plugin-readonly-*")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(directory)
	if _, _, err := runORAS("cp", pinned, "--to-oci-layout", directory); err != nil {
		return "", err
	}
	var index struct {
		Manifests []pulledDescriptor `json:"manifests"`
	}
	indexData, err := os.ReadFile(filepath.Join(directory, "index.json"))
	if err != nil {
		return "", err
	}
	if err := json.Unmarshal(indexData, &index); err != nil {
		return "", err
	}
	if len(index.Manifests) != 1 || index.Manifests[0].Digest != entry.Digest {
		return "", errors.New("pulled OCI layout digest mismatch")
	}
	readBlob := func(descriptor pulledDescriptor) ([]byte, error) {
		if !digestPattern.MatchString(descriptor.Digest) {
			return nil, errors.New("invalid blob digest")
		}
		data, err := os.ReadFile(filepath.Join(directory, "blobs/sha256", strings.TrimPrefix(descriptor.Digest, "sha256:")))
		if err != nil {
			return nil, err
		}
		if "sha256:"+sha256Hex(data) != descriptor.Digest || int64(len(data)) != descriptor.Size {
			return nil, errors.New("pulled blob digest or size mismatch")
		}
		return data, nil
	}
	manifestData, err := readBlob(index.Manifests[0])
	if err != nil {
		return "", err
	}
	var manifest pulledPluginManifest
	if err := json.Unmarshal(manifestData, &manifest); err != nil {
		return "", err
	}
	var wasm []byte
	var config []byte
	wasmCount := 0
	for _, descriptor := range manifest.Layers {
		data, err := readBlob(descriptor)
		if err != nil {
			return "", err
		}
		switch descriptor.MediaType {
		case wasmConfigMediaType:
			config = data
		case wasmContentMediaType:
			wasm, wasmCount = data, wasmCount+1
		case "application/vnd.docker.image.rootfs.diff.tar.gzip", "application/vnd.oci.image.layer.v1.tar+gzip":
			wasm, err = wasmFromTar(data)
			if err != nil {
				return "", err
			}
			wasmCount++
		}
	}
	if wasmCount != 1 || len(wasm) < 8 || !bytes.Equal(wasm[:8], []byte{0, 'a', 's', 'm', 1, 0, 0, 0}) {
		return "", errors.New("artifact must contain exactly one valid Wasm module")
	}
	if entryProvenance(entry) == "candidate" {
		created, err := runGit(root, "show", "-s", "--format=%cI", entry.SourceCommit)
		if err != nil {
			return "", err
		}
		manifestPath, configPath, wasmPath := filepath.Join(directory, "manifest.json"), filepath.Join(directory, "config.json"), filepath.Join(directory, "plugin.wasm")
		for filename, data := range map[string][]byte{manifestPath: manifestData, configPath: config, wasmPath: wasm} {
			if err := os.WriteFile(filename, data, 0o600); err != nil {
				return "", err
			}
		}
		if err := verifyPulledPlugin(manifestPath, configPath, wasmPath, entry.Digest, entry.SourceCommit, created, entry.Version, entry.InputHash, "candidate"); err != nil {
			return "", err
		}
	}
	hash := sha256Hex(wasm)
	if entry.WasmSHA256 != "" && entry.WasmSHA256 != hash {
		return "", errors.New("resolved Wasm bytes do not match wasmSha256")
	}
	return hash, nil
}

func wasmFromTar(data []byte) ([]byte, error) {
	reader, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	defer reader.Close()
	archive := tar.NewReader(reader)
	var wasm []byte
	for {
		header, err := archive.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		if path.IsAbs(header.Name) || path.Clean(header.Name) != header.Name || strings.Contains(header.Name, "\\") || strings.HasPrefix(header.Name, "../") || header.Typeflag == tar.TypeSymlink || header.Typeflag == tar.TypeLink {
			return nil, errors.New("unsafe legacy OCI tar entry")
		}
		if header.Typeflag != tar.TypeReg || !strings.HasSuffix(header.Name, ".wasm") {
			continue
		}
		if wasm != nil || header.Size > 64*1024*1024 {
			return nil, errors.New("ambiguous or oversized legacy Wasm layer")
		}
		wasm, err = io.ReadAll(io.LimitReader(archive, 64*1024*1024+1))
		if err != nil {
			return nil, err
		}
	}
	if wasm == nil {
		return nil, errors.New("legacy OCI tar contains no Wasm")
	}
	return wasm, nil
}

func calibrateHistoryProbe(previous Snapshot, evidence *HistoryEvidence) error {
	if !matchesHistoryControl(previous, evidence) {
		return errors.New("history control does not bind a previous non-excluded entry")
	}
	manifest, err := ociManifestResolver(evidence.ControlRef)
	if err != nil {
		return fmt.Errorf("history control calibration failed: %w", err)
	}
	if manifest.Digest != evidence.ControlDigest {
		return errors.New("history control digest mismatch")
	}
	return nil
}

func classifyHistoryCurrent(root string, entry SnapshotEntry) (HistoryEvidenceEntry, error) {
	record := HistoryEvidenceEntry{Version: entry.Version, Digest: entry.Digest, WasmSHA256: entry.WasmSHA256}
	if marker := entry.Migration; marker != nil {
		record.Publication, record.ObservedDigest, record.ObservedVersion, record.ObservedRevision, record.ObservedInputHash = "excluded", marker.ExistingDigest, marker.ExistingVersion, marker.ExistingRevision, marker.ExistingInputHash
		return record, nil
	}
	manifest, err := ociManifestResolver(entry.OCIRef)
	if err != nil {
		if classifyOCIFailure(err, entry.OCIRef) == ociFailureNotFound && entryProvenance(entry) == "candidate" {
			record.Publication = "absent"
			return record, nil
		}
		return record, fmt.Errorf("history %s cannot classify public artifact: %w", entry.LogicalID, err)
	}
	if manifest.Digest != entry.Digest {
		return record, errors.New("history public tag points to a different digest")
	}
	if entryProvenance(entry) == "candidate" && (manifest.Annotations["org.opencontainers.image.version"] != entry.Version || manifest.Annotations["org.opencontainers.image.revision"] != entry.SourceCommit || manifest.Annotations["io.higress.plugin.input-hash"] != entry.InputHash) {
		return record, errors.New("history public candidate annotations mismatch")
	}
	record.Publication, record.ObservedDigest = "published", manifest.Digest
	record.ObservedVersion, record.ObservedRevision, record.ObservedInputHash = manifest.Annotations["org.opencontainers.image.version"], manifest.Annotations["org.opencontainers.image.revision"], manifest.Annotations["io.higress.plugin.input-hash"]
	record.WasmSHA256, err = ociWasmHashResolver(root, entry, entry.OCIRef)
	return record, err
}

func captureHistoryEvidence(root, previousPath string, plan Plan, previous Snapshot) (*HistoryEvidence, error) {
	if len(historySubjects(plan, previous)) == 0 {
		return nil, nil
	}
	candidates, err := migrationControlTags(previousPath)
	if err != nil {
		return nil, err
	}
	evidence := &HistoryEvidence{SchemaVersion: 1, Entries: map[string]HistoryEvidenceEntry{}}
	for _, candidate := range candidates {
		manifest, err := ociManifestResolver(candidate.ref)
		if err != nil {
			if classifyOCIFailure(err, candidate.ref) == ociFailureNotFound {
				continue
			}
			return nil, err
		}
		if manifest.Digest != candidate.digest {
			return nil, errors.New("history calibration digest conflict")
		}
		evidence.ControlRef, evidence.ControlDigest = candidate.ref, candidate.digest
		break
	}
	if evidence.ControlRef == "" {
		return nil, errors.New("no calibratable public history control")
	}
	for id, entry := range historySubjects(plan, previous) {
		record, err := classifyHistoryCurrent(root, entry)
		if err != nil {
			return nil, err
		}
		evidence.Entries[id] = record
	}
	if err := validateHistoryEvidence(plan, previous, evidence); err != nil {
		return nil, err
	}
	return evidence, nil
}

func replayHistoryEvidence(root string, plan Plan, previous Snapshot, evidence *HistoryEvidence) error {
	if err := validateHistoryEvidence(plan, previous, evidence); err != nil {
		return err
	}
	if evidence == nil {
		return nil
	}
	if err := calibrateHistoryProbe(previous, evidence); err != nil {
		return err
	}
	for id, entry := range historySubjects(plan, previous) {
		actual, err := classifyHistoryCurrent(root, entry)
		if err != nil {
			return err
		}
		expected := evidence.Entries[id]
		if expected.Publication != "published" && expected.WasmSHA256 != "" && entry.WasmSHA256 == "" {
			hash, err := ociWasmHashResolver(root, entry, entry.CandidateRef)
			if err != nil {
				return err
			}
			if hash != expected.WasmSHA256 {
				return fmt.Errorf("%s: optional unpublished Wasm hash does not match candidate bytes", id)
			}
		}
		if actual.Publication != expected.Publication || actual.ObservedDigest != expected.ObservedDigest || actual.ObservedVersion != expected.ObservedVersion || actual.ObservedRevision != expected.ObservedRevision || actual.ObservedInputHash != expected.ObservedInputHash || actual.Publication == "published" && actual.WasmSHA256 != expected.WasmSHA256 {
			return fmt.Errorf("%s: history evidence classification does not replay against registry", id)
		}
	}
	return nil
}
