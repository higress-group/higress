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

import "github.com/alibaba/higress/plugins/wasm-go/pkg/mcp/protocol"

const (
	resultTypeComplete = "complete"
	cacheScopePrivate  = "private"
	serverInfoMetaKey  = "io.modelcontextprotocol/serverInfo"
)

// ShapeResult adds fields required by the modern profile while returning the
// legacy value untouched. The source maps are never mutated.
func ShapeResult(request *protocol.RequestContext, serverName string, semantic protocol.SemanticResult) map[string]any {
	value, _ := semantic.Value.(map[string]any)
	if request == nil || request.Era != protocol.EraModern {
		return value
	}

	shaped := cloneResultMap(value)
	resultType := semantic.ResultType
	if resultType == "" {
		resultType = resultTypeComplete
	}
	shaped["resultType"] = resultType

	metadata := make(map[string]any, len(semantic.Meta)+1)
	if existing, ok := value["_meta"].(map[string]any); ok {
		for key, item := range existing {
			metadata[key] = item
		}
	}
	for key, item := range semantic.Meta {
		metadata[key] = item
	}
	metadata[serverInfoMetaKey] = map[string]any{
		"name":    serverName,
		"version": serverImplementationVersion,
	}
	shaped["_meta"] = metadata

	switch request.Envelope.Method {
	case "server/discover", "tools/list":
		shaped["ttlMs"] = 0
		shaped["cacheScope"] = cacheScopePrivate
	}
	return shaped
}

func cloneResultMap(value map[string]any) map[string]any {
	cloned := make(map[string]any, len(value)+4)
	for key, item := range value {
		cloned[key] = item
	}
	return cloned
}
