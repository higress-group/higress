// Copyright 2026 Higress Authors
// Licensed under the Apache License, Version 2.0.

package main

import (
	"strings"
	"testing"
)

func layer(mediaType string) ociLayer {
	return ociLayer{MediaType: mediaType, Digest: "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}
}

func TestEnvoyWasmLayoutAcceptsCompatVariants(t *testing.T) {
	for name, layers := range map[string][]ociLayer{
		"single-tar-gzip":           {layer(ociLayerGzipMediaType)},
		"spec-doc-wasm":             {layer("application/vnd.module.wasm.spec.v1+json"), layer("application/vnd.module.wasm.doc.v1+markdown"), layer(ociLayerGzipMediaType)},
		"doc-en-doc-wasm":           {layer("application/vnd.module.wasm.doc.v1.EN+markdown"), layer("application/vnd.module.wasm.doc.v1+markdown"), layer(ociLayerGzipMediaType)},
		"oci-two-layer-config-wasm": {layer(wasmConfigMediaType), layer(wasmContentMediaType)},
	} {
		t.Run(name, func(t *testing.T) {
			got, err := envoyWasmLayout(layers)
			if err != nil {
				t.Fatalf("accepted compat/oci layout rejected: %v", err)
			}
			want := wasmLayoutCompat
			if name == "oci-two-layer-config-wasm" {
				want = wasmLayoutOCI
			}
			if got != want {
				t.Fatalf("layout = %q, want %q", got, want)
			}
		})
	}
}

// The 2026-08-13 v2.2.4 release published candidates whose only layer had the
// raw wasm media type. Neither Envoy-accepted layout matches that manifest, so
// every affected gateway failed to fetch the plugin. This is the exact
// regression shape for incident #4528.
func TestIssue4528IncidentSingleLayerRawWasmLayoutIsRejected(t *testing.T) {
	layers := []ociLayer{layer(wasmContentMediaType)}
	_, err := envoyWasmLayout(layers)
	if err == nil {
		t.Fatal("the incident's single-layer raw-wasm manifest must be rejected")
	}
	message := err.Error()
	for _, required := range []string{
		wasmContentMediaType,
		"do not form an Envoy-loadable Wasm OCI image",
		wasmConfigMediaType,
		ociLayerGzipMediaType,
	} {
		if !strings.Contains(message, required) {
			t.Fatalf("rejection %q lacks %q", message, required)
		}
	}
}

func TestEnvoyWasmLayoutRejectsNonLoadableVariants(t *testing.T) {
	for name, layers := range map[string][]ociLayer{
		"empty":                    {},
		"docker-rootfs":            {layer("application/vnd.docker.image.rootfs.diff.tar.gzip")},
		"tar-gzip-not-final":       {layer(ociLayerGzipMediaType), layer("application/vnd.module.wasm.doc.v1+markdown")},
		"oci-variant-one-layer":    {layer(wasmConfigMediaType)},
		"oci-variant-swapped":      {layer(wasmContentMediaType), layer(wasmConfigMediaType)},
		"oci-variant-three-layers": {layer(wasmConfigMediaType), layer(wasmContentMediaType), layer(wasmContentMediaType)},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := envoyWasmLayout(layers); err == nil {
				t.Fatalf("non-loadable layout %q was accepted", name)
			}
		})
	}
}

const (
	layoutTestCommit = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	layoutTestHash   = "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	layoutTestDigest = "sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"
)

func TestVerifyOCIGatesCandidateProvenanceOnEnvoyLayout(t *testing.T) {
	entry := SnapshotEntry{
		LogicalID: "demo", OCIRef: "registry.example/plugins/demo:1.0.0", Version: "1.0.0",
		Digest: layoutTestDigest, InputHash: layoutTestHash, SourceCommit: layoutTestCommit,
		CandidateRef: "registry.example/candidates/demo@" + layoutTestDigest, ProvenanceMode: "candidate",
	}
	annotations := map[string]string{
		"org.opencontainers.image.revision": layoutTestCommit,
		"org.opencontainers.image.version":  "1.0.0",
		"io.higress.plugin.input-hash":      layoutTestHash,
	}

	t.Run("incident-single-layer-rejected", func(t *testing.T) {
		withManifestResolver(t, func(string) (ociManifest, error) {
			return ociManifest{Digest: entry.Digest, Annotations: annotations, Layers: []ociLayer{layer(wasmContentMediaType)}}, nil
		})
		err := verifyOCI(entry, "candidate", "candidate", "candidate")
		if err == nil {
			t.Fatal("incident-layout candidate passed OCI verification")
		}
		for _, required := range []string{entry.CandidateRef, wasmContentMediaType, "not promotable"} {
			if !strings.Contains(err.Error(), required) {
				t.Fatalf("rejection %q lacks %q", err.Error(), required)
			}
		}
	})

	t.Run("two-layer-candidate-accepted", func(t *testing.T) {
		withManifestResolver(t, func(string) (ociManifest, error) {
			return ociManifest{Digest: entry.Digest, Annotations: annotations, Layers: []ociLayer{layer(wasmConfigMediaType), layer(wasmContentMediaType)}}, nil
		})
		if err := verifyOCI(entry, "candidate", "candidate", "candidate"); err != nil {
			t.Fatalf("2-layer OCI candidate rejected: %v", err)
		}
	})

	t.Run("historical-public-import-keeps-digest-only-verification", func(t *testing.T) {
		public := entry
		public.ProvenanceMode = "public"
		public.CandidateRef = ""
		withManifestResolver(t, func(string) (ociManifest, error) {
			return ociManifest{Digest: public.Digest, Layers: []ociLayer{layer("application/vnd.docker.image.rootfs.diff.tar.gzip")}}, nil
		})
		if err := verifyOCI(public, "public", "mixed", "public"); err != nil {
			t.Fatalf("historical public import rejected on layout: %v", err)
		}
	})
}

func TestCommandVerifyOCILayoutReportsAcceptedLayoutAndNamesOffendingRef(t *testing.T) {
	ociLayoutRef := "registry.example/plugins/demo:1.0.0"
	incidentRef := "registry.example/plugins/ai-statistics:latest"
	withManifestResolver(t, func(ref string) (ociManifest, error) {
		if ref == ociLayoutRef {
			return ociManifest{Digest: layoutTestDigest, Layers: []ociLayer{layer(wasmConfigMediaType), layer(wasmContentMediaType)}}, nil
		}
		return ociManifest{Digest: layoutTestDigest, Layers: []ociLayer{layer(wasmContentMediaType)}}, nil
	})

	if err := commandVerifyOCILayout([]string{"--ref", ociLayoutRef}); err != nil {
		t.Fatalf("accepted layout failed: %v", err)
	}

	err := commandVerifyOCILayout([]string{"--ref", incidentRef})
	if err == nil {
		t.Fatal("incident layout passed verify-oci-layout")
	}
	for _, required := range []string{incidentRef, wasmContentMediaType, "not loadable by Envoy as a Wasm OCI image"} {
		if !strings.Contains(err.Error(), required) {
			t.Fatalf("rejection %q lacks %q", err.Error(), required)
		}
	}

	if err := commandVerifyOCILayout([]string{}); err == nil || !strings.Contains(err.Error(), "--ref is required") {
		t.Fatalf("missing --ref not rejected: %v", err)
	}
}
