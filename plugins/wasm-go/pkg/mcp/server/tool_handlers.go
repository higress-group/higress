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
	"bytes"
	"encoding"
	"encoding/json"
	"reflect"
	"sort"

	"github.com/alibaba/higress/plugins/wasm-go/pkg/mcp/protocol"
	"github.com/alibaba/higress/plugins/wasm-go/pkg/mcp/utils"
	"github.com/higress-group/wasm-go/pkg/wrapper"
	"github.com/tidwall/gjson"
)

type directToolEntry struct {
	name         string
	description  string
	inputSchema  map[string]any
	outputSchema map[string]any
	serializable bool
	tool         Tool
}

// directToolSnapshot owns descriptors for one published generation. Tool
// execution continues to use the server's historical lookup/Create/Call path.
type directToolSnapshot struct {
	ordered []directToolEntry
}

type capturedInputSchemaTool interface {
	capturedInputSchema() (map[string]any, bool)
}

func captureDirectToolSnapshot(server Server) directToolSnapshot {
	entries := snapshotTools(server)
	snapshot := directToolSnapshot{ordered: make([]directToolEntry, 0, len(entries))}
	for _, entry := range entries {
		var input map[string]any
		var serializable bool
		if captured, ok := entry.tool.(capturedInputSchemaTool); ok {
			input, serializable = captured.capturedInputSchema()
			if serializable {
				input, serializable = cloneRegistrySchema(input)
			}
		} else {
			input, serializable = cloneRegistrySchema(entry.tool.InputSchema())
		}
		var output map[string]any
		if tool, ok := entry.tool.(ToolWithOutputSchema); ok {
			var outputSerializable bool
			output, outputSerializable = cloneRegistrySchema(tool.OutputSchema())
			serializable = serializable && outputSerializable
		}
		snapshot.ordered = append(snapshot.ordered, directToolEntry{
			name: entry.name, description: entry.tool.Description(), inputSchema: input,
			outputSchema: output, serializable: serializable, tool: entry.tool,
		})
	}
	return snapshot
}

func (snapshot directToolSnapshot) buildToolList(effectiveAllowTools *map[string]struct{}) []map[string]any {
	tools := make([]map[string]any, 0, len(snapshot.ordered))
	for _, entry := range snapshot.ordered {
		if !entry.serializable {
			continue
		}
		if effectiveAllowTools != nil {
			if _, allowed := (*effectiveAllowTools)[entry.name]; !allowed {
				continue
			}
		}
		input, _ := cloneRegistrySchema(entry.inputSchema)
		tool := map[string]any{"name": entry.name, "description": entry.description, "inputSchema": input}
		if len(entry.outputSchema) > 0 {
			tool["outputSchema"], _ = cloneRegistrySchema(entry.outputSchema)
		}
		tools = append(tools, tool)
	}
	return tools
}

// cloneRegistrySchema uses normal JSON serialization to define what can be
// published. The copy preserves plain Go values for existing registry callers;
// custom/opaque values freeze to their serialized JSON instead of sharing state.
func cloneRegistrySchema(schema map[string]any) (map[string]any, bool) {
	raw, err := json.Marshal(schema)
	if err != nil {
		return nil, false
	}
	if schema == nil {
		return nil, true
	}
	if cloned, ok := cloneDescriptorValue(reflect.ValueOf(schema), raw).(map[string]any); ok {
		return cloned, true
	}
	// A Marshaler may change its source containers while encoding. Preserve
	// the completed JSON even when its original Go shape no longer matches.
	members, _ := protocol.DecodeBusinessJSONObject(raw)
	cloned := make(map[string]any, len(members))
	for name, value := range members {
		cloned[name] = json.RawMessage(bytes.Clone(value))
	}
	return cloned, true
}

