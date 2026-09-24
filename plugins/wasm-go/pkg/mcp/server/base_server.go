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
	"encoding/base64"
	"encoding/json"
	"sort"
	"sync"

	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm"

	"github.com/higress-group/wasm-go/pkg/log"
)

// BaseMCPServer provides common functionality for MCP servers
type BaseMCPServer struct {
	mu    sync.RWMutex
	state runtimeSnapshot
}

// runtimeSnapshot is immutable after publication. Mutations publish a new
// value so a request can keep one coherent tool/config view across a reload.
type runtimeSnapshot struct {
	tools  map[string]Tool
	config []byte
}

type namedTool struct {
	name string
	tool Tool
}

// NewBaseMCPServer creates a new BaseMCPServer
func NewBaseMCPServer() BaseMCPServer {
	return BaseMCPServer{
		state: runtimeSnapshot{tools: make(map[string]Tool)},
	}
}

// AddMCPTool adds a tool to the server
func (s *BaseMCPServer) AddMCPTool(name string, tool Tool) Server {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exist := s.state.tools[name]; exist {
		log.Errorf("Conflict! There is a tool with the same name:%s", name)
		return s
	}
	tools := cloneTools(s.state.tools)
	tools[name] = tool
	s.state = runtimeSnapshot{tools: tools, config: s.state.config}
	return s
}

// GetMCPTools returns all tools registered with the server
func (s *BaseMCPServer) GetMCPTools() map[string]Tool {
	return cloneTools(s.snapshot().tools)
}

// SetConfig sets the server configuration
func (s *BaseMCPServer) SetConfig(config []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.state = runtimeSnapshot{
		tools:  s.state.tools,
		config: bytes.Clone(config),
	}
}

// GetConfig gets the server configuration
// It first tries to get the config from the request header, then falls back to the stored config
func (s *BaseMCPServer) GetConfig(v any) {
	var config []byte
	serverConfigBase64, _ := proxywasm.GetHttpRequestHeader("x-higress-mcpserver-config")
	proxywasm.RemoveHttpRequestHeader("x-higress-mcpserver-config")
	if serverConfigBase64 != "" {
		serverConfig, err := base64.StdEncoding.DecodeString(serverConfigBase64)
		if err != nil {
			log.Errorf("base64 decode mcp server config failed:%s, bytes:%s", err, serverConfigBase64)
		} else {
			config = serverConfig
		}
		log.Infof("parse server config from request, config:%s", serverConfig)
	} else {
		config = bytes.Clone(s.snapshot().config)
	}
	if len(config) == 0 {
		return
	}
	err := json.Unmarshal(config, v)
	if err != nil {
		log.Errorf("json unmarshal server config failed:%v, config:%s", err, config)
	}
}

// Clone creates a copy of the server
// This method should be overridden by derived types
func (s *BaseMCPServer) Clone() Server {
	panic("Clone method must be implemented by derived types")
}

// CloneBase creates a copy of the base server
func (s *BaseMCPServer) CloneBase() BaseMCPServer {
	snapshot := s.snapshot()
	return BaseMCPServer{
		state: runtimeSnapshot{
			tools:  cloneTools(snapshot.tools),
			config: bytes.Clone(snapshot.config),
		},
	}
}

func (s *BaseMCPServer) snapshot() runtimeSnapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.state
}

func cloneTools(tools map[string]Tool) map[string]Tool {
	cloned := make(map[string]Tool, len(tools))
	for name, tool := range tools {
		cloned[name] = tool
	}
	return cloned
}

// snapshotTools captures membership once and orders it by the public tool
// name. Later configuration publication cannot change an in-flight listing.
func snapshotTools(server Server) []namedTool {
	tools := server.GetMCPTools()
	names := make([]string, 0, len(tools))
	for name := range tools {
		names = append(names, name)
	}
	sort.Strings(names)

	snapshot := make([]namedTool, 0, len(names))
	for _, name := range names {
		snapshot = append(snapshot, namedTool{name: name, tool: tools[name]})
	}
	return snapshot
}
