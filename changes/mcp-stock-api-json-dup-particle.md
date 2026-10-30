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

# Fix duplicated Chinese particle in mcp-stock-history-data api.json

`plugins/wasm-go/mcp-servers/mcp-stock-history-data/api.json` said
`要查询的的开始时间` for the MACD `start` query parameter. Collapse the
duplicated particle to `要查询的开始时间`, matching the already-correct
`mcp-server.yaml` description.
