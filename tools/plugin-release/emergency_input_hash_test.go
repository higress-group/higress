// Copyright 2026 Higress Authors
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import "testing"

// TestEmergencyInputHashMatchesSnapshot pins the critical property of the
// emergency subcommand: for the mcp-server 2.0.1 release recorded in the
// checked-in 2.2.4 snapshot (source commit 7774663e...), it reproduces the
// exact inputHash the release pipeline recorded (sha256:6f5f2097...).
// If this test breaks, the emergency workflow would stamp wrong provenance
// annotations onto overwritten tags.
func TestEmergencyInputHashMatchesSnapshot(t *testing.T) {
	if testing.Short() {
		t.Skip("requires git history")
	}
	const (
		snapshotCommit = "7774663e4353652f5c3cecd7e98a1a43b05ee15c"
		want           = "sha256:6f5f2097da93edfe478d1432496509bfcd8f861db71786d8e0f96fb7563c8c55"
	)
	c, _, err := loadCatalog("../../plugins/release/catalog.json")
	if err != nil {
		t.Fatalf("load catalog: %v", err)
	}
	var plugin Plugin
	for _, p := range c.Plugins {
		if p.LogicalID == "mcp-server" {
			plugin = p
		}
	}
	if plugin.LogicalID == "" {
		t.Fatal("mcp-server missing from catalog")
	}
	got, err := inputHash("../..", snapshotCommit, "2.0.1", c, plugin)
	if err != nil {
		t.Fatalf("inputHash: %v", err)
	}
	if got != want {
		t.Fatalf("emergency hash drift: got %s want %s (snapshot provenance would be forged)", got, want)
	}
}
