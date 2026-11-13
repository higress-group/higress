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

# Skip unrelated Ingress scans for cached resource snapshots

WasmPlugin and ServiceEntry snapshots now read their existing converted and
registry caches directly, avoiding full Ingress listing, sorting and annotation
parsing for resource types that do not use those wrappers. Route conversion,
Gateway API generated plugins, template substitution and xDS debounce are unchanged.

Related: #5247. This removes one confirmed source of redundant control-plane work;
it does not establish that the reported end-to-end propagation delay is fixed.
