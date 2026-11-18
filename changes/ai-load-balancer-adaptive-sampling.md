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

# Bound AdaptiveScore sampling with duplicate service names

Deduplicate filtered `ai-load-balancer` AdaptiveScore candidates by exact service
name before sampling without replacement. Repeated names no longer leave local
selection or global in-flight Lua argument construction in an endless sampling
loop that also blocks other requests on the same Envoy worker. Comparing all
candidates preserves their first-occurrence order without modifying configuration
slices.

Repeated `service_list` entries could previously produce undocumented implicit
weighting or random tie behavior. Deduplication intentionally removes those side
effects in AdaptiveScore. Identical names remain one candidate even when they
refer to different clusters. Other scheduling modes, scoring, counters and Redis
scripts are unchanged. Explicit weighting requires a separate Proposal.

Fixes [#5345](https://github.com/higress-group/higress/issues/5345).
Design: [#5346](https://github.com/higress-group/higress/issues/5346).
