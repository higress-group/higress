// Copyright (c) 2022 Alibaba Group Holding Ltd.
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
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const protocolVersion = "2026-07-28"

func main() {
	root := os.Getenv("MCP_INTEROP_ROOT")
	if root == "" {
		panic("MCP_INTEROP_ROOT is required")
	}
	for _, path := range []string{"direct", "proxy-modern", "proxy-legacy"} {
		if err := exercise(root + "/" + path); err != nil {
			panic(fmt.Errorf("%s: %w", path, err))
		}
		fmt.Printf("go-sdk v1.7.0: %s negotiated, listed, and called successfully\n", path)
	}
}

func exercise(endpoint string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	client := mcp.NewClient(
		&mcp.Implementation{Name: "higress-go-interop", Version: "1.0.0"},
		&mcp.ClientOptions{Capabilities: &mcp.ClientCapabilities{}},
	)
	session, err := client.Connect(ctx, &mcp.StreamableClientTransport{
		Endpoint:             endpoint,
		DisableStandaloneSSE: true,
	}, nil)
	if err != nil {
		return fmt.Errorf("modern discovery connect: %w", err)
	}
	defer session.Close()
	if got := session.InitializeResult().ProtocolVersion; got != protocolVersion {
		return fmt.Errorf("negotiated protocol %q, want %q (legacy fallback is forbidden)", got, protocolVersion)
	}

	listed, err := session.ListTools(ctx, nil)
	if err != nil {
		return fmt.Errorf("tools/list: %w", err)
	}
	if len(listed.Tools) != 1 || listed.Tools[0].Name != "get_weather" {
		return fmt.Errorf("unexpected tools/list result: %#v", listed.Tools)
	}
	called, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "get_weather",
		Arguments: map[string]any{"location": "New York"},
	})
	if err != nil {
		return fmt.Errorf("tools/call: %w", err)
	}
	for _, item := range called.Content {
		if text, ok := item.(*mcp.TextContent); ok && strings.Contains(text.Text, "weather for New York") {
			return nil
		}
	}
	return fmt.Errorf("tools/call result omitted deterministic text: %#v", called.Content)
}
