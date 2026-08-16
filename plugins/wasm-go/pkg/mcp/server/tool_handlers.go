// Copyright (c) 2022 Alibaba Group Holding Ltd.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
package server

import (
	"fmt"

	"github.com/alibaba/higress/plugins/wasm-go/pkg/mcp/protocol"
	"github.com/alibaba/higress/plugins/wasm-go/pkg/mcp/utils"
	"github.com/higress-group/wasm-go/pkg/wrapper"
)

type directToolEntry struct {
	name        string
	description string
	inputSchema map[string]any
	validator   *compiledInputSchema
	tool        Tool
}

// directToolSnapshot binds discovery and invocation to one validated tool
// generation. It is immutable after configuration publication.
type directToolSnapshot struct {
	ordered    []directToolEntry
	byName     map[string]directToolEntry
	legacyOnly map[string]string
}

func compileDirectToolSnapshot(server Server) (directToolSnapshot, error) {
	entries := snapshotTools(server)
	snapshot := directToolSnapshot{
		ordered:    make([]directToolEntry, 0, len(entries)),
		byName:     make(map[string]directToolEntry, len(entries)),
		legacyOnly: make(map[string]string),
	}
	for _, entry := range entries {
		validator, err := compileToolInputSchema(entry.tool.InputSchema())
		if compatibility, ok := entry.tool.(legacySchemaCompatibleTool); ok && compatibility.legacyOnlyInputSchema() {
			reason := "configured as legacy-only"
			if err != nil {
				reason = err.Error()
			}
			snapshot.legacyOnly[entry.name] = reason
			continue
		}
		if err != nil {
			return directToolSnapshot{}, fmt.Errorf("tool %q has invalid input schema: %w", entry.name, err)
		}
		validated := directToolEntry{
			name:        entry.name,
			description: entry.tool.Description(),
			inputSchema: validator.descriptor,
			validator:   validator,
			tool:        entry.tool,
		}
		snapshot.ordered = append(snapshot.ordered, validated)
		snapshot.byName[entry.name] = validated
	}
	return snapshot, nil
}

type legacySchemaCompatibleTool interface {
	legacyOnlyInputSchema() bool
}

func (snapshot directToolSnapshot) buildModernToolList(effectiveAllowTools *map[string]struct{}) []map[string]any {
	tools := make([]map[string]any, 0, len(snapshot.ordered))
	for _, entry := range snapshot.ordered {
		if effectiveAllowTools != nil {
			if _, allowed := (*effectiveAllowTools)[entry.name]; !allowed {
				continue
			}
		}
		tools = append(tools, map[string]any{
			"name":        entry.name,
			"description": entry.description,
			"inputSchema": entry.inputSchema,
		})
	}
	return tools
}

func installDirectToolResultAdapter(ctx wrapper.HttpContext, serverName string) {
	request, modern := ModernRequestContext(ctx)
	if !modern {
		utils.SetMCPResultAdapter(ctx, nil)
		return
	}
	utils.SetMCPResultAdapter(ctx, func(result map[string]any) map[string]any {
		return ShapeResult(request, serverName, protocol.SemanticResult{
			Value:      result,
			ResultType: resultTypeComplete,
		})
	})
}

func sendToolExecutionError(ctx wrapper.HttpContext, err error, debugInfo string) {
	utils.OnMCPToolCallError(ctx, err, debugInfo)
}
