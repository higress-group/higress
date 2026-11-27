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

# Fix UpdateProxyWrapper canceling the wrong deferred-delete registration

`UpdateProxyWrapper` in `registry/memory/cache.go` checked and deleted from
`deferredDeleteServices` instead of `deferredDeleteProxies` when a proxy was
updated. A same-named proxy update therefore canceled a *service*'s pending
deferred deletion, and `PurgeStaleItems` skipped it: a deleted registry's
ServiceEntry stayed in the discovery cache and kept being pushed, so traffic
kept reaching the dead backend. The proxy's own registration was never
canceled either, so a revived proxy could be purged.

Operate on `deferredDeleteProxies` (4-line change) and add two black-box
regression tests covering both registration tables.

References: Issue #5437. Design: #5438.
