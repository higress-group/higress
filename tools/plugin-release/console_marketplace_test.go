// Copyright 2026 Higress Authors
// Licensed under the Apache License, Version 2.0.

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestConsoleMarketplaceCoverageIncludesStableAndNonAlphaPrerelease(t *testing.T) {
	for _, version := range []string{"1.0.0", "1.1.0-beta.1", "1.1.0-rc.1"} {
		t.Run(version, func(t *testing.T) {
			root, catalog := marketplaceFixture(t, version, true)
			if err := validateCatalog(root, catalog); err == nil || !strings.Contains(err.Error(), "requires a reviewed Console marketplace mapping") {
				t.Fatalf("version %s must require a marketplace bundle, got %v", version, err)
			}
		})
	}
}

func TestConsoleMarketplaceCoverageExcludesAlphaAndReleaseIneligible(t *testing.T) {
	root, catalog := marketplaceFixture(t, "1.0.0-alpha", true)
	if err := validateCatalog(root, catalog); err != nil {
		t.Fatalf("alpha must remain outside Console release projection: %v", err)
	}
	root, catalog = marketplaceFixture(t, "1.0.0", false)
	if err := validateCatalog(root, catalog); err != nil {
		t.Fatalf("release-ineligible plugin must remain outside Console release projection: %v", err)
	}
}

func TestConsoleMarketplaceBundleRejectsMissingMalformedAndUnsafeInputs(t *testing.T) {
	root, catalogPath := marketplaceFixture(t, "1.0.0", true)
	var catalog Catalog
	if _, err := readJSON(catalogPath, &catalog); err != nil {
		t.Fatal(err)
	}
	p := &catalog.Plugins[0]
	p.Consumers.Console = &ConsoleConsumer{PropertyKey: "demo", ResourceDir: "demo", URLForm: "oci"}
	bundle := validFixtureBundle(t, root)
	catalog.ConsoleMarketplace.Bundles["demo"] = bundle
	if err := writeCanonical(catalogPath, catalog); err != nil {
		t.Fatal(err)
	}
	if err := validateCatalog(root, catalogPath); err != nil {
		t.Fatalf("valid reviewed bundle rejected: %v", err)
	}

	cases := map[string]func(*ConsoleMarketplaceBundle){
		"missing docs":   func(b *ConsoleMarketplaceBundle) { b.Files = b.Files[:2] },
		"malformed hash": func(b *ConsoleMarketplaceBundle) { b.Files[0].SHA256 = strings.Repeat("a", 64) },
		"unsafe path":    func(b *ConsoleMarketplaceBundle) { b.Files[0].SourcePath = "../spec.yaml" },
		"identity drift": func(b *ConsoleMarketplaceBundle) {
			mustWrite(t, filepath.Join(root, "market/spec.yaml"), strings.ReplaceAll(fixtureSpec, "name: demo", "name: other"))
			b.Files[0].SHA256 = sha256Hex(mustRead(t, filepath.Join(root, "market/spec.yaml")))
		},
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			candidate := cloneBundle(bundle)
			mutate(&candidate)
			if err := validateConsoleMarketplaceBundle(root, *p, candidate); err == nil {
				t.Fatal("expected invalid marketplace bundle to fail")
			}
		})
	}
}

func TestConsoleMarketplaceRejectsDuplicateResourceDirectory(t *testing.T) {
	root, catalogPath := marketplaceFixture(t, "1.0.0", true)
	var catalog Catalog
	if _, err := readJSON(catalogPath, &catalog); err != nil {
		t.Fatal(err)
	}
	first := &catalog.Plugins[0]
	first.ReleaseEligible = false
	first.UnmanagedReason = "fixture exclusion"
	first.Consumers.Console = &ConsoleConsumer{PropertyKey: "demo", ResourceDir: "shared", URLForm: "oci"}
	catalog.Plugins = append(catalog.Plugins, Plugin{
		LogicalID: "other", Implementation: "go", SourceDir: "plugins/wasm-go/extensions/other",
		Image: "plugins/other", ReleaseEligible: false, UnmanagedReason: "fixture exclusion",
		ArtifactInputs: []string{"plugins/wasm-go/extensions/other/**"},
		Consumers:      PluginConsumers{Console: &ConsoleConsumer{PropertyKey: "other", ResourceDir: "shared", URLForm: "oci"}},
	})
	mustWrite(t, filepath.Join(root, "plugins/wasm-go/extensions/other/main.go"), "package main\n")
	if err := writeCanonical(catalogPath, catalog); err != nil {
		t.Fatal(err)
	}
	if err := validateCatalog(root, catalogPath); err == nil || !strings.Contains(err.Error(), "resourceDir") {
		t.Fatalf("duplicate Console resourceDir must fail closed, got %v", err)
	}
}

