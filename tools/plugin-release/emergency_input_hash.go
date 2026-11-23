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

import (
	"flag"
	"fmt"
)

// commandEmergencyInputHash prints the deterministic release-tool input hash
// for one catalog plugin at an exact commit and version. It exists ONLY for
// the maintainer-run emergency same-version tag overwrite workflow: that
// workflow must stamp provenance annotations identical to the release
// pipeline, and plan/apply refuse same-version re-plans by design. The hash
// is computed with the exact inputHash() semantics used by plan so the
// overwritten artifact's io.higress.plugin.input-hash annotation remains
// verifiable against the repo state.
func commandEmergencyInputHash(args []string) error {
	fs := flag.NewFlagSet("emergency-input-hash", flag.ExitOnError)
	root := fs.String("root", ".", "repository root")
	catalogPath := fs.String("catalog", "plugins/release/catalog.json", "release catalog path")
	id := fs.String("id", "", "plugin logical ID")
	commit := fs.String("commit", "HEAD", "exact commit to hash inputs at")
	version := fs.String("version", "", "stable version string stamped into the hash")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *id == "" || *version == "" {
		return fmt.Errorf("--id and --version are required")
	}
	c, _, err := loadCatalog(*catalogPath)
	if err != nil {
		return err
	}
	var plugin Plugin
	found := false
	for _, p := range c.Plugins {
		if p.LogicalID == *id {
			plugin, found = p, true
			break
		}
	}
	if !found {
		return fmt.Errorf("plugin %q not found in catalog", *id)
	}
	hash, err := inputHash(*root, *commit, *version, c, plugin)
	if err != nil {
		return err
	}
	fmt.Println(hash)
	return nil
}
