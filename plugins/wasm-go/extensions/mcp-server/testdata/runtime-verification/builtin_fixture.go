// Copyright (c) 2026 Alibaba Group Holding Ltd.
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at http://www.apache.org/licenses/LICENSE-2.0
// Unless required by applicable law or agreed to in writing, software distributed
// under the License is distributed on an "AS IS" BASIS, WITHOUT WARRANTIES OR
// CONDITIONS OF ANY KIND, either express or implied.

// This test-only exporter preserves the actual builtin tools/arguments while
// remapping external vendor requests to the existing observable REST fixture.
package main

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

func main() {
	root, out := os.Args[1], os.Args[2]
	manifest := make(map[string]any)
	for _, name := range []string{"e2bdev", "firecrawl"} {
		path := filepath.Join(root, "plugins/wasm-go/mcp-servers/mcp-"+name+"/mcp-server.yaml")
		raw, err := os.ReadFile(path)
		if err != nil {
			panic(err)
		}
		var config map[string]any
		if err := yaml.Unmarshal(raw, &config); err != nil {
			panic(err)
		}
		server := config["server"].(map[string]any)
		server["type"], server["name"] = "rest", "builtin-"+name
		tools := config["tools"].([]any)
		names := make([]string, 0, len(tools))
		for _, value := range tools {
			tool := value.(map[string]any)
			if _, exists := tool["legacyOnly"]; exists {
				panic("obsolete builtin legacyOnly flag")
			}
			toolName := tool["name"].(string)
			names = append(names, toolName)
			if template, ok := tool["requestTemplate"].(map[string]any); ok {
				template["url"], template["method"] = "http://backend-primary:8080/unified/rest/"+name+"/"+toolName, "POST"
			}
			delete(tool, "responseTemplate")
		}
		encoded, err := json.MarshalIndent(config, "", "  ")
		if err != nil {
			panic(err)
		}
		if err := os.WriteFile(filepath.Join(out, "builtin-"+name+".json"), encoded, 0644); err != nil {
			panic(err)
		}
		manifest[name] = map[string]any{"source": path, "sourceSha256": fmt.Sprintf("%x", sha256.Sum256(raw)),
			"fixtureSha256": fmt.Sprintf("%x", sha256.Sum256(encoded)), "tools": names,
			"adaptations": []string{"test server type/name", "observable POST target", "omit vendor response template"}}
	}
	encoded, _ := json.MarshalIndent(manifest, "", "  ")
	if err := os.WriteFile(filepath.Join(out, "builtin-manifest.json"), encoded, 0644); err != nil {
		panic(err)
	}
}
