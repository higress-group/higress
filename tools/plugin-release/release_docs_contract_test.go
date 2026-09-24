// Copyright 2026 Higress Authors
// Licensed under the Apache License, Version 2.0.

package main

import (
	"os"
	"strings"
	"testing"
)

func TestReleaseDocsCreateMetadataPRAfterDependenciesConverge(t *testing.T) {
	paths := []string{
		"../../docs/developers/immutable-plugin-releases.md",
		"../../RELEASE.md",
	}
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		doc := strings.Join(strings.Fields(string(data)), " ")
		for _, required := range []string{
			"#4019", "exact main", "snapshot", "Console",
			"manually edit", "plugin `VERSION`", "Console",
			"helm dependency update", "Chart.lock", "tag authorization", "reprepare",
		} {
			if !strings.Contains(doc, required) {
				t.Fatalf("%s lacks release-cut ordering contract %q", path, required)
			}
		}
		freeze := strings.Index(doc, "exact main")
		snapshot := strings.Index(doc[freeze:], "snapshot")
		console := strings.Index(doc[freeze:], "Console")
		releasePR := strings.Index(doc[freeze:], "create or update")
		chartLock := strings.Index(doc[freeze:], "Chart.lock")
		authorize := strings.Index(doc[freeze:], "tag authorization")
		if freeze < 0 || snapshot < 0 || console < 0 || releasePR < 0 || chartLock < 0 || authorize < 0 || snapshot > console || console > releasePR || releasePR > chartLock || chartLock > authorize {
			t.Fatalf("%s does not order exact main -> snapshot -> Console -> #4019 -> Chart.lock -> authorization", path)
		}
	}
}

func TestReleaseNotesUseCanonicalGeneratedNotesAPI(t *testing.T) {
	data, err := os.ReadFile("../../.github/workflows/generate-release-notes.yaml")
	if err != nil {
		t.Fatal(err)
	}
	workflow := string(data)
	for _, required := range []string{
		`GITHUB_REPO_OWNER: higress-group`,
		`environment: higress-release-manager`,
		`actions/create-github-app-token@d72941d797fd3113feb6b93fd0dec494b13a2547`,
		`repositories: higress,higress-console`,
		`permission-contents: write`,
		`GITHUB_PERSONAL_ACCESS_TOKEN: ${{ steps.release-reader.outputs.token }}`,
		`/releases/generate-notes`,
		`Authorization: Bearer ${GITHUB_PERSONAL_ACCESS_TOKEN}`,
		`jq -er .body generated_release_notes.json`,
		`No PR numbers found in release notes`,
		`exit 1`,
		`never use that mutable body as the source of included PRs`,
	} {
		if !strings.Contains(workflow, required) {
			t.Fatalf("release notes workflow lacks generated-notes contract %q", required)
		}
	}
	for _, forbidden := range []string{
		`GITHUB_REPO_OWNER: alibaba`,
		`GITHUB_PERSONAL_ACCESS_TOKEN: ${{ secrets.GITHUB_TOKEN }}`,
		`https://github.com/${GITHUB_REPO_OWNER}/${GITHUB_REPO_NAME}/releases/tag/`,
		`release_page.html`,
		`exit 0`,
	} {
		if strings.Contains(workflow, forbidden) {
			t.Fatalf("release notes workflow retains mutable/legacy source %q", forbidden)
		}
	}
}
