// Copyright 2026 Higress Authors
//
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
	"errors"
	"fmt"
	"reflect"
	"sort"
)

const (
	releaseModeDefault   = ""
	releaseModeCPPHotfix = "cpp-hotfix"
	cppHotfixVersion     = "2.0.1"
)

var cppHotfixSources = map[string]string{
	"basic-auth":      "basic_auth",
	"bot-detect":      "bot_detect",
	"custom-response": "custom_response",
	"hmac-auth":       "hmac_auth",
	"jwt-auth":        "jwt_auth",
	"key-auth":        "key_auth",
	"key-rate-limit":  "key_rate_limit",
	"oauth":           "oauth",
	"request-block":   "request_block",
}

func validateReleaseMode(mode string) error {
	if mode != releaseModeDefault && mode != releaseModeCPPHotfix {
		return fmt.Errorf("unsupported release mode %q", mode)
	}
	return nil
}

func isCPPHotfix(mode string) bool {
	return mode == releaseModeCPPHotfix
}

func pluginSelectedForMode(p Plugin, mode string) bool {
	if isCPPHotfix(mode) {
		_, ok := cppHotfixSources[p.LogicalID]
		return ok && p.Implementation == "cpp"
	}
	return p.ReleaseEligible
}

func consumersForMode(c Catalog, p Plugin, mode string) PluginConsumers {
	if isCPPHotfix(mode) {
		return PluginConsumers{}
	}
	return catalogConsumers(c, p)
}

func validateCPPHotfixCatalog(c Catalog) error {
	seen := map[string]bool{}
	for _, p := range c.Plugins {
		source, ok := cppHotfixSources[p.LogicalID]
		if !ok {
			continue
		}
		expectedSource := "plugins/wasm-cpp/extensions/" + source
		if p.Implementation != "cpp" || p.SourceDir != expectedSource || p.Image != "plugins/"+p.LogicalID || p.ReleaseEligible {
			return fmt.Errorf("%s: invalid cpp-hotfix catalog identity or ordinary release eligibility", p.LogicalID)
		}
		if !reflect.DeepEqual(p.ArtifactInputs, []string{expectedSource + "/**"}) || !reflect.DeepEqual(p.SharedInputGroups, []string{"cpp-workspace"}) {
			return fmt.Errorf("%s: cpp-hotfix must hash its complete extension and cpp-workspace inputs", p.LogicalID)
		}
		seen[p.LogicalID] = true
	}
	if len(seen) != len(cppHotfixSources) {
		missing := []string{}
		for id := range cppHotfixSources {
			if !seen[id] {
				missing = append(missing, id)
			}
		}
		sort.Strings(missing)
		return fmt.Errorf("cpp-hotfix catalog is incomplete: missing %v", missing)
	}
	return nil
}

func validateCPPHotfixPlan(plan Plan) error {
	if plan.PreviousRelease != "" || len(plan.Deferred) != 0 || len(plan.Plugins) != len(cppHotfixSources) {
		return errors.New("cpp-hotfix plan must contain exactly nine new 2.0.1 entries without previous or deferred entries")
	}
	seen := map[string]bool{}
	for _, entry := range plan.Plugins {
		source, ok := cppHotfixSources[entry.LogicalID]
		if !ok || seen[entry.LogicalID] || entry.Version != cppHotfixVersion || entry.Implementation != "cpp" ||
			entry.SourceDir != "plugins/wasm-cpp/extensions/"+source || entry.Image != "plugins/"+entry.LogicalID ||
			entry.PreviousVersion != "" || entry.Backfill || !digestPattern.MatchString(entry.InputHash) {
			return fmt.Errorf("invalid cpp-hotfix plan entry %q", entry.LogicalID)
		}
		seen[entry.LogicalID] = true
	}
	return nil
}

func validateCPPHotfixSnapshot(snapshot Snapshot) error {
	if snapshot.ProvenanceMode != "candidate" || snapshot.PreviousRelease != "" || snapshot.BootstrapEvidence != nil ||
		snapshot.MigrationPreflight != nil || len(snapshot.Plugins) != len(cppHotfixSources) {
		return errors.New("cpp-hotfix snapshot must contain exactly nine candidate entries without ordinary release state")
	}
	seen := map[string]bool{}
	for _, entry := range snapshot.Plugins {
		if !pluginSelectedForMode(Plugin{LogicalID: entry.LogicalID, Implementation: entry.Implementation}, releaseModeCPPHotfix) ||
			seen[entry.LogicalID] || entry.Version != cppHotfixVersion || entry.ProvenanceMode != "candidate" ||
			entry.Backfill || entry.Migration != nil || !reflect.DeepEqual(entry.Consumers, PluginConsumers{}) {
			return fmt.Errorf("invalid cpp-hotfix snapshot entry %q", entry.LogicalID)
		}
		seen[entry.LogicalID] = true
	}
	return nil
}