func cloneDescriptorValue(value reflect.Value, raw json.RawMessage) any {
	for value.IsValid() && value.Kind() == reflect.Interface {
		value = value.Elem()
	}
	if !value.IsValid() {
		return nil
	}
	if _, ok := value.Interface().(json.Marshaler); ok {
		return json.RawMessage(bytes.Clone(raw))
	}
	if _, ok := value.Interface().(encoding.TextMarshaler); ok {
		return json.RawMessage(bytes.Clone(raw))
	}
	if value.CanAddr() {
		if _, ok := value.Addr().Interface().(json.Marshaler); ok {
			return json.RawMessage(bytes.Clone(raw))
		}
		if _, ok := value.Addr().Interface().(encoding.TextMarshaler); ok {
			return json.RawMessage(bytes.Clone(raw))
		}
	}
	switch value.Kind() {
	case reflect.Map:
		if value.Type().Key().Kind() != reflect.String {
			break
		}
		if value.IsNil() {
			return reflect.Zero(value.Type()).Interface()
		}
		keys := value.MapKeys()
		sort.Slice(keys, func(i, j int) bool { return keys[i].String() < keys[j].String() })
		// Match encoding/json's string-key order. Looking up normalized key
		// strings would lose distinct Go keys containing invalid UTF-8 bytes.
		children := make([]json.RawMessage, 0, len(keys))
		gjson.ParseBytes(raw).ForEach(func(_, child gjson.Result) bool {
			children = append(children, raw[child.Index:child.Index+len(child.Raw)])
			return true
		})
		if len(children) != len(keys) {
			break
		}
		cloned := reflect.MakeMapWithSize(value.Type(), value.Len())
		for index, key := range keys {
			copy := reflect.ValueOf(cloneDescriptorValue(value.MapIndex(key), children[index]))
			if !copy.IsValid() {
				copy = reflect.Zero(value.Type().Elem())
			}
			if !copy.Type().AssignableTo(value.Type().Elem()) {
				return json.RawMessage(bytes.Clone(raw))
			}
			cloned.SetMapIndex(key, copy)
		}
		// A later Marshaler may rename sibling keys without changing map size.
		// Keep the captured JSON if the reconstructed Go map no longer matches.
		if encoded, err := json.Marshal(cloned.Interface()); err == nil && bytes.Equal(encoded, raw) {
			return cloned.Interface()
		}
	case reflect.Slice, reflect.Array:
		if value.Kind() == reflect.Slice && value.IsNil() {
			return reflect.Zero(value.Type()).Interface()
		}
		// JSON encodes byte slices as strings, which must remain byte slices in
		// Go views. RawMessage was already frozen through its Marshaler above.
		if value.Kind() == reflect.Slice && value.Type().Elem().Kind() == reflect.Uint8 {
			cloned := reflect.MakeSlice(value.Type(), value.Len(), value.Len())
			reflect.Copy(cloned, value)
			if encoded, err := json.Marshal(cloned.Interface()); err == nil && bytes.Equal(encoded, raw) {
				return cloned.Interface()
			}
			break
		}
		var cloned reflect.Value
		if value.Kind() == reflect.Array {
			cloned = reflect.New(value.Type()).Elem()
		} else {
			cloned = reflect.MakeSlice(value.Type(), value.Len(), value.Len())
		}
		index, compatible := 0, true
		gjson.ParseBytes(raw).ForEach(func(_, child gjson.Result) bool {
			if index >= value.Len() {
				compatible = false
				return false
			}
			copy := reflect.ValueOf(cloneDescriptorValue(value.Index(index), raw[child.Index:child.Index+len(child.Raw)]))
			if !copy.IsValid() {
				copy = reflect.Zero(value.Type().Elem())
			}
			if !copy.Type().AssignableTo(value.Type().Elem()) {
				compatible = false
				return false
			}
			cloned.Index(index).Set(copy)
			index++
			return true
		})
		if compatible && index == value.Len() {
			return cloned.Interface()
		}
	case reflect.Bool, reflect.String, reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr, reflect.Float32, reflect.Float64:
		if encoded, err := json.Marshal(value.Interface()); err == nil && bytes.Equal(encoded, raw) {
			return value.Interface()
		}
	}
	// Serialization succeeded already. Unusual Go shapes need no special
	// visitor support and must never become a new descriptor admission gate.
	return json.RawMessage(bytes.Clone(raw))
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
