// Copyright 2026 Higress Authors
// Licensed under the Apache License, Version 2.0.

package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

func listGoBatchSelection(t *testing.T, scope string) []string {
	t.Helper()
	root := filepath.Clean("../..")
	cmd := exec.Command("bash", "tools/hack/build-wasm-plugins.sh")
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "PLUGIN_TYPE=GO", "PLUGIN_BATCH_LIST_ONLY=true", "PLUGIN_GO_BATCH_SCOPE="+scope)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("list %s Go batch: %v\n%s", scope, err, out)
	}
	selected := make([]string, 0)
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "plugins/") {
			selected = append(selected, line)
		}
	}
	return selected
}

func TestGoBatchBuildUsesReleaseEligibleCatalogSelection(t *testing.T) {
	root := filepath.Clean("../..")
	selected := listGoBatchSelection(t, "release")

	catalog, _, err := loadCatalog(filepath.Join(root, "plugins/release/catalog.json"))
	if err != nil {
		t.Fatalf("load catalog: %v", err)
	}
	want := make([]string, 0)
	for _, plugin := range catalog.Plugins {
		if plugin.Implementation == "go" && plugin.ReleaseEligible {
			want = append(want, plugin.SourceDir)
		}
	}
	sort.Strings(want)
	if strings.Join(selected, "\n") != strings.Join(want, "\n") {
		t.Fatalf("Go batch selection must exactly match release-eligible catalog entries\nwant: %s\n got: %s", want, selected)
	}

	assertSelected := func(sourceDir string) {
		t.Helper()
		for _, candidate := range selected {
			if candidate == sourceDir {
				return
			}
		}
		t.Fatalf("release-eligible Go plugin %q was not selected", sourceDir)
	}
	assertSelected("plugins/wasm-go/extensions/cache-control")     // stable SemVer, not a prerelease
	assertSelected("plugins/wasm-go/extensions/replay-protection") // alpha version
	assertSelected("plugins/wasm-go/extensions/mcp-server")
	version, err := os.ReadFile(filepath.Join(root, "plugins/wasm-go/extensions/cache-control/VERSION"))
	if err != nil {
		t.Fatalf("read stable cache-control VERSION fixture: %v", err)
	}
	stableVersion, err := parseSemver(string(version))
	if err != nil || stableVersion.prerelease != "" {
		t.Fatalf("fixture must exercise stable cache-control VERSION, got %q (%v)", version, err)
	}
	version, err = os.ReadFile(filepath.Join(root, "plugins/wasm-go/extensions/replay-protection/VERSION"))
	if err != nil || !strings.HasSuffix(strings.TrimSpace(string(version)), "-alpha") {
		t.Fatalf("fixture must exercise alpha replay-protection VERSION, got %q (%v)", version, err)
	}
}

func TestGoE2EBatchIsProductionUnionManifestDependencies(t *testing.T) {
	root := filepath.Clean("../..")
	releaseSelected := listGoBatchSelection(t, "release")
	e2eSelected := listGoBatchSelection(t, "e2e")

	wantSet := make(map[string]struct{}, len(releaseSelected))
	for _, sourceDir := range releaseSelected {
		wantSet[sourceDir] = struct{}{}
	}
	fileURL := regexp.MustCompile(`file:///opt/(plugins/wasm-go/extensions/[a-z0-9][a-z0-9-]*)/plugin\.wasm`)
	err := filepath.WalkDir(filepath.Join(root, "test/e2e/conformance/tests"), func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || filepath.Ext(path) != ".yaml" {
			return nil
		}
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		for _, match := range fileURL.FindAllSubmatch(data, -1) {
			wantSet[string(match[1])] = struct{}{}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("discover manifest dependencies: %v", err)
	}
	want := make([]string, 0, len(wantSet))
	for sourceDir := range wantSet {
		want = append(want, sourceDir)
	}
	sort.Strings(want)
	if strings.Join(e2eSelected, "\n") != strings.Join(want, "\n") {
		t.Fatalf("Go e2e batch must exactly equal production plus manifest dependencies\nwant: %s\n got: %s", want, e2eSelected)
	}

	testOnly := "plugins/wasm-go/extensions/test-redis-inject-spin"
	if _, found := wantSet[testOnly]; !found {
		t.Fatalf("manifest-referenced conformance plugin %q was not selected", testOnly)
	}
	for _, sourceDir := range releaseSelected {
		if sourceDir == testOnly {
			t.Fatalf("conformance-only plugin %q entered the production batch", testOnly)
		}
	}
	countSelected := func(sourceDir string) int {
		count := 0
		for _, candidate := range e2eSelected {
			if candidate == sourceDir {
				count++
			}
		}
		return count
	}
	if count := countSelected("plugins/wasm-go/extensions/mcp-server"); count != 1 {
		t.Fatalf("mcp-server must occur exactly once in the e2e union, got %d", count)
	}
	for _, excluded := range []string{
		"plugins/wasm-go/extensions/ai-image-reader", // unrelated ineligible plugin
		"plugins/wasm-go/examples/basic-auth",        // example tree
		"plugins/wasm-rust/extensions/ai-data-masking",
		"plugins/wasm-cpp/extensions/basic_auth",
	} {
		if count := countSelected(excluded); count != 0 {
			t.Fatalf("non-conformance dependency %q entered the Go e2e batch", excluded)
		}
	}
}

func TestGoWasmPluginWorkflowBuildsMCPSingleBatch(t *testing.T) {
	data, err := os.ReadFile("../../.github/workflows/build-and-test-plugin.yaml")
	if err != nil {
		t.Fatalf("read workflow: %v", err)
	}
	workflow := string(data)
	if strings.Contains(workflow, "make build-mcp-server-wasmplugin") {
		t.Fatal("Go wasm plugin workflow must not prebuild mcp-server outside the catalog batch")
	}
	for _, required := range []string{
		`sudo rm -rf -- "${GITHUB_WORKSPACE:?}/out"`,
		`mkdir -p -- "${GITHUB_WORKSPACE:?}/out"`,
		"PLUGIN_TYPE=${{ matrix.wasmPluginType }}",
		"PLUGIN_GO_BATCH_SCOPE=e2e",
		"make higress-wasmplugin-test",
		"timeout_minutes: 60",
		"max_attempts: 2",
	} {
		if !strings.Contains(workflow, required) {
			t.Fatalf("workflow lacks required catalog-batch contract %q", required)
		}
	}
}

func TestGoWasmPluginWorkflowHasSingleGoCacheAuthority(t *testing.T) {
	data, err := os.ReadFile("../../.github/workflows/build-and-test-plugin.yaml")
	if err != nil {
		t.Fatalf("read workflow: %v", err)
	}
	workflow := string(data)
	jobStart := strings.Index(workflow, "  higress-wasmplugin-test:")
	jobEnd := strings.Index(workflow, "\n  publish:")
	if jobStart < 0 || jobEnd < 0 || jobEnd <= jobStart {
		t.Fatal("could not isolate higress-wasmplugin-test workflow job")
	}
	job := workflow[jobStart:jobEnd]
	if strings.Count(job, "actions/setup-go@v5") != 1 {
		t.Fatalf("wasm plugin job must have exactly one setup-go cache authority:\n%s", job)
	}
	if !strings.Contains(job, "cache: true") {
		t.Fatal("setup-go cache must remain explicitly enabled for the wasm plugin job")
	}
	if strings.Contains(job, "actions/cache@") {
		t.Fatalf("wasm plugin job must not restore Go cache paths with a second cache action:\n%s", job)
	}
}
