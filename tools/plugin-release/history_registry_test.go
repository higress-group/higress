package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestManagedResolveVerifiesActualOCIManifestAndWasmBytes(t *testing.T) {
	for _, mutation := range []string{"valid", "blob-tamper", "manifest-tamper", "wrong-wasm-hash", "missing-module", "annotation-mismatch"} {
		t.Run(mutation, func(t *testing.T) {
			root := t.TempDir()
			mustRun(t, root, "git", "init", "-q")
			mustRun(t, root, "git", "config", "user.name", "test")
			mustRun(t, root, "git", "config", "user.email", "test@example.com")
			mustWrite(t, filepath.Join(root, "source"), "fixed source\n")
			mustRun(t, root, "git", "add", ".")
			mustRun(t, root, "git", "commit", "-qm", "source")
			source, _ := resolveCommit(root, "HEAD")
			created, _ := runGit(root, "show", "-s", "--format=%cI", source)
			manifestPath, configPath, wasmPath, _ := writePulledPluginFixture(t)
			mutatePulledManifest(t, manifestPath, func(manifest map[string]any) {
				annotations := manifest["annotations"].(map[string]any)
				annotations["org.opencontainers.image.revision"] = source
				annotations["org.opencontainers.image.created"] = created
				if mutation == "annotation-mismatch" {
					annotations["io.higress.plugin.input-hash"] = "sha256:" + strings.Repeat("0", 64)
				}
			})
			manifestData := mustRead(t, manifestPath)
			manifestDigest := "sha256:" + sha256Hex(manifestData)
			var manifest pulledPluginManifest
			if err := json.Unmarshal(manifestData, &manifest); err != nil {
				t.Fatal(err)
			}
			layout := filepath.Join(root, "layout")
			mustCanonical(t, filepath.Join(layout, "index.json"), map[string]any{"manifests": []pulledDescriptor{{Digest: manifestDigest, Size: int64(len(manifestData)), MediaType: ociImageManifestMediaType}}})
			for digest, data := range map[string][]byte{manifestDigest: manifestData, manifest.Layers[0].Digest: mustRead(t, configPath), manifest.Layers[1].Digest: mustRead(t, wasmPath)} {
				mustWrite(t, filepath.Join(layout, "blobs/sha256", strings.TrimPrefix(digest, "sha256:")), string(data))
			}
			entry := SnapshotEntry{LogicalID: "demo", Version: pulledVer, Digest: manifestDigest, SourceCommit: source, InputHash: pulledInput, ProvenanceMode: "candidate", CandidateRef: "registry.example/candidates/demo:original-tag@" + manifestDigest, WasmSHA256: sha256Hex(mustRead(t, wasmPath))}
			switch mutation {
			case "blob-tamper":
				mustWrite(t, filepath.Join(layout, "blobs/sha256", strings.TrimPrefix(manifest.Layers[1].Digest, "sha256:")), "corrupt blob")
			case "manifest-tamper":
				mustWrite(t, filepath.Join(layout, "blobs/sha256", strings.TrimPrefix(manifestDigest, "sha256:")), "{}")
			case "wrong-wasm-hash":
				entry.WasmSHA256 = strings.Repeat("0", 64)
			case "missing-module":
				if err := os.Remove(filepath.Join(layout, "blobs/sha256", strings.TrimPrefix(manifest.Layers[1].Digest, "sha256:"))); err != nil {
					t.Fatal(err)
				}
			}
			binary := filepath.Join(root, "bin/oras")
			mustWrite(t, binary, "#!/usr/bin/env bash\nset -euo pipefail\ntest \"$1\" = cp\ntest \"$3\" = --to-oci-layout\nprintf '%s\\n' \"$*\" >> \"$OCI_CALLS\"\ncp -R \"$OCI_FIXTURE/.\" \"$4/\"\n")
			if err := os.Chmod(binary, 0o755); err != nil {
				t.Fatal(err)
			}
			t.Setenv("PATH", filepath.Dir(binary)+string(os.PathListSeparator)+os.Getenv("PATH"))
			t.Setenv("OCI_FIXTURE", layout)
			t.Setenv("OCI_CALLS", filepath.Join(root, "calls"))
			hash, err := resolveOCIWasmHash(root, entry, entry.CandidateRef)
			if mutation == "valid" {
				if err != nil || hash != entry.WasmSHA256 {
					t.Fatalf("valid final bytes rejected: %s %v", hash, err)
				}
			} else if err == nil {
				t.Fatal("forged OCI bytes/provenance accepted")
			}
			calls := string(mustRead(t, filepath.Join(root, "calls")))
			if strings.Contains(calls, "push") || strings.Contains(calls, "tag ") || !strings.Contains(calls, "cp registry.example/candidates/demo@"+manifestDigest+" --to-oci-layout") {
				t.Fatalf("unexpected mutating OCI operation: %s", calls)
			}
		})
	}
}
