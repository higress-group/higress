// Copyright 2026 Higress Authors
// Licensed under the Apache License, Version 2.0.

package main

import (
	"fmt"
	"strings"
)

// Envoy's Wasm OCI image resolver accepts exactly two image layouts. Any other
// layer composition fails the gateway with "the given image is in invalid
// format as an OCI image" (incident #4528).
const (
	// ociLayerGzipMediaType is the wasm layer of the legacy compat layout:
	// oras-push layers whose final entry is a tar+gzip containing plugin.wasm,
	// optionally preceded by wasm.spec / wasm.doc layers.
	ociLayerGzipMediaType = "application/vnd.oci.image.layer.v1.tar+gzip"
	// wasmConfigMediaType and wasmContentMediaType are the two layers of the
	// OCI variant defined by the wasm module spec: a config JSON layer followed
	// by the raw Wasm module layer.
	wasmConfigMediaType  = "application/vnd.module.wasm.config.v1+json"
	wasmContentMediaType = "application/vnd.module.wasm.content.layer.v1+wasm"
)

const (
	wasmLayoutCompat = "compat"
	wasmLayoutOCI    = "oci"
)

type ociLayer struct {
	MediaType string `json:"mediaType"`
	Digest    string `json:"digest"`
}

// envoyWasmLayout reports which of the two Envoy-accepted Wasm OCI layouts the
// given manifest layers form. The OCI variant requires exactly two layers,
// wasm config JSON first and raw wasm second. The compat variant requires at
// least one layer whose final entry is a tar+gzip; preceding layers are the
// legacy optional spec/doc layers and are not constrained. This predicate is
// the shared lower bound of the runtime resolvers (the Istio-side fetcher used
// by Higress gateways reads the final layer for the compat variant), so a
// manifest accepted here is loadable by every gateway version in the wild.
func envoyWasmLayout(layers []ociLayer) (string, error) {
	mediaTypes := make([]string, 0, len(layers))
	for _, layer := range layers {
		mediaTypes = append(mediaTypes, layer.MediaType)
	}
	if len(layers) == 2 && layers[0].MediaType == wasmConfigMediaType && layers[1].MediaType == wasmContentMediaType {
		return wasmLayoutOCI, nil
	}
	if len(layers) > 0 && layers[len(layers)-1].MediaType == ociLayerGzipMediaType {
		return wasmLayoutCompat, nil
	}
	return "", fmt.Errorf(
		"layer media types [%s] do not form an Envoy-loadable Wasm OCI image; accepted layouts are the oci variant (exactly two layers: %s then %s) or the compat variant (final layer %s, optional preceding wasm.spec/wasm.doc layers)",
		strings.Join(mediaTypes, ", "), wasmConfigMediaType, wasmContentMediaType, ociLayerGzipMediaType,
	)
}
