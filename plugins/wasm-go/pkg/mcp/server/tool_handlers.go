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
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/alibaba/higress/plugins/wasm-go/pkg/mcp/protocol"
	"github.com/alibaba/higress/plugins/wasm-go/pkg/mcp/utils"
	"github.com/higress-group/wasm-go/pkg/wrapper"
)

type directToolSchemaState uint8

const (
	directToolSchemaValidated directToolSchemaState = iota
	directToolSchemaValidationUnavailable
	directToolSchemaExplicitLegacyOnly
)

type directToolEntry struct {
	name         string
	description  string
	inputSchema  map[string]any
	validator    *compiledInputSchema
	schemaState  directToolSchemaState
	serializable bool
	tool         Tool
}

// directToolSnapshot binds discovery and invocation to one analyzed tool
// generation. It is immutable after configuration publication.
type directToolSnapshot struct {
	ordered    []directToolEntry
	byName     map[string]directToolEntry
	legacyOnly map[string]string
	degraded   []directToolDiagnostic
}

type directToolDiagnostic struct {
	name   string
	reason schemaDiagnosticReason
	detail string
}

type capturedInputSchemaTool interface {
	capturedInputSchema() (map[string]any, bool)
}

func compileDirectToolSnapshot(server Server) directToolSnapshot {
	entries := snapshotTools(server)
	snapshot := directToolSnapshot{
		ordered:    make([]directToolEntry, 0, len(entries)),
		byName:     make(map[string]directToolEntry, len(entries)),
		legacyOnly: make(map[string]string),
	}
	for _, entry := range entries {
		var descriptor map[string]any
		var serializable bool
		var validator *compiledInputSchema
		var preparationErr error
		if captured, ok := entry.tool.(capturedInputSchemaTool); ok {
			descriptor, serializable = captured.capturedInputSchema()
			if serializable {
				descriptor, serializable, validator, preparationErr = prepareToolInputSchema(descriptor)
			} else {
				preparationErr = schemaCompileError(schemaDiagnosticSerializationFailure, "input schema is not JSON-compatible")
			}
		} else {
			descriptor, serializable, validator, preparationErr = prepareToolInputSchema(entry.tool.InputSchema())
		}
		if compatibility, ok := entry.tool.(legacySchemaCompatibleTool); ok && compatibility.legacyOnlyInputSchema() {
			reason := "configured as legacy-only"
			if preparationErr != nil {
				reason = preparationErr.Error()
			}
			snapshot.legacyOnly[entry.name] = reason
			explicit := directToolEntry{
				name:         entry.name,
				description:  entry.tool.Description(),
				inputSchema:  descriptor,
				validator:    validator,
				schemaState:  directToolSchemaExplicitLegacyOnly,
				serializable: serializable,
				tool:         entry.tool,
			}
			snapshot.ordered = append(snapshot.ordered, explicit)
			snapshot.byName[entry.name] = explicit
			continue
		}
		state := directToolSchemaValidated
		var reason schemaDiagnosticReason
		if preparationErr != nil {
			state = directToolSchemaValidationUnavailable
			reason = schemaPreparationDiagnosticReason(preparationErr)
		}
		validated := directToolEntry{
			name:         entry.name,
			description:  entry.tool.Description(),
			inputSchema:  descriptor,
			validator:    validator,
			schemaState:  state,
			serializable: serializable,
			tool:         entry.tool,
		}
		snapshot.ordered = append(snapshot.ordered, validated)
		snapshot.byName[entry.name] = validated
		if state == directToolSchemaValidationUnavailable {
			snapshot.degraded = append(snapshot.degraded, directToolDiagnostic{
				name:   entry.name,
				reason: reason,
				detail: boundedSchemaDiagnosticField(preparationErr.Error(), maxSchemaDiagnosticDetailRunes),
			})
		}
	}
	return snapshot
}

const (
	maxSchemaDiagnosticRecords       = 8
	maxSchemaDiagnosticServerRunes   = 128
	maxSchemaDiagnosticToolNameRunes = 128
	maxSchemaDiagnosticDetailRunes   = 256
	maxSchemaDiagnosticWarningRunes  = 8192
)

