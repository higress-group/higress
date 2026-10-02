// Copyright 2026 Higress Authors
// Licensed under the Apache License, Version 2.0.

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// cppFixture builds a minimal tree whose only official extension is a C++ one, so
// the validator's per-entry rules are exercised without the go/rust sweep finding
// anything to classify.
func cppFixture(t *testing.T, implementation, sourceDir string, eligible bool) (string, string) {
	t.Helper()
	root := t.TempDir()
	mustRun(t, root, "git", "init", "-q")
	mustRun(t, root, "git", "config", "user.name", "test")
	mustRun(t, root, "git", "config", "user.email", "test@example.com")
	mustWrite(t, filepath.Join(root, sourceDir, "plugin.cc"), "// fixture\n")
	mustWrite(t, filepath.Join(root, sourceDir, "VERSION"), "1.0.0\n")
	for _, dir := range []string{"plugins/wasm-go/extensions", "plugins/wasm-rust/extensions"} {
		if err := os.MkdirAll(filepath.Join(root, dir), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	mustRun(t, root, "git", "add", ".")
	mustRun(t, root, "git", "commit", "-q", "-m", "fixture")
	plugin := Plugin{
		LogicalID:       "demo-cpp",
		Implementation:  implementation,
		SourceDir:       sourceDir,
		Image:           "plugins/demo-cpp",
		ReleaseEligible: eligible,
		ArtifactInputs:  []string{sourceDir + "/**"},
	}
	if !eligible {
		plugin.UnmanagedReason = "fixture exclusion"
	}
	catalog := Catalog{SchemaVersion: catalogSchemaVersion, Registry: "registry.example", Plugins: []Plugin{plugin}}
	path := filepath.Join(root, "catalog.json")
	if err := writeCanonical(path, catalog); err != nil {
		t.Fatal(err)
	}
	return root, path
}

func TestValidatorAcceptsCppImplementation(t *testing.T) {
	root, catalog := cppFixture(t, "cpp", "plugins/wasm-cpp/extensions/demo_cpp", false)
	if err := validateCatalog(root, catalog); err != nil {
		t.Fatalf("cpp implementation must validate: %v", err)
	}
}

func TestValidatorStillRejectsUnknownImplementationAndForeignSourceDir(t *testing.T) {
	root, catalog := cppFixture(t, "zig", "plugins/wasm-cpp/extensions/demo_cpp", false)
	err := validateCatalog(root, catalog)
	if err == nil || !strings.Contains(err.Error(), "implementation must be go, rust or cpp") {
		t.Fatalf("unknown implementation must fail closed, got %v", err)
	}

	// A cpp entry whose sourceDir lives outside the C++ extension root is rejected
	// by the same prefix invariant that guards go and rust.
	root, catalog = cppFixture(t, "cpp", "plugins/wasm-go/extensions/demo_cpp", false)
	err = validateCatalog(root, catalog)
	if err == nil || !strings.Contains(err.Error(), "outside the official cpp extension root") {
		t.Fatalf("cpp sourceDir outside the C++ root must fail closed, got %v", err)
	}
}

func TestRealCatalogClassifiesCppPluginsWithoutConsumers(t *testing.T) {
	root := filepath.Clean(filepath.Join("..", ".."))
	catalogPath := filepath.Join(root, "plugins/release/catalog.json")
	if err := validateCatalog(root, catalogPath); err != nil {
		t.Fatal(err)
	}
	c, _, err := loadCatalog(catalogPath)
	if err != nil {
		t.Fatal(err)
	}

	byID := map[string]Plugin{}
	for _, p := range c.Plugins {
		if p.Implementation == "cpp" {
			byID[p.LogicalID] = p
		}
	}
	// The ten C++ extensions whose published identity is not already owned by a Go
	// catalog entry. model-mapper and model-router are deliberately absent: their
	// logicalId and image belong to the Go plugins of the same name.
	want := []string{
		"basic-auth", "bot-detect", "custom-response", "hmac-auth", "jwt-auth",
		"key-auth", "key-rate-limit", "oauth", "request-block", "sni-misdirect",
	}
	if len(byID) != len(want) {
		t.Fatalf("cpp catalog entries = %d, want %d: %v", len(byID), len(want), byID)
	}
	for _, id := range want {
		p, ok := byID[id]
		if !ok {
			t.Fatalf("missing cpp catalog entry %q", id)
		}
		if !strings.HasPrefix(p.SourceDir, "plugins/wasm-cpp/extensions/") {
			t.Fatalf("%s: unexpected sourceDir %q", id, p.SourceDir)
		}
		if p.Image != "plugins/"+id {
			t.Fatalf("%s: unexpected image %q", id, p.Image)
		}
		if len(p.ArtifactInputs) == 0 {
			t.Fatalf("%s: artifactInputs is empty", id)
		}
		// Console onboarding is deferred, so these must stay out of the release
		// pipeline rather than claiming a stable identity they cannot back.
		if p.ReleaseEligible {
			t.Fatalf("%s: cpp plugin must not be release-eligible until a Console marketplace mapping is reviewed", id)
		}
		if strings.TrimSpace(p.UnmanagedReason) == "" {
			t.Fatalf("%s: release-ineligible cpp plugin requires unmanagedReason", id)
		}
		if p.Consumers.Console != nil || p.Consumers.PluginServer != nil {
			t.Fatalf("%s: cpp consumers must remain unclaimed while onboarding is deferred", id)
		}
	}

	// The shadowed identities stay owned by their Go entries.
	for _, id := range []string{"model-mapper", "model-router"} {
		var found *Plugin
		for i := range c.Plugins {
			if c.Plugins[i].LogicalID == id {
				found = &c.Plugins[i]
			}
		}
		if found == nil {
			t.Fatalf("%s must remain in the catalog", id)
		}
		if found.Implementation != "go" {
			t.Fatalf("%s identity must stay with the Go plugin, got %q", id, found.Implementation)
		}
	}
}

// Adding C++ entries that claim no consumers must not disturb the one-time Console
// recovery contract, which binds only its eight reviewed plugins.
func TestConsoleRecoveryUnaffectedByCppCatalogEntries(t *testing.T) {
	root := filepath.Clean(filepath.Join("..", ".."))
	catalogPath := filepath.Join(root, "plugins/release/catalog.json")
	manifest := filepath.Join(root, "plugins/release/console-recovery/2.2.4.json")

	c, _, err := loadCatalog(catalogPath)
	if err != nil {
		t.Fatal(err)
	}
	hasCpp := false
	for _, p := range c.Plugins {
		if p.Implementation == "cpp" {
			hasCpp = true
		}
	}
	if !hasCpp {
		t.Fatal("fixture expects the real catalog to classify cpp plugins")
	}
	if err := validateConsoleRecovery(root, catalogPath, manifest); err != nil {
		t.Fatalf("console recovery must be unaffected by consumer-less cpp entries: %v", err)
	}
}