func TestConsoleMarketplaceBundleRejectsSymlinkSources(t *testing.T) {
	for _, name := range []string{"file", "directory"} {
		t.Run(name, func(t *testing.T) {
			root, catalogPath := marketplaceFixture(t, "1.0.0", true)
			var catalog Catalog
			if _, err := readJSON(catalogPath, &catalog); err != nil {
				t.Fatal(err)
			}
			plugin := &catalog.Plugins[0]
			plugin.Consumers.Console = &ConsoleConsumer{PropertyKey: "demo", ResourceDir: "demo", URLForm: "oci"}
			bundle := validFixtureBundle(t, root)
			catalog.ConsoleMarketplace.Bundles["demo"] = bundle
			if name == "file" {
				source := filepath.Join(root, "market", "README.md")
				outside := filepath.Join(root, "outside.md")
				mustWrite(t, outside, string(mustRead(t, source)))
				if err := os.Remove(source); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(outside, source); err != nil {
					t.Fatal(err)
				}
			} else {
				market := filepath.Join(root, "market")
				reviewed := filepath.Join(root, "reviewed")
				if err := os.Rename(market, reviewed); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(reviewed, market); err != nil {
					t.Fatal(err)
				}
			}
			if err := writeCanonical(catalogPath, catalog); err != nil {
				t.Fatal(err)
			}
			if err := validateCatalog(root, catalogPath); err == nil || !strings.Contains(err.Error(), "symlink") {
				t.Fatalf("Console marketplace %s symlink must fail closed, got %v", name, err)
			}
		})
	}
}

func TestRealHMACConsoleSpecAllowsDisablingClockSkewValidation(t *testing.T) {
	data := mustRead(t, filepath.Join("..", "..", "plugins/release/console/hmac-auth-apisix/spec.yaml"))
	text := string(data)
	if !strings.Contains(text, "        clock_skew:\n          type: integer\n          minimum: 0\n          default: 300") {
		t.Fatal("hmac-auth-apisix Console schema must allow clock_skew=0 to disable timestamp validation")
	}
}

func TestRealConsoleRecoveryManifestBindsUnchangedSnapshot(t *testing.T) {
	root := filepath.Clean(filepath.Join("..", ".."))
	manifest := filepath.Join(root, "plugins/release/console-recovery/2.2.4.json")
	if err := validateConsoleRecovery(root, filepath.Join(root, "plugins/release/catalog.json"), manifest); err != nil {
		t.Fatal(err)
	}
	var recovery ConsoleRecoveryManifest
	if _, err := readJSON(manifest, &recovery); err != nil {
		t.Fatal(err)
	}
	recovery.GatewayVersion = "2.2.5"
	tampered := filepath.Join(t.TempDir(), "recovery.json")
	if err := writeCanonical(tampered, recovery); err != nil {
		t.Fatal(err)
	}
	if err := validateConsoleRecovery(root, filepath.Join(root, "plugins/release/catalog.json"), tampered); err == nil {
		t.Fatal("recovery must reject every version other than 2.2.4")
	}
	if _, err := readJSON(manifest, &recovery); err != nil {
		t.Fatal(err)
	}
	recovery.OriginalImageDigest = "sha256:" + strings.Repeat("0", 64)
	if err := writeCanonical(tampered, recovery); err != nil {
		t.Fatal(err)
	}
	if err := validateConsoleRecovery(root, filepath.Join(root, "plugins/release/catalog.json"), tampered); err == nil {
		t.Fatal("recovery must reject a manifest with a different fixed original image digest")
	}
}

const fixtureSpec = `apiVersion: 1.0.0
info:
  name: demo
  version: 1.0.0
spec:
  configSchema:
    openAPIV3Schema:
      type: object
`

func marketplaceFixture(t *testing.T, version string, eligible bool) (string, string) {
	t.Helper()
	root := t.TempDir()
	mustRun(t, root, "git", "init", "-q")
	mustRun(t, root, "git", "config", "user.name", "test")
	mustRun(t, root, "git", "config", "user.email", "test@example.com")
	dir := "plugins/wasm-go/extensions/demo"
	mustWrite(t, filepath.Join(root, dir, "main.go"), "package main\n")
	mustWrite(t, filepath.Join(root, dir, "VERSION"), version+"\n")
	mustWrite(t, filepath.Join(root, "plugins/wasm-rust/extensions/.keep"), "")
	mustRun(t, root, "git", "add", ".")
	mustRun(t, root, "git", "commit", "-q", "-m", "fixture")
	catalog := Catalog{SchemaVersion: 1, Registry: "registry.example", ConsoleMarketplace: &ConsoleMarketplacePolicy{RequiredForStable: true, Bundles: map[string]ConsoleMarketplaceBundle{}}, Plugins: []Plugin{{
		LogicalID: "demo", Implementation: "go", SourceDir: dir, Image: "plugins/demo", ReleaseEligible: eligible,
		UnmanagedReason: func() string {
			if eligible {
				return ""
			}
			return "fixture exclusion"
		}(), ArtifactInputs: []string{dir + "/**"},
	}}}
	path := filepath.Join(root, "catalog.json")
	if err := writeCanonical(path, catalog); err != nil {
		t.Fatal(err)
	}
	return root, path
}

func validFixtureBundle(t *testing.T, root string) ConsoleMarketplaceBundle {
	t.Helper()
	files := map[string]string{"spec.yaml": fixtureSpec, "README.md": "# Demo\n", "README_EN.md": "# Demo\n"}
	bundle := ConsoleMarketplaceBundle{Repository: "higress-group/higress"}
	for _, target := range []string{"spec.yaml", "README.md", "README_EN.md"} {
		path := filepath.Join(root, "market", target)
		mustWrite(t, path, files[target])
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		bundle.Files = append(bundle.Files, ConsoleMarketplaceBundleFile{SourcePath: "market/" + target, TargetPath: target, SHA256: sha256Hex(data)})
	}
	return bundle
}

func cloneBundle(in ConsoleMarketplaceBundle) ConsoleMarketplaceBundle {
	out := in
	out.Files = append([]ConsoleMarketplaceBundleFile(nil), in.Files...)
	return out
}