func boundedSchemaDiagnosticToolName(name string) string {
	return boundedSchemaDiagnosticField(name, maxSchemaDiagnosticToolNameRunes)
}

func boundedSchemaDiagnosticField(value string, limit int) string {
	clean := strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || unicode.In(r, unicode.Cf, unicode.Zl, unicode.Zp) {
			return ' '
		}
		return r
	}, value)
	runes := []rune(clean)
	if len(runes) <= limit {
		return clean
	}
	return string(runes[:limit]) + "..."
}

func quoteSchemaDiagnosticField(value string) string {
	// The bounded fields contain no control characters. Quote the two remaining
	// delimiters so one tool cannot make another record ambiguous.
	return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(value) + `"`
}

func (snapshot directToolSnapshot) degradedPublicationWarning(serverName string) string {
	return snapshot.degradedPublicationWarningWithin(serverName, maxSchemaDiagnosticWarningRunes)
}

func (snapshot directToolSnapshot) degradedPublicationWarningWithin(serverName string, warningRuneLimit int) string {
	if len(snapshot.degraded) == 0 {
		return ""
	}
	diagnostics := append([]directToolDiagnostic(nil), snapshot.degraded...)
	sort.Slice(diagnostics, func(i, j int) bool {
		if diagnostics[i].name != diagnostics[j].name {
			return diagnostics[i].name < diagnostics[j].name
		}
		if diagnostics[i].reason != diagnostics[j].reason {
			return diagnostics[i].reason < diagnostics[j].reason
		}
		return diagnostics[i].detail < diagnostics[j].detail
	})
	recordCount := min(len(diagnostics), maxSchemaDiagnosticRecords)
	records := make([]string, 0, recordCount)
	for _, diagnostic := range diagnostics[:recordCount] {
		tool := boundedSchemaDiagnosticToolName(diagnostic.name)
		detail := boundedSchemaDiagnosticField(diagnostic.detail, maxSchemaDiagnosticDetailRunes)
		records = append(records, fmt.Sprintf("{tool=%s reason=%s detail=%s}",
			quoteSchemaDiagnosticField(tool), diagnostic.reason.String(), quoteSchemaDiagnosticField(detail)))
	}
	server := boundedSchemaDiagnosticField(serverName, maxSchemaDiagnosticServerRunes)
	warning := fmt.Sprintf("Direct tools published without local schema validation: server=%s total=%d records=[%s] omitted=%d; modern tools/call will be rejected; legacy calls remain available",
		quoteSchemaDiagnosticField(server), len(diagnostics), strings.Join(records, ","), len(diagnostics)-recordCount)
	if utf8.RuneCountInString(warning) <= warningRuneLimit {
		return warning
	}
	// This defensive fallback preserves the aggregate signal if future format
	// changes accidentally outgrow the complete-line budget.
	return fmt.Sprintf("Direct tools published without local schema validation: server=%s total=%d records=[] omitted=%d; modern tools/call will be rejected; legacy calls remain available",
		quoteSchemaDiagnosticField(server), len(diagnostics), len(diagnostics))
}

type legacySchemaCompatibleTool interface {
	legacyOnlyInputSchema() bool
}

func (snapshot directToolSnapshot) buildModernToolList(effectiveAllowTools *map[string]struct{}) []map[string]any {
	tools := make([]map[string]any, 0, len(snapshot.ordered))
	for _, entry := range snapshot.ordered {
		if entry.schemaState == directToolSchemaExplicitLegacyOnly || !entry.serializable {
			continue
		}
		if effectiveAllowTools != nil {
			if _, allowed := (*effectiveAllowTools)[entry.name]; !allowed {
				continue
			}
		}
		tool := map[string]any{
			"name":        entry.name,
			"description": entry.description,
			"inputSchema": entry.inputSchema,
		}
		tools = append(tools, tool)
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

func sendSchemaValidationUnavailable(ctx wrapper.HttpContext) {
	request, modern := ModernRequestContext(ctx)
	if !modern {
		return
	}
	utils.SendProtocolError(
		ctx,
		request.Envelope.ID,
		protocol.SchemaValidationUnavailable(),
		"schema_validation_unavailable",
	)
}
