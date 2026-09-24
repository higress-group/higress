// Copyright 2026 Higress Authors
// Licensed under the Apache License, Version 2.0.

package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strings"
)

var (
	safeIDPattern              = regexp.MustCompile(`^[a-z0-9]+(?:[a-z0-9._-]*[a-z0-9])?$`)
	digestPattern              = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)
	commitPattern              = regexp.MustCompile(`^[0-9a-f]{40}$`)
	credentialURLPattern       = regexp.MustCompile(`(?i)([a-z][a-z0-9+.-]*://)[^/\s@]+@`)
	sensitiveValuePattern      = regexp.MustCompile(`(?i)(\b(?:password|token|secret|credential|access[_-]?key|api[_-]?key)\b(?:\s*[:=]\s*|\s+))([^\s,;]+)`)
	authorizationValuePattern  = regexp.MustCompile(`(?i)(\bauthorization\b\s*[:=]\s*)([^\s,;]+(?:\s+[^\s,;]+)?)`)
	authorizationPhrasePattern = regexp.MustCompile(`(?i)unauthorized|forbidden|denied|authentication required|authorization required`)
	authorizationStatusPattern = regexp.MustCompile(`(?i)HTTP(?:/[0-9.]+)?\s+(?:401|403)(?:[^0-9]|$)|(?:^|[^a-z])status(?: code)?(?:\s*[:=]\s*|\s+)(?:401|403)(?:[^0-9]|$)`)
	notFoundStatusPattern      = regexp.MustCompile(`(?i)HTTP(?:/[0-9.]+)?\s+404(?:[^0-9]|$)|response status(?: code)?(?:\s*[:=]\s*|\s+)404(?:[^0-9]|$)|status code(?:\s*[:=]\s*|\s+)404(?:[^0-9]|$)|404\s+not\s+found`)
	// ACR reports an absent manifest as a provider-structured registry error
	// ending in a fully qualified OCI reference followed by ": not found".
	// The explicit registry prefix and qualified reference are both required;
	// a bare or local "not found" remains an unclassified failure.
	registryQualifiedReferencePattern = regexp.MustCompile(`^(?:[a-z0-9](?:[a-z0-9.-]*[a-z0-9])?(?::[0-9]+)?)/(?:[a-z0-9._-]+/)*[a-z0-9._-]+(?::[a-z0-9_][a-z0-9._-]*|@sha256:[0-9a-f]{64})$`)
)

type ociFailureClass int

const (
	ociFailureOther ociFailureClass = iota
	ociFailureNotFound
	ociFailureUnauthorized
)

// classifyOCIFailure reports whether a sanitized registry error means the
// artifact is genuinely absent or the caller lacks authorization. The
// authorization markers are checked first after removing the exact expected
// OCI reference, so digits or words inside a content-addressed reference are
// never treated as registry status. Numeric 401/403 markers require HTTP/status
// context; explicit authorization phrases still fail closed. Absence requires
// explicit registry evidence (404-class, manifest/name unknown), or a
// provider-structured registry error naming the fully qualified reference; a
// local executable/file failure or generic "not found" text fails closed.
func classifyOCIFailure(err error, expectedRef string) ociFailureClass {
	msg := strings.ToLower(err.Error())
	expectedRef = strings.ToLower(expectedRef)
	classificationMsg := msg
	if expectedRef != "" {
		classificationMsg = strings.ReplaceAll(classificationMsg, expectedRef, "")
	}
	if authorizationPhrasePattern.MatchString(classificationMsg) || authorizationStatusPattern.MatchString(classificationMsg) {
		return ociFailureUnauthorized
	}
	if notFoundStatusPattern.MatchString(classificationMsg) {
		return ociFailureNotFound
	}
	for _, marker := range []string{"manifest unknown", "name unknown", "repository does not exist"} {
		if strings.Contains(classificationMsg, marker) {
			return ociFailureNotFound
		}
	}
	if strings.Contains(msg, "error response from registry:") &&
		registryQualifiedReferencePattern.MatchString(expectedRef) &&
		strings.Contains(msg, expectedRef+": not found") {
		return ociFailureNotFound
	}
	return ociFailureOther
}

type ociManifest struct {
	Digest      string
	Annotations map[string]string
}

// ociManifestResolver is a seam for the bootstrap and verification tests. It
// resolves the supplied reference itself, rather than trusting a digest copied
// from workflow input.
var ociManifestResolver = resolveOCIManifest

// orasRunner keeps the command boundary testable without requiring a registry
// or an ORAS binary in unit tests.
var orasRunner = runORAS

func loadCatalog(path string) (Catalog, []byte, error) {
	var c Catalog
	data, err := readJSON(path, &c)
	return c, data, err
}

