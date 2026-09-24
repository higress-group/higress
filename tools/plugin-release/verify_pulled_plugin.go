// Copyright 2026 Higress Authors
// Licensed under the Apache License, Version 2.0.

package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
)

const (
	ociImageManifestMediaType = "application/vnd.oci.image.manifest.v1+json"
	unknownConfigMediaType    = "application/vnd.unknown.config.v1+json"
	emptyObjectDigest         = "sha256:44136fa355b3678a1146ad16f7e8649e94fb4fc21fe77e8310c060f61caaff8a"
	wasmConfigMediaType       = "application/vnd.module.wasm.config.v1+json"
	wasmContentMediaType      = "application/vnd.module.wasm.content.layer.v1+wasm"
)

type pulledDescriptor struct {
	MediaType   string            `json:"mediaType"`
	Digest      string            `json:"digest"`
	Size        int64             `json:"size"`
	Annotations map[string]string `json:"annotations"`
}

type pulledPluginManifest struct {
	SchemaVersion int                `json:"schemaVersion"`
	MediaType     string             `json:"mediaType"`
	Config        pulledDescriptor   `json:"config"`
	Layers        []pulledDescriptor `json:"layers"`
	Annotations   map[string]string  `json:"annotations"`
}

func commandVerifyPulledPlugin(args []string) error {
	fs := flag.NewFlagSet("verify-pulled-plugin", flag.ContinueOnError)
	manifestPath := fs.String("manifest", "", "raw digest-pinned OCI manifest")
	configPath := fs.String("config", "", "pulled config.json layer")
	wasmPath := fs.String("wasm", "", "pulled plugin.wasm layer")
	digest := fs.String("digest", "", "expected manifest digest")
	sourceCommit := fs.String("source-commit", "", "expected source revision")
	sourceCreated := fs.String("source-created", "", "expected RFC3339 source commit time")
	version := fs.String("version", "", "expected stable plugin version")
	inputHash := fs.String("input-hash", "", "expected plugin input hash")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *manifestPath == "" || *configPath == "" || *wasmPath == "" {
		return errors.New("--manifest, --config, and --wasm are required")
	}
	return verifyPulledPlugin(*manifestPath, *configPath, *wasmPath, *digest, *sourceCommit, *sourceCreated, *version, *inputHash)
}

func verifyPulledPlugin(manifestPath, configPath, wasmPath, expectedDigest, expectedSource, expectedCreated, expectedVersion, expectedInputHash string) error {
	if !digestPattern.MatchString(expectedDigest) {
		return errors.New("--digest must be a lowercase sha256 digest")
	}
	if !commitPattern.MatchString(expectedSource) {
		return errors.New("--source-commit must be a full lowercase Git commit")
	}
	if _, err := time.Parse(time.RFC3339, expectedCreated); err != nil {
		return errors.New("--source-created must be an RFC3339 timestamp")
	}
	parsedVersion, err := parseSemver(expectedVersion)
	if err != nil || parsedVersion.prerelease != "" {
		return errors.New("--version must be stable SemVer")
	}
	if !digestPattern.MatchString(expectedInputHash) {
		return errors.New("--input-hash must be a lowercase sha256 digest")
	}

	manifestBytes, err := os.ReadFile(manifestPath)
	if err != nil {
		return fmt.Errorf("read pulled manifest: %w", err)
	}
	// Hashing the fetched file is a digest check, not a re-serialization: oras
	// 1.2.3, the version every release workflow pins, writes the manifest bytes
	// the registry served verbatim (confirmed empirically against a production
	// manifest whose fetched sha256 equalled its registry digest). An oras that
	// ever reformatted its output would fail this comparison closed.
	manifestSum := sha256.Sum256(manifestBytes)
	if got := "sha256:" + hex.EncodeToString(manifestSum[:]); got != expectedDigest {
		return fmt.Errorf("pulled manifest digest %s does not match expected digest %s", got, expectedDigest)
	}
	var manifest pulledPluginManifest
	if err := decodeSingleJSON(manifestBytes, &manifest); err != nil {
		return fmt.Errorf("parse pulled manifest as a canonical OCI v1.0 schema 2 image manifest: %w", err)
	}
	if manifest.SchemaVersion != 2 || manifest.MediaType != ociImageManifestMediaType {
		return errors.New("pulled manifest must be a canonical OCI v1.0 schema 2 image manifest")
	}
	if manifest.Config.MediaType != unknownConfigMediaType || manifest.Config.Digest != emptyObjectDigest || manifest.Config.Size != 2 || len(manifest.Config.Annotations) != 0 {
		return errors.New("pulled manifest OCI config descriptor must identify the canonical empty JSON object")
	}
	if len(manifest.Annotations) != 4 ||
		manifest.Annotations["org.opencontainers.image.created"] != expectedCreated ||
		manifest.Annotations["org.opencontainers.image.revision"] != expectedSource ||
		manifest.Annotations["org.opencontainers.image.version"] != expectedVersion ||
		manifest.Annotations["io.higress.plugin.input-hash"] != expectedInputHash {
		return errors.New("pulled manifest annotations do not match the deterministic source creation time, revision, version, and input hash")
	}
	if len(manifest.Layers) != 2 {
		return fmt.Errorf("pulled manifest must contain exactly two layers, got %d", len(manifest.Layers))
	}
	configDescriptor, wasmDescriptor := manifest.Layers[0], manifest.Layers[1]
	if configDescriptor.MediaType != wasmConfigMediaType || len(configDescriptor.Annotations) != 1 || configDescriptor.Annotations["org.opencontainers.image.title"] != "config.json" {
		return errors.New("pulled manifest layer 0 must be the exactly titled config.json Wasm config layer")
	}
	if wasmDescriptor.MediaType != wasmContentMediaType || len(wasmDescriptor.Annotations) != 1 || wasmDescriptor.Annotations["org.opencontainers.image.title"] != "plugin.wasm" {
		return errors.New("pulled manifest layer 1 must be the exactly titled plugin.wasm Wasm content layer")
	}

	configBytes, err := os.ReadFile(configPath)
	if err != nil {
		return fmt.Errorf("read pulled config.json: %w", err)
	}
	if err := verifyPulledLayer("config.json", configBytes, configDescriptor); err != nil {
		return err
	}
	var configValue any
	if err := decodeSingleJSON(configBytes, &configValue); err != nil {
		return fmt.Errorf("pulled config.json is invalid JSON: %w", err)
	}
	if _, ok := configValue.(map[string]any); !ok {
		return errors.New("pulled config.json must contain a JSON object")
	}
	if !bytes.Equal(configBytes, []byte(`{}`)) {
		return errors.New("pulled config.json must be the canonical empty JSON object")
	}

	wasmBytes, err := os.ReadFile(wasmPath)
	if err != nil {
		return fmt.Errorf("read pulled plugin.wasm: %w", err)
	}
	if err := verifyPulledLayer("plugin.wasm", wasmBytes, wasmDescriptor); err != nil {
		return err
	}
	if err := validateProxyWasmModule(wasmBytes); err != nil {
		return fmt.Errorf("pulled plugin.wasm: %w", err)
	}
	return nil
}

