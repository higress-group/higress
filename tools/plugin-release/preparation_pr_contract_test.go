// Copyright 2026 Higress Authors
// Licensed under the Apache License, Version 2.0.

package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestPreparationPRContractLabelsAndReports executes the preparation PR
// publication block against a stubbed gh CLI. The release label is what
// authorizes promote, so it must be applied on both the create and the update
// path, created only when missing, and its absence must fail the step. An
// all-green sweep must leave the PR body byte-identical to today's body.
func TestPreparationPRContractLabelsAndReports(t *testing.T) {
	const gateway = "2.2.6"
	const baseBody = "Immutable candidate snapshot; review VERSION edits and digest provenance."
	report := MigrationReportFile{
		SchemaVersion: migrationReportSchemaVersion, GatewayVersion: gateway,
		SourceCommit: strings.Repeat("a", 40), PlanID: "sha256:" + strings.Repeat("0", 64),
		Registry: "registry.example", ProbeMode: migrationProbeCandidateDigest,
		ControlRef: "registry.example/plugins/demo:1.0.0", ControlDigest: testDigest("control"),
		Entries: []MigrationReportEntry{{
			LogicalID: "ai-context-limit", Version: "1.0.0", OCIRef: "registry.example/plugins/ai-context-limit:1.0.0",
			ExistingDigest: testDigest("legacy"), PlannedDigest: testDigest("planned"), InputHash: testDigest("inputs"),
			SourceComparison: migrationSourceUnannotated, State: migrationStateBlocked, Recommendation: migrationRecommendDeleteLegacy,
		}},
	}
	// The workflow reads the sweep output at these fixed paths.
	reportPath := "/tmp/migration-report.json"
	markdownPath := "/tmp/migration-report.md"
	t.Cleanup(func() {
		_ = os.Remove(reportPath)
		_ = os.Remove(markdownPath)
	})

	for _, tc := range []struct {
		name           string
		blocked        bool
		labelExists    bool
		labelFails     bool
		prCreateFails  bool
		wantBody       string
		wantBodySuffix string
		wantLog        []string
		wantAbsent     []string
		wantError      bool
	}{
		{
			name:           "all-green",
			wantBody:       baseBody,
			wantBodySuffix: "\nARG --label\nARG release/" + gateway + "\n",
			wantLog: []string{
				"=== label\nARG create\nARG release/" + gateway,
				"=== pr\nARG create\nARG --base\nARG main",
			},
		},
		{
			name:        "blocked-plugins-append-report",
			blocked:     true,
			labelExists: true,
			wantBody:    baseBody + "\n\n## Plugin migration preflight\n",
			wantLog:     []string{"**delete-legacy**", testDigest("legacy"), testDigest("planned"), "=== pr\nARG create"},
			wantAbsent:  []string{"=== label\nARG create"},
		},
		{
			name:           "existing-pr-falls-back-to-edit",
			labelExists:    true,
			prCreateFails:  true,
			wantBody:       baseBody,
			wantBodySuffix: "\nARG --add-label\nARG release/" + gateway + "\n",
			wantLog: []string{
				"=== pr\nARG edit\nARG release/plugin-snapshot-" + gateway,
			},
		},
		{
			name:       "label-cannot-be-ensured",
			labelFails: true,
			wantError:  true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			bin := filepath.Join(root, "bin")
			if err := os.MkdirAll(bin, 0o755); err != nil {
				t.Fatal(err)
			}
			current := report
			markdown := "## Plugin migration preflight\n\nNo planned version tag is occupied by a different artifact.\n"
			if tc.blocked {
				markdown = renderMigrationMarkdown(report)
			} else {
				current.Entries = []MigrationReportEntry{}
			}
			if err := writeCanonical(reportPath, current); err != nil {
				t.Fatal(err)
			}
			if err := writeText(markdownPath, markdown); err != nil {
				t.Fatal(err)
			}
			logPath := filepath.Join(root, "gh.log")
			statePath := filepath.Join(root, "label.state")
			if tc.labelExists {
				if err := os.WriteFile(statePath, []byte("release/"+gateway), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			writeExecutableFixture(t, filepath.Join(bin, "gh"), `#!/usr/bin/env bash
set -uo pipefail
command=$1
{
  printf '=== %s\n' "$command"
  for argument in "${@:2}"; do printf 'ARG %s\n' "$argument"; done
} >> "$GH_LOG"
case "$command" in
  api)
    # The same endpoint is used to probe and to confirm the label, so the
    # stubbed registry of labels is the state file gh label create writes.
    if [ ! -f "$GH_LABEL_STATE" ]; then exit 1; fi
    if [ "${3:-}" = "--jq" ]; then cat "$GH_LABEL_STATE"; fi
    exit 0
    ;;
  label)
    if [ "${2:-}" != create ]; then echo "unexpected gh label invocation: $*" >&2; exit 2; fi
    if [ "${GH_LABEL_CREATE_FAILS:-false}" = true ]; then exit 1; fi
    printf '%s' "$3" > "$GH_LABEL_STATE"
    exit 0
    ;;
  pr)
    if [ "${2:-}" = create ] && [ "${GH_PR_CREATE_FAILS:-false}" = true ]; then exit 1; fi
    if [ "${2:-}" != create ] && [ "${2:-}" != edit ]; then echo "unexpected gh pr invocation: $*" >&2; exit 2; fi
    exit 0
    ;;
esac
echo "unexpected gh invocation: $*" >&2
exit 2
`)
			contract := workflowShellContract(t, "prepare-plugin-release.yaml", "preparation-pr-contract")
			script := "set -euo pipefail\nbranch=\"release/plugin-snapshot-$GATEWAY_VERSION\"\n" + contract
			cmd := exec.Command("bash", "-c", script)
			cmd.Dir = root
			cmd.Env = append(os.Environ(),
				"PATH="+bin+":"+os.Getenv("PATH"),
				"GH_LOG="+logPath,
				"GH_LABEL_STATE="+statePath,
				"GH_LABEL_CREATE_FAILS="+boolString(tc.labelFails),
				"GH_PR_CREATE_FAILS="+boolString(tc.prCreateFails),
				"GH_TOKEN=fixture-token",
				"GITHUB_REPOSITORY=higress-group/higress",
				"GATEWAY_VERSION="+gateway,
			)
			output, err := cmd.CombinedOutput()
			if tc.wantError {
				if err == nil {
					t.Fatalf("preparation PR was published without its authorizing label:\n%s", output)
				}
				if strings.Contains(string(output), "fixture-token") {
					t.Fatalf("preparation PR publication leaked its token:\n%s", output)
				}
				return
			}
			if err != nil {
				t.Fatalf("preparation PR publication failed: %v\n%s", err, output)
			}
			logBytes, readErr := os.ReadFile(logPath)
			if readErr != nil {
				t.Fatal(readErr)
			}
			log := string(logBytes)
			body := "ARG --body\nARG " + tc.wantBody + tc.wantBodySuffix
			if !strings.Contains(log, body) {
				t.Fatalf("preparation PR body is not exact:\nwant %q\ngot log:\n%s", body, log)
			}
			for _, required := range tc.wantLog {
				if !strings.Contains(log, required) {
					t.Fatalf("preparation PR publication lacks %q:\n%s", required, log)
				}
			}
			for _, forbidden := range tc.wantAbsent {
				if strings.Contains(log, forbidden) {
					t.Fatalf("preparation PR publication unexpectedly ran %q:\n%s", forbidden, log)
				}
			}
		})
	}
}

func boolString(value bool) string {
	if value {
		return "true"
	}
	return "false"
}