func validateCatalog(root, path string) error {
	c, _, err := loadCatalog(path)
	if err != nil {
		return err
	}
	if c.SchemaVersion != catalogSchemaVersion {
		return fmt.Errorf("catalog schemaVersion must be %d", catalogSchemaVersion)
	}
	if c.Registry == "" || strings.Contains(c.Registry, "://") {
		return errors.New("catalog registry must be a host without a URL scheme")
	}
	seenLogical, seenSource, seenImage := map[string]bool{}, map[string]bool{}, map[string]bool{}
	dirClassifications := map[string]bool{}
	consumerMappings := map[string]map[string]string{"console": {}, "pluginServer": {}}
	consoleResourceDirs := map[string]string{}
	aliases := map[string]string{}
	for _, p := range c.Plugins {
		if !safeIDPattern.MatchString(p.LogicalID) {
			return fmt.Errorf("plugin has unsafe logicalId %q", p.LogicalID)
		}
		if seenLogical[p.LogicalID] {
			return fmt.Errorf("duplicate logicalId %q", p.LogicalID)
		}
		seenLogical[p.LogicalID] = true
		if seenSource[p.SourceDir] {
			return fmt.Errorf("duplicate sourceDir %q", p.SourceDir)
		}
		seenSource[p.SourceDir] = true
		if seenImage[p.Image] {
			return fmt.Errorf("duplicate image %q", p.Image)
		}
		seenImage[p.Image] = true
		expectedPrefix := "plugins/wasm-" + p.Implementation + "/extensions/"
		if p.Implementation != "go" && p.Implementation != "rust" {
			return fmt.Errorf("%s: implementation must be go or rust", p.LogicalID)
		}
		if !strings.HasPrefix(p.SourceDir, expectedPrefix) || strings.Contains(p.SourceDir, "/examples/") || strings.Contains(p.SourceDir, "/example/") {
			return fmt.Errorf("%s: sourceDir %q is outside the official %s extension root", p.LogicalID, p.SourceDir, p.Implementation)
		}
		if _, err := os.Stat(filepath.Join(root, p.SourceDir)); err != nil {
			return fmt.Errorf("%s: sourceDir: %w", p.LogicalID, err)
		}
		dirClassifications[p.SourceDir] = true
		if !safeIDPattern.MatchString(filepath.Base(p.Image)) || !strings.HasPrefix(p.Image, "plugins/") {
			return fmt.Errorf("%s: unsafe or non-plugin image %q", p.LogicalID, p.Image)
		}
		stableRelease := false
		if p.ReleaseEligible {
			if p.UnmanagedReason != "" {
				return fmt.Errorf("%s: release-eligible plugin cannot have unmanagedReason", p.LogicalID)
			}
			versionData, err := os.ReadFile(filepath.Join(root, p.SourceDir, "VERSION"))
			if err != nil {
				return fmt.Errorf("%s: release-eligible plugin lacks VERSION: %w", p.LogicalID, err)
			}
			version, err := parseSemver(strings.TrimSpace(string(versionData)))
			if err != nil {
				return fmt.Errorf("%s: %w", p.LogicalID, err)
			}
			// Alpha denotes in-flight development and is excluded. A reviewed
			// non-alpha prerelease remains on the stabilization path, so it must
			// already have a complete Console marketplace contract.
			stableRelease = !isAlphaPrerelease(version.prerelease)
			versionPath := p.SourceDir + "/VERSION"
			// In a Git worktree, reject ignored/local VERSION files so CI sees the
			// same release inputs as a clean checkout. A git archive intentionally
			// has no .git directory; its extracted contents are already the exact
			// tracked tree and remain valid for archive-based verification.
			if err := exec.Command("git", "-C", root, "rev-parse", "--is-inside-work-tree").Run(); err == nil {
				tracked := exec.Command("git", "-C", root, "ls-files", "--error-unmatch", "--", versionPath)
				if err := tracked.Run(); err != nil {
					return fmt.Errorf("%s: release-eligible VERSION must be tracked by Git: %s", p.LogicalID, versionPath)
				}
			}
		} else if strings.TrimSpace(p.UnmanagedReason) == "" {
			return fmt.Errorf("%s: release-ineligible plugin requires unmanagedReason", p.LogicalID)
		}
		if len(p.ArtifactInputs) == 0 {
			return fmt.Errorf("%s: artifactInputs is empty", p.LogicalID)
		}
		for _, group := range p.SharedInputGroups {
			if _, ok := c.SharedInputGroups[group]; !ok {
				return fmt.Errorf("%s: unknown shared input group %q", p.LogicalID, group)
			}
		}
		if p.Consumers.Console != nil {
			cc := p.Consumers.Console
			if cc.PropertyKey == "" || cc.ResourceDir == "" || cc.URLForm != "oci" {
				return fmt.Errorf("%s: incomplete Console mapping", p.LogicalID)
			}
			if other := consumerMappings["console"][cc.PropertyKey]; other != "" {
				return fmt.Errorf("Console key %q belongs to both %s and %s", cc.PropertyKey, other, p.LogicalID)
			}
			consumerMappings["console"][cc.PropertyKey] = p.LogicalID
			if other := consoleResourceDirs[cc.ResourceDir]; other != "" {
				return fmt.Errorf("Console resourceDir %q belongs to both %s and %s", cc.ResourceDir, other, p.LogicalID)
			}
			consoleResourceDirs[cc.ResourceDir] = p.LogicalID
			bundle := cc.Marketplace
			if bundle == nil && c.ConsoleMarketplace != nil {
				if configured, ok := c.ConsoleMarketplace.Bundles[p.LogicalID]; ok {
					bundle = &configured
				}
			}
			if bundle != nil {
				if err := validateConsoleMarketplaceBundle(root, p, *bundle); err != nil {
					return err
				}
			}
		}
		if c.ConsoleMarketplace != nil && c.ConsoleMarketplace.RequiredForStable && stableRelease {
			_, hasBundle := c.ConsoleMarketplace.Bundles[p.LogicalID]
			if p.Consumers.Console == nil || (p.Consumers.Console.Marketplace == nil && !hasBundle) {
				return fmt.Errorf("%s: stable release-eligible plugin requires a reviewed Console marketplace mapping", p.LogicalID)
			}
		}
		if p.Consumers.PluginServer != nil {
			pc := p.Consumers.PluginServer
			if pc.InventoryKey == "" || pc.HTTPPath == "" {
				return fmt.Errorf("%s: incomplete plugin-server mapping", p.LogicalID)
			}
			if other := consumerMappings["pluginServer"][pc.InventoryKey]; other != "" {
				return fmt.Errorf("plugin-server key %q belongs to both %s and %s", pc.InventoryKey, other, p.LogicalID)
			}
			consumerMappings["pluginServer"][pc.InventoryKey] = p.LogicalID
			for _, alias := range pc.Aliases {
				if other := aliases[alias]; other != "" {
					return fmt.Errorf("plugin-server alias %q belongs to both %s and %s", alias, other, p.LogicalID)
				}
				aliases[alias] = p.LogicalID
			}
		}
	}
	for _, implementation := range []string{"go", "rust"} {
		rootDir := filepath.Join(root, "plugins", "wasm-"+implementation, "extensions")
		entries, err := os.ReadDir(rootDir)
		if err != nil {
			return err
		}
		for _, entry := range entries {
			if entry.IsDir() {
				source := filepath.ToSlash(filepath.Join("plugins", "wasm-"+implementation, "extensions", entry.Name()))
				if !dirClassifications[source] {
					return fmt.Errorf("unclassified official extension %s", source)
				}
			}
		}
	}
	for consumer, entries := range c.ConsumerInventories {
		mapping, ok := consumerMappings[consumer]
		if !ok {
			return fmt.Errorf("unknown consumer inventory %q", consumer)
		}
		seen := map[string]bool{}
		for _, entry := range entries {
			if seen[entry.Key] {
				return fmt.Errorf("%s inventory has duplicate %q", consumer, entry.Key)
			}
			seen[entry.Key] = true
			switch entry.Classification {
			case "managed":
				if entry.LogicalID == "" || mapping[entry.Key] != entry.LogicalID {
					return fmt.Errorf("%s inventory key %q has unresolved managed mapping %q", consumer, entry.Key, entry.LogicalID)
				}
			case "unmanaged":
				if entry.Reason == "" || entry.LogicalID != "" {
					return fmt.Errorf("%s inventory key %q requires only an unmanaged reason", consumer, entry.Key)
				}
			default:
				return fmt.Errorf("%s inventory key %q has invalid classification %q", consumer, entry.Key, entry.Classification)
			}
		}
		for key := range mapping {
			if !seen[key] {
				return fmt.Errorf("%s managed key %q is absent from classified inventory", consumer, key)
			}
		}
	}
	if c.ConsoleMarketplace != nil {
		for id := range c.ConsoleMarketplace.Bundles {
			if !seenLogical[id] {
				return fmt.Errorf("Console marketplace bundle names unknown plugin %q", id)
			}
		}
	}
	return nil
}

func validateConsoleMarketplaceBundle(root string, p Plugin, bundle ConsoleMarketplaceBundle) error {
	if bundle.Repository != "higress-group/higress" && bundle.Repository != "higress-group/higress-console" {
		return fmt.Errorf("%s: unsupported Console marketplace source repository %q", p.LogicalID, bundle.Repository)
	}
	if bundle.Repository == "higress-group/higress" {
		if bundle.SourceCommit != "" {
			return fmt.Errorf("%s: Higress marketplace bundle must use the exact dispatch commit", p.LogicalID)
		}
	} else if !commitPattern.MatchString(bundle.SourceCommit) {
		return fmt.Errorf("%s: external marketplace bundle requires an immutable full source commit", p.LogicalID)
	}
	if len(bundle.Files) == 0 {
		return fmt.Errorf("%s: Console marketplace bundle has no files", p.LogicalID)
	}
	required := map[string]bool{"spec.yaml": false, "README.md": false, "README_EN.md": false}
	seenTarget := map[string]bool{}
	for _, file := range bundle.Files {
		if !safeRepositoryPath(file.SourcePath) {
			return fmt.Errorf("%s: unsafe Console marketplace source path %q", p.LogicalID, file.SourcePath)
		}
		if file.TargetPath != "spec.yaml" && file.TargetPath != "README.md" && file.TargetPath != "README_EN.md" && file.TargetPath != "icon.png" {
			return fmt.Errorf("%s: unsupported Console marketplace target path %q", p.LogicalID, file.TargetPath)
		}
		if seenTarget[file.TargetPath] {
			return fmt.Errorf("%s: duplicate Console marketplace target path %q", p.LogicalID, file.TargetPath)
		}
		seenTarget[file.TargetPath] = true
		if _, ok := required[file.TargetPath]; ok {
			required[file.TargetPath] = true
		}
		if !regexp.MustCompile(`^[0-9a-f]{64}$`).MatchString(file.SHA256) {
			return fmt.Errorf("%s: invalid SHA-256 for Console marketplace target %q", p.LogicalID, file.TargetPath)
		}
		if bundle.Repository == "higress-group/higress" {
			data, err := readRepositoryFileWithoutSymlinks(root, file.SourcePath)
			if err != nil {
				return fmt.Errorf("%s: read Console marketplace source %q: %w", p.LogicalID, file.SourcePath, err)
			}
			if sha256Hex(data) != file.SHA256 {
				return fmt.Errorf("%s: Console marketplace source hash drift for %q", p.LogicalID, file.SourcePath)
			}
			if file.TargetPath == "spec.yaml" {
				text := string(data)
				if !regexp.MustCompile(`(?m)^  name:\s*`+regexp.QuoteMeta(p.Consumers.Console.ResourceDir)+`\s*$`).MatchString(text) ||
					!strings.Contains(text, "openAPIV3Schema:") || !regexp.MustCompile(`(?m)^      type:\s*object\s*$`).MatchString(text) {
					return fmt.Errorf("%s: reviewed Console spec has identity or schema drift", p.LogicalID)
				}
			}
		}
	}
	for target, present := range required {
		if !present {
			return fmt.Errorf("%s: Console marketplace bundle lacks required %s", p.LogicalID, target)
		}
	}
	return nil
}