func decodeSingleJSON(data []byte, value any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	decoder.UseNumber()
	if err := decoder.Decode(value); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return errors.New("multiple JSON values")
		}
		return err
	}
	return nil
}

func verifyPulledLayer(name string, data []byte, descriptor pulledDescriptor) error {
	if !digestPattern.MatchString(descriptor.Digest) {
		return fmt.Errorf("%s layer descriptor has an invalid digest", name)
	}
	if descriptor.Size < 0 || int64(len(data)) != descriptor.Size {
		return fmt.Errorf("%s size %d does not match layer descriptor size %d", name, len(data), descriptor.Size)
	}
	sum := sha256.Sum256(data)
	if got := "sha256:" + hex.EncodeToString(sum[:]); got != descriptor.Digest {
		return fmt.Errorf("%s digest %s does not match layer descriptor digest %s", name, got, descriptor.Digest)
	}
	return nil
}

func validateProxyWasmModule(data []byte) error {
	ctx := context.Background()
	runtime := wazero.NewRuntimeWithConfig(ctx, wazero.NewRuntimeConfigInterpreter())
	defer runtime.Close(ctx)

	module, err := runtime.CompileModule(ctx, data)
	if err != nil {
		return fmt.Errorf("invalid WebAssembly module: %w", err)
	}
	defer module.Close(ctx)

	if len(module.ExportedMemories()) == 0 {
		return errors.New("required exported memory is missing")
	}
	exportedFunctions := module.ExportedFunctions()
	callbackSignature := []api.ValueType{api.ValueTypeI32, api.ValueTypeI32}
	callbackResult := []api.ValueType{api.ValueTypeI32}
	for _, name := range []string{"proxy_on_vm_start", "proxy_on_configure"} {
		definition, ok := exportedFunctions[name]
		if !ok {
			return fmt.Errorf("required function export %q is missing", name)
		}
		if !sameValueTypes(definition.ParamTypes(), callbackSignature) || !sameValueTypes(definition.ResultTypes(), callbackResult) {
			return fmt.Errorf("required function export %q must have signature (i32,i32)->i32", name)
		}
	}
	for name, definition := range exportedFunctions {
		if strings.HasPrefix(name, "proxy_abi_version_0_2_") && len(name) > len("proxy_abi_version_0_2_") && len(definition.ParamTypes()) == 0 && len(definition.ResultTypes()) == 0 {
			return nil
		}
	}
	return errors.New("required proxy_abi_version_0_2_* function export with signature ()->() is missing")
}

func sameValueTypes(got, want []api.ValueType) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range want {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}
