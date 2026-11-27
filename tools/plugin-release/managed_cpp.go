package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
)

var managedCPPSources = map[string]string{
	"basic-auth": "basic_auth", "bot-detect": "bot_detect", "custom-response": "custom_response",
	"hmac-auth": "hmac_auth", "jwt-auth": "jwt_auth", "key-auth": "key_auth",
	"key-rate-limit": "key_rate_limit", "oauth": "oauth", "request-block": "request_block",
}

var excludedCPPSources = []string{"model_mapper", "model_router", "sni_misdirect"}

func validateManagedCPPCatalog(root string, catalog Catalog) error {
	seen := map[string]bool{}
	var managed *bool
	for _, plugin := range catalog.Plugins {
		if plugin.Implementation != "cpp" {
			continue
		}
		source, known := managedCPPSources[plugin.LogicalID]
		expected := "plugins/wasm-cpp/extensions/" + source
		if !known || seen[plugin.LogicalID] || plugin.SourceDir != expected || plugin.Image != "plugins/"+plugin.LogicalID {
			return fmt.Errorf("%s: unknown or substituted managed C++ identity", plugin.LogicalID)
		}
		if !reflect.DeepEqual(plugin.ArtifactInputs, []string{expected + "/**"}) || !reflect.DeepEqual(plugin.SharedInputGroups, []string{"cpp-workspace"}) {
			return fmt.Errorf("%s: C++ artifact inputs must cover the complete extension and cpp-workspace", plugin.LogicalID)
		}
		if managed != nil && *managed != plugin.ReleaseEligible {
			return errors.New("partial managed C++ activation is forbidden")
		}
		state := plugin.ReleaseEligible
		managed = &state
		seen[plugin.LogicalID] = true
		consumers := plugin.Consumers
		if state {
			if consumers.Console == nil || consumers.PluginServer == nil || consumers.Console.PropertyKey != plugin.LogicalID ||
				consumers.Console.ResourceDir != plugin.LogicalID || consumers.Console.URLForm != "oci" ||
				consumers.PluginServer.InventoryKey != plugin.LogicalID || consumers.PluginServer.HTTPPath != plugin.LogicalID || len(consumers.PluginServer.Aliases) != 0 {
				return fmt.Errorf("%s: incomplete fixed managed C++ consumers", plugin.LogicalID)
			}
			if catalog.ConsoleMarketplace == nil || !catalog.ConsoleMarketplace.RequiredForStable {
				return errors.New("managed C++ requires the stable marketplace policy")
			}
			bundle := catalogConsumers(catalog, plugin).Console.Marketplace
			if bundle == nil || bundle.Repository != "higress-group/higress-console" || !commitPattern.MatchString(bundle.SourceCommit) || len(bundle.Files) != 3 {
				return fmt.Errorf("%s: missing immutable C++ marketplace bundle", plugin.LogicalID)
			}
			targets := map[string]bool{"spec.yaml": true, "README.md": true, "README_EN.md": true}
			for _, file := range bundle.Files {
				if !targets[file.TargetPath] || file.SourcePath != "backend/sdk/src/main/resources/plugins/"+plugin.LogicalID+"/"+file.TargetPath || !wasmHashPattern.MatchString(file.SHA256) {
					return errors.New("invalid fixed C++ marketplace resources")
				}
				delete(targets, file.TargetPath)
			}
		} else if !reflect.DeepEqual(consumers, PluginConsumers{}) {
			return fmt.Errorf("%s: unmanaged C++ entries cannot have consumers", plugin.LogicalID)
		}
		for _, consumer := range []string{"console", "pluginServer"} {
			found := false
			for _, item := range catalog.ConsumerInventories[consumer] {
				if item.Key != plugin.LogicalID {
					continue
				}
				found = true
				if state && (item.Classification != "managed" || item.LogicalID != plugin.LogicalID || item.Reason != "") ||
					!state && (item.Classification != "unmanaged" || item.LogicalID != "" || item.Reason == "") {
					return fmt.Errorf("%s: %s inventory disagrees with C++ activation", plugin.LogicalID, consumer)
				}
			}
			if !found {
				return fmt.Errorf("%s: missing %s inventory classification", plugin.LogicalID, consumer)
			}
		}
	}
	entries, err := os.ReadDir(filepath.Join(root, "plugins/wasm-cpp/extensions"))
	if len(seen) == 0 && os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if len(seen) != len(managedCPPSources) {
		return errors.New("C++ catalog must contain all nine fixed identities")
	}
	expectedDirs := map[string]bool{}
	for _, source := range managedCPPSources {
		expectedDirs[source] = true
	}
	for _, source := range excludedCPPSources {
		expectedDirs[source] = true
	}
	for _, entry := range entries {
		if entry.Type()&os.ModeSymlink != 0 || entry.IsDir() && !expectedDirs[entry.Name()] {
			return fmt.Errorf("unclassified or symlink C++ extension %s", entry.Name())
		}
		if entry.IsDir() {
			delete(expectedDirs, entry.Name())
		}
	}
	if len(expectedDirs) != 0 {
		return fmt.Errorf("missing classified C++ extension directories: %v", expectedDirs)
	}
	return nil
}
