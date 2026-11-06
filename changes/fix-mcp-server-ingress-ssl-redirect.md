<!--
Copyright 2026 alibaba

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
-->

# Ignore redirect annotations on MCP server Ingresses

MCP server traffic is fully intercepted by the mcp-server plugin before
routing, and the plugin's tool calls reuse the matched route's cluster to
reach the backend. When `higress.io/ssl-redirect: "true"` (set by Console
when the domain enables "Force HTTPS") is present on an MCP server Ingress,
the HTTP listener route becomes a redirect route without a cluster, so every
MCP `tools/call` fails with a confusing local `308` instead of reaching the
backend. Redirect annotations are now ignored for MCP server Ingresses so
the route keeps its real destination on both listeners.