func readRepositoryFileWithoutSymlinks(root, relative string) ([]byte, error) {
	if !safeRepositoryPath(relative) {
		return nil, fmt.Errorf("unsafe repository path %q", relative)
	}
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	checkPath := func() (os.FileInfo, string, error) {
		rootInfo, err := os.Lstat(absRoot)
		if err != nil {
			return nil, "", err
		}
		if rootInfo.Mode()&os.ModeSymlink != 0 || !rootInfo.IsDir() {
			return nil, "", errors.New("repository root must be a real directory")
		}
		current := absRoot
		var info os.FileInfo
		parts := strings.Split(relative, "/")
		for i, part := range parts {
			current = filepath.Join(current, part)
			info, err = os.Lstat(current)
			if err != nil {
				return nil, "", err
			}
			if info.Mode()&os.ModeSymlink != 0 {
				return nil, "", fmt.Errorf("repository path %q contains a symlink", relative)
			}
			if i < len(parts)-1 && !info.IsDir() {
				return nil, "", fmt.Errorf("repository path %q has a non-directory component", relative)
			}
		}
		if info == nil || !info.Mode().IsRegular() {
			return nil, "", fmt.Errorf("repository path %q is not a regular file", relative)
		}
		return info, current, nil
	}
	before, path, err := checkPath()
	if err != nil {
		return nil, err
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil {
		return nil, err
	}
	after, afterPath, err := checkPath()
	if err != nil {
		return nil, err
	}
	if path != afterPath || !os.SameFile(before, opened) || !os.SameFile(after, opened) {
		return nil, fmt.Errorf("repository path %q changed while it was validated", relative)
	}
	return io.ReadAll(file)
}

func safeRepositoryPath(path string) bool {
	if path == "" || filepath.IsAbs(path) || strings.Contains(path, `\`) {
		return false
	}
	clean := filepath.ToSlash(filepath.Clean(path))
	return clean == path && clean != "." && !strings.HasPrefix(clean, "../")
}

func buildPlan(root, catalogPath, previousPath, baseRef, targetRef, gatewayVersion, overridesPath string) (Plan, error) {
	if _, err := parseSemver(gatewayVersion); err != nil {
		return Plan{}, fmt.Errorf("gateway version: %w", err)
	}
	if err := validateCatalog(root, catalogPath); err != nil {
		return Plan{}, err
	}
	if previousPath == "" {
		return Plan{}, errors.New("no previous snapshot: import a reviewed digest baseline with bootstrap-snapshot before planning; bootstrap never rebuilds existing public tags")
	}
	c, catalogData, err := loadCatalog(catalogPath)
	if err != nil {
		return Plan{}, err
	}
	target, err := resolveCommit(root, targetRef)
	if err != nil {
		return Plan{}, err
	}
	var previous Snapshot
	previousEntries := map[string]SnapshotEntry{}
	explicitBase := baseRef != ""
	if previousPath != "" {
		if _, err := readJSON(previousPath, &previous); err != nil {
			return Plan{}, err
		}
		if previous.ProvenanceMode == "bootstrap-public" {
			if !explicitBase {
				return Plan{}, errors.New("a bootstrap-public previous snapshot requires one exact --base comparison commit")
			}
		} else if explicitBase {
			return Plan{}, errors.New("--base is allowed only with a bootstrap-public previous snapshot")
		}
		for _, entry := range previous.Plugins {
			previousEntries[entry.LogicalID] = entry
		}
		if baseRef == "" {
			baseRef = previous.SourceCommit
		}
	}
	base := ""
	if baseRef != "" {
		if explicitBase && !commitPattern.MatchString(baseRef) {
			return Plan{}, errors.New("bootstrap comparison --base must be a full lowercase 40-character commit")
		}
		base, err = resolveCommit(root, baseRef)
		if err != nil {
			return Plan{}, err
		}
		if explicitBase {
			if err := requireAncestor(root, base, target); err != nil {
				return Plan{}, err
			}
		}
	}
	paths, err := changedPaths(root, base, target)
	if err != nil {
		return Plan{}, err
	}
	overrides := map[string]string{}
	if overridesPath != "" {
		if _, err := readJSON(overridesPath, &overrides); err != nil {
			return Plan{}, err
		}
	}
	plugins := append([]Plugin(nil), c.Plugins...)
	sort.Slice(plugins, func(i, j int) bool { return plugins[i].LogicalID < plugins[j].LogicalID })
	plan := Plan{SchemaVersion: planSchemaVersion, GatewayVersion: gatewayVersion, SourceCommit: target, BaseCommit: base,
		PreviousRelease: previous.GatewayVersion, CatalogSHA256: sha256Hex(catalogData)}
	for _, p := range plugins {
		if !p.ReleaseEligible {
			continue
		}
		currentRaw, err := fileAtCommit(root, target, p.SourceDir+"/VERSION")
		if err != nil {
			return Plan{}, fmt.Errorf("%s VERSION: %w", p.LogicalID, err)
		}
		current := strings.TrimSpace(currentRaw)
		currentVersion, err := parseSemver(current)
		if err != nil {
			return Plan{}, fmt.Errorf("%s: %w", p.LogicalID, err)
		}
		// An alpha VERSION is a development build: it is deferred from release
		// selection and never creates candidates, public tags, latest movement,
		// or a new snapshot entry.
		if isAlphaPrerelease(currentVersion.prerelease) {
			plan.Deferred = append(plan.Deferred, DeferredPlugin{LogicalID: p.LogicalID, Version: current, Reason: "alpha-prerelease"})
			continue
		}
		changed := []string{}
		for _, path := range paths {
			if pluginInputMatches(c, p, path) || path == p.SourceDir+"/VERSION" {
				changed = append(changed, path)
			}
		}
		prev, hasPrevious := previousEntries[p.LogicalID]
		_, hasOverride := overrides[p.LogicalID]
		affected := base == "" || len(changed) > 0 || hasOverride
		if previous.ProvenanceMode == "bootstrap-public" && hasPrevious {
			previousVersion, parseErr := parseSemver(prev.Version)
			if parseErr != nil {
				return Plan{}, fmt.Errorf("%s previous bootstrap version: %w", p.LogicalID, parseErr)
			}
			// Do not carry a legacy public prerelease into the first managed
			// snapshot: promote it once and build immutable candidate evidence.
			affected = affected || previousVersion.prerelease != ""
		}
		if !hasPrevious && previousPath != "" {
			affected = true
		}
		if !affected {
			continue
		}
		version := current
		previousVersion := ""
		if hasPrevious {
			previousVersion = prev.Version
			if previous.ProvenanceMode == "bootstrap-public" {
				version, err = nextBootstrapVersion(prev.Version, current)
			} else {
				version, err = nextVersion(prev.Version, current)
			}
			if err != nil {
				return Plan{}, fmt.Errorf("%s: %w", p.LogicalID, err)
			}
		}
		if override, ok := overrides[p.LogicalID]; ok {
			overrideVersion, err := parseSemver(override)
			if err != nil || overrideVersion.prerelease != "" {
				return Plan{}, fmt.Errorf("%s: override %q must be stable SemVer", p.LogicalID, override)
			}
			if hasPrevious {
				prevVersion, _ := parseSemver(prev.Version)
				if compareSemver(overrideVersion, prevVersion) <= 0 {
					return Plan{}, fmt.Errorf("%s: override %q must be greater than previous %q", p.LogicalID, override, prev.Version)
				}
			}
			version = override
		}
		hash, err := inputHash(root, target, version, c, p)
		if err != nil {
			return Plan{}, fmt.Errorf("%s: %w", p.LogicalID, err)
		}
		// A plugin absent from the bootstrap baseline has no public artifact:
		// its candidate deterministically backfills the stable public tag. The
		// marker is bootstrap-only provenance; latest follows the normal
		// serialized monotonic policy. In a managed (non-bootstrap) release
		// the same path covers newly added catalog plugins without the marker.
		backfill := previous.ProvenanceMode == "bootstrap-public" && !hasPrevious
		plan.Plugins = append(plan.Plugins, PlanEntry{LogicalID: p.LogicalID, Implementation: p.Implementation,
			SourceDir: p.SourceDir, Image: p.Image, PreviousVersion: previousVersion, Version: version,
			InputHash: hash, ChangedPaths: changed, Backfill: backfill})
	}
	for id := range overrides {
		found := false
		for _, entry := range plan.Plugins {
			found = found || entry.LogicalID == id
		}
		if !found {
			for _, d := range plan.Deferred {
				if d.LogicalID == id {
					return Plan{}, fmt.Errorf("override %q targets %s, which is deferred from release selection as an alpha prerelease", overrides[id], id)
				}
			}
			return Plan{}, fmt.Errorf("override references unknown or release-ineligible plugin %q", id)
		}
	}
	plan.PlanID = "sha256:" + canonicalObjectHash(plan, true)
	return plan, nil
}

func canonicalObjectHash(value any, clearPlanID bool) string {
	if clearPlanID {
		if plan, ok := value.(Plan); ok {
			plan.PlanID = ""
			value = plan
		}
	}
	data, _ := json.Marshal(value)
	return sha256Hex(data)
}

func applyPlan(root string, plan Plan) error {
	for _, entry := range plan.Plugins {
		path := filepath.Join(root, entry.SourceDir, "VERSION")
		if err := os.WriteFile(path, []byte(entry.Version+"\n"), 0o644); err != nil {
			return err
		}
	}
	return nil
}

func renderSnapshot(catalogPath, planPath, previousPath, evidencePath, bootstrapEvidencePath string) (Snapshot, error) {
	c, catalogData, err := loadCatalog(catalogPath)
	if err != nil {
		return Snapshot{}, err
	}
	var plan Plan
	if _, err := readJSON(planPath, &plan); err != nil {
		return Snapshot{}, err
	}
	if plan.CatalogSHA256 != sha256Hex(catalogData) {
		return Snapshot{}, errors.New("plan catalogSha256 does not match catalog")
	}
	if plan.SchemaVersion != planSchemaVersion || !commitPattern.MatchString(plan.SourceCommit) || !digestPattern.MatchString(plan.PlanID) {
		return Snapshot{}, errors.New("plan has an unsupported schema or invalid immutable provenance")
	}
	var previous Snapshot
	previousEntries := map[string]SnapshotEntry{}
	if previousPath != "" {
		if _, err := readJSON(previousPath, &previous); err != nil {
			return Snapshot{}, err
		}
		for _, entry := range previous.Plugins {
			previousEntries[entry.LogicalID] = entry
		}
	}
	bootstrap := previous.ProvenanceMode == "bootstrap-public"
	var bootstrapEvidence BootstrapEvidenceFile
	var bootstrapEvidenceData []byte
	if bootstrap {
		if bootstrapEvidencePath == "" {
			return Snapshot{}, errors.New("a bootstrap-public previous snapshot requires --bootstrap-evidence so the first managed release carries an explicit committed marker")
		}
		data, err := readJSON(bootstrapEvidencePath, &bootstrapEvidence)
		if err != nil {
			return Snapshot{}, err
		}
		bootstrapEvidenceData = data
	} else if bootstrapEvidencePath != "" {
		return Snapshot{}, errors.New("--bootstrap-evidence is allowed only with a bootstrap-public previous snapshot")
	}
	var evidence CandidateEvidenceFile
	if _, err := readJSON(evidencePath, &evidence); err != nil {
		return Snapshot{}, err
	}
	planEntries := map[string]PlanEntry{}
	for _, entry := range plan.Plugins {
		if _, exists := planEntries[entry.LogicalID]; exists {
			return Snapshot{}, fmt.Errorf("plan contains duplicate plugin %s", entry.LogicalID)
		}
		if entry.Backfill {
			// The backfill marker is bootstrap-only stable candidate
			// provenance: it can never be introduced by a later managed
			// release, a prerelease, or over an existing baseline entry.
			if !bootstrap {
				return Snapshot{}, fmt.Errorf("plan marks %s backfill outside the first bootstrap release", entry.LogicalID)
			}
			if _, carried := previousEntries[entry.LogicalID]; carried {
				return Snapshot{}, fmt.Errorf("plan marks %s backfill although the bootstrap baseline already carries it", entry.LogicalID)
			}
			plannedVersion, err := parseSemver(entry.Version)
			if err != nil || plannedVersion.prerelease != "" {
				return Snapshot{}, fmt.Errorf("plan backfill %s must be a stable version, got %q", entry.LogicalID, entry.Version)
			}
		}
		planEntries[entry.LogicalID] = entry
	}
	deferred := map[string]bool{}
	for _, d := range plan.Deferred {
		if d.Reason != "alpha-prerelease" {
			return Snapshot{}, fmt.Errorf("plan defers %s with unsupported reason %q", d.LogicalID, d.Reason)
		}
		if _, ok := planEntries[d.LogicalID]; ok {
			return Snapshot{}, fmt.Errorf("plan both plans and defers %s", d.LogicalID)
		}
		if deferred[d.LogicalID] {
			return Snapshot{}, fmt.Errorf("plan defers %s twice", d.LogicalID)
		}
		deferred[d.LogicalID] = true
	}
	plugins := append([]Plugin(nil), c.Plugins...)
	sort.Slice(plugins, func(i, j int) bool { return plugins[i].LogicalID < plugins[j].LogicalID })
	snapshot := Snapshot{SchemaVersion: snapshotSchemaVersion, GatewayVersion: plan.GatewayVersion,
		SourceCommit: plan.SourceCommit, PreviousRelease: plan.PreviousRelease, CatalogSHA256: plan.CatalogSHA256, PlanID: plan.PlanID, ProvenanceMode: "candidate"}
	if bootstrap {
		snapshot.BootstrapEvidence = &SnapshotBootstrapEvidence{
			Path:   "plugins/release/bootstrap-evidence/" + plan.GatewayVersion + ".json",
			SHA256: sha256Hex(bootstrapEvidenceData),
		}
	}
	eligible := map[string]bool{}
	for _, p := range plugins {
		if !p.ReleaseEligible {
			continue
		}
		eligible[p.LogicalID] = true
		if planned, ok := planEntries[p.LogicalID]; ok {
			e, ok := evidence.Plugins[p.LogicalID]
			if !ok {
				return Snapshot{}, fmt.Errorf("missing candidate evidence for %s", p.LogicalID)
			}
			if !digestPattern.MatchString(e.Digest) || e.SourceCommit != plan.SourceCommit || e.InputHash != planned.InputHash || !strings.HasSuffix(e.CandidateRef, "@"+e.Digest) {
				return Snapshot{}, fmt.Errorf("candidate evidence for %s does not match plan provenance", p.LogicalID)
			}
			snapshot.Plugins = append(snapshot.Plugins, SnapshotEntry{LogicalID: p.LogicalID, Implementation: p.Implementation,
				SourceDir: p.SourceDir, Image: p.Image, Version: planned.Version,
				OCIRef: c.Registry + "/" + p.Image + ":" + planned.Version, Digest: e.Digest,
				InputHash: planned.InputHash, SourceCommit: plan.SourceCommit, CandidateRef: e.CandidateRef,
				ProvenanceMode: "candidate", Backfill: planned.Backfill, Consumers: catalogConsumers(c, p)})
			continue
		}
		old, ok := previousEntries[p.LogicalID]
		if !ok {
			// A deferred alpha plugin without a previous release simply has no
			// snapshot entry; it is ignored for release selection.
			if deferred[p.LogicalID] {
				continue
			}
			return Snapshot{}, fmt.Errorf("release-eligible plugin %s is neither planned nor present in previous snapshot", p.LogicalID)
		}
		old.Consumers = catalogConsumers(c, p)
		if old.ProvenanceMode == "" {
			old.ProvenanceMode = entryProvenance(old)
		}
		if old.ProvenanceMode == "public" {
			snapshot.ProvenanceMode = "mixed"
		}
		snapshot.Plugins = append(snapshot.Plugins, old)
	}
	for id := range deferred {
		if !eligible[id] {
			return Snapshot{}, fmt.Errorf("plan defers unknown or release-ineligible plugin %q", id)
		}
	}
	if bootstrap {
		if err := bindBootstrapEvidence(snapshot, bootstrapEvidence); err != nil {
			return Snapshot{}, err
		}
	}
	return snapshot, nil
}

// bindBootstrapEvidence ties the first managed snapshot to the reviewed
// bootstrap evidence byte-for-byte semantics: every imported public entry must
// match a "public" classification with the identical version and digest, every
// backfill entry must match a "missing" classification with the identical
// version, every "missing" claim must be realized as exactly one backfill
// entry, and a deferred alpha plugin must not appear in the snapshot.
func bindBootstrapEvidence(snapshot Snapshot, evidence BootstrapEvidenceFile) error {
	backfilled := map[string]bool{}
	for _, entry := range snapshot.Plugins {
		e, ok := evidence.Plugins[entry.LogicalID]
		if !ok {
			return fmt.Errorf("bootstrap evidence lacks an entry for %s", entry.LogicalID)
		}
		switch {
		case entry.Backfill:
			if e.Status != "missing" || e.Version != entry.Version {
				return fmt.Errorf("backfill entry %s does not match a reviewed missing bootstrap classification", entry.LogicalID)
			}
			backfilled[entry.LogicalID] = true
		case entryProvenance(entry) == "public":
			if e.Status != "public" || e.Version != entry.Version || e.Digest != entry.Digest {
				return fmt.Errorf("imported public entry %s does not match the reviewed bootstrap evidence", entry.LogicalID)
			}
		}
	}
	for id, e := range evidence.Plugins {
		switch e.Status {
		case "missing":
			if !backfilled[id] {
				return fmt.Errorf("reviewed missing bootstrap artifact %s was not realized as a backfill entry", id)
			}
		case "deferred":
			for _, entry := range snapshot.Plugins {
				if entry.LogicalID == id {
					return fmt.Errorf("deferred alpha plugin %s must not appear in the first managed snapshot", id)
				}
			}
		}
	}
	return nil
}

func verifySnapshot(root, catalogPath, snapshotPath, expectedSource, committedSource string, resolve bool, ociSource string) error {
	return verifySnapshotBindings(root, catalogPath, snapshotPath, "", "", expectedSource, committedSource, resolve, ociSource)
}

func verifySnapshotBindings(root, catalogPath, snapshotPath, planPath, previousPath, expectedSource, committedSource string, resolve bool, ociSource string) error {
	if err := validateCatalog(root, catalogPath); err != nil {
		return err
	}
	c, catalogData, err := loadCatalog(catalogPath)
	if err != nil {
		return err
	}
	var snapshot Snapshot
	if _, err := readJSON(snapshotPath, &snapshot); err != nil {
		return err
	}
	if snapshot.SchemaVersion != snapshotSchemaVersion || snapshot.CatalogSHA256 != sha256Hex(catalogData) {
		return errors.New("snapshot schema or catalog hash mismatch")
	}
	if _, err := parseSemver(snapshot.GatewayVersion); err != nil {
		return fmt.Errorf("snapshot gatewayVersion: %w", err)
	}
	if !commitPattern.MatchString(snapshot.SourceCommit) {
		return errors.New("snapshot sourceCommit must be a full lowercase Git commit")
	}
	if snapshot.ProvenanceMode != "candidate" && snapshot.ProvenanceMode != "bootstrap-public" && snapshot.ProvenanceMode != "mixed" {
		return fmt.Errorf("snapshot has unsupported provenanceMode %q", snapshot.ProvenanceMode)
	}
	if ociSource != "candidate" && ociSource != "public" {
		return fmt.Errorf("OCI source must be candidate or public, got %q", ociSource)
	}
	if err := validateSnapshotMigration(snapshot); err != nil {
		return err
	}
	if expectedSource != "" {
		commit, err := resolveCommit(root, expectedSource)
		if err != nil || commit != snapshot.SourceCommit {
			return fmt.Errorf("snapshot sourceCommit %s does not match expected source %s", snapshot.SourceCommit, expectedSource)
		}
	}
	committed := ""
	if committedSource != "" {
		var err error
		committed, err = resolveCommit(root, committedSource)
		if err != nil {
			return err
		}
	}
	plugins := map[string]Plugin{}
	deferred := map[string]string{}
	for _, p := range c.Plugins {
		plugins[p.LogicalID] = p
		if !p.ReleaseEligible {
			continue
		}
		// Deferral is recomputed independently from the exact snapshot source
		// commit so a snapshot cannot smuggle or drop an alpha-versioned entry.
		raw, err := fileAtCommit(root, snapshot.SourceCommit, p.SourceDir+"/VERSION")
		if err != nil {
			return fmt.Errorf("%s VERSION at snapshot source commit: %w", p.LogicalID, err)
		}
		version := strings.TrimSpace(raw)
		parsed, err := parseSemver(version)
		if err != nil {
			return fmt.Errorf("%s: %w", p.LogicalID, err)
		}
		if isAlphaPrerelease(parsed.prerelease) {
			deferred[p.LogicalID] = version
		}
	}
	seen := map[string]bool{}
	last := ""
	for _, entry := range snapshot.Plugins {
		p, ok := plugins[entry.LogicalID]
		if !ok || !p.ReleaseEligible || seen[entry.LogicalID] {
			return fmt.Errorf("snapshot has unknown, ineligible, or duplicate plugin %q", entry.LogicalID)
		}
		if last != "" && entry.LogicalID < last {
			return errors.New("snapshot plugins are not sorted by logicalId")
		}
		last, seen[entry.LogicalID] = entry.LogicalID, true
		if entry.Image != p.Image || entry.SourceDir != p.SourceDir || entry.Implementation != p.Implementation {
			return fmt.Errorf("%s snapshot identity differs from catalog", entry.LogicalID)
		}
		if !reflect.DeepEqual(entry.Consumers, catalogConsumers(c, p)) {
			return fmt.Errorf("%s snapshot consumer mappings differ from catalog", entry.LogicalID)
		}
		if _, err := parseSemver(entry.Version); err != nil {
			return fmt.Errorf("%s: %w", entry.LogicalID, err)
		}
		if _, isDeferred := deferred[entry.LogicalID]; isDeferred {
			// A deferred alpha plugin may only carry an earlier stable release
			// forward; the alpha build itself never becomes a snapshot entry.
			entryVersion, _ := parseSemver(entry.Version)
			if entryVersion.prerelease != "" {
				return fmt.Errorf("%s is deferred as an alpha prerelease and must not hold a prerelease snapshot entry", entry.LogicalID)
			}
		}
		expectedRef := c.Registry + "/" + p.Image + ":" + entry.Version
		if entry.OCIRef != expectedRef || !digestPattern.MatchString(entry.Digest) || !digestPattern.MatchString(entry.InputHash) {
			return fmt.Errorf("%s has invalid OCI or provenance fields", entry.LogicalID)
		}
		provenance := entryProvenance(entry)
		if provenance == "candidate" && !strings.HasSuffix(entry.CandidateRef, "@"+entry.Digest) {
			return fmt.Errorf("%s candidate provenance does not pin the snapshot digest", entry.LogicalID)
		}
		if provenance == "public" && entry.CandidateRef != "" {
			return fmt.Errorf("%s bootstrap snapshot must not claim candidate provenance", entry.LogicalID)
		}
		if provenance != "candidate" && provenance != "public" {
			return fmt.Errorf("%s has invalid artifact provenance", entry.LogicalID)
		}
		if entry.Backfill {
			entryVersion, _ := parseSemver(entry.Version)
			if provenance != "candidate" || entryVersion.prerelease != "" {
				return fmt.Errorf("%s backfill entries must be stable candidate provenance", entry.LogicalID)
			}
		}
		hash, err := inputHash(root, entry.SourceCommit, entry.Version, c, p)
		if err != nil || hash != entry.InputHash {
			return fmt.Errorf("%s input hash does not recompute from sourceCommit and proposed version", entry.LogicalID)
		}
		_, isDeferred := deferred[entry.LogicalID]
		if committed != "" && !isDeferred {
			versionAtCommit, err := fileAtCommit(root, committed, p.SourceDir+"/VERSION")
			if err != nil || strings.TrimSpace(versionAtCommit) != entry.Version {
				return fmt.Errorf("%s VERSION at committed source does not equal snapshot version", entry.LogicalID)
			}
			committedHash, err := inputHash(root, committed, entry.Version, c, p)
			if err != nil || committedHash != entry.InputHash {
				return fmt.Errorf("%s committed source artifact inputs differ from snapshot", entry.LogicalID)
			}
		}
		if resolve {
			if ociSource == "candidate" && provenance == "public" {
				continue
			}
			if ociSource == "public" && entry.Migration != nil {
				// The prepare-time sweep already proved this public tag serves a
				// different digest. That divergence is the recorded exclusion
				// from this promote batch, not a verification failure; the
				// candidate for the same entry is still resolved above.
				continue
			}
			if err := verifyOCI(entry, provenance, snapshot.ProvenanceMode, ociSource); err != nil {
				return fmt.Errorf("%s: %w", entry.LogicalID, err)
			}
		}
	}
	for _, p := range c.Plugins {
		if !p.ReleaseEligible {
			continue
		}
		if _, isDeferred := deferred[p.LogicalID]; isDeferred {
			continue
		}
		if !seen[p.LogicalID] {
			return fmt.Errorf("release-eligible plugin %s is missing from snapshot", p.LogicalID)
		}
	}
	return verifySnapshotProvenanceBindings(root, snapshot, planPath, previousPath, deferred)
}

// verifySnapshotProvenanceBindings binds the snapshot to the exact plan and
// previous snapshot it claims to derive from and validates the committed
// bootstrap evidence marker. Binding is fail-closed in both directions: a
// planned backfill without the marker, a marker whose committed evidence is
// absent or different, and a snapshot entry bound to neither plan nor previous
// snapshot (when binding is requested) are all rejected. The marker is never
// inferred from a missing previous-snapshot file; it is explicit snapshot
// provenance validated against committed evidence bytes.
func verifySnapshotProvenanceBindings(root string, snapshot Snapshot, planPath, previousPath string, deferred map[string]string) error {
	var planEntries map[string]PlanEntry
	if planPath != "" {
		var plan Plan
		if _, err := readJSON(planPath, &plan); err != nil {
			return err
		}
		gatewayVersion, err := parseSemver(plan.GatewayVersion)
		if err != nil || gatewayVersion.prerelease != "" {
			return errors.New("plan gatewayVersion must be stable SemVer")
		}
		if plan.SchemaVersion != planSchemaVersion || !commitPattern.MatchString(plan.SourceCommit) ||
			!digestPattern.MatchString(plan.PlanID) || (plan.BaseCommit != "" && !commitPattern.MatchString(plan.BaseCommit)) {
			return errors.New("plan has an unsupported schema or invalid immutable provenance")
		}
		if plan.PreviousRelease != "" {
			previousVersion, err := parseSemver(plan.PreviousRelease)
			if err != nil || previousVersion.prerelease != "" {
				return errors.New("plan previousRelease must be stable SemVer")
			}
		}
		if want := "sha256:" + canonicalObjectHash(plan, true); plan.PlanID != want {
			return fmt.Errorf("planId does not match canonical plan content: got %s, want %s", plan.PlanID, want)
		}
		if plan.GatewayVersion != snapshot.GatewayVersion || plan.SourceCommit != snapshot.SourceCommit ||
			plan.PreviousRelease != snapshot.PreviousRelease || plan.PlanID != snapshot.PlanID || plan.CatalogSHA256 != snapshot.CatalogSHA256 {
			return errors.New("plan and snapshot immutable provenance differ")
		}
		planEntries = map[string]PlanEntry{}
		planBackfill := false
		for _, entry := range plan.Plugins {
			if _, exists := planEntries[entry.LogicalID]; exists {
				return fmt.Errorf("plan contains duplicate plugin %s", entry.LogicalID)
			}
			planEntries[entry.LogicalID] = entry
			planBackfill = planBackfill || entry.Backfill
		}
		if planBackfill && snapshot.BootstrapEvidence == nil {
			return errors.New("plan marks backfill but the snapshot carries no committed bootstrap evidence marker")
		}
		planDeferred := map[string]DeferredPlugin{}
		for _, d := range plan.Deferred {
			if _, duplicate := planDeferred[d.LogicalID]; duplicate {
				return fmt.Errorf("plan defers %s more than once", d.LogicalID)
			}
			if _, planned := planEntries[d.LogicalID]; planned {
				return fmt.Errorf("plan both plans and defers %s", d.LogicalID)
			}
			version, isDeferred := deferred[d.LogicalID]
			if !isDeferred || d.Version != version || d.Reason != "alpha-prerelease" {
				return fmt.Errorf("plan deferral for %s does not match the alpha prerelease at the snapshot source", d.LogicalID)
			}
			planDeferred[d.LogicalID] = d
		}
		for id, version := range deferred {
			if d, ok := planDeferred[id]; !ok || d.Version != version {
				return fmt.Errorf("plan omits the exact alpha deferral for %s at version %s", id, version)
			}
		}
	}
	var previousEntries map[string]SnapshotEntry
	if previousPath != "" {
		var previous Snapshot
		if _, err := readJSON(previousPath, &previous); err != nil {
			return err
		}
		previousVersion, err := parseSemver(previous.GatewayVersion)
		if err != nil || previousVersion.prerelease != "" || previous.SchemaVersion != snapshotSchemaVersion ||
			previous.GatewayVersion != snapshot.PreviousRelease {
			return errors.New("previous snapshot does not match the exact stable previousRelease")
		}
		if snapshot.BootstrapEvidence != nil && previous.ProvenanceMode != "bootstrap-public" {
			return errors.New("bootstrap evidence marker is allowed only on the first managed release from the bootstrap baseline")
		}
		previousEntries = map[string]SnapshotEntry{}
		for _, entry := range previous.Plugins {
			if _, duplicate := previousEntries[entry.LogicalID]; duplicate {
				return fmt.Errorf("previous snapshot contains duplicate plugin %s", entry.LogicalID)
			}
			previousEntries[entry.LogicalID] = entry
		}
	}
	snapshotEntries := make(map[string]SnapshotEntry, len(snapshot.Plugins))
	for _, entry := range snapshot.Plugins {
		snapshotEntries[entry.LogicalID] = entry
	}
	if planEntries != nil || previousEntries != nil {
		for _, entry := range snapshot.Plugins {
			if planned, ok := planEntries[entry.LogicalID]; ok {
				if planned.LogicalID != entry.LogicalID || planned.Implementation != entry.Implementation ||
					planned.SourceDir != entry.SourceDir || planned.Image != entry.Image || planned.Version != entry.Version ||
					planned.InputHash != entry.InputHash || planned.Backfill != entry.Backfill ||
					entry.SourceCommit != snapshot.SourceCommit {
					return fmt.Errorf("%s snapshot entry differs from its plan entry", entry.LogicalID)
				}
				continue
			}
			if previousEntries != nil {
				old, ok := previousEntries[entry.LogicalID]
				if !ok {
					return fmt.Errorf("%s is neither planned nor carried by the previous snapshot", entry.LogicalID)
				}
				// Consumers are re-cloned from the catalog on carry, so the
				// exact-field comparison excludes them deliberately. A carried
				// migration exclusion is compared verbatim: the entry stays out
				// of the promote batch until a later release re-plans the plugin
				// after its disposition completes, and dropping the marker would
				// re-expose the immutable tag conflict that created it.
				if old.Version != entry.Version || old.OCIRef != entry.OCIRef || old.Digest != entry.Digest ||
					old.InputHash != entry.InputHash || old.SourceCommit != entry.SourceCommit ||
					old.CandidateRef != entry.CandidateRef || old.ProvenanceMode != entry.ProvenanceMode ||
					old.Backfill != entry.Backfill || !reflect.DeepEqual(old.Migration, entry.Migration) {
					return fmt.Errorf("%s carried snapshot entry differs from the previous snapshot", entry.LogicalID)
				}
				continue
			}
			// The first managed release carries public baseline entries whose
			// previous snapshot is the uncommitted bootstrap baseline; those
			// entries are bound by the committed bootstrap evidence instead.
			if snapshot.BootstrapEvidence == nil || entryProvenance(entry) != "public" {
				return fmt.Errorf("%s is bound to neither a plan, a previous snapshot, nor committed bootstrap evidence", entry.LogicalID)
			}
		}
		for id := range planEntries {
			if _, present := snapshotEntries[id]; !present {
				return fmt.Errorf("planned plugin %s is missing from the snapshot", id)
			}
		}
		if previousEntries != nil {
			for id := range deferred {
				if _, existed := previousEntries[id]; !existed {
					continue
				}
				if _, carried := snapshotEntries[id]; !carried {
					return fmt.Errorf("deferred alpha plugin %s dropped its previous stable snapshot entry", id)
				}
			}
		}
	}
	if snapshot.BootstrapEvidence == nil {
		return nil
	}
	if snapshot.ProvenanceMode == "bootstrap-public" {
		return errors.New("the bootstrap baseline snapshot itself never carries a bootstrap evidence marker")
	}
	marker := snapshot.BootstrapEvidence
	wantPath := "plugins/release/bootstrap-evidence/" + snapshot.GatewayVersion + ".json"
	if marker.Path != wantPath {
		return fmt.Errorf("bootstrap evidence marker path %q must be the deterministic committed path %q", marker.Path, wantPath)
	}
	evidencePath := filepath.Join(root, filepath.FromSlash(marker.Path))
	data, err := os.ReadFile(evidencePath)
	if err != nil {
		return fmt.Errorf("committed bootstrap evidence %s: %w", marker.Path, err)
	}
	if sha256Hex(data) != marker.SHA256 {
		return fmt.Errorf("committed bootstrap evidence %s does not match the snapshot marker sha256", marker.Path)
	}
	var evidence BootstrapEvidenceFile
	if _, err := readJSON(evidencePath, &evidence); err != nil {
		return err
	}
	return bindBootstrapEvidence(snapshot, evidence)
}

func entryProvenance(entry SnapshotEntry) string {
	if entry.ProvenanceMode != "" {
		return entry.ProvenanceMode
	}
	if entry.CandidateRef != "" {
		return "candidate"
	}
	return "public"
}

func verifyOCI(entry SnapshotEntry, provenanceMode, snapshotMode, ociSource string) error {
	ref := entry.OCIRef
	if ociSource == "candidate" {
		ref = entry.CandidateRef
	}
	manifest, err := ociManifestResolver(ref)
	if err != nil {
		return fmt.Errorf("resolve %s: %w", ref, err)
	}
	if manifest.Digest != entry.Digest {
		return fmt.Errorf("resolved digest %s does not match snapshot digest %s", manifest.Digest, entry.Digest)
	}
	if snapshotMode == "bootstrap-public" {
		if ociSource != "public" || provenanceMode != "public" || entry.CandidateRef != "" {
			return errors.New("bootstrap snapshots may resolve only exact public artifacts")
		}
		return nil
	}
	// Historical production images predate release annotations. For a public
	// entry the reviewed bootstrap evidence plus the resolved digest is the
	// only provenance available; verification must not require or invent
	// source/input annotations for it.
	if provenanceMode == "public" {
		return nil
	}
	if manifest.Annotations["org.opencontainers.image.revision"] != entry.SourceCommit ||
		manifest.Annotations["io.higress.plugin.input-hash"] != entry.InputHash ||
		manifest.Annotations["org.opencontainers.image.version"] != entry.Version {
		return errors.New("OCI provenance annotations do not match snapshot")
	}
	return nil
}

func resolveOCIManifest(ref string) (ociManifest, error) {
	descriptorOut, err := runORASManifestFetch("oras manifest fetch --descriptor", "manifest", "fetch", ref, "--descriptor")
	if err != nil {
		return ociManifest{}, err
	}
	var descriptor struct {
		Digest string `json:"digest"`
	}
	if err := json.Unmarshal(descriptorOut, &descriptor); err != nil {
		return ociManifest{}, err
	}
	manifestOut, err := runORASManifestFetch("oras manifest fetch", "manifest", "fetch", ref)
	if err != nil {
		return ociManifest{}, err
	}
	var manifest struct {
		Annotations map[string]string `json:"annotations"`
	}
	if err := json.Unmarshal(manifestOut, &manifest); err != nil {
		return ociManifest{}, err
	}
	return ociManifest{Digest: descriptor.Digest, Annotations: manifest.Annotations}, nil
}

func runORAS(args ...string) ([]byte, string, error) {
	cmd := exec.Command("oras", args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	output, err := cmd.Output()
	return output, stderr.String(), err
}

func runORASManifestFetch(operation string, args ...string) ([]byte, error) {
	output, stderr, err := orasRunner(args...)
	if err == nil {
		return output, nil
	}
	if detail := sanitizeCommandStderr(stderr); detail != "" {
		return nil, fmt.Errorf("%s failed: %w: %s", operation, err, detail)
	}
	return nil, fmt.Errorf("%s failed: %w", operation, err)
}

func sanitizeCommandStderr(stderr string) string {
	detail := strings.TrimSpace(stderr)
	detail = credentialURLPattern.ReplaceAllString(detail, "$1[REDACTED]@")
	detail = sensitiveValuePattern.ReplaceAllString(detail, "$1[REDACTED]")
	detail = authorizationValuePattern.ReplaceAllString(detail, "$1[REDACTED]")
	return detail
}

func digestReference(ociRef, digest string) (string, error) {
	ref := strings.TrimPrefix(ociRef, "oci://")
	slash := strings.LastIndex(ref, "/")
	colon := strings.LastIndex(ref, ":")
	if slash < 0 || colon <= slash || !digestPattern.MatchString(digest) {
		return "", fmt.Errorf("invalid OCI reference or digest")
	}
	return ref[:colon] + "@" + digest, nil
}

func parseCommon(name string, args []string) (*flag.FlagSet, *string, *string) {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	root := fs.String("root", ".", "repository root")
	catalog := fs.String("catalog", "plugins/release/catalog.json", "catalog path")
	return fs, root, catalog
}

func commandValidate(args []string) error {
	fs, root, catalog := parseCommon("validate-catalog", args)
	if err := fs.Parse(args); err != nil {
		return err
	}
	return validateCatalog(*root, *catalog)
}

func commandValidateConsoleRecovery(args []string) error {
	fs, root, catalog := parseCommon("validate-console-recovery", args)
	manifest := fs.String("manifest", "plugins/release/console-recovery/2.2.4.json", "one-time Console recovery manifest")
	if err := fs.Parse(args); err != nil {
		return err
	}
	return validateConsoleRecovery(*root, *catalog, *manifest)
}

func validateConsoleRecovery(root, catalogPath, manifestPath string) error {
	if err := validateCatalog(root, catalogPath); err != nil {
		return err
	}
	catalog, _, err := loadCatalog(catalogPath)
	if err != nil {
		return err
	}
	var manifest ConsoleRecoveryManifest
	if _, err := readJSON(manifestPath, &manifest); err != nil {
		return err
	}
	if manifest.SchemaVersion != 1 || manifest.GatewayVersion != "2.2.4" ||
		manifest.SnapshotPath != "plugins/release/snapshots/2.2.4.json" ||
		manifest.ImageRepository != "higress-registry.cn-hangzhou.cr.aliyuncs.com/higress/console" ||
		manifest.OriginalConsoleCommit != "36aa9c67fb0057164dab9b1fe687b38fe5b8a022" ||
		manifest.OriginalImageDigest != "sha256:c8cb47ad0a550e58df4cfee57f2f358eb0b1635a0812c77e04388dfb17bbebb6" ||
		manifest.RequiredSourceBranch != "main" {
		return errors.New("Console recovery contract is restricted to the not-yet-public higress/console:2.2.4 image from merged main")
	}
	snapshotPath := filepath.Join(root, filepath.FromSlash(manifest.SnapshotPath))
	snapshotData, err := os.ReadFile(snapshotPath)
	if err != nil {
		return err
	}
	if !regexp.MustCompile(`^[0-9a-f]{64}$`).MatchString(manifest.SnapshotSHA256) || sha256Hex(snapshotData) != manifest.SnapshotSHA256 {
		return errors.New("Console recovery manifest does not bind the unchanged 2.2.4 snapshot bytes")
	}
	var snapshot Snapshot
	if err := json.Unmarshal(snapshotData, &snapshot); err != nil {
		return err
	}
	if snapshot.GatewayVersion != "2.2.4" {
		return errors.New("Console recovery snapshot is not 2.2.4")
	}
	expectedIDs := []string{"ai-context-limit", "gw-error-format", "hmac-auth-apisix", "log-request-response", "mcp-router", "nginx-rewrite-compatible", "response-cache", "simple-jwt-auth"}
	snapshotByID := map[string]SnapshotEntry{}
	for _, entry := range snapshot.Plugins {
		snapshotByID[entry.LogicalID] = entry
	}
	catalogByID := map[string]Plugin{}
	for _, plugin := range catalog.Plugins {
		catalogByID[plugin.LogicalID] = plugin
	}
	if len(manifest.Plugins) != len(expectedIDs) {
		return fmt.Errorf("Console recovery must contain exactly %d reviewed plugins", len(expectedIDs))
	}
	for i, id := range expectedIDs {
		entry := manifest.Plugins[i]
		if entry.LogicalID != id {
			return fmt.Errorf("Console recovery plugins must be sorted and exact: expected %s at index %d", id, i)
		}
		snapshotEntry, ok := snapshotByID[id]
		if !ok || entry.Version != snapshotEntry.Version || entry.OCIRef != snapshotEntry.OCIRef || entry.Digest != snapshotEntry.Digest {
			return fmt.Errorf("%s: recovery artifact identity differs from unchanged 2.2.4 snapshot", id)
		}
		plugin, ok := catalogByID[id]
		expectedConsumers := catalogConsumers(catalog, plugin)
		if !ok || expectedConsumers.Console == nil || !reflect.DeepEqual(entry.Console, *expectedConsumers.Console) {
			return fmt.Errorf("%s: recovery Console mapping differs from reviewed catalog", id)
		}
	}
	return nil
}

func commandPlan(args []string) error {
	fs, root, catalog := parseCommon("plan", args)
	previous := fs.String("previous", "", "previous snapshot")
	base := fs.String("base", "", "base Git ref (defaults to previous sourceCommit)")
	target := fs.String("target", "HEAD", "exact target Git ref")
	gateway := fs.String("gateway-version", "", "target gateway version")
	overrides := fs.String("overrides", "", "reviewed logicalId-to-version JSON")
	output := fs.String("output", "-", "plan output")
	if err := fs.Parse(args); err != nil {
		return err
	}
	plan, err := buildPlan(*root, *catalog, *previous, *base, *target, *gateway, *overrides)
	if err != nil {
		return err
	}
	return writeCanonical(*output, plan)
}

// captureBootstrapEvidence classifies the exact public tag selected by every
// release-eligible VERSION in the target tree. The evidence intentionally has
// no source/input fields: embedding the commit of the PR that commits the
// evidence would be self-referential. bootstrap-snapshot recomputes source and
// input provenance independently when it consumes these reviewed public refs.
//
// An alpha prerelease VERSION is a development build and is deferred without
// resolution. A stable VERSION whose public tag is genuinely absent is marked
// missing so the plan can backfill it with a content-addressed candidate.
// A 401/403 is an authorization/configuration error and aborts the capture; it
// is never recorded as an absent artifact.
func captureBootstrapEvidence(root, catalogPath, source string) (BootstrapEvidenceFile, error) {
	if !commitPattern.MatchString(source) {
		return BootstrapEvidenceFile{}, errors.New("bootstrap evidence source must be a full lowercase 40-character commit")
	}
	if err := validateCatalog(root, catalogPath); err != nil {
		return BootstrapEvidenceFile{}, err
	}
	c, _, err := loadCatalog(catalogPath)
	if err != nil {
		return BootstrapEvidenceFile{}, err
	}
	commit, err := resolveCommit(root, source)
	if err != nil {
		return BootstrapEvidenceFile{}, err
	}
	plugins := append([]Plugin(nil), c.Plugins...)
	sort.Slice(plugins, func(i, j int) bool { return plugins[i].LogicalID < plugins[j].LogicalID })
	evidence := BootstrapEvidenceFile{Plugins: map[string]BootstrapEvidence{}}
	for _, p := range plugins {
		if !p.ReleaseEligible {
			continue
		}
		raw, err := fileAtCommit(root, commit, p.SourceDir+"/VERSION")
		if err != nil {
			return BootstrapEvidenceFile{}, fmt.Errorf("%s VERSION: %w", p.LogicalID, err)
		}
		version := strings.TrimSpace(raw)
		parsed, err := parseSemver(version)
		if err != nil {
			return BootstrapEvidenceFile{}, fmt.Errorf("%s: %w", p.LogicalID, err)
		}
		if isAlphaPrerelease(parsed.prerelease) {
			evidence.Plugins[p.LogicalID] = BootstrapEvidence{Status: "deferred", Version: version}
			continue
		}
		publicRef := c.Registry + "/" + p.Image + ":" + version
		manifest, err := ociManifestResolver(publicRef)
		if err != nil {
			switch classifyOCIFailure(err, publicRef) {
			case ociFailureUnauthorized:
				return BootstrapEvidenceFile{}, fmt.Errorf("bootstrap public artifact %s: registry authorization failed; configure the documented least-privilege read-only registry credential, because a 401/403 is never an absent artifact: %w", p.LogicalID, err)
			case ociFailureNotFound:
				if parsed.prerelease != "" {
					return BootstrapEvidenceFile{}, fmt.Errorf("bootstrap public artifact %s is absent and %q is a non-alpha prerelease; only stable versions may be backfilled: %w", p.LogicalID, version, err)
				}
				evidence.Plugins[p.LogicalID] = BootstrapEvidence{Status: "missing", Version: version, PublicRef: publicRef}
				continue
			default:
				return BootstrapEvidenceFile{}, fmt.Errorf("resolve bootstrap public artifact %s: %w", p.LogicalID, err)
			}
		}
		if !digestPattern.MatchString(manifest.Digest) {
			return BootstrapEvidenceFile{}, fmt.Errorf("bootstrap public artifact %s returned invalid digest %q", p.LogicalID, manifest.Digest)
		}
		evidence.Plugins[p.LogicalID] = BootstrapEvidence{Status: "public", Version: version, PublicRef: publicRef, Digest: manifest.Digest}
	}
	return evidence, nil
}

func commandCaptureBootstrapEvidence(args []string) error {
	fs, root, catalog := parseCommon("capture-bootstrap-evidence", args)
	source := fs.String("source", "HEAD", "exact target source commit")
	output := fs.String("output", "-", "canonical public evidence output")
	if err := fs.Parse(args); err != nil {
		return err
	}
	evidence, err := captureBootstrapEvidence(*root, *catalog, *source)
	if err != nil {
		return err
	}
	return writeCanonical(*output, evidence)
}

// bootstrap-snapshot imports already-published production artifacts into the
// first reviewed baseline. It never builds or writes an OCI tag: public
// evidence must resolve to the current public digest and match the checked-in
// VERSION/input. A stable artifact classified missing is left absent so the
// plan can backfill it with a content-addressed candidate, and a deferred
// alpha VERSION has no baseline entry at all.
func commandBootstrap(args []string) error {
	fs, root, catalog := parseCommon("bootstrap-snapshot", args)
	gateway := fs.String("gateway-version", "", "baseline gateway version")
	source := fs.String("source", "HEAD", "exact source commit")
	evidencePath := fs.String("existing-evidence", "", "reviewed public digest evidence JSON")
	output := fs.String("output", "-", "snapshot output")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *evidencePath == "" {
		return errors.New("--existing-evidence is required")
	}
	if _, err := parseSemver(*gateway); err != nil {
		return err
	}
	if err := validateCatalog(*root, *catalog); err != nil {
		return err
	}
	c, bytes, err := loadCatalog(*catalog)
	if err != nil {
		return err
	}
	commit, err := resolveCommit(*root, *source)
	if err != nil {
		return err
	}
	var evidence BootstrapEvidenceFile
	if _, err := readJSON(*evidencePath, &evidence); err != nil {
		return err
	}
	snapshot := Snapshot{SchemaVersion: snapshotSchemaVersion, GatewayVersion: *gateway, SourceCommit: commit, CatalogSHA256: sha256Hex(bytes), PlanID: "bootstrap:" + commit, ProvenanceMode: "bootstrap-public"}
	plugins := append([]Plugin(nil), c.Plugins...)
	sort.Slice(plugins, func(i, j int) bool { return plugins[i].LogicalID < plugins[j].LogicalID })
	for _, p := range plugins {
		if !p.ReleaseEligible {
			continue
		}
		raw, err := fileAtCommit(*root, commit, p.SourceDir+"/VERSION")
		if err != nil {
			return err
		}
		version := strings.TrimSpace(raw)
		parsed, err := parseSemver(version)
		if err != nil {
			return err
		}
		e, ok := evidence.Plugins[p.LogicalID]
		publicRef := c.Registry + "/" + p.Image + ":" + version
		if !ok || e.Version != version {
			return fmt.Errorf("bootstrap evidence for %s is incomplete or mismatched", p.LogicalID)
		}
		if isAlphaPrerelease(parsed.prerelease) {
			if e.Status != "deferred" {
				return fmt.Errorf("bootstrap evidence for %s has status %q but VERSION %q is a deferred alpha prerelease", p.LogicalID, e.Status, version)
			}
			continue
		}
		switch e.Status {
		case "public":
			if !digestPattern.MatchString(e.Digest) || e.PublicRef != publicRef {
				return fmt.Errorf("bootstrap evidence for %s is incomplete or mismatched", p.LogicalID)
			}
			resolved, err := ociManifestResolver(e.PublicRef)
			if err != nil || resolved.Digest != e.Digest {
				if err != nil {
					return fmt.Errorf("bootstrap public artifact %s: %w", p.LogicalID, err)
				}
				return fmt.Errorf("bootstrap public artifact %s resolved digest %s, expected %s", p.LogicalID, resolved.Digest, e.Digest)
			}
		case "missing":
			if parsed.prerelease != "" {
				return fmt.Errorf("bootstrap evidence for %s marks non-alpha prerelease %q missing; only stable versions may be backfilled", p.LogicalID, version)
			}
			if e.PublicRef != publicRef || e.Digest != "" {
				return fmt.Errorf("bootstrap evidence for %s is incomplete or mismatched", p.LogicalID)
			}
			// The reviewed absence must still hold. If the tag now resolves,
			// the evidence is stale and the capture must be re-reviewed.
			resolved, err := ociManifestResolver(publicRef)
			if err == nil {
				return fmt.Errorf("bootstrap evidence for %s is stale: %s now resolves to %s; re-capture bootstrap evidence", p.LogicalID, publicRef, resolved.Digest)
			}
			if classifyOCIFailure(err, publicRef) != ociFailureNotFound {
				return fmt.Errorf("bootstrap public artifact %s: %w", p.LogicalID, err)
			}
			continue
		default:
			return fmt.Errorf("bootstrap evidence for %s has unsupported status %q", p.LogicalID, e.Status)
		}
		hash, err := inputHash(*root, commit, version, c, p)
		if err != nil {
			return err
		}
		snapshot.Plugins = append(snapshot.Plugins, SnapshotEntry{LogicalID: p.LogicalID, Implementation: p.Implementation, SourceDir: p.SourceDir, Image: p.Image, Version: version, OCIRef: publicRef, Digest: e.Digest, InputHash: hash, SourceCommit: commit, ProvenanceMode: "public", Consumers: catalogConsumers(c, p)})
	}
	return writeCanonical(*output, snapshot)
}

func commandApply(args []string) error {
	fs := flag.NewFlagSet("apply-plan", flag.ContinueOnError)
	root := fs.String("root", ".", "repository root")
	planPath := fs.String("plan", "", "plan path")
	if err := fs.Parse(args); err != nil {
		return err
	}
	var plan Plan
	if *planPath == "" {
		return errors.New("--plan is required")
	}
	if _, err := readJSON(*planPath, &plan); err != nil {
		return err
	}
	return applyPlan(*root, plan)
}

func commandRender(args []string) error {
	fs, _, catalog := parseCommon("render-snapshot", args)
	plan := fs.String("plan", "", "plan path")
	previous := fs.String("previous", "", "previous snapshot")
	evidence := fs.String("candidate-evidence", "", "candidate evidence path")
	bootstrapEvidence := fs.String("bootstrap-evidence", "", "bootstrap evidence path (first managed release only)")
	migrationReport := fs.String("migration-report", "", "prepare-time public registry sweep bound into the snapshot")
	output := fs.String("output", "-", "snapshot output")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *plan == "" || *evidence == "" {
		return errors.New("--plan and --candidate-evidence are required")
	}
	snapshot, err := renderSnapshot(*catalog, *plan, *previous, *evidence, *bootstrapEvidence)
	if err != nil {
		return err
	}
	if *migrationReport != "" {
		snapshot, err = applyMigrationReport(snapshot, *migrationReport)
		if err != nil {
			return err
		}
	} else if err := requireNoCarriedMigration(snapshot); err != nil {
		return err
	}
	return writeCanonical(*output, snapshot)
}

func commandVerify(args []string) error {
	fs, root, catalog := parseCommon("verify-snapshot", args)
	snapshot := fs.String("snapshot", "", "snapshot path")
	plan := fs.String("plan", "", "plan path for exact plan-to-snapshot binding")
	previous := fs.String("previous", "", "previous snapshot path for carried-entry binding")
	expected := fs.String("expected-source", "", "expected exact pre-merge source ref")
	committed := fs.String("committed-source", "", "exact merged source ref whose VERSION files must match")
	resolve := fs.Bool("resolve", false, "resolve OCI manifests and verify provenance annotations")
	ociSource := fs.String("oci-source", "candidate", "OCI source to resolve: candidate or public")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *snapshot == "" {
		return errors.New("--snapshot is required")
	}
	return verifySnapshotBindings(*root, *catalog, *snapshot, *plan, *previous, *expected, *committed, *resolve, *ociSource)
}

func commandCompare(args []string) error {
	fs := flag.NewFlagSet("semver-compare", flag.ContinueOnError)
	current := fs.String("current", "", "current stable version")
	candidate := fs.String("candidate", "", "candidate stable version")
	if err := fs.Parse(args); err != nil {
		return err
	}
	a, err := parseSemver(*current)
	if err != nil || a.prerelease != "" {
		return errors.New("--current must be stable SemVer")
	}
	b, err := parseSemver(*candidate)
	if err != nil || b.prerelease != "" {
		return errors.New("--candidate must be stable SemVer")
	}
	if compareSemver(b, a) < 0 {
		return fmt.Errorf("candidate %s would move latest backwards from %s", *candidate, *current)
	}
	return nil
}
